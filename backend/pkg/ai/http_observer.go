package ai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	workbenchHTTPObserveTimeout = 8 * time.Second
	workbenchHTTPObserveLimit   = 5
	workbenchHTTPBodyLimit      = 128 * 1024
)

// WorkbenchHTTPObservationResult 是 AI 通用 HTTP 观察的汇总结果。
type WorkbenchHTTPObservationResult struct {
	Targets      []WorkbenchHTTPTargetObservation `json:"targets"`
	Skipped      []string                         `json:"skipped"`
	Forms        int                              `json:"forms"`
	Scripts      int                              `json:"scripts"`
	APIRoutes    []string                         `json:"apiRoutes"`
	LoginSignals []string                         `json:"loginSignals"`
}

// WorkbenchHTTPTargetObservation 记录单个 URL 的低侵入观察。
type WorkbenchHTTPTargetObservation struct {
	URL            string            `json:"url"`
	StatusCode     int               `json:"statusCode"`
	ContentType    string            `json:"contentType"`
	Title          string            `json:"title"`
	Server         string            `json:"server"`
	PoweredBy      string            `json:"poweredBy"`
	Forms          int               `json:"forms"`
	Scripts        int               `json:"scripts"`
	APIRoutes      []string          `json:"apiRoutes"`
	RequestHeaders map[string]string `json:"requestHeaders,omitempty"`
	Headers        map[string]string `json:"headers"`
	Error          string            `json:"error,omitempty"`
}

// ObserveWorkbenchHTTPTargets 对任务范围内的 HTTP/HTTPS URL 执行只读 GET 观察。
func ObserveWorkbenchHTTPTargets(ctx context.Context, target string, scope []string) WorkbenchHTTPObservationResult {
	return ObserveWorkbenchHTTPTargetsWithClient(ctx, target, scope, nil)
}

// ObserveWorkbenchHTTPTargetsWithHeaders 对目标执行只读 GET 观察，并允许调用方注入已授权的认证头。
func ObserveWorkbenchHTTPTargetsWithHeaders(ctx context.Context, target string, scope []string, headers map[string]string) WorkbenchHTTPObservationResult {
	return ObserveWorkbenchHTTPTargetsWithClientAndHeaders(ctx, target, scope, nil, headers)
}

// ObserveWorkbenchHTTPTargetsWithClient 允许测试注入 http.Client。
func ObserveWorkbenchHTTPTargetsWithClient(ctx context.Context, target string, scope []string, client *http.Client) WorkbenchHTTPObservationResult {
	return ObserveWorkbenchHTTPTargetsWithClientAndHeaders(ctx, target, scope, client, nil)
}

// ObserveWorkbenchHTTPTargetsWithClientAndHeaders 允许测试注入 http.Client 和请求头。
func ObserveWorkbenchHTTPTargetsWithClientAndHeaders(ctx context.Context, target string, scope []string, client *http.Client, headers map[string]string) WorkbenchHTTPObservationResult {
	urls, skipped := collectWorkbenchHTTPObservationURLs(target, scope)
	result := WorkbenchHTTPObservationResult{
		Skipped:   skipped,
		APIRoutes: make([]string, 0, 16),
	}
	if client == nil {
		client = &http.Client{Timeout: workbenchHTTPObserveTimeout}
	}

	for _, targetURL := range urls {
		observation := observeWorkbenchHTTPURL(ctx, client, targetURL, headers)
		result.Targets = append(result.Targets, observation)
		result.Forms += observation.Forms
		result.Scripts += observation.Scripts
		result.APIRoutes = appendDedupStrings(result.APIRoutes, observation.APIRoutes...)
		if hasWorkbenchLoginSignal(observation) {
			result.LoginSignals = appendDedupStrings(result.LoginSignals, observation.URL)
		}
	}
	sort.Strings(result.APIRoutes)
	sort.Strings(result.LoginSignals)
	return result
}

func (r WorkbenchHTTPObservationResult) Summary() string {
	if len(r.Targets) == 0 {
		if len(r.Skipped) == 0 {
			return "未发现可执行 HTTP 观察的 URL。"
		}
		return "未发现可执行 HTTP 观察的 URL；已跳过非 HTTP 目标: " + strings.Join(r.Skipped, ", ")
	}
	lines := []string{
		fmt.Sprintf("已完成 %d 个 HTTP/HTTPS URL 的只读观察，发现表单 %d 个、脚本 %d 个、接口线索 %d 条。", len(r.Targets), r.Forms, r.Scripts, len(r.APIRoutes)),
	}
	for _, item := range r.Targets {
		status := fmt.Sprintf("%d", item.StatusCode)
		if item.Error != "" {
			status = "error"
		}
		lines = append(lines, fmt.Sprintf("- %s [%s] %s %s", item.URL, status, item.Title, item.Error))
		if item.Server != "" || item.PoweredBy != "" {
			lines = append(lines, fmt.Sprintf("  头部: Server=%s X-Powered-By=%s", item.Server, item.PoweredBy))
		}
		if len(item.APIRoutes) > 0 {
			lines = append(lines, "  接口线索: "+strings.Join(item.APIRoutes, ", "))
		}
	}
	if len(r.LoginSignals) > 0 {
		lines = append(lines, "登录/认证线索: "+strings.Join(r.LoginSignals, ", "))
	}
	if len(r.Skipped) > 0 {
		lines = append(lines, "已跳过非 HTTP 目标: "+strings.Join(r.Skipped, ", "))
	}
	return strings.Join(lines, "\n")
}

