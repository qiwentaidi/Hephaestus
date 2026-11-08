package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hephaestus-backend/pkg/config"
)

func TestValidateAIBaseURLAllowsLocalGatewayAddresses(t *testing.T) {
	tests := []string{
		"http://192.168.200.1:31417/v1",
		"http://127.0.0.1:31417/v1",
		"http://localhost:31417/v1",
	}

	for _, raw := range tests {
		if _, err := validateAIBaseURL(raw); err != nil {
			t.Fatalf("validateAIBaseURL(%q) returned error: %v", raw, err)
		}
	}
}

func TestValidateAIBaseURLRejectsInvalidTargets(t *testing.T) {
	tests := []string{
		"ftp://192.168.200.1:31417/v1",
		"http://user:pass@192.168.200.1:31417/v1",
		"http://0.0.0.0:31417/v1",
		"http://224.0.0.1:31417/v1",
	}

	for _, raw := range tests {
		if _, err := validateAIBaseURL(raw); err == nil {
			t.Fatalf("validateAIBaseURL(%q) returned nil error", raw)
		}
	}
}

func TestValidateBaseURLCanStillRejectRestrictedAddresses(t *testing.T) {
	if _, err := validateBaseURL("http://192.168.200.1:31417/v1", false); err == nil {
		t.Fatal("validateBaseURL should reject private addresses when restricted addresses are disabled")
	}
}

func TestOpenAIChatCompletionEndpointsAddsV1FallbackForRootURL(t *testing.T) {
	endpoints := openAIChatCompletionEndpoints("https://dsai.ak03.cn")
	expected := []string{
		"https://dsai.ak03.cn/chat/completions",
		"https://dsai.ak03.cn/v1/chat/completions",
	}

	if len(endpoints) != len(expected) {
		t.Fatalf("expected endpoints %#v, got %#v", expected, endpoints)
	}
	for index := range expected {
		if endpoints[index] != expected[index] {
			t.Fatalf("expected endpoint %d to be %q, got %q", index, expected[index], endpoints[index])
		}
	}
}

func TestOpenAIChatCompletionEndpointsKeepsVersionedBaseURL(t *testing.T) {
	endpoints := openAIChatCompletionEndpoints("https://dashscope.aliyuncs.com/compatible-mode/v1")
	expected := "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"

	if len(endpoints) != 1 || endpoints[0] != expected {
		t.Fatalf("expected versioned endpoint %q, got %#v", expected, endpoints)
	}
}

func TestOpenAIChatCompletionEndpointsKeepsExplicitEndpoint(t *testing.T) {
	endpoints := openAIChatCompletionEndpoints("https://proxy.example.com/v1/chat/completions")
	expected := "https://proxy.example.com/v1/chat/completions"

	if len(endpoints) != 1 || endpoints[0] != expected {
		t.Fatalf("expected explicit endpoint %q, got %#v", expected, endpoints)
	}
}

func TestAnthropicMessagesEndpointsAddsV1FallbackForRootURL(t *testing.T) {
	endpoints := anthropicMessagesEndpoints("https://claude-proxy.example.com")
	expected := []string{
		"https://claude-proxy.example.com/messages",
		"https://claude-proxy.example.com/v1/messages",
	}

	if len(endpoints) != len(expected) {
		t.Fatalf("expected endpoints %#v, got %#v", expected, endpoints)
	}
	for index := range expected {
		if endpoints[index] != expected[index] {
			t.Fatalf("expected endpoint %d to be %q, got %q", index, expected[index], endpoints[index])
		}
	}
}

func TestFormatAIServiceParseErrorExplainsHTMLResponse(t *testing.T) {
	err := formatAIServiceParseError("AI 服务", "text/html; charset=utf-8", []byte("<html>not an api</html>"), assertJSONError{})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "非 JSON 响应") || !strings.Contains(msg, "Base URL") {
		t.Fatalf("expected helpful non-JSON response message, got %q", msg)
	}
}

type assertJSONError struct{}

func (assertJSONError) Error() string {
	return "invalid character '<' looking for beginning of value"
}

func TestAvailabilityTriesV1FallbackAfterHTMLRootEndpoint(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/chat/completions":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<html>not an api</html>"))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	err := TestAvailability(context.Background(), config.AIConfig{
		Enabled:  true,
		Protocol: config.AIProtocolOpenAICompatible,
		API:      server.URL,
		Key:      "test-key",
		Model:    "test-model",
	})
	if err != nil {
		t.Fatalf("TestAvailability() error = %v", err)
	}
	if len(paths) != 2 || paths[0] != "/chat/completions" || paths[1] != "/v1/chat/completions" {
		t.Fatalf("expected fallback request paths, got %#v", paths)
	}
}
