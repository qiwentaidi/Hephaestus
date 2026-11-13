package jsscan

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	tbconfig "github.com/qiwentaidi/trailblazer/pkg/config"
	tbdb "github.com/qiwentaidi/trailblazer/pkg/core/database"
	sdk "github.com/qiwentaidi/trailblazer/pkg/sdk"
	"hephaestus-backend/pkg/ai"
	sysconfig "hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/findings"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/taskstate"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var trailblazerScanGate = make(chan struct{}, 1)
var trailblazerCrawlAPIAssets = sdk.CrawlAPIAssets

// A target must not hold the serialized Trailblazer crawl gate while static
// JavaScript collection or browser startup is stalled. The SDK's runtime
// capture has its own shorter timeout, but the complete asset crawl also
// performs sequential JavaScript requests afterwards.
const jsScanTargetTimeout = 60 * time.Second

func crawlTrailblazerAPIAssets(ctx context.Context, target string, options *sdk.APICrawlOptions) (*sdk.APIAssetResult, error) {
	// Keep SDK work serialized to preserve the existing task concurrency behavior.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	select {
	case trailblazerScanGate <- struct{}{}:
		defer func() { <-trailblazerScanGate }()
		options.Context = ctx
		return trailblazerCrawlAPIAssets(target, options)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func detectTrailblazerOperationVulns(ctx context.Context, assets *sdk.APIAssetResult, options *sdk.DetectOptions) (*sdk.DetectResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	select {
	case trailblazerScanGate <- struct{}{}:
		defer func() { <-trailblazerScanGate }()
		return sdk.DetectOperationVulns(assets, options)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func newTrailblazerAuthorizationSender(proxy string) sdk.AuthzSender {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return nil
	}
	proxyURL, err := url.Parse(proxy)
	if err != nil || proxyURL.Scheme == "" || proxyURL.Host == "" {
		return nil // CrawlAPIAssets validates this configuration before detection.
	}
	client := newTrailblazerHTTPClient(proxy)
	return func(req *http.Request) (*http.Response, []byte, error) {
		resp, err := client.Do(req)
		if err != nil {
			return nil, nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		return resp, body, err
	}
}

// newTrailblazerOperationFuzzers maps the saved detection settings onto the
// separate SDK detection phase. Upload probes require response verification.
func newTrailblazerOperationFuzzers(cfg VulnDetection, uploadChecker sdk.FileUploadAIChecker) []sdk.OperationFuzzer {
	if !cfg.Enabled {
		return nil
	}
	fuzzers := make([]sdk.OperationFuzzer, 0, 6)
	if cfg.SQLInjection.Enabled {
		rules := make([]tbconfig.SQLiPayloadRule, 0, len(cfg.SQLInjection.Rules))
		for _, rule := range cfg.SQLInjection.Rules {
			rules = append(rules, tbconfig.SQLiPayloadRule{Payloads: rule.Payloads, Type: rule.Type, BodyContains: rule.BodyContains})
		}
		fuzzers = append(fuzzers, sdk.NewSQLInjectionFuzzer(sdk.SQLInjectionConfig{Enabled: true, Rules: rules}))
	}
	if cfg.LFI.Enabled {
		rules := make([]tbconfig.LFIPayloadRule, 0, len(cfg.LFI.Rules))
		for _, rule := range cfg.LFI.Rules {
			rules = append(rules, tbconfig.LFIPayloadRule{Payloads: rule.Payloads, MatchType: rule.MatchType, Regex: rule.Regex, Words: rule.Words, Condition: rule.Condition})
		}
		fuzzers = append(fuzzers, sdk.NewLFIFuzzer(sdk.LFIConfig{Enabled: true, ParamKeywords: cfg.LFI.ParamKeywords, Rules: rules}))
	}
	if cfg.SSRF.Enabled {
		fuzzers = append(fuzzers, sdk.NewSSRFFuzzer(sdk.SSRFConfig{Enabled: true, ParamKeywords: cfg.SSRF.ParamKeywords}))
	}
	if cfg.Redirect.Enabled {
		fuzzers = append(fuzzers, sdk.NewRedirectFuzzer(sdk.RedirectConfig{Enabled: true, ParamKeywords: cfg.Redirect.ParamKeywords}))
	}
	if cfg.XSS.Enabled {
		rules := make([]tbconfig.XSSPayloadRule, 0, len(cfg.XSS.Rules))
		for _, rule := range cfg.XSS.Rules {
			rules = append(rules, tbconfig.XSSPayloadRule{Payloads: rule.Payloads, Type: rule.Type, StatusEquals: rule.StatusEquals, Words: rule.Words, Condition: rule.Condition, HeaderContains: rule.HeaderContains})
		}
		fuzzers = append(fuzzers, sdk.NewXSSFuzzer(sdk.XSSConfig{Enabled: true, Rules: rules}))
	}
	if cfg.Upload.Enabled && uploadChecker != nil {
		fuzzers = append(fuzzers, sdk.NewFileUploadFuzzer(sdk.UploadConfig{Enabled: true, ParamKeywords: cfg.Upload.ParamKeywords, TestContent: cfg.Upload.TestContent, TestFileName: cfg.Upload.TestFileName}, uploadChecker))
	}
	return fuzzers
}

// ScanConfig JS扫描配置（完整配置，支持SDK所有参数）
type ScanConfig struct {
	Targets        []string          `json:"targets"`        // 扫描目标URL列表
	BlackDomain    []string          `json:"blackDomain"`    // 黑名单域名
	HighRiskRouter []string          `json:"highRiskRouter"` // 高风险路由关键词
	Authentication []string          `json:"authentication"` // 认证失败关键词
	Placeholder    map[string]string `json:"placeholder"`    // 占位符映射
	Proxy          string            `json:"proxy"`          // 代理配置
	VulnDetection  VulnDetection     `json:"vulnDetection"`  // 漏洞检测配置
	OpenAI         OpenAIConfig      `json:"openai"`         // OpenAI配置
}

// VulnDetection 漏洞检测配置
type VulnDetection struct {
	Enabled      bool               `json:"enabled"`
	SQLInjection SQLInjectionConfig `json:"sqlInjection"`
	LFI          LFIConfig          `json:"lfi"`
	SSRF         SSRFConfig         `json:"ssrf"`
	Redirect     RedirectConfig     `json:"redirect"`
	XSS          XSSConfig          `json:"xss"`
	Upload       UploadConfig       `json:"upload"`
}

// SQLInjectionConfig SQL注入检测配置
type SQLInjectionConfig struct {
	Enabled bool              `json:"enabled"`
	Rules   []SQLiPayloadRule `json:"rules,omitempty"` // SQL注入规则
}

// SQLiPayloadRule SQL注入payload规则
type SQLiPayloadRule struct {
	Payloads     []string `json:"payloads"`               // 要注入的载荷数组
	Type         string   `json:"type"`                   // 注入类型：error-based, boolean-based
	BodyContains []string `json:"bodyContains,omitempty"` // 响应体需包含的关键词
}

// LFIConfig 本地文件包含检测配置
type LFIConfig struct {
	Enabled       bool             `json:"enabled"`
	ParamKeywords []string         `json:"paramKeywords"`
	Rules         []LFIPayloadRule `json:"rules,omitempty"` // LFI规则
}

// LFIPayloadRule LFI payload规则
type LFIPayloadRule struct {
	Payloads  []string `json:"payloads"`            // 要注入的文件路径payload
	MatchType string   `json:"matchType"`           // 匹配类型：regex 或 word
	Regex     []string `json:"regex,omitempty"`     // 正则表达式列表
	Words     []string `json:"words,omitempty"`     // 关键词列表
	Condition string   `json:"condition,omitempty"` // 匹配条件：and 或 or
}

// SSRFConfig SSRF检测配置
type SSRFConfig struct {
	Enabled       bool     `json:"enabled"`
	ParamKeywords []string `json:"paramKeywords"`
}

// RedirectConfig 重定向检测配置
type RedirectConfig struct {
	Enabled       bool     `json:"enabled"`
	ParamKeywords []string `json:"paramKeywords"`
}

// XSSConfig XSS检测配置
type XSSConfig struct {
	Enabled bool             `json:"enabled"`
	Rules   []XSSPayloadRule `json:"rules,omitempty"` // XSS规则
}

// XSSPayloadRule XSS payload规则
type XSSPayloadRule struct {
	Payloads       []string          `json:"payloads"`                 // 要注入的XSS payload
	Type           string            `json:"type"`                     // 漏洞类型：reflected, dom-based
	StatusEquals   int               `json:"statusEquals,omitempty"`   // 期望的HTTP状态码
	Words          []string          `json:"words,omitempty"`          // 关键词列表
	Condition      string            `json:"condition,omitempty"`      // 匹配条件：and 或 or
	HeaderContains map[string]string `json:"headerContains,omitempty"` // 响应头需包含的键值
}

// UploadConfig 文件上传检测配置
type UploadConfig struct {
	Enabled       bool     `json:"enabled"`
	ParamKeywords []string `json:"paramKeywords,omitempty"` // URL或参数关键词
	TestContent   string   `json:"testContent"`
	TestFileName  string   `json:"testFileName"`
}

// OpenAIConfig OpenAI配置
type OpenAIConfig struct {
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"apiKey"`
	BaseURL string `json:"baseURL"`
	Model   string `json:"model"`
}

type jsScanAIReviewer struct{ config sysconfig.AIConfig }

type jsScanUploadChecker struct {
	ctx    context.Context
	target string
	proxy  string
}

func newJSScanUploadChecker(ctx context.Context, target, proxy string) sdk.FileUploadAIChecker {
	return jsScanUploadChecker{ctx: ctx, target: target, proxy: proxy}
}

func (c jsScanUploadChecker) CheckFileUpload(responseBody, testFileName, testContent string) (bool, string, error) {
	base, err := url.Parse(c.target)
	if err != nil || base.Host == "" || testContent == "" {
		return false, "", nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(responseBody), &decoded); err != nil {
		return false, "", nil
	}
	client := newTrailblazerHTTPClient(c.proxy)
	client.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if !strings.EqualFold(req.URL.Scheme, base.Scheme) || !strings.EqualFold(req.URL.Host, base.Host) {
			return http.ErrUseLastResponse
		}
		return nil
	}
	for _, raw := range uploadResponseLocations(decoded, testFileName) {
		candidate, err := url.Parse(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		resolved := base.ResolveReference(candidate)
		if !strings.EqualFold(resolved.Scheme, base.Scheme) || !strings.EqualFold(resolved.Host, base.Host) {
			continue
		}
		ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolved.String(), nil)
		if err != nil {
			cancel()
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		resp.Body.Close()
		cancel()
		if readErr == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 && strings.Contains(string(body), testContent) {
			return true, resolved.String(), nil
		}
	}
	return false, "", nil
}

func uploadResponseLocations(value any, fileName string) []string {
	locations := make([]string, 0, 4)
	var visit func(any, string)
	visit = func(current any, key string) {
		switch typed := current.(type) {
		case map[string]any:
			for name, item := range typed {
				visit(item, strings.ToLower(name))
			}
		case []any:
			for _, item := range typed {
				visit(item, key)
			}
		case string:
			if strings.Contains(key, "url") || strings.Contains(key, "path") || key == "location" || (fileName != "" && strings.Contains(typed, fileName)) {
				locations = append(locations, typed)
			}
		}
	}
	visit(value, "")
	return locations
}

func newTrailblazerHTTPClient(proxy string) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL, err := url.Parse(strings.TrimSpace(proxy)); err == nil && proxyURL.Scheme != "" && proxyURL.Host != "" {
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second}
}

type jsScanAIReviewItem struct {
	ID       int    `json:"id"`
	Valuable bool   `json:"valuable"`
	Reason   string `json:"reason"`
}

func newJSScanAIReviewer(enabled bool) *jsScanAIReviewer {
	if !enabled {
		return nil
	}
	configured, err := sysconfig.GetAIConfig()
	if err != nil {
		logger.Warning("[JS Scan] 加载 AI 配置失败，跳过 AI 研判: %v", err)
		return nil
	}
	configured = sysconfig.ResolveAIRequestConfigForRoute(configured, sysconfig.AIRouteJSScan)
	if !configured.Enabled || strings.TrimSpace(configured.Key) == "" || strings.TrimSpace(configured.Model) == "" {
		logger.Warning("[JS Scan] AI 研判已启用，但 JS 扫描路由未配置可用模型，继续保留规则引擎结果")
		return nil
	}
	return &jsScanAIReviewer{config: configured}
}

func (r *jsScanAIReviewer) review(ctx context.Context, kind string, candidates any) (map[int]jsScanAIReviewItem, error) {
	if r == nil {
		return nil, nil
	}
	reviewCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	payload, err := json.Marshal(map[string]any{"kind": kind, "candidates": candidates})
	if err != nil {
		return nil, err
	}
	result, err := ai.GenerateText(reviewCtx, r.config, []ai.ChatMessage{{
		Role: "system",
		Content: "你是应用安全研判器。只返回 JSON: {\"items\":[{\"id\":数字,\"valuable\":布尔值,\"reason\":\"简短中文理由\"}]}。" +
			"仅当证据表明敏感项可能是可用凭据、个人信息或会造成实际影响的配置泄露时 valuable=true；示例、占位符、构建产物或无法体现影响的关键词为 false。" +
			"仅当未授权接口泄露业务数据、账户/管理能力或可操作敏感对象时 valuable=true；健康检查、公开文档、静态资源、登录页、公开信息或普通匿名接口为 false。不得编造证据。",
	}, {Role: "user", Content: string(payload)}}, 1000, 0)
	if err != nil {
		return nil, err
	}
	result = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(result), "```json"), "```"))
	var decoded struct {
		Items []jsScanAIReviewItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		return nil, fmt.Errorf("解析 AI 研判结果: %w", err)
	}
	items := make(map[int]jsScanAIReviewItem, len(decoded.Items))
	for _, item := range decoded.Items {
		items[item.ID] = item
	}
	return items, nil
}

// GetDefaultConfig 获取默认配置
func GetDefaultConfig() *ScanConfig {
	return &ScanConfig{
		Targets: []string{},
		BlackDomain: []string{
			"github.com",
			"google.com",
			"amazon.com",
			"gitee.com",
			"w3.org",
			"qq.com",
			"tianditu.gov.cn",
			"amap.com",
			"alicdn.com",
		},
		HighRiskRouter: []string{
			"logout",
			"loginout",
			"insert",
			"update",
			"remove",
			"add",
			"change",
			"save",
			"import",
			"create",
			"enable",
			"del",
			"disable",
		},
		Authentication: []string{
			"token不能为空",
			"token失效",
			"令牌不能为空",
			"令牌已过期",
			"Unauthorized",
			"Access Denied",
			"认证失败",
			`"code":(4[0-9]{2})`,
			`"code":"(4[0-9]{2})"`,
			"未登录",
			"没有登录",
			"token verify fail",
			"身份验证失败",
			"404 NOT_FOUND",
			"accessToken错误",
			"请登录",
			"登录状态异常",
			"登录异常",
			"没有令牌",
			"登录信息已失效",
			"登录信息失效",
			`"success":false`,
			"未登陆",
			"登陆超时",
			"登录信息超时",
		},
		Placeholder: map[string]string{
			"default":   "test",
			"id":        "1",
			"keyword":   "test",
			"name":      "test",
			"time":      "2020-01-01 10:10:10",
			"timestamp": "1758791720",
		},
		VulnDetection: VulnDetection{
			Enabled: true,
			SQLInjection: SQLInjectionConfig{
				Enabled: true,
				Rules: []SQLiPayloadRule{
					{
						Payloads:     []string{"'", "\"", "1'", "1\""},
						Type:         "error-based",
						BodyContains: []string{"SQL syntax", "mysql_", "ORA-", "PostgreSQL", "syntax error"},
					},
				},
			},
			LFI: LFIConfig{
				Enabled: true,
				ParamKeywords: []string{
					"file", "include", "path", "doc", "template",
					"view", "filepath", "filename", "page", "document",
				},
				Rules: []LFIPayloadRule{
					{
						Payloads:  []string{"../etc/passwd", "....//....//etc/passwd", "/etc/passwd"},
						MatchType: "word",
						Words:     []string{"root:x:0:0:", "daemon:x:", "/bin/bash"},
						Condition: "or",
					},
					{
						Payloads:  []string{"..\\windows\\win.ini", "....\\\\....\\\\windows\\win.ini"},
						MatchType: "word",
						Words:     []string{"[fonts]", "[extensions]", "for 16-bit app support"},
						Condition: "and",
					},
				},
			},
			SSRF: SSRFConfig{
				Enabled: true,
				ParamKeywords: []string{
					"url", "link", "src", "source", "target",
					"redirect", "callback", "return_url", "next", "jump", "goto",
				},
			},
			Redirect: RedirectConfig{
				Enabled: true,
				ParamKeywords: []string{
					"url", "link", "src", "source", "target",
					"redirect", "callback", "return_url", "next", "jump", "goto",
				},
			},
			XSS: XSSConfig{
				Enabled: true,
				Rules: []XSSPayloadRule{
					{
						Payloads:  []string{"<script>alert(1)</script>", "<img src=x onerror=alert(1)>"},
						Type:      "reflected",
						Words:     []string{"<script>alert(1)</script>", "<img src=x onerror=alert(1)>"},
						Condition: "or",
					},
				},
			},
			Upload: UploadConfig{
				Enabled:       true,
				ParamKeywords: []string{"upload", "file", "attach", "import"},
				TestContent:   "<h1>uploadtest</h1>",
				TestFileName:  "test.html",
			},
		},
		OpenAI: OpenAIConfig{Enabled: false},
	}
}

// ExecuteJSScanWithContext 执行JS扫描（带 context 支持，用于漏洞扫描模块）
// 使用 VulnScanTask 表和 VulnScanJS* 系列结果表
func ExecuteJSScanWithContext(ctx context.Context, taskID string, targets []string, configMap map[string]interface{}) error {
	// 检查 context 是否已被取消
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	// 获取任务的当前版本号
	var task database.VulnScanTask
	if err := db.Where("id = ?", taskID).Order("version DESC").First(&task).Error; err != nil {
		return fmt.Errorf("获取任务信息失败: %w", err)
	}
	currentVersion := task.Version
	isCombinationTask := parseCombinationTaskFlag(configMap)
	isAgentItemTask := parseAgentItemTaskFlag(configMap)
	checkpoint := taskstate.ScanCheckpoint(task, "jsscan", isCombinationTask)
	saveCheckpoint := taskstate.SaveCheckpoint
	checkpointAndPause := taskstate.CheckpointAndPause
	if isCombinationTask {
		saveCheckpoint, checkpointAndPause = taskstate.CombinationCheckpointHandlers("jsscan")
	}

	// 解析配置为 ScanConfig
	scanConfig, err := parseConfigToScanConfig(configMap, targets)
	if err != nil {
		return fmt.Errorf("解析配置失败: %w", err)
	}
	aiReviewer := newJSScanAIReviewer(scanConfig.OpenAI.Enabled)

	cancelled := false
	crawlOptions := &sdk.APICrawlOptions{
		Context:                 ctx,
		Proxy:                   strings.TrimSpace(scanConfig.Proxy),
		MaxJSResources:          0,
		EnableSupplementalCrawl: false,
	}

	// 逐个目标扫描，精确控制进度
	totalTargets := len(scanConfig.Targets)
	completedTargets := 0
	if checkpoint.Stage == "jsscan:scanning" {
		if raw, ok := checkpoint.Meta["processedTargets"]; ok {
			switch value := raw.(type) {
			case float64:
				completedTargets = int(value)
			case int:
				completedTargets = value
			}
			if completedTargets < 0 {
				completedTargets = 0
			}
			if completedTargets > totalTargets {
				completedTargets = totalTargets
			}
		}
	}

	for index, target := range scanConfig.Targets {
		if index < completedTargets {
			continue
		}

		// 检查是否被取消
		select {
		case <-ctx.Done():
			cancelled = true
			break
		default:
		}

		if cancelled {
			break
		}

		// 在扫描每个目标前检查是否取消
		select {
		case <-ctx.Done():
			logger.Info("[JS Scan] 任务被取消，停止扫描剩余目标")
			cancelled = true
			break
		default:
		}

		// 扫描单个目标
		logger.Info("[JS Scan] 开始扫描目标 %d/%d: %s", completedTargets+1, totalTargets, target)
		targetCtx, cancelTarget := context.WithTimeout(ctx, jsScanTargetTimeout)
		targetOptions := *crawlOptions
		targetOptions.Context = targetCtx
		assets, err := crawlTrailblazerAPIAssets(targetCtx, target, &targetOptions)
		if targetErr := targetCtx.Err(); err == nil && targetErr != nil {
			err = targetErr
		}
		targetTimedOut := errors.Is(targetCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil

		if err != nil {
			if ctx.Err() != nil {
				logger.Info("[JS Scan] 目标 %s 扫描已取消: %v", target, err)
				cancelled = true
			} else if targetTimedOut {
				logger.Warning("[JS Scan] 目标 %s 达到单目标超时 %s，保存已有结果并跳过剩余检测", target, jsScanTargetTimeout)
			} else {
				// 记录错误但继续扫描其他目标
				logger.Error("[JS Scan] 目标 %s 扫描失败: %v", target, err)
			}
		}
		if cancelled || ctx.Err() != nil {
			cancelled = true
			cancelTarget()
			break
		}

		// 第一阶段：新 SDK 入口只返回 API 资产、运行时流量和静态请求蓝图。
		if assets != nil {
			if saveErr := saveJSScanAPIAssetsForVulnScanTask(targetCtx, db, taskID, currentVersion, target, assets, aiReviewer); saveErr != nil {
				logger.Error("[JS Scan] 保存目标 %s 的 API 资产失败: %v", target, saveErr)
			}
			if errors.Is(targetCtx.Err(), context.DeadlineExceeded) {
				targetTimedOut = true
				logger.Warning("[JS Scan] 目标 %s 达到单目标超时 %s，跳过漏洞检测并继续处理后续目标", target, jsScanTargetTimeout)
			}

			// 第二阶段：基于已采集的资产执行独立检测。DetectOperationVulns 的
			// 事件载荷为 core/database.VulnRecord，而非已废弃 PerformScan
			// 路径中的 sdk.VulnerabilityItem。
			if scanConfig.VulnDetection.Enabled && !targetTimedOut {
				detectResult, detectErr := detectTrailblazerOperationVulns(targetCtx, assets, &sdk.DetectOptions{
					Context: targetCtx,
					Sender:  newTrailblazerAuthorizationSender(scanConfig.Proxy),
					TaskID:  taskID,
					Version: currentVersion,
					Target:  target,
					Fuzzers: newTrailblazerOperationFuzzers(scanConfig.VulnDetection, newJSScanUploadChecker(targetCtx, target, scanConfig.Proxy)),
					OnFinding: func(event sdk.ScanEvent) bool {
						if ctx.Err() != nil {
							return false
						}
						if record, ok := event.Data.(tbdb.VulnRecord); ok && aiReviewer != nil && isUnauthorizedTrailblazerFinding(record) {
							verdicts, reviewErr := aiReviewer.review(targetCtx, "unauthorized_access", []map[string]any{{
								"id": 0, "title": record.Title, "type": record.Type, "url": record.URL,
								"method": record.Method, "responseType": record.ResponseType, "responseLength": record.ResponseLength,
								"confidence": record.Confidence, "reason": record.ConfidenceReason,
								"responseBody": rawUnauthorizedResponseForAI(record),
							}})
							if reviewErr != nil {
								logger.Warning("[JS Scan] 未授权访问 AI 研判失败，保留规则引擎结果: %v", reviewErr)
							} else if verdict, exists := verdicts[0]; exists {
								if !verdict.Valuable {
									logger.Info("[JS Scan] AI 过滤低价值未授权候选: %s (%s)", record.URL, verdict.Reason)
									return true
								}
								record.AIVerified = true
								record.AIReviewVerdict = "valuable"
								record.AIReviewReason = strings.TrimSpace(verdict.Reason)
								event.Data = record
							}
						}
						handleTrailblazerVulnFindingForVulnScanTask(db, taskID, currentVersion, event)
						return true
					},
				})
				if detectErr != nil {
					if ctx.Err() != nil {
						logger.Info("[JS Scan] 目标 %s 漏洞检测已取消: %v", target, detectErr)
						cancelled = true
					} else if errors.Is(targetCtx.Err(), context.DeadlineExceeded) {
						targetTimedOut = true
						logger.Warning("[JS Scan] 目标 %s 漏洞检测达到单目标超时 %s，继续处理后续目标", target, jsScanTargetTimeout)
					} else {
						logger.Error("[JS Scan] 目标 %s 漏洞检测失败: %v", target, detectErr)
					}
				} else if detectResult != nil {
					logger.Info("[JS Scan] 目标 %s 漏洞检测完成: 漏洞 %d 条，授权对照 %d 次", target, len(detectResult.Vulnerabilities), len(detectResult.Checks))
				}
			}
		}

		// 再次检查是否取消（扫描过程中可能被取消）
		select {
		case <-ctx.Done():
			logger.Info("[JS Scan] 任务在扫描过程中被取消")
			cancelled = true
			break
		default:
		}
		if cancelled {
			cancelTarget()
			break
		}

		// 目标扫描完成，增加计数并更新进度
		completedTargets++
		currentProgress := int(float64(completedTargets) / float64(totalTargets) * 100)

		logger.Info("[JS Scan Progress] TaskID: %s, 已完成: %d/%d, 进度: %d%%",
			taskID, completedTargets, totalTargets, currentProgress)

		if !isCombinationTask {
			db.Model(&database.VulnScanTask{}).Where("id = ? AND version = ?", taskID, currentVersion).Update("progress", currentProgress)
		}

		if !isAgentItemTask {
			if err := checkpointAndPause(ctx, db, taskID, currentVersion, "jsscan:scanning", currentProgress, map[string]interface{}{
				"processedTargets": completedTargets,
				"totalTargets":     totalTargets,
			}); err != nil {
				cancelTarget()
				if err == taskstate.ErrTaskPaused {
					return err
				}
				return err
			}
		}
		cancelTarget()
	}

	// 检查是否是因为取消而停止
	if cancelled {
		var current database.VulnScanTask
		if err := db.Select("status").Where("id = ? AND version = ?", taskID, currentVersion).First(&current).Error; err == nil {
			if current.Status == taskstate.StatusPaused {
				return taskstate.ErrTaskPaused
			}
		}
		return ctx.Err()
	}

	if !isAgentItemTask {
		if err := saveCheckpoint(db, taskID, currentVersion, "completed", 100, map[string]interface{}{
			"processedTargets": completedTargets,
			"totalTargets":     totalTargets,
		}); err != nil {
			return fmt.Errorf("保存 JS 扫描完成断点失败: %w", err)
		}
	}

	if !isCombinationTask {
		db.Model(&database.VulnScanTask{}).Where("id = ? AND version = ?", taskID, currentVersion).Updates(map[string]interface{}{
			"progress":   100,
			"updated_at": time.Now(),
		})
	}

	return nil
}

func parseCombinationTaskFlag(config map[string]interface{}) bool {
	if parseAgentItemTaskFlag(config) {
		return true
	}
	raw, ok := config["isCombination"]
	if !ok {
		return false
	}
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	default:
		return false
	}
}

func parseAgentItemTaskFlag(config map[string]interface{}) bool {
	raw, ok := config["agentItemMode"]
	if !ok {
		return false
	}
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	default:
		return false
	}
}

// parseConfigToScanConfig 将 map[string]interface{} 配置解析为 ScanConfig
func parseConfigToScanConfig(configMap map[string]interface{}, targets []string) (*ScanConfig, error) {
	// 先将 config 转换为 JSON，再反序列化为 ScanConfig
	configJSON, err := json.Marshal(configMap)
	if err != nil {
		return nil, fmt.Errorf("序列化配置失败: %w", err)
	}

	scanConfig := *GetDefaultConfig()
	if err := json.Unmarshal(configJSON, &scanConfig); err != nil {
		return nil, fmt.Errorf("反序列化配置失败: %w", err)
	}

	// 如果配置中没有 targets，使用传入的 targets
	if len(scanConfig.Targets) == 0 {
		scanConfig.Targets = targets
	}

	return &scanConfig, nil
}

// handleJSScanEventForVulnScanTask 处理JS扫描事件（用于 VulnScanTask 表）
func handleJSScanEventForVulnScanTask(db *gorm.DB, taskID string, version int, event sdk.ScanEvent) {
	switch event.Type {
	case sdk.EventTypeProgress:
		// 不再处理 progress 事件，进度由外层循环控制
		// 进度基于目标数量：已完成目标数 / 总目标数
		return

	case sdk.EventTypeVulnerability:
		// 保存漏洞到 vuln_scan_js_vulnerability 表
		if vuln, ok := event.Data.(sdk.VulnerabilityItem); ok {
			logger.Info("[JS Scan] 保存漏洞: %s | %s | %s", vuln.Title, vuln.Type, vuln.URL)
			vulnRecord := database.VulnScanJSVulnerability{
				ID:                 uuid.New().String(),
				TaskID:             taskID,
				Version:            version,
				Title:              vuln.Title,
				Level:              vuln.Level,
				Type:               vuln.Type,
				URL:                vuln.URL,
				Method:             vuln.Method,
				Request:            vuln.Request,
				Response:           vuln.Response,
				ResponseType:       vuln.ResponseType,
				TraceID:            vuln.TraceID,
				HasProtocolTrace:   vuln.HasProtocolTrace,
				ResponseCiphertext: vuln.ResponseCiphertext,
				DecryptionStatus:   vuln.DecryptionStatus,
				DecryptionDetail:   vuln.DecryptionDetail,
				ResponseLength:     vuln.ResponseLength,
				Confidence:         vuln.Confidence,
				ConfidenceReason:   vuln.ConfidenceReason,
				StaticContexts:     convertStaticContexts(vuln.StaticContexts),
				Description:        vuln.Description,
				AIVerified:         vuln.AIVerified,
				CreatedAt:          time.Now(),
			}
			if err := upsertJSScanVulnerabilityRecord(db, &vulnRecord); err != nil {
				logger.Error("[JS Scan] 保存漏洞失败: %v", err)
			} else {
				ingestJSScanFinding(db, &vulnRecord)
			}
		}

	case sdk.EventTypeAsset:
		// 保存资产到 vuln_scan_js_asset 表。
		// 敏感信息类型同时也会生成风险项，但资产表仍需保留原始发现结果，
		// 否则前端无法展示邮箱/手机号/身份证等分类资产。
		if asset, ok := event.Data.(map[string]interface{}); ok {
			assetType := fmt.Sprintf("%v", asset["type"])

			assetRecord := database.VulnScanJSAsset{
				ID:        uuid.New().String(),
				TaskID:    taskID,
				Version:   version,
				Type:      assetType,
				Value:     fmt.Sprintf("%v", asset["value"]),
				Source:    fmt.Sprintf("%v", asset["source"]),
				CreatedAt: time.Now(),
			}
			var existing database.VulnScanJSAsset
			err := db.Where("task_id = ? AND version = ? AND type = ? AND value = ? AND source = ?", taskID, version, assetRecord.Type, assetRecord.Value, assetRecord.Source).
				First(&existing).Error
			if err == gorm.ErrRecordNotFound {
				db.Create(&assetRecord)
			}
			saveSecretLeakFindings(db, taskID, version, assetRecord.Source, assetRecord.Value)
		}

	case sdk.EventTypeAPIRecord:
		if record, ok := event.Data.(sdk.APIRecord); ok {
			apiRecord := database.VulnScanJSAPIResource{
				ID:               uuid.New().String(),
				TaskID:           taskID,
				Version:          version,
				URL:              record.URL,
				Method:           record.Method,
				TraceID:          record.TraceID,
				HasProtocolTrace: record.HasProtocolTrace,
				RequestHeaders:   record.RequestHeaders,
				RequestBody:      record.RequestBody,
				ResponseHeaders:  record.ResponseHeaders,
				ResponseBody:     record.ResponseBody,
				ResponseCode:     record.ResponseCode,
				Headers:          record.ResponseHeaders,
				FetchedAt:        record.FetchedAt,
			}
			if apiRecord.FetchedAt.IsZero() {
				apiRecord.FetchedAt = time.Now()
			}
			var existing database.VulnScanJSAPIResource
			query := db.Where("task_id = ? AND version = ? AND url = ? AND method = ? AND trace_id = ?", taskID, version, apiRecord.URL, apiRecord.Method, apiRecord.TraceID)
			if err := query.First(&existing).Error; err == gorm.ErrRecordNotFound {
				_ = db.Create(&apiRecord).Error
			} else if err == nil {
				apiRecord.ID = existing.ID
				_ = db.Model(&existing).Updates(&apiRecord).Error
			}

			// Only inspect server responses. Request credentials are supplied by the
			// scanner itself and must not create false-positive leak findings.
			saveSecretLeakFindings(db, taskID, version, record.URL, record.ResponseBody)
			for name, value := range record.ResponseHeaders {
				saveSecretLeakFindings(db, taskID, version, record.URL, name+": "+value)
			}
		}

	}
}

// handleTrailblazerVulnFindingForVulnScanTask persists findings emitted by
// sdk.DetectOperationVulns. The new API deliberately emits the core
// database.VulnRecord model so applications can consume an inspectable,
// request/response-backed result without depending on PerformScan.
func handleTrailblazerVulnFindingForVulnScanTask(db *gorm.DB, taskID string, version int, event sdk.ScanEvent) {
	if event.Type != sdk.EventTypeVulnerability {
		return
	}

	record, ok := event.Data.(tbdb.VulnRecord)
	if !ok {
		return
	}

	logger.Info("[JS Scan] 保存漏洞: %s | %s | %s", record.Title, record.Type, record.URL)
	vulnerability := database.VulnScanJSVulnerability{
		ID:                 uuid.New().String(),
		TaskID:             taskID,
		Version:            version,
		Title:              record.Title,
		Level:              record.Level,
		Type:               record.Type,
		URL:                record.URL,
		Method:             record.Method,
		Request:            record.Request,
		Response:           record.Response,
		ResponseType:       record.ResponseType,
		TraceID:            record.TraceID,
		HasProtocolTrace:   record.HasProtocolTrace,
		ResponseCiphertext: record.ResponseCiphertext,
		DecryptionStatus:   record.DecryptionStatus,
		DecryptionDetail:   record.DecryptionDetail,
		ResponseLength:     record.ResponseLength,
		Confidence:         record.Confidence,
		ConfidenceReason:   record.ConfidenceReason,
		DenyTemplateID:     record.DenyTemplateID,
		DenyTemplateKind:   record.DenyTemplateKind,
		DenyTemplateLabel:  record.DenyTemplateLabel,
		DenyTemplateCount:  record.DenyTemplateCount,
		StaticContexts:     convertStaticContexts(record.StaticContexts),
		Description:        record.Description,
		AIVerified:         record.AIVerified,
		CreatedAt:          record.CreatedAt,
	}
	if vulnerability.CreatedAt.IsZero() {
		vulnerability.CreatedAt = time.Now()
	}
	if err := upsertJSScanVulnerabilityRecord(db, &vulnerability); err != nil {
		logger.Error("[JS Scan] 保存漏洞失败: %v", err)
	} else {
		ingestJSScanFinding(db, &vulnerability)
	}
}

func isUnauthorizedTrailblazerFinding(record tbdb.VulnRecord) bool {
	return strings.EqualFold(strings.TrimSpace(record.Type), "authorization") ||
		strings.Contains(strings.ToLower(record.Type), "unauth") ||
		strings.Contains(record.Type, "未授权") ||
		strings.Contains(record.Title, "未授权")
}

// rawUnauthorizedResponseForAI returns the full raw response body for AI
// review. The caller explicitly authorizes this data to be sent to its
// configured AI provider so it can assess the actual exposure impact.
func rawUnauthorizedResponseForAI(record tbdb.VulnRecord) string {
	response := strings.TrimSpace(record.ResponsePlaintext)
	if response == "" {
		response = strings.TrimSpace(record.Response)
	}
	return response
}

func ingestJSScanFinding(db *gorm.DB, record *database.VulnScanJSVulnerability) {
	if db == nil || record == nil {
		return
	}
	identity := strings.TrimSpace(record.Type) + "|" + strings.TrimSpace(record.Title)
	if _, err := findings.Ingest(db, findings.Input{
		Source:            "jsscan",
		SourceID:          record.ID,
		TaskID:            record.TaskID,
		TaskVersion:       record.Version,
		Title:             record.Title,
		VulnerabilityType: record.Type,
		Target:            record.URL,
		Severity:          record.Level,
		Status:            record.DisposalStatus,
		Evidence:          record.Description,
		ObservedAt:        record.CreatedAt,
		Identity:          identity,
	}); err != nil {
		logger.Error("[JS Scan] 写入漏洞归因失败: %v", err)
	}
}

func upsertJSScanVulnerabilityRecord(db *gorm.DB, record *database.VulnScanJSVulnerability) error {
	var existing database.VulnScanJSVulnerability
	query := buildJSScanVulnerabilityIdentityQuery(db, record)
	err := query.First(&existing).Error
	if err == nil {
		return db.Model(&existing).Updates(map[string]interface{}{
			"level":               record.Level,
			"method":              record.Method,
			"request":             record.Request,
			"response":            record.Response,
			"response_type":       record.ResponseType,
			"trace_id":            record.TraceID,
			"has_protocol_trace":  record.HasProtocolTrace,
			"response_ciphertext": record.ResponseCiphertext,
			"decryption_status":   record.DecryptionStatus,
			"decryption_detail":   record.DecryptionDetail,
			"response_length":     record.ResponseLength,
			"confidence":          record.Confidence,
			"confidence_reason":   record.ConfidenceReason,
			"deny_template_id":    record.DenyTemplateID,
			"deny_template_kind":  record.DenyTemplateKind,
			"deny_template_label": record.DenyTemplateLabel,
			"deny_template_count": record.DenyTemplateCount,
			"static_contexts":     record.StaticContexts,
			"description":         record.Description,
			"ai_verified":         record.AIVerified,
		}).Error
	}
	if err == gorm.ErrRecordNotFound {
		return db.Create(record).Error
	}
	return err
}

func buildJSScanVulnerabilityIdentityQuery(db *gorm.DB, record *database.VulnScanJSVulnerability) *gorm.DB {
	query := db.Where(
		"task_id = ? AND version = ? AND title = ? AND type = ? AND url = ?",
		record.TaskID,
		record.Version,
		record.Title,
		record.Type,
		record.URL,
	)
	if record.Method != "" {
		query = query.Where("method = ?", record.Method)
	}
	if shouldUseDescriptionInJSVulnIdentity(record) {
		query = query.Where("description = ?", record.Description)
	}
	return query
}

func shouldUseDescriptionInJSVulnIdentity(record *database.VulnScanJSVulnerability) bool {
	if strings.TrimSpace(record.Description) == "" {
		return false
	}
	return record.Title == "敏感关键词泄露" || record.Type == "敏感信息泄露" || record.Type == "api-key-exposure"
}

func extractOptionalBoolField(value interface{}, fieldName string) bool {
	if value == nil || strings.TrimSpace(fieldName) == "" {
		return false
	}

	current := reflect.ValueOf(value)
	if current.Kind() == reflect.Pointer {
		if current.IsNil() {
			return false
		}
		current = current.Elem()
	}
	if current.Kind() != reflect.Struct {
		return false
	}

	field := current.FieldByName(fieldName)
	if !field.IsValid() || field.Kind() != reflect.Bool {
		return false
	}
	return field.Bool()
}

func convertStaticContexts(items []tbdb.VulnStaticContext) []database.VulnStaticContext {
	if len(items) == 0 {
		return nil
	}

	result := make([]database.VulnStaticContext, 0, len(items))
	for _, item := range items {
		result = append(result, database.VulnStaticContext{
			SourceURL: strings.TrimSpace(item.SourceURL),
			Snippet:   item.Snippet,
		})
	}
	return result
}

// saveJSScanAPIAssetsForVulnScanTask persists the bounded API-asset crawl
// result. Unlike the legacy PerformScan result, it deliberately contains no
// active payload-scan findings or protocol traces.
func saveJSScanAPIAssetsForVulnScanTask(ctx context.Context, db *gorm.DB, taskID string, version int, target string, assets *sdk.APIAssetResult, aiReviewer *jsScanAIReviewer) error {
	if assets == nil {
		return nil
	}
	if len(assets.SiteTree) > 0 {
		treeJSON, err := json.Marshal(assets.SiteTree)
		if err != nil {
			return fmt.Errorf("序列化网站树 %s: %w", target, err)
		}
		var existing database.VulnScanJSSiteTree
		err = db.Where("task_id = ? AND version = ? AND target_url = ?", taskID, version, target).First(&existing).Error
		if err == nil {
			err = db.Model(&existing).Update("tree_data", string(treeJSON)).Error
		} else if err == gorm.ErrRecordNotFound {
			err = db.Create(&database.VulnScanJSSiteTree{
				ID: uuid.NewString(), TaskID: taskID, Version: version,
				TargetURL: target, TreeData: string(treeJSON), CreatedAt: time.Now(),
			}).Error
		}
		if err != nil {
			return fmt.Errorf("保存网站树 %s: %w", target, err)
		}
	}

	for _, resource := range assets.JSResources {
		row := database.VulnScanJSResource{
			ID:           uuid.New().String(),
			TaskID:       taskID,
			Version:      version,
			URL:          resource.URL,
			Content:      resource.Content,
			ResponseCode: resource.ResponseCode,
			Headers:      resource.Headers,
			Size:         resource.Size,
			FetchedAt:    resource.FetchedAt,
		}
		if row.FetchedAt.IsZero() {
			row.FetchedAt = time.Now()
		}

		var existing database.VulnScanJSResource
		err := db.Where("task_id = ? AND version = ? AND url = ?", taskID, version, row.URL).First(&existing).Error
		if err == nil {
			// 用结构体更新，headers 才会走 serializer:json；map 形式的
			// Updates 不应用字段 serializer，会把 map 直接传给数据库驱动。
			err = db.Model(&existing).Select("content", "response_code", "headers", "size", "fetched_at").
				Updates(&database.VulnScanJSResource{
					Content:      row.Content,
					ResponseCode: row.ResponseCode,
					Headers:      row.Headers,
					Size:         row.Size,
					FetchedAt:    row.FetchedAt,
				}).Error
		} else if err == gorm.ErrRecordNotFound {
			err = db.Create(&row).Error
		}
		if err != nil {
			return fmt.Errorf("保存 JS 资源 %s: %w", row.URL, err)
		}
	}

	for _, record := range assets.RuntimeRecords {
		row := database.VulnScanJSAPIResource{
			ID:              uuid.New().String(),
			TaskID:          taskID,
			Version:         version,
			URL:             record.URL,
			Method:          record.Method,
			RequestHeaders:  record.RequestHeaders,
			RequestBody:     record.RequestBody,
			ResponseHeaders: record.ResponseHeaders,
			ResponseBody:    record.ResponseBody,
			ResponseCode:    record.ResponseCode,
			Headers:         record.ResponseHeaders,
			FetchedAt:       record.FetchedAt,
		}
		if row.FetchedAt.IsZero() {
			row.FetchedAt = time.Now()
		}

		var existing database.VulnScanJSAPIResource
		err := db.Where("task_id = ? AND version = ? AND url = ? AND method = ?", taskID, version, row.URL, row.Method).First(&existing).Error
		if err == nil {
			err = db.Model(&existing).
				Select("request_headers", "request_body", "response_headers", "response_body", "response_code", "headers", "fetched_at").
				Updates(&database.VulnScanJSAPIResource{
					RequestHeaders:  row.RequestHeaders,
					RequestBody:     row.RequestBody,
					ResponseHeaders: row.ResponseHeaders,
					ResponseBody:    row.ResponseBody,
					ResponseCode:    row.ResponseCode,
					Headers:         row.Headers,
					FetchedAt:       row.FetchedAt,
				}).Error
		} else if err == gorm.ErrRecordNotFound {
			err = db.Create(&row).Error
		}
		if err != nil {
			return fmt.Errorf("保存运行时 API %s: %w", row.URL, err)
		}
		saveSecretLeakFindings(db, taskID, version, row.URL, row.ResponseBody)
	}

	// Keep the static operation templates before APIAssetResult's in-memory
	// store is discarded. Dynamic values remain explicitly marked by the SDK.
	for _, spec := range assets.OperationSpecs {
		operationJSON, err := json.Marshal(spec)
		if err != nil {
			return fmt.Errorf("序列化 API 操作模板 %s %s: %w", spec.Method, spec.PathTemplate, err)
		}
		operationID := strings.TrimSpace(spec.ID)
		if operationID == "" {
			operationID = strings.ToUpper(strings.TrimSpace(spec.Method)) + " " + strings.TrimSpace(spec.PathTemplate)
		}
		row := database.VulnScanJSOperation{
			ID: uuid.NewString(), TaskID: taskID, Version: version, TargetURL: target,
			OperationID: operationID, Method: strings.ToUpper(strings.TrimSpace(spec.Method)),
			PathTemplate: strings.TrimSpace(spec.PathTemplate), Source: strings.TrimSpace(spec.ExtractionSource),
			Confidence: strings.TrimSpace(spec.Confidence), RequiresRuntime: len(spec.RequiresRuntime) > 0,
			OperationJSON: string(operationJSON),
		}
		var existing database.VulnScanJSOperation
		err = db.Where("task_id = ? AND version = ? AND target_url = ? AND operation_id = ?", taskID, version, target, operationID).First(&existing).Error
		if err == nil {
			err = db.Model(&existing).Updates(map[string]interface{}{
				"method": row.Method, "path_template": row.PathTemplate, "source": row.Source,
				"confidence": row.Confidence, "requires_runtime": row.RequiresRuntime, "operation_json": row.OperationJSON,
			}).Error
		} else if err == gorm.ErrRecordNotFound {
			err = db.Create(&row).Error
		}
		if err != nil {
			return fmt.Errorf("保存 API 操作模板 %s: %w", operationID, err)
		}
	}

	if openAPI, err := sdk.ExportOpenAPIAssets(assets, "Hephaestus JS/API scan: "+target); err != nil {
		return fmt.Errorf("导出 OpenAPI 文档: %w", err)
	} else {
		row := database.VulnScanJSOpenAPIDocument{ID: uuid.NewString(), TaskID: taskID, Version: version, TargetURL: target, Document: string(openAPI)}
		var existing database.VulnScanJSOpenAPIDocument
		err = db.Where("task_id = ? AND version = ? AND target_url = ?", taskID, version, target).First(&existing).Error
		if err == nil {
			err = db.Model(&existing).Update("document", row.Document).Error
		} else if err == gorm.ErrRecordNotFound {
			err = db.Create(&row).Error
		}
		if err != nil {
			return fmt.Errorf("保存 OpenAPI 文档: %w", err)
		}
	}

	seenAssets := make(map[string]struct{})
	addAsset := func(assetType, value, source string) error {
		assetType = strings.TrimSpace(assetType)
		value = strings.TrimSpace(value)
		source = strings.TrimSpace(source)
		if assetType == "" || value == "" {
			return nil
		}
		identity := assetType + "\x00" + value + "\x00" + source
		if _, exists := seenAssets[identity]; exists {
			return nil
		}
		seenAssets[identity] = struct{}{}
		row := database.VulnScanJSAsset{
			ID:        uuid.New().String(),
			TaskID:    taskID,
			Version:   version,
			Type:      assetType,
			Value:     value,
			Source:    source,
			CreatedAt: time.Now(),
		}
		var existing database.VulnScanJSAsset
		err := db.Where("task_id = ? AND version = ? AND type = ? AND value = ? AND source = ?", taskID, version, row.Type, row.Value, row.Source).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			return db.Create(&row).Error
		}
		return err
	}

	if assets.Store != nil {
		for _, apiContext := range assets.Store.List() {
			if apiContext != nil {
				if err := addAsset("apiRoute", apiContext.PathTemplate, target); err != nil {
					return fmt.Errorf("保存观测接口路径: %w", err)
				}
			}
		}
	}
	for _, spec := range assets.OperationSpecs {
		if err := addAsset("apiRoute", spec.PathTemplate, target); err != nil {
			return fmt.Errorf("保存静态接口路径: %w", err)
		}
	}
	for _, root := range assets.APIRootCandidates {
		if err := addAsset("apiRoot", root, target); err != nil {
			return fmt.Errorf("保存 API Root 候选: %w", err)
		}
	}

	sensitiveAssets, err := sdk.AnalyzeSensitiveAssets(assets)
	if err != nil {
		return fmt.Errorf("分析敏感资产: %w", err)
	}
	sensitiveVerdicts := map[int]jsScanAIReviewItem(nil)
	if aiReviewer != nil && len(sensitiveAssets.Items) > 0 {
		candidates := make([]map[string]any, 0, len(sensitiveAssets.Items))
		for index, item := range sensitiveAssets.Items {
			candidates = append(candidates, map[string]any{
				"id": index, "type": item.Type, "value": item.Value,
				"source": item.Source, "rule": item.Rule,
			})
		}
		var reviewErr error
		sensitiveVerdicts, reviewErr = aiReviewer.review(ctx, "sensitive_asset", candidates)
		if reviewErr != nil {
			logger.Warning("[JS Scan] 敏感资产 AI 研判失败，保留规则引擎结果: %v", reviewErr)
			sensitiveVerdicts = nil
		}
	}
	for index, item := range sensitiveAssets.Items {
		if verdict, reviewed := sensitiveVerdicts[index]; reviewed && !verdict.Valuable {
			logger.Info("[JS Scan] AI 过滤低价值敏感候选: %s (%s)", item.Source, verdict.Reason)
			continue
		}
		assetType := normalizeTrailblazerSensitiveAssetType(item.Type)
		if err := addAsset(assetType, item.Value, item.Source); err != nil {
			return fmt.Errorf("保存敏感资产: %w", err)
		}
	}

	return nil
}

func normalizeTrailblazerSensitiveAssetType(assetType string) string {
	switch strings.TrimSpace(assetType) {
	case "id_card":
		return "idCard"
	case "ip_url":
		return "ipUrl"
	case "email", "phone":
		return strings.TrimSpace(assetType)
	default:
		return "sensitive"
	}
}

// saveJSScanResultsForVulnScanTask 保存JS扫描结果到数据库（用于 VulnScanTask 表）
func saveJSScanResultsForVulnScanTask(db *gorm.DB, taskID string, version int, result *sdk.ScanResult) error {
	// 保存网站树到 vuln_scan_js_sitemap 表
	if len(result.Targets) > 0 {
		for _, targetResult := range result.Targets {
			// 保存网站树
			if len(targetResult.SiteTree) > 0 {
				treeJSON, err := json.Marshal(targetResult.SiteTree)
				if err != nil {
					continue
				}

				siteTree := database.VulnScanJSSiteTree{
					ID:        uuid.New().String(),
					TaskID:    taskID,
					Version:   version,
					TargetURL: targetResult.Target,
					TreeData:  string(treeJSON),
					CreatedAt: time.Now(),
				}
				var existing database.VulnScanJSSiteTree
				err = db.Where("task_id = ? AND version = ? AND target_url = ?", taskID, version, siteTree.TargetURL).First(&existing).Error
				if err == nil {
					err = db.Model(&existing).Update("tree_data", siteTree.TreeData).Error
				} else if err == gorm.ErrRecordNotFound {
					err = db.Create(&siteTree).Error
				}
				if err != nil {
					logger.Error("[JS Scan] 保存网站树失败: %v", err)
				}
			}

			for _, resource := range targetResult.JSResources {
				jsResource := database.VulnScanJSResource{
					ID:           uuid.New().String(),
					TaskID:       taskID,
					Version:      version,
					URL:          resource.URL,
					Content:      resource.Content,
					ResponseCode: resource.ResponseCode,
					Headers:      resource.Headers,
					Size:         resource.Size,
					FetchedAt:    resource.FetchedAt,
				}
				if jsResource.FetchedAt.IsZero() {
					jsResource.FetchedAt = time.Now()
				}

				var existing database.VulnScanJSResource
				err := db.Where("task_id = ? AND version = ? AND url = ?", taskID, version, jsResource.URL).
					First(&existing).Error
				if err == nil {
					err = db.Model(&existing).Updates(&database.VulnScanJSResource{
						Content:      jsResource.Content,
						ResponseCode: jsResource.ResponseCode,
						Headers:      jsResource.Headers,
						Size:         jsResource.Size,
						FetchedAt:    jsResource.FetchedAt,
					}).Error
				} else if err == gorm.ErrRecordNotFound {
					err = db.Create(&jsResource).Error
				}
				if err != nil && err != gorm.ErrRecordNotFound {
					logger.Error("[JS Scan] 保存 JS 资源失败: %v", err)
				}
			}

			for _, record := range targetResult.APIRecords {
				apiResource := database.VulnScanJSAPIResource{
					ID:               uuid.New().String(),
					TaskID:           taskID,
					Version:          version,
					URL:              record.URL,
					Method:           record.Method,
					TraceID:          record.TraceID,
					HasProtocolTrace: record.HasProtocolTrace,
					RequestHeaders:   record.RequestHeaders,
					RequestBody:      record.RequestBody,
					ResponseHeaders:  record.ResponseHeaders,
					ResponseBody:     record.ResponseBody,
					ResponseCode:     record.ResponseCode,
					Headers:          record.ResponseHeaders,
					FetchedAt:        record.FetchedAt,
				}
				if apiResource.FetchedAt.IsZero() {
					apiResource.FetchedAt = time.Now()
				}

				var existing database.VulnScanJSAPIResource
				err := db.Where("task_id = ? AND version = ? AND url = ? AND method = ?", taskID, version, apiResource.URL, apiResource.Method).
					First(&existing).Error
				if err == nil {
					err = db.Model(&existing).Updates(&database.VulnScanJSAPIResource{
						TraceID:          apiResource.TraceID,
						HasProtocolTrace: apiResource.HasProtocolTrace,
						RequestHeaders:   apiResource.RequestHeaders,
						RequestBody:      apiResource.RequestBody,
						ResponseHeaders:  apiResource.ResponseHeaders,
						ResponseBody:     apiResource.ResponseBody,
						ResponseCode:     apiResource.ResponseCode,
						Headers:          apiResource.Headers,
						FetchedAt:        apiResource.FetchedAt,
					}).Error
				} else if err == gorm.ErrRecordNotFound {
					err = db.Create(&apiResource).Error
				}
				if err != nil && err != gorm.ErrRecordNotFound {
					logger.Error("[JS Scan] 保存 API 资源失败: %v", err)
				}
			}

			// 保存 API 路由作为资产
			for _, route := range targetResult.Assets.APIRoutes {
				asset := database.VulnScanJSAsset{
					ID:        uuid.New().String(),
					TaskID:    taskID,
					Version:   version,
					Type:      "apiRoute",
					Value:     route,
					Source:    targetResult.Target,
					CreatedAt: time.Now(),
				}
				var existing database.VulnScanJSAsset
				err := db.Where("task_id = ? AND version = ? AND type = ? AND value = ? AND source = ?", taskID, version, asset.Type, asset.Value, asset.Source).
					First(&existing).Error
				if err == gorm.ErrRecordNotFound {
					err = db.Create(&asset).Error
				}
				if err != nil && err != gorm.ErrRecordNotFound {
					logger.Error("[JS Scan] 保存 API 路由资产失败: %v", err)
				}
			}

			// 保存 API 根路径作为资产
			for _, root := range targetResult.Assets.APIRoots {
				asset := database.VulnScanJSAsset{
					ID:        uuid.New().String(),
					TaskID:    taskID,
					Version:   version,
					Type:      "apiRoot",
					Value:     root,
					Source:    targetResult.Target,
					CreatedAt: time.Now(),
				}
				var existing database.VulnScanJSAsset
				err := db.Where("task_id = ? AND version = ? AND type = ? AND value = ? AND source = ?", taskID, version, asset.Type, asset.Value, asset.Source).
					First(&existing).Error
				if err == gorm.ErrRecordNotFound {
					err = db.Create(&asset).Error
				}
				if err != nil && err != gorm.ErrRecordNotFound {
					logger.Error("[JS Scan] 保存 API 根路径资产失败: %v", err)
				}
			}
		}
	}

	return nil
}
