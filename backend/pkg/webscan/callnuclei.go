package webscan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/types"

	"hephaestus-backend/pkg/logger"

	nuclei "github.com/projectdiscovery/nuclei/v3/lib"
	"github.com/projectdiscovery/nuclei/v3/pkg/output"
	syncutil "github.com/projectdiscovery/utils/sync"
	arrayutil "github.com/qiwentaidi/utils/array"
)

const (
	maxResponseSize = 1024 * 64 // 最大响应大小64KB
)

// GetPoCDir 获取 PoC 文件夹路径（与 config.yaml 同级的 config/pocs 目录）
func GetPoCDir() string {
	return filepath.Join(config.GetConfigResourceRoot(), "pocs")
}

// NewThreadSafeNucleiEngine 创建线程安全的Nuclei引擎并执行扫描
func NewThreadSafeNucleiEngine(ctx context.Context, nucleiOpts types.NucleiOptions, callback VulnerabilityCallback, jobDone NucleiJobDoneCallback) error {
	// 在函数入口检查 context
	if ctx.Err() != nil {
		logNucleiContextStop(ctx.Err())
		return ctx.Err()
	}
	if err := RegisterCodeTemplateSignerForNuclei(); err != nil {
		logger.Warning("[nuclei] code template signer verifier initialization failed: %v", err)
	}

	count := len(nucleiOpts.Jobs)
	logProgressOffset := nucleiOpts.LogProgressOffset
	logProgressTotal := nucleiOpts.LogProgressTotal
	if logProgressTotal <= 0 {
		logProgressTotal = count
	}
	engineOpts := buildThreadSafeEngineOptions(nucleiOpts.Headers, nucleiOpts.DisableMaxHostErr, nucleiOpts.Interactsh, nucleiOpts.Proxy)
	ne, err := nuclei.NewThreadSafeNucleiEngineCtx(ctx, engineOpts...)
	if err != nil {
		logger.Error("[nuclei] init engine err: %v", err)
		return err
	}
	defer ne.Close() // 确保引擎关闭

	var startedCount int32
	var completedCount int32
	threadSize := nucleiOpts.Threads
	if threadSize < 1 {
		threadSize = 5
	}
	sg, err := syncutil.New(syncutil.WithSize(threadSize))
	if err != nil {
		logger.Error("[nuclei] init sync group err: %v", err)
		return err
	}

	ne.GlobalResultCallback(func(event *output.ResultEvent) {
		// 在回调中检查 context
		if ctx.Err() != nil {
			return
		}

		if showMatched(event) == "" {
			return
		}
		severity := strings.ToUpper(event.Info.SeverityHolder.Severity.String())
		var reference string
		if event.Info.Reference != nil && !event.Info.Reference.IsEmpty() {
			reference = strings.Join(event.Info.Reference.ToSlice(), ",")
		}

		// 判断是否为原理检测
		principleDetection := false
		if val, ok := event.Info.Metadata["principle-detection"]; ok {
			if boolVal, ok := val.(bool); ok && boolVal {
				principleDetection = true
			}
		}

		// 构建漏洞信息
		vuln := types.VulnerabilityInfo{
			ID:                 event.TemplateID,
			Name:               event.Info.Name,
			Description:        event.Info.Description,
			Impact:             event.Info.Impact,
			Remediation:        event.Info.Remediation,
			Reference:          reference,
			URL:                showMatched(event),
			Request:            showRequest(event),
			Response:           showResponse(event),
			ResponseTime:       limitDecimalPlaces(event.ResponseTime),
			Extract:            strings.Join(event.ExtractedResults, " | "),
			Type:               strings.ToUpper(event.Type),
			Severity:           strings.ToUpper(event.Info.SeverityHolder.Severity.String()),
			PrincipleDetection: principleDetection,
		}

		// 输出到命令行
		principleTag := ""
		if principleDetection {
			principleTag = " [原理检测]"
		}
		logger.Vulnerability("[%s] [%s] %s %s%s\n", event.TemplateID, severity, showMatched(event), strings.Join(event.ExtractedResults, " | "), principleTag)

		// 调用回调函数前检查 context（GORM 本身是线程安全的，不需要额外加锁）
		if callback != nil && ctx.Err() == nil {
			callback(vuln)
		}
	})

	// 提交扫描任务
	for _, job := range nucleiOpts.Jobs {
		// 在循环中检查 context
		if ctx.Err() != nil {
			logNucleiSubmitStop(ctx.Err())
			break
		}

		// 先检查是否需要跳过，避免 sg.Add() 后 continue 导致 Wait() 一直等待
		if nucleiOpts.RequireTemplateTags && len(job.Tags) == 0 {
			logger.Info("[nuclei] %s does not have tags, scan skipped", job.URL)
			completed := int(atomic.AddInt32(&completedCount, 1)) + logProgressOffset
			logger.Info("vulnerability scanning skipped %s %d/%d", job.URL, completed, logProgressTotal)
			if jobDone != nil {
				jobDone(job.URL)
			}
			continue
		}

		templatesToUse := resolveTemplatesForJob(job)
		if len(templatesToUse) == 0 {
			logger.Info("[nuclei] %s did not match any template, scan skipped", job.URL)
			completed := int(atomic.AddInt32(&completedCount, 1)) + logProgressOffset
			logger.Info("vulnerability scanning skipped %s %d/%d", job.URL, completed, logProgressTotal)
			if jobDone != nil {
				jobDone(job.URL)
			}
			continue
		}

		sg.Add()
		tempOpts := nuclei.WithTemplatesOrWorkflows(nuclei.TemplateSources{
			Templates: templatesToUse,
		})
		execTarget := normalizeNucleiTarget(job.URL)
		started := int(atomic.AddInt32(&startedCount, 1)) + logProgressOffset
		logger.Info("vulnerability scanning started %s, load templates %d %d/%d", job.URL, len(templatesToUse), started, logProgressTotal)
		go func(url string, execTarget string, template nuclei.NucleiSDKOptions) {
			defer sg.Done()

			defer func() {
				// ⚠️ 关键：添加 panic recovery，防止 goroutine panic 导致 sg.Done() 不执行
				if r := recover(); r != nil {
					logger.Error("[nuclei] goroutine panic recovered for %s: %v", url, r)
				}
				completed := int(atomic.AddInt32(&completedCount, 1)) + logProgressOffset
				logger.Info("vulnerability scanning finished %s %d/%d", url, completed, logProgressTotal)
				if jobDone != nil {
					jobDone(url)
				}
			}()

			// ⚠️ 在 goroutine 开始时检查 context 是否已取消
			select {
			case <-ctx.Done():
				logger.Warning("[nuclei] context stopped for %s: %v", url, ctx.Err())
				return
			default:
			}

			err := ne.ExecuteNucleiWithOpts([]string{execTarget}, tempOpts)
			if err != nil {
				// 区分是否是 context 取消导致的错误
				if ctx.Err() != nil {
					logger.Warning("[nuclei] execution stopped for %s: %v", url, ctx.Err())
				} else if isBenignNucleiSkipError(err) {
					logger.Info("[nuclei] %s has no executable templates after filtering unsigned, invalid, or unsupported templates: %v", url, err)
				} else {
					logger.Error("[nuclei] execute callback err: %v", err)
				}
				return
			}
		}(job.URL, execTarget, tempOpts)
	}
	sg.Wait()
	if ctx.Err() != nil {
		logNucleiContextStop(ctx.Err())
		return ctx.Err()
	}
	return nil
}

