package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"hephaestus-backend/pkg/config"
)

const testRequestTimeout = 15 * time.Second

// TestAvailability validates the endpoint and performs a minimal OpenAI-compatible request.
func TestAvailability(ctx context.Context, cfg config.AIConfig) error {
	cfg = config.ResolveAIRequestConfig(cfg)
	if strings.TrimSpace(cfg.Protocol) == config.AIProtocolAnthropic {
		return testAnthropicAvailability(ctx, cfg)
	}

	baseURL, err := validateAIBaseURL(cfg.API)
	if err != nil {
		return err
	}

	if strings.TrimSpace(cfg.Key) == "" {
		return fmt.Errorf("API Key 不能为空")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return fmt.Errorf("模型不能为空")
	}

	payload := map[string]any{
		"model": cfg.Model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "Reply with OK only.",
			},
		},
		"max_tokens":  1,
		"temperature": 0,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化测试请求失败: %w", err)
	}

	client := &http.Client{
		Timeout:   testRequestTimeout,
		Transport: newSafeTransport(true),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("重定向次数过多")
			}
			_, err := validateAIBaseURL(req.URL.String())
			return err
		},
	}

	endpoints := openAIChatCompletionEndpoints(baseURL.String())
	var lastErr error
	for index, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("创建测试请求失败: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Key))

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("AI 服务请求失败: %w", err)
			continue
		}

		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 8192))
		_ = resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("读取 AI 服务响应失败: %w", readErr)
		}

		contentType := resp.Header.Get("Content-Type")
		if resp.StatusCode != http.StatusOK {
			lastErr = formatAIServiceStatusError("AI 服务", resp.Status, contentType, respBody)
			if shouldTryNextAIEndpoint(resp.StatusCode, contentType, respBody) && index < len(endpoints)-1 {
				continue
			}
			return lastErr
		}

		var result struct {
			Choices []json.RawMessage `json:"choices"`
		}
		if err := json.Unmarshal(respBody, &result); err != nil {
			lastErr = formatAIServiceParseError("AI 服务", contentType, respBody, err)
			if shouldTryNextAIEndpoint(resp.StatusCode, contentType, respBody) && index < len(endpoints)-1 {
				continue
			}
			return lastErr
		}
		if len(result.Choices) == 0 {
			return fmt.Errorf("AI 服务响应缺少 choices 字段")
		}

		return nil
	}

	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("AI 服务请求失败")
}

func testAnthropicAvailability(ctx context.Context, cfg config.AIConfig) error {
	cfg.Enabled = true
	_, err := generateAnthropicChatCompletionWithTools(ctx, cfg, []ChatMessage{
		{Role: "user", Content: "Reply with OK only."},
	}, nil, 1, 0)
	return err
}

func validateAIBaseURL(raw string) (*url.URL, error) {
	return validateBaseURL(raw, true)
}

func openAIChatCompletionEndpoints(base string) []string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return nil
	}
	if strings.HasSuffix(base, "/chat/completions") {
		return []string{base}
	}

	endpoints := []string{base + "/chat/completions"}
	if shouldAddV1EndpointFallback(base) {
		endpoints = append(endpoints, base+"/v1/chat/completions")
	}
	return uniqueStrings(endpoints)
}

func anthropicMessagesEndpoints(base string) []string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return nil
	}
	if strings.HasSuffix(base, "/messages") {
		return []string{base}
	}

	endpoints := []string{base + "/messages"}
	if shouldAddV1EndpointFallback(base) {
		endpoints = append(endpoints, base+"/v1/messages")
	}
	return uniqueStrings(endpoints)
}

func shouldAddV1EndpointFallback(base string) bool {
	parsed, err := url.Parse(base)
	if err != nil {
		return !strings.Contains(base, "/v1/")
	}
	path := strings.Trim(parsed.Path, "/")
	if path == "" {
		return true
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "v1" {
			return false
		}
	}
	return true
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func validateBaseURL(raw string, allowRestricted bool) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("Base URL 不能为空")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("Base URL 格式无效: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, fmt.Errorf("Base URL 仅支持 http 或 https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("Base URL 缺少主机名")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("Base URL 不允许包含用户信息")
	}

	host := parsed.Hostname()
	if host == "" {
		return nil, fmt.Errorf("Base URL 缺少主机名")
	}
	if isBlockedHost(host, allowRestricted) {
		return nil, fmt.Errorf("Base URL 指向受限地址")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("解析 Base URL 失败: %w", err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("Base URL 未解析到可用地址")
	}
	for _, ip := range ips {
		if isBlockedNetip(ip, allowRestricted) {
			return nil, fmt.Errorf("Base URL 指向受限地址")
		}
	}

	return parsed, nil
}

func newSafeTransport(allowRestricted bool) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	return &http.Transport{
		Proxy:                 nil,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("目标地址无效: %w", err)
			}
			if isBlockedHost(host, allowRestricted) {
				return nil, fmt.Errorf("目标地址受限")
			}

			ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("解析目标地址失败: %w", err)
			}
			for _, ip := range ips {
				if isBlockedNetip(ip, allowRestricted) {
					return nil, fmt.Errorf("目标地址受限")
				}
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("目标地址未解析到可用地址")
			}

			var lastErr error
			for _, ip := range ips {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, fmt.Errorf("连接目标地址失败")
		},
	}
}