func collectWorkbenchHTTPObservationURLs(target string, scope []string) ([]string, []string) {
	targets := normalizeWorkbenchScanTargets(target, scope)
	urls := make([]string, 0, len(targets))
	skipped := make([]string, 0, len(targets))
	seen := make(map[string]bool, len(targets))
	for _, item := range targets {
		parsed, err := url.Parse(item)
		if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			skipped = append(skipped, item)
			continue
		}
		normalized := strings.TrimRight(parsed.String(), "/")
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		urls = append(urls, normalized)
		if len(urls) >= workbenchHTTPObserveLimit {
			break
		}
	}
	return urls, skipped
}

func observeWorkbenchHTTPURL(ctx context.Context, client *http.Client, targetURL string, headers map[string]string) WorkbenchHTTPTargetObservation {
	observation := WorkbenchHTTPTargetObservation{
		URL:            targetURL,
		RequestHeaders: redactWorkbenchHTTPRequestHeaders(headers),
		Headers:        make(map[string]string),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		observation.Error = err.Error()
		return observation
	}
	req.Header.Set("User-Agent", "Hephaestus-AI-Workbench/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,text/plain;q=0.9,*/*;q=0.8")
	for key, value := range sanitizeWorkbenchHTTPRequestHeaders(headers) {
		req.Header.Set(key, value)
	}
	observation.RequestHeaders["Accept"] = req.Header.Get("Accept")
	observation.RequestHeaders["User-Agent"] = req.Header.Get("User-Agent")

	resp, err := client.Do(req)
	if err != nil {
		observation.Error = err.Error()
		return observation
	}
	defer resp.Body.Close()

	observation.StatusCode = resp.StatusCode
	observation.ContentType = resp.Header.Get("Content-Type")
	observation.Server = resp.Header.Get("Server")
	observation.PoweredBy = resp.Header.Get("X-Powered-By")
	for _, key := range []string{"Content-Type", "Server", "X-Powered-By", "Location", "Set-Cookie"} {
		if value := resp.Header.Get(key); value != "" {
			observation.Headers[key] = truncateWorkbenchText(value, 180)
		}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, workbenchHTTPBodyLimit))
	if err != nil {
		observation.Error = err.Error()
		return observation
	}
	enrichWorkbenchHTTPObservationFromBody(&observation, body)
	return observation
}

func sanitizeWorkbenchHTTPRequestHeaders(headers map[string]string) map[string]string {
	cleaned := map[string]string{}
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" || strings.ContainsAny(key, "\r\n") {
			continue
		}
		cleaned[key] = strings.TrimSpace(value)
	}
	return cleaned
}

func redactWorkbenchHTTPRequestHeaders(headers map[string]string) map[string]string {
	redacted := map[string]string{}
	for key, value := range sanitizeWorkbenchHTTPRequestHeaders(headers) {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "authorization") || strings.Contains(lower, "cookie") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "key") {
			redacted[key] = "***"
			continue
		}
		redacted[key] = value
	}
	return redacted
}

func enrichWorkbenchHTTPObservationFromBody(observation *WorkbenchHTTPTargetObservation, body []byte) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		text := string(body)
		observation.APIRoutes = extractWorkbenchAPIRoutes(text)
		return
	}
	observation.Title = truncateWorkbenchText(strings.TrimSpace(doc.Find("title").First().Text()), 120)
	observation.Forms = doc.Find("form").Length()
	observation.Scripts = doc.Find("script").Length()

	doc.Find("a[href], form[action], script[src]").Each(func(_ int, selection *goquery.Selection) {
		for _, attr := range []string{"href", "action", "src"} {
			value, exists := selection.Attr(attr)
			if !exists {
				continue
			}
			if strings.Contains(strings.ToLower(value), "api") || strings.Contains(strings.ToLower(value), "auth") || strings.Contains(strings.ToLower(value), "login") {
				observation.APIRoutes = appendDedupStrings(observation.APIRoutes, truncateWorkbenchText(value, 160))
			}
		}
	})
	observation.APIRoutes = appendDedupStrings(observation.APIRoutes, extractWorkbenchAPIRoutes(string(body))...)
	sort.Strings(observation.APIRoutes)
}

func extractWorkbenchAPIRoutes(text string) []string {
	result := make([]string, 0, 8)
	for _, marker := range []string{"/api/", "/auth/", "/login", "/graphql", "/oauth"} {
		searchStart := 0
		lower := strings.ToLower(text)
		for {
			index := strings.Index(lower[searchStart:], marker)
			if index < 0 {
				break
			}
			start := searchStart + index
			if start > 0 && !isWorkbenchRouteDelimiter(text[start-1]) {
				searchStart = start + len(marker)
				continue
			}
			end := start
			for end < len(text) {
				ch := text[end]
				if isWorkbenchRouteDelimiter(ch) {
					break
				}
				end++
			}
			if end > start {
				result = appendDedupStrings(result, truncateWorkbenchText(text[start:end], 160))
			}
			searchStart = start + len(marker)
		}
	}
	return result
}

func isWorkbenchRouteDelimiter(ch byte) bool {
	switch ch {
	case '"', '\'', '<', '>', ' ', '\n', '\r', '\t', '`', '(', ')', '[', ']', '{', '}':
		return true
	default:
		return false
	}
}

func hasWorkbenchLoginSignal(observation WorkbenchHTTPTargetObservation) bool {
	text := strings.ToLower(strings.Join(append([]string{observation.URL, observation.Title}, observation.APIRoutes...), " "))
	return strings.Contains(text, "login") || strings.Contains(text, "auth") || strings.Contains(text, "oauth")
}

func appendDedupStrings(values []string, candidates ...string) []string {
	seen := make(map[string]bool, len(values)+len(candidates))
	for _, value := range values {
		seen[value] = true
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		values = append(values, candidate)
	}
	return values
}