func logNucleiContextStop(err error) {
	switch err {
	case nil:
		return
	case context.Canceled:
		logger.Info("Nuclei 扫描已取消")
	case context.DeadlineExceeded:
		logger.Warning("Nuclei 扫描因超时停止: %v", err)
	default:
		logger.Warning("Nuclei 扫描已停止: %v", err)
	}
}

func logNucleiSubmitStop(err error) {
	switch err {
	case nil:
		return
	case context.Canceled:
		logger.Info("Nuclei 扫描已取消，停止提交新任务")
	case context.DeadlineExceeded:
		logger.Warning("Nuclei 扫描因超时停止提交新任务: %v", err)
	default:
		logger.Warning("Nuclei 扫描停止提交新任务: %v", err)
	}
}

// 在初始化引擎配置的全局环境
func buildThreadSafeEngineOptions(headers []string, disableMaxHostErr bool, interactshOpts types.InteractshOptions, proxy string) []nuclei.NucleiSDKOptions {
	sdkOpts := []nuclei.NucleiSDKOptions{
		nuclei.DisableUpdateCheck(),  // -duc
		nuclei.EnableCodeTemplates(), // -ect
	}

	customInteractshConfigured := strings.TrimSpace(interactshOpts.InteractshURL) != "" || strings.TrimSpace(interactshOpts.InteractshToken) != ""
	// 1) OOB 关闭：显式禁用
	// 2) OOB 开启且 URL/Token 都为空：不传 WithInteractshOptions（走 SDK 默认）
	// 3) OOB 开启且 URL 或 Token 非空：传自定义 OOB 配置
	if !interactshOpts.OOBEnabled || customInteractshConfigured {
		sdkOpts = append(sdkOpts, nuclei.WithInteractshOptions(buildInteractshOptions(interactshOpts)))
	}

	if networkCfg, ok := buildNetworkCardOptions(disableMaxHostErr); ok {
		sdkOpts = append(sdkOpts, nuclei.WithNetworkConfig(networkCfg))
	}

	if len(headers) != 0 {
		sdkOpts = append(sdkOpts, nuclei.WithHeaders(headers))
	}
	if strings.TrimSpace(proxy) != "" {
		sdkOpts = append(sdkOpts, nuclei.WithProxy([]string{strings.TrimSpace(proxy)}, true))
	}

	return sdkOpts
}