func isBlockedHost(host string, allowRestricted bool) bool {
	normalized := strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	if normalized == "" {
		return true
	}
	if normalized == "localhost" || normalized == "localhost.localdomain" {
		return !allowRestricted
	}

	ip, err := netip.ParseAddr(normalized)
	if err == nil {
		return isBlockedNetip(ip, allowRestricted)
	}

	return false
}

func isPublicIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	return isPublicNetip(addr)
}

func isPublicNetip(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() {
		return false
	}
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsMulticast() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return false
	}
	if addr.IsUnspecified() {
		return false
	}
	if addr.Is6() && addr.IsInterfaceLocalMulticast() {
		return false
	}
	return true
}

func isBlockedNetip(addr netip.Addr, allowRestricted bool) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsUnspecified() || addr.IsMulticast() || addr.IsInterfaceLocalMulticast() {
		return true
	}
	if isPublicNetip(addr) {
		return false
	}
	if !allowRestricted {
		return true
	}
	return !(addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast())
}

func extractErrorMessage(body []byte) string {
	var payload struct {
		Error any `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}

	switch value := payload.Error.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]any:
		if msg, ok := value["message"].(string); ok {
			return strings.TrimSpace(msg)
		}
	}
	return ""
}

func formatAIServiceStatusError(service, status string, contentType string, body []byte) error {
	if isLikelyNonJSONResponse(contentType, body) {
		return nonJSONServiceResponseError(service, contentType, body)
	}
	msg := extractErrorMessage(body)
	if msg == "" {
		msg = strings.TrimSpace(string(body))
	}
	if msg == "" {
		msg = status
	}
	return fmt.Errorf("%s返回异常: %s", service, msg)
}

func formatAIServiceParseError(service, contentType string, body []byte, err error) error {
	if isLikelyNonJSONResponse(contentType, body) {
		return nonJSONServiceResponseError(service, contentType, body)
	}
	return fmt.Errorf("解析 %s响应失败: %w", service, err)
}

func shouldTryNextAIEndpoint(statusCode int, contentType string, body []byte) bool {
	return statusCode == http.StatusNotFound ||
		statusCode == http.StatusMethodNotAllowed ||
		isLikelyNonJSONResponse(contentType, body)
}

func isLikelyNonJSONResponse(contentType string, body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	normalizedContentType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if strings.Contains(normalizedContentType, "html") {
		return true
	}
	if len(trimmed) > 0 && trimmed[0] == '<' {
		return true
	}
	if normalizedContentType == "" {
		return false
	}
	return !strings.Contains(normalizedContentType, "json") && !strings.Contains(normalizedContentType, "event-stream")
}

func nonJSONServiceResponseError(service, contentType string, body []byte) error {
	detail := "请检查 Base URL 是否填写为 API 根地址；OpenAI 兼容通常以 /v1 结尾，Claude 原生通常以 /v1 结尾"
	preview := compactResponsePreview(body, 160)
	if preview != "" && !strings.HasPrefix(preview, "<") {
		detail += "。响应片段: " + preview
	}
	if strings.TrimSpace(contentType) != "" {
		detail += "。Content-Type: " + strings.TrimSpace(contentType)
	}
	return fmt.Errorf("%s返回了非 JSON 响应，%s", service, detail)
}

func compactResponsePreview(body []byte, maxLen int) string {
	preview := strings.Join(strings.Fields(strings.TrimSpace(string(body))), " ")
	if preview == "" {
		return ""
	}
	if len(preview) <= maxLen {
		return preview
	}
	if maxLen <= 3 {
		return preview[:maxLen]
	}
	return preview[:maxLen-3] + "..."
}
