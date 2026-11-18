package config

import (
	"fmt"
	"net/textproto"
	"net/url"
	"strings"
)

const (
	maxAIProviderHeaders          = 32
	maxAIProviderHeaderNameLength = 128
	maxAIProviderHeaderValLength  = 4096
)

// AIProviderHeader is one custom HTTP header attached to every model request
// sent through a provider. Values live in the same settings store as the API
// key, so they must never be logged.
type AIProviderHeader struct {
	Name  string `yaml:"name" json:"name"`
	Value string `yaml:"value,omitempty" json:"value,omitempty"`
}

// Hop-by-hop and transport-owned headers cannot be set per provider: the HTTP
// client owns them, and overriding Content-Type would break the JSON body.
var aiProviderForbiddenHeaders = map[string]struct{}{
	"connection":        {},
	"content-length":    {},
	"content-type":      {},
	"host":              {},
	"keep-alive":        {},
	"proxy-connection":  {},
	"te":                {},
	"trailer":           {},
	"transfer-encoding": {},
	"upgrade":           {},
}

// NormalizeAIProviderHeaders trims, validates, and deduplicates configured
// headers. Invalid or empty entries are dropped so an old or hand-edited
// config cannot break every model request. The last value wins for duplicate
// names, matching the "override" semantics users expect.
func NormalizeAIProviderHeaders(headers []AIProviderHeader) []AIProviderHeader {
	if len(headers) == 0 {
		return nil
	}
	normalized := make([]AIProviderHeader, 0, min(len(headers), maxAIProviderHeaders))
	indexByName := make(map[string]int, len(headers))
	for _, header := range headers {
		name := strings.TrimSpace(header.Name)
		value := strings.TrimSpace(header.Value)
		if !validAIProviderHeaderName(name) || len(name) > maxAIProviderHeaderNameLength {
			continue
		}
		canonical := textproto.CanonicalMIMEHeaderKey(name)
		if canonical == "" {
			continue
		}
		lower := strings.ToLower(canonical)
		if _, forbidden := aiProviderForbiddenHeaders[lower]; forbidden {
			continue
		}
		if value == "" || len(value) > maxAIProviderHeaderValLength || strings.ContainsAny(value, "\r\n") {
			continue
		}
		if index, ok := indexByName[lower]; ok {
			normalized[index].Value = value
			continue
		}
		if len(normalized) >= maxAIProviderHeaders {
			break
		}
		indexByName[lower] = len(normalized)
		normalized = append(normalized, AIProviderHeader{Name: canonical, Value: value})
	}
	return normalized
}

// ValidateAIProviderHeaders rejects malformed header rows before they are
// saved. The UI keeps an empty row while editing, so an entirely empty row is
// ignored rather than treated as an error.
func ValidateAIProviderHeaders(aiConfig AIConfig) error {
	for _, provider := range aiConfig.Providers {
		count := 0
		seen := make(map[string]struct{}, len(provider.Headers))
		for index, header := range provider.Headers {
			name := strings.TrimSpace(header.Name)
			value := strings.TrimSpace(header.Value)
			if name == "" {
				if value == "" {
					continue
				}
				return fmt.Errorf("渠道 %q 的第 %d 个请求头缺少名称", aiProviderLabel(provider), index+1)
			}
			if !validAIProviderHeaderName(name) || len(name) > maxAIProviderHeaderNameLength {
				return fmt.Errorf("渠道 %q 的请求头名称 %q 不合法", aiProviderLabel(provider), name)
			}
			canonical := textproto.CanonicalMIMEHeaderKey(name)
			lower := strings.ToLower(canonical)
			if _, forbidden := aiProviderForbiddenHeaders[lower]; forbidden {
				return fmt.Errorf("渠道 %q 的请求头 %q 由客户端管理，不能自定义", aiProviderLabel(provider), canonical)
			}
			if len(value) > maxAIProviderHeaderValLength || strings.ContainsAny(value, "\r\n") {
				return fmt.Errorf("渠道 %q 的请求头 %q 取值不合法", aiProviderLabel(provider), canonical)
			}
			if _, ok := seen[lower]; ok {
				return fmt.Errorf("渠道 %q 的请求头 %q 重复，请合并后保存", aiProviderLabel(provider), canonical)
			}
			seen[lower] = struct{}{}
			count++
		}
		if count > maxAIProviderHeaders {
			return fmt.Errorf("渠道 %q 最多可配置 %d 个请求头", aiProviderLabel(provider), maxAIProviderHeaders)
		}
	}
	return nil
}

func aiProviderLabel(provider AIProviderConfig) string {
	return firstNonEmpty(provider.Name, provider.ID, "AI Provider")
}

// LiveAIProviderHeaders returns the saved headers for the provider behind one
// request. Embedded task-engine profiles identify their channel by ID; profiles
// without a host ID fall back to the same endpoint/credential match used by
// LiveAIRequestTimeout.
func LiveAIProviderHeaders(providerID, baseURL, apiKey, model string) []AIProviderHeader {
	cfg, err := GetAIConfig()
	if err != nil {
		return nil
	}
	for _, provider := range cfg.Providers {
		matches := providerID != "" && provider.ID == providerID
		if providerID == "" {
			matches = aiTimeoutEndpoint(provider.API) == aiTimeoutEndpoint(baseURL) &&
				strings.TrimSpace(provider.Key) == strings.TrimSpace(apiKey) &&
				provider.Model == model
		}
		if matches {
			return provider.Headers
		}
	}
	return nil
}

// IsOpenCodeAPI reports whether a base URL points at the OpenCode gateway.
// OpenCode requires a stable x-opencode-session header on every request; the
// header is injected automatically for these endpoints.
func IsOpenCodeAPI(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "opencode.ai" || strings.HasSuffix(host, ".opencode.ai")
}

func validAIProviderHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '!', c == '#', c == '$', c == '%', c == '&', c == '\'', c == '*',
			c == '+', c == '-', c == '.', c == '^', c == '_', c == '`', c == '|', c == '~':
		default:
			return false
		}
	}
	return true
}
