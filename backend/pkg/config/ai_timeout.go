package config

import (
	"fmt"
	"strings"
	"time"
)

const DefaultAIRequestTimeoutMinutes = 30
const MaxAIRequestTimeoutMinutes = 120

func EffectiveAIRequestTimeoutMinutes(minutes int) int {
	if minutes <= 0 {
		return DefaultAIRequestTimeoutMinutes
	}
	return min(minutes, MaxAIRequestTimeoutMinutes)
}

// ValidateAIRequestTimeouts accepts zero for older clients that omit the field.
func ValidateAIRequestTimeouts(cfg AIConfig) error {
	if cfg.RequestTimeoutMinutes < 0 || cfg.RequestTimeoutMinutes > MaxAIRequestTimeoutMinutes {
		return fmt.Errorf("单次模型请求超时须为 1～120 分钟（未配置时默认 30 分钟）")
	}
	for _, provider := range cfg.Providers {
		if provider.RequestTimeoutMinutes < 0 || provider.RequestTimeoutMinutes > MaxAIRequestTimeoutMinutes {
			return fmt.Errorf("渠道 %q：单次模型请求超时须为 1～120 分钟（未配置时默认 30 分钟）", provider.Name)
		}
	}
	return nil
}

// LiveAIRequestTimeout is sampled once when each HTTP request starts. Existing
// streams retain their deadline; retries and subsequent turns see saved changes.
// Snapshot-based callers (including embedded task-engine profiles) can identify
// their channel by endpoint and credential when they do not carry a provider ID.
func LiveAIRequestTimeout(providerID, baseURL, apiKey, model string, fallbackMinutes int) time.Duration {
	minutes := EffectiveAIRequestTimeoutMinutes(fallbackMinutes)
	if cfg, err := GetAIConfig(); err == nil {
		for _, provider := range cfg.Providers {
			matches := providerID != "" && provider.ID == providerID
			if providerID == "" {
				matches = aiTimeoutEndpoint(provider.API) == aiTimeoutEndpoint(baseURL) && strings.TrimSpace(provider.Key) == strings.TrimSpace(apiKey) && provider.Model == model
			}
			if matches {
				minutes = EffectiveAIRequestTimeoutMinutes(provider.RequestTimeoutMinutes)
				break
			}
		}
	}
	return time.Duration(minutes) * time.Minute
}

func aiTimeoutEndpoint(endpoint string) string {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	for _, suffix := range []string{"/chat/completions", "/messages", "/responses", "/v1"} {
		endpoint = strings.TrimSuffix(endpoint, suffix)
	}
	return endpoint
}
