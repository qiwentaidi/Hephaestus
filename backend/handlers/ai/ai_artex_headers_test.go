package ai

import (
	"testing"

	"hephaestus-backend/pkg/config"
)

func TestOpenCodeSessionHeaderKey(t *testing.T) {
	if got := openCodeSessionHeaderKey("https://opencode.ai/inference/go/openai/v1", nil); got != "x-opencode-session" {
		t.Fatalf("opencode endpoint key = %q", got)
	}
	if got := openCodeSessionHeaderKey("https://zen.opencode.ai/openai/v1", nil); got != "x-opencode-session" {
		t.Fatalf("opencode subdomain key = %q", got)
	}
	if got := openCodeSessionHeaderKey("https://gateway.example.com/v1", []config.AIProviderHeader{
		{Name: "X-OpenCode-Session", Value: "ses_1"},
	}); got != "x-opencode-session" {
		t.Fatalf("explicit header key = %q", got)
	}
	if got := openCodeSessionHeaderKey("https://gateway.example.com/v1", []config.AIProviderHeader{
		{Name: "X-Other", Value: "1"},
	}); got != "" {
		t.Fatalf("unrelated provider should not get a session header, got %q", got)
	}
}