// 构建OOB配置，只能在初始化引擎的时候调用，支持自定义地址
func buildInteractshOptions(o types.InteractshOptions) nuclei.InteractshOpts {
	serverURL := ""
	if normalized, err := normalizeInteractshServerURL(o.InteractshURL); err != nil {
		logger.Warning("[nuclei] invalid interactsh URL %q, fallback to default server: %v", o.InteractshURL, err)
	} else {
		serverURL = normalized
	}

	opts := nuclei.InteractshOpts{
		CacheSize:           5000,
		Eviction:            60 * time.Second,
		CooldownPeriod:      5 * time.Second,
		PollDuration:        5 * time.Second,
		DisableHttpFallback: true,
		NoColor:             false,
		ServerURL:           serverURL,
		Authorization:       strings.TrimSpace(o.InteractshToken),
	}

	if !o.OOBEnabled {
		opts.NoInteractsh = true
		logger.Info("[nuclei] interactsh explicitly disabled by task config")
		return opts
	}

	opts.NoInteractsh = false
	if serverURL != "" {
		logger.Info("[nuclei] interactsh enabled, using custom server: %s", serverURL)
	} else {
		logger.Info("[nuclei] interactsh enabled, using default server pool")
	}

	return opts
}

func normalizeInteractshServerURL(raw string) (string, error) {
	target := strings.TrimSpace(raw)
	if target == "" {
		return "", nil
	}
	if !strings.Contains(target, "://") {
		target = "https://" + target
	}

	parsed, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("missing host")
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("unsupported scheme: %s", parsed.Scheme)
	}

	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

// 从进程的环境变量中获取网卡配置，只在初始化时使用
func buildNetworkCardOptions(disableMaxHostErr bool) (nuclei.NetworkConfig, bool) {
	cfg := nuclei.NetworkConfig{
		// Stable-first defaults for bulk scans: tolerate slower targets and transient failures.
		MaxHostError:      defaultNucleiMaxHostErrors,
		DisableMaxHostErr: disableMaxHostErr,
		Timeout:           defaultNucleiTimeout,
		Retries:           defaultNucleiRetries,
	}
	v := strings.TrimSpace(os.Getenv("DEFAULT_NETWORK"))
	if v == "" || strings.EqualFold(v, "auto") {
		return cfg, true
	}

	// IP => SourceIP
	if ip := net.ParseIP(v); ip != nil {
		cfg.SourceIP = ip.String()
		return cfg, true
	}

	// 网卡名 => Interface (+ 尝试解析一个 IPv4 到 SourceIP 提升兼容性)
	cfg.Interface = v
	iface, err := net.InterfaceByName(v)
	if err == nil {
		if ip, err2 := firstUsableIPv4(iface); err2 == nil && ip != nil {
			cfg.SourceIP = ip.String()
		}
	}
	return cfg, true
}

func firstUsableIPv4(iface *net.Interface) (net.IP, error) {
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		var ip net.IP
		switch x := addr.(type) {
		case *net.IPNet:
			ip = x.IP
		case *net.IPAddr:
			ip = x.IP
		}
		if ip == nil {
			continue
		}
		if v4 := ip.To4(); v4 != nil && !v4.IsLoopback() {
			// 可选：过滤 169.254.x.x
			if v4[0] == 169 && v4[1] == 254 {
				continue
			}
			return v4, nil
		}
	}
	return nil, fmt.Errorf("no usable ipv4")
}

func resolveTemplatesForJob(job types.NucleiScanJob) []string {
	var templatesToUse []string
	isWeb := true
	u, err := url.Parse(job.URL)
	if err != nil || u.Scheme == "" {
		isWeb = false
	} else if u.Scheme != "http" && u.Scheme != "https" {
		// 非web协议目标（如DNS、TCP等）优先使用协议名作为标签匹配模板，增强指纹匹配的准确性
		isWeb = false
		job.Tags = append(job.Tags, u.Scheme)
	}

	// 未指定模板时根据指纹进行选择
	if len(job.Template) == 0 {
		templates := findTagsFile(isWeb, job.Tags, nil)
		templatesToUse = append(templatesToUse, templates...)
	} else {
		for _, t := range job.Template {
			t = ResolveTemplateSelection(t)
			// 先尝试获取文件信息
			if info, err := os.Stat(t); err == nil && info.IsDir() {
				// 如果是目录，使用 findTagsFile 查找该目录下符合 tag 的模板
				templates := findTagsFile(isWeb, job.Tags, []string{t})
				templatesToUse = append(templatesToUse, templates...)
			} else {
				// 否则当作单个模板文件（或路径不存在时也直接追加，让 nuclei 自行处理错误）
				// 这里使用 filepath.Clean 保持路径规范
				templatesToUse = append(templatesToUse, filepath.Clean(t))
			}
		}
	}

	return arrayutil.RemoveDuplicates(templatesToUse)
}

