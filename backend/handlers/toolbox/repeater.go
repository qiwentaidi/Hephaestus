package toolbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/repeater"
	"hephaestus-backend/pkg/webscan"

	"github.com/gin-gonic/gin"
	"github.com/logrusorgru/aurora"
	nuclei "github.com/projectdiscovery/nuclei/v3/lib"
	"github.com/projectdiscovery/nuclei/v3/pkg/output"
	httputil "github.com/qiwentaidi/utils/http"
)

const repeaterTemplateContextLimit = 12000
const repeaterTemplateDebugMaxBytes = 512 * 1024

type nucleiTemplatePolishRequest struct {
	ID           string                        `json:"id"`
	Name         string                        `json:"name"`
	Severity     string                        `json:"severity"`
	Description  string                        `json:"description"`
	Impact       string                        `json:"impact"`
	Remediation  string                        `json:"remediation"`
	RawRequest   string                        `json:"rawRequest"`
	Response     string                        `json:"response"`
	TemplateYAML string                        `json:"templateYaml"`
	Locale       string                        `json:"locale"`
	Matchers     []nucleiTemplatePolishMatcher `json:"matchers"`
}

type nucleiTemplatePolishMatcher struct {
	Type    string   `json:"type"`
	Part    string   `json:"part,omitempty"`
	Status  []int    `json:"status,omitempty"`
	Words   []string `json:"words,omitempty"`
	Summary string   `json:"summary,omitempty"`
}

type nucleiTemplatePolishResult struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Impact      string `json:"impact"`
	Remediation string `json:"remediation"`
}

type nucleiTemplateDebugRequest struct {
	Target         string `json:"target"`
	TemplateBase64 string `json:"templateBase64"`
}

type nucleiTemplateDebugEvent struct {
	Type     string `json:"type"`
	Message  string `json:"message"`
	Request  string `json:"request,omitempty"`
	Response string `json:"response,omitempty"`
}

type nucleiTemplateDebugResult struct {
	Target  string                      `json:"target"`
	Matched bool                        `json:"matched"`
	Status  string                      `json:"status"`
	Events  []nucleiTemplateDebugEvent  `json:"events"`
	Results []nucleiTemplateDebugOutput `json:"results,omitempty"`
}