func normalizeNucleiTarget(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" {
		return raw
	}

	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme == "http" || scheme == "https" {
		return raw
	}

	if parsed.Host != "" {
		return parsed.Host
	}

	return raw
}

func isBenignNucleiSkipError(err error) bool {
	return errors.Is(err, nuclei.ErrNoTemplatesAvailable)
}

// 获取在指定模板路径下对应指纹的poc文件
func findTagsFile(isWeb bool, inputTags, templateDirs []string) []string {
	var tempFileList []string

	// 使用读锁保护 WorkFlowDB 的访问
	mutx.RLock()
	dbCopy := make(map[string]Template, len(WorkFlowDB))
	maps.Copy(dbCopy, WorkFlowDB)
	mutx.RUnlock()

	// 获取正确的模板路径
	for _, template := range dbCopy {
		if len(templateDirs) > 0 && !isTemplateInDirs(template.FilePath, templateDirs) {
			continue
		}

		// metadata.dast=true 的模板需要对全部Web目标进行检测
		if isWeb && templateMetadataBool(template.Info.Metadata, "dast") {
			tempFileList = append(tempFileList, template.FilePath)
			continue
		}

		tags := strings.Split(template.Info.Tags, ",")
		for _, inputTag := range inputTags {
			// 使用忽略大小写增强指纹匹配
			if arrayutil.ArrayContainsIgnoreCase(inputTag, tags) {
				tempFileList = append(tempFileList, template.FilePath)
				break
			}
		}
	}

	// 去重
	return arrayutil.RemoveDuplicates(tempFileList)
}

func isTemplateInDirs(templatePath string, templateDirs []string) bool {
	templateAbs, err := filepath.Abs(templatePath)
	if err != nil {
		templateAbs = templatePath
	}
	templateAbs = filepath.Clean(templateAbs)

	for _, dir := range templateDirs {
		dir = normalizePOCTemplateFolderPath(dir)
		if dir == "" {
			continue
		}
		dirPrefix := dir + string(os.PathSeparator)
		if templateAbs == dir || strings.HasPrefix(templateAbs, dirPrefix) {
			return true
		}
	}
	return false
}

func templateMetadataBool(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	raw, exists := metadata[key]
	if !exists {
		return false
	}

	switch value := raw.(type) {
	case bool:
		return value
	case string:
		normalized := strings.TrimSpace(strings.ToLower(value))
		return normalized == "true" || normalized == "1" || normalized == "yes"
	case int:
		return value != 0
	case int64:
		return value != 0
	case float64:
		return value != 0
	default:
		return false
	}
}

// 展开获取文件夹下的所有yaml文件路径
func expandYamlFiles(dirs []string) []string {
	var files []string
	for _, dir := range dirs {
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".yaml") {
				files = append(files, path)
			}
			return nil
		})
	}
	return files
}

// 兼容DNS等漏洞显示匹配URL地址
func showMatched(event *output.ResultEvent) string {
	if event.Matched != "" {
		return event.Matched
	}
	return event.URL
}

// 兼容DNS等漏洞显示请求内容
func showRequest(event *output.ResultEvent) string {
	if event.Request != "" {
		return event.Request
	}
	if event.Interaction != nil {
		return event.Interaction.RawRequest
	}
	return ""
}

// 兼容DNS限制显示响应包的最大内容数
func showResponse(event *output.ResultEvent) string {
	if event.Response != "" {
		byteResponse := []byte(event.Response)
		// ⚠️ 优化：将响应体大小限制从 512KB 降低到 64KB，减少内存占用
		if len(byteResponse) > maxResponseSize {
			return string(byteResponse[:maxResponseSize]) + " ...[truncated]"
		}
		return event.Response
	}
	if event.Interaction != nil {
		return event.Interaction.RawResponse
	}
	return ""
}

// 限制小数为2位，用于截取时间字符串
func limitDecimalPlaces(value string) string {
	parts := strings.Split(value, ".")
	if len(parts) == 2 && len(parts[1]) > 2 {
		value = parts[0] + "." + parts[1][:2] // 截取小数点后两位
	}
	return value
}