type nucleiTemplateDebugOutput struct {
	TemplateID    string `json:"templateId"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	MatcherStatus bool   `json:"matcherStatus"`
	MatchedAt     string `json:"matchedAt"`
	Request       string `json:"request,omitempty"`
	Response      string `json:"response,omitempty"`
	Error         string `json:"error,omitempty"`
}

type nucleiTemplateDebugWriter struct {
	mu      sync.Mutex
	events  []nucleiTemplateDebugEvent
	results []nucleiTemplateDebugOutput
	matched bool
	aurora  aurora.Aurora
}

// SendRequestHandler 发送HTTP请求
func SendRequestHandler(c *gin.Context) {
	var req struct {
		Raw        string `json:"raw" binding:"required"` // 原始HTTP请求字符串
		ForceHttps bool   `json:"forceHttps"`             // 强制HTTPS
		Redirect   bool   `json:"redirect"`               // 是否跟随重定向
		ProxyURL   string `json:"proxyURL"`               // 代理URL
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 发送请求
	resp, duration, err := repeater.SendRequestWithRaw(c.Request.Context(), req.Raw, req.ForceHttps, req.Redirect, req.ProxyURL)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(c.Request.Context().Err(), context.Canceled) {
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if resp.RawResponse != nil && resp.RawResponse.Body != nil {
		defer resp.RawResponse.Body.Close()
	}

	var responseBody string
	responseLength := inferRepeaterResponseLength(resp.RawResponse)
	if resp.RawResponse != nil && shouldHandleRepeaterResponseAsBinary(resp.RawResponse) {
		responseBody, err = dumpRepeaterBinaryResponse(resp.RawResponse)
	} else {
		responseBody, err = httputil.DumpResponseHeadersAndDecodedBody(resp.RawResponse)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "读取响应体失败: " + err.Error(),
		})
		return
	}

	// 检测是否有重定向（301, 302, 303, 307, 308）
	statusCode := resp.StatusCode()
	hasRedirect := (statusCode == 301 || statusCode == 302 || statusCode == 303 ||
		statusCode == 307 || statusCode == 308) && !req.Redirect

	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"statusCode":     statusCode,
		"response":       responseBody,
		"responseLength": responseLength,
		"responseTime":   duration,
		"hasRedirect":    hasRedirect,
		"headers":        resp.Header(),
	})
}

// PolishNucleiTemplateHandler 使用现有 AI 配置润色 Repeater 生成的 Nuclei 模板元信息。
func PolishNucleiTemplateHandler(c *gin.Context) {
	var req nucleiTemplatePolishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	if strings.TrimSpace(req.RawRequest) == "" && strings.TrimSpace(req.Response) == "" && strings.TrimSpace(req.TemplateYAML) == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "模板内容不能为空",
		})
		return
	}

	aiConfig, err := config.GetAIConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "加载 AI 配置失败: " + err.Error(),
		})
		return
	}
	aiConfig = config.ResolveAIRequestConfigForRoute(aiConfig, config.AIRouteRepeaterPolish)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	result, err := polishNucleiTemplateWithAI(ctx, aiConfig, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// DebugNucleiTemplateHandler 使用前端生成的临时模板对指定目标执行一次 nuclei 验证。
func DebugNucleiTemplateHandler(c *gin.Context) {
	var req nucleiTemplateDebugRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	target, err := normalizeRepeaterTemplateDebugTarget(req.Target)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	templateBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.TemplateBase64))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "模板 base64 解码失败: " + err.Error(),
		})
		return
	}
	if len(templateBytes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "模板内容不能为空",
		})
		return
	}
	if len(templateBytes) > repeaterTemplateDebugMaxBytes {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "模板内容过大",
		})
		return
	}

	result, err := runRepeaterNucleiTemplateDebug(c.Request.Context(), target, templateBytes)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
			"data":    result,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetRepeaterTabsHandler 获取所有标签页
func GetRepeaterTabsHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	var tabs []database.RepeaterTab
	// 优先按 sort_order 排序，如果相同则按 id（时间戳）排序
	if err := db.Order("sort_order ASC, id ASC").Find(&tabs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取标签页失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tabs,
	})
}

// UpdateRepeaterTabHandler 更新标签页
func UpdateRepeaterTabHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	id := c.Param("id")
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	var tab database.RepeaterTab
	if err := db.Where("id = ?", id).First(&tab).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "标签页不存在",
		})
		return
	}

	if err := db.Model(&tab).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "更新标签页失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tab,
	})
}

// DeleteRepeaterTabHandler 删除标签页
func DeleteRepeaterTabHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	id := c.Param("id")

	if err := db.Where("id = ?", id).Delete(&database.RepeaterTab{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "删除标签页失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// SaveAllRepeaterTabsHandler 批量保存标签页（用于同步整个标签页列表）
func SaveAllRepeaterTabsHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	var tabs []database.RepeaterTab
	if err := c.ShouldBindJSON(&tabs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 使用事务确保原子性
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 删除所有现有标签页
	if err := tx.Exec("DELETE FROM repeater_tab").Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "清空标签页失败: " + err.Error(),
		})
		return
	}

	// 根据数组索引设置 sort_order，确保顺序正确
	for i := range tabs {
		tabs[i].SortOrder = i
		sanitizeRepeaterTabForPersistence(&tabs[i])
	}

	// 批量插入新标签页
	if len(tabs) > 0 {
		if err := tx.Create(&tabs).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "保存标签页失败: " + err.Error(),
			})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "提交事务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tabs,
	})
}

func sanitizeRepeaterTabForPersistence(tab *database.RepeaterTab) {
	if tab == nil {
		return
	}

	tab.Name = sanitizeRepeaterTextForPersistence(tab.Name)
	tab.Method = sanitizeRepeaterTextForPersistence(tab.Method)
	tab.URL = sanitizeRepeaterTextForPersistence(tab.URL)
	tab.RawRequest = sanitizeRepeaterTextForPersistence(tab.RawRequest)
	tab.Response = sanitizeRepeaterTextForPersistence(tab.Response)
}

func sanitizeRepeaterTextForPersistence(value string) string {
	if value == "" {
		return value
	}
	return strings.ReplaceAll(value, "\x00", `\x00`)
}

func normalizeNucleiTemplatePolishLocale(locale string) string {
	normalized := strings.ToLower(strings.TrimSpace(locale))
	if strings.HasPrefix(normalized, "en") {
		return "en"
	}
	return "zh"
}

func nucleiTemplatePolishLanguageInstruction(locale string) string {
	if normalizeNucleiTemplatePolishLocale(locale) == "en" {
		return "name、description、impact、remediation must be written in English, matching the current program language."
	}
	return "name、description、impact、remediation 必须使用中文，匹配当前程序语言。"
}

func polishNucleiTemplateWithAI(ctx context.Context, aiConfig config.AIConfig, req nucleiTemplatePolishRequest) (nucleiTemplatePolishResult, error) {
	polishLocale := normalizeNucleiTemplatePolishLocale(req.Locale)
	payload := map[string]any{
		"current": map[string]any{
			"id":          req.ID,
			"name":        req.Name,
			"severity":    req.Severity,
			"description": req.Description,
			"impact":      req.Impact,
			"remediation": req.Remediation,
			"matchers":    req.Matchers,
		},
		"templateYaml": truncateRepeaterTemplateContext(req.TemplateYAML),
		"rawRequest":   truncateRepeaterTemplateContext(req.RawRequest),
		"response":     truncateRepeaterTemplateContext(req.Response),
		"locale":       polishLocale,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nucleiTemplatePolishResult{}, err
	}

	content, err := ai.GenerateText(ctx, aiConfig, []ai.ChatMessage{
		{
			Role: "system",
			Content: strings.Join([]string{
				"你是资深 Web 漏洞检测工程师，负责把 HTTP 请求/响应特征或已有 Nuclei YAML 模板整理为模板元信息。",
				"只返回 JSON，不要 Markdown，不要解释。",
				"字段必须包含 id、name、severity、description、impact、remediation。",
				"id 使用小写英文、数字和连字符，简洁描述检测目标，不能包含空格。",
				"name 使用清晰的漏洞或暴露风险名称。",
				"severity 只能是 info、low、medium、high、critical。",
				"如果提供了 templateYaml，优先根据模板 id、info、http、matchers、dsl、路径和关键字判断漏洞语义。",
				"如果缺少响应内容，不要因此拒绝，基于模板行为和匹配条件推断描述。",
				"name、description、impact、remediation 的字段内容不要额外使用首尾引号包裹。",
				nucleiTemplatePolishLanguageInstruction(polishLocale),
				"避免编造 CVE 编号或厂商版本。",
			}, "\n"),
		},
		{
			Role:    "user",
			Content: string(payloadBytes),
		},
	}, 1200, 0.2)
	if err != nil {
		return nucleiTemplatePolishResult{}, err
	}

	result, err := parseNucleiTemplatePolishResult(content)
	if err != nil {
		return nucleiTemplatePolishResult{}, err
	}

	result.ID = sanitizeNucleiTemplateID(firstNonEmptyString(result.ID, req.ID, "generated-http-template"))
	result.Name = strings.TrimSpace(firstNonEmptyString(result.Name, req.Name, defaultNucleiTemplatePolishName(polishLocale)))
	result.Severity = normalizeNucleiSeverity(firstNonEmptyString(result.Severity, req.Severity, "info"))
	result.Description = strings.TrimSpace(firstNonEmptyString(result.Description, req.Description, defaultNucleiTemplatePolishDescription(polishLocale)))
	result.Impact = strings.TrimSpace(result.Impact)
	result.Remediation = strings.TrimSpace(result.Remediation)

	return result, nil
}

func defaultNucleiTemplatePolishName(locale string) string {
	if normalizeNucleiTemplatePolishLocale(locale) == "en" {
		return "Generated HTTP Template"
	}
	return "生成的 HTTP 模板"
}

func defaultNucleiTemplatePolishDescription(locale string) string {
	if normalizeNucleiTemplatePolishLocale(locale) == "en" {
		return "Generated from Request Repeater"
	}
	return "基于请求转发生成"
}

func truncateRepeaterTemplateContext(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= repeaterTemplateContextLimit {
		return value
	}
	return value[:repeaterTemplateContextLimit] + "\n...[truncated]"
}

func parseNucleiTemplatePolishResult(content string) (nucleiTemplatePolishResult, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var result nucleiTemplatePolishResult
	if err := json.Unmarshal([]byte(content), &result); err == nil {
		return result, nil
	}

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(content[start:end+1]), &result); err == nil {
			return result, nil
		}
	}

	return nucleiTemplatePolishResult{}, errors.New("AI 返回内容不是有效 JSON")
}

var nucleiTemplateIDInvalidChars = regexp.MustCompile(`[^a-z0-9]+`)

func sanitizeNucleiTemplateID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = nucleiTemplateIDInvalidChars.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "generated-http-template"
	}
	return value
}

func normalizeNucleiSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "medium", "high", "critical", "info":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "info"
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeRepeaterTemplateDebugTarget(raw string) (string, error) {
	target := strings.TrimSpace(raw)
	if target == "" {
		return "", errors.New("验证目标不能为空")
	}

	parsed, err := url.Parse(target)
	if err != nil || parsed.Host == "" {
		return "", errors.New("验证目标必须是有效的 http/https URL")
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("验证目标仅支持 http/https")
	}

	return scheme + "://" + parsed.Host + "/", nil
}

func runRepeaterNucleiTemplateDebug(ctx context.Context, target string, templateBytes []byte) (nucleiTemplateDebugResult, error) {
	result := nucleiTemplateDebugResult{
		Target: target,
		Status: "not_matched",
		Events: []nucleiTemplateDebugEvent{
			{Type: "info", Message: "Sending HTTP requests..."},
		},
	}

	if err := webscan.RegisterCodeTemplateSignerForNuclei(); err != nil {
		result.Status = "failed"
		result.Events = append(result.Events, nucleiTemplateDebugEvent{Type: "error", Message: "初始化 nuclei 签名器失败: " + err.Error()})
		return result, err
	}

	tmpFile, err := os.CreateTemp("", "hephaestus-repeater-nuclei-*.yaml")
	if err != nil {
		result.Status = "failed"
		result.Events = append(result.Events, nucleiTemplateDebugEvent{Type: "error", Message: "创建临时模板失败: " + err.Error()})
		return result, err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(templateBytes); err != nil {
		_ = tmpFile.Close()
		result.Status = "failed"
		result.Events = append(result.Events, nucleiTemplateDebugEvent{Type: "error", Message: "写入临时模板失败: " + err.Error()})
		return result, err
	}
	if err := tmpFile.Close(); err != nil {
		result.Status = "failed"
		result.Events = append(result.Events, nucleiTemplateDebugEvent{Type: "error", Message: "关闭临时模板失败: " + err.Error()})
		return result, err
	}

	writer := newNucleiTemplateDebugWriter()
	engine, err := nuclei.NewNucleiEngineCtx(
		ctx,
		nuclei.DisableUpdateCheck(),
		nuclei.EnableCodeTemplates(),
		nuclei.EnableMatcherStatus(),
		nuclei.WithTemplatesOrWorkflows(nuclei.TemplateSources{Templates: []string{tmpPath}}),
		nuclei.WithVerbosity(nuclei.VerbosityOptions{
			Debug:         true,
			DebugRequest:  true,
			DebugResponse: true,
		}),
		nuclei.WithNetworkConfig(nuclei.NetworkConfig{
			DisableMaxHostErr: true,
			Timeout:           15,
			Retries:           0,
		}),
		nuclei.UseOutputWriter(writer),
	)
	if err != nil {
		result.Status = "failed"
		result.Events = append(result.Events, nucleiTemplateDebugEvent{Type: "error", Message: "初始化 nuclei 引擎失败: " + err.Error()})
		return result, err
	}
	defer engine.Close()

	result.Events = append(result.Events, nucleiTemplateDebugEvent{Type: "info", Message: "Analyzing responses..."})
	engine.LoadTargets([]string{target}, false)
	if err := engine.ExecuteWithCallback(nil); err != nil {
		result.Status = "failed"
		result.Events = append(result.Events, writer.snapshotEvents()...)
		result.Events = append(result.Events, nucleiTemplateDebugEvent{Type: "error", Message: "执行 nuclei 验证失败: " + err.Error()})
		result.Results = writer.snapshotResults()
		return result, err
	}

	result.Events = append(result.Events, nucleiTemplateDebugEvent{Type: "info", Message: "Checking for matches..."})
	result.Events = append(result.Events, writer.snapshotEvents()...)
	result.Results = writer.snapshotResults()
	result.Matched = writer.hasMatch()
	if result.Matched {
		result.Status = "matched"
	} else {
		result.Status = "not_matched"
	}
	result.Events = append(result.Events, nucleiTemplateDebugEvent{Type: "success", Message: "Scan completed successfully"})

	return result, nil
}

func newNucleiTemplateDebugWriter() *nucleiTemplateDebugWriter {
	return &nucleiTemplateDebugWriter{
		aurora: aurora.NewAurora(false),
	}
}

func (w *nucleiTemplateDebugWriter) Close() {}

func (w *nucleiTemplateDebugWriter) Colorizer() aurora.Aurora {
	return w.aurora
}

func (w *nucleiTemplateDebugWriter) ResultCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.results)
}

func (w *nucleiTemplateDebugWriter) Request(templateID, input, requestType string, requestErr error) {
	if requestErr != nil {
		w.appendEvent(nucleiTemplateDebugEvent{
			Type:    "error",
			Message: "[" + templateID + "] " + requestType + " request failed: " + requestErr.Error(),
		})
		return
	}
	w.appendEvent(nucleiTemplateDebugEvent{
		Type:    "request",
		Message: "[" + templateID + "] Sent [" + strings.ToUpper(requestType) + "] request to " + input,
	})
}

func (w *nucleiTemplateDebugWriter) RequestStatsLog(statusCode, response string) {}

func (w *nucleiTemplateDebugWriter) WriteStoreDebugData(host, templateID, eventType string, data string) {
	eventType = strings.ToLower(strings.TrimSpace(eventType))
	debugEvent := nucleiTemplateDebugEvent{
		Type:    "info",
		Message: "[" + templateID + "] Debug data for " + host,
	}
	if strings.Contains(data, "Dumped HTTP request") || strings.Contains(data, "request for") {
		debugEvent.Type = "request"
		debugEvent.Request = data
		debugEvent.Message = "[" + templateID + "] Sent [" + strings.ToUpper(eventType) + "] request"
	} else if strings.Contains(data, "Dumped HTTP response") || strings.Contains(data, "response for") {
		debugEvent.Type = "response"
		debugEvent.Response = data
		debugEvent.Message = "[" + templateID + "] Received [" + strings.ToUpper(eventType) + "] response"
	}
	w.appendEvent(debugEvent)
}

func (w *nucleiTemplateDebugWriter) Write(event *output.ResultEvent) error {
	if event == nil {
		return nil
	}

	matchedAt := event.Matched
	if matchedAt == "" {
		matchedAt = event.URL
	}
	templateName := event.Info.Name
	debugOutput := nucleiTemplateDebugOutput{
		TemplateID:    event.TemplateID,
		Name:          templateName,
		Type:          event.Type,
		MatcherStatus: event.MatcherStatus,
		MatchedAt:     matchedAt,
		Request:       event.Request,
		Response:      event.Response,
		Error:         event.Error,
	}

	debugType := "miss"
	message := "[" + event.TemplateID + "] Not matched"
	if event.Error != "" {
		debugType = "error"
		message = "[" + event.TemplateID + "] " + event.Error
	} else if event.MatcherStatus {
		debugType = "match"
		message = "[" + event.TemplateID + "] Matched " + matchedAt
	}

	debugEvent := nucleiTemplateDebugEvent{
		Type:     debugType,
		Message:  message,
		Request:  event.Request,
		Response: event.Response,
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if event.MatcherStatus && event.Error == "" {
		w.matched = true
	}
	w.results = append(w.results, debugOutput)
	w.events = append(w.events, debugEvent)
	return nil
}

func (w *nucleiTemplateDebugWriter) WriteFailure(wrappedEvent *output.InternalWrappedEvent) error {
	if wrappedEvent == nil {
		return nil
	}
	for _, result := range wrappedEvent.Results {
		result.MatcherStatus = false
		if err := w.Write(result); err != nil {
			return err
		}
	}
	if len(wrappedEvent.Results) > 0 {
		return nil
	}

	event := wrappedEvent.InternalEvent
	templateID := strings.TrimSpace(toDebugString(event["template-id"]))
	if templateID == "" {
		templateID = "template"
	}
	request := toDebugString(event["request"])
	response := toDebugString(event["response"])
	errText := toDebugString(event["error"])

	debugType := "miss"
	message := "[" + templateID + "] Not matched"
	if errText != "" {
		debugType = "error"
		message = "[" + templateID + "] " + errText
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.results = append(w.results, nucleiTemplateDebugOutput{
		TemplateID:    templateID,
		Type:          toDebugString(event["type"]),
		MatcherStatus: false,
		MatchedAt:     toDebugString(event["host"]),
		Request:       request,
		Response:      response,
		Error:         errText,
	})
	w.events = append(w.events, nucleiTemplateDebugEvent{
		Type:     debugType,
		Message:  message,
		Request:  request,
		Response: response,
	})
	return nil
}

func (w *nucleiTemplateDebugWriter) appendEvent(event nucleiTemplateDebugEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, event)
}

func (w *nucleiTemplateDebugWriter) snapshotEvents() []nucleiTemplateDebugEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	events := make([]nucleiTemplateDebugEvent, len(w.events))
	copy(events, w.events)
	return events
}

func (w *nucleiTemplateDebugWriter) snapshotResults() []nucleiTemplateDebugOutput {
	w.mu.Lock()
	defer w.mu.Unlock()
	results := make([]nucleiTemplateDebugOutput, len(w.results))
	copy(results, w.results)
	return results
}

func (w *nucleiTemplateDebugWriter) hasMatch() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.matched
}

func toDebugString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return strings.TrimSpace(strings.ReplaceAll(strings.TrimSpace(strings.Trim(fmt.Sprint(value), "\n")), "\x00", ""))
}
