package modelrequest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hephaestus-backend/pkg/config"
)

func TestApplyProviderHeadersAddsStaticAndOpenCodeSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://opencode.ai/inference/go/openai/v1/chat/completions", nil)
	headers := []config.AIProviderHeader{{Name: "X-Gateway", Value: "gateway"}}
	ApplyProviderHeaders(req, headers, "provider-1", "https://opencode.ai/inference/go/openai/v1", "")
	if got := req.Header.Get("X-Gateway"); got != "gateway" {
		t.Fatalf("custom header = %q", got)
	}
	session := req.Header.Get(openCodeSessionHeader)
	if !strings.HasPrefix(session, "ses_") {
		t.Fatalf("opencode session header = %q", session)
	}
	again := httptest.NewRequest(http.MethodPost, "https://opencode.ai/inference/go/openai/v1/chat/completions", nil)
	ApplyProviderHeaders(again, headers, "provider-1", "https://opencode.ai/inference/go/openai/v1", "")
	if again.Header.Get(openCodeSessionHeader) != session {
		t.Fatalf("session id is not stable: %q != %q", again.Header.Get(openCodeSessionHeader), session)
	}
}

func TestApplyProviderHeadersExplicitSessionWins(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://opencode.ai/inference/go/openai/v1/chat/completions", nil)
	ApplyProviderHeaders(req, []config.AIProviderHeader{{Name: "x-opencode-session", Value: "ses_explicit"}}, "provider-1", "", "")
	if got := req.Header.Get(openCodeSessionHeader); got != "ses_explicit" {
		t.Fatalf("session header = %q, want explicit value", got)
	}
}

func TestApplyProviderHeadersSkipsNonOpenCodeAndStripsOnHostChange(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://api.example.com/v1/chat/completions", nil)
	ApplyProviderHeaders(req, []config.AIProviderHeader{{Name: "X-Gateway", Value: "gateway"}}, "provider-1", "https://api.example.com/v1", "")
	if got := req.Header.Get("X-Gateway"); got != "gateway" {
		t.Fatalf("custom header = %q", got)
	}
	if got := req.Header.Get(openCodeSessionHeader); got != "" {
		t.Fatalf("non-opencode request got session header %q", got)
	}

	redirected := httptest.NewRequest(http.MethodPost, "https://other.example.com/v1/chat/completions", nil)
	redirected.Header.Set("X-Gateway", "gateway")
	redirected.Header.Set(openCodeSessionHeader, "ses_explicit")
	ApplyProviderHeaders(redirected, []config.AIProviderHeader{{Name: "X-Gateway", Value: "gateway"}}, "provider-1", "https://api.example.com/v1", "")
	if got := redirected.Header.Get("X-Gateway"); got != "" {
		t.Fatalf("redirect leaked custom header %q", got)
	}
	if got := redirected.Header.Get(openCodeSessionHeader); got != "" {
		t.Fatalf("redirect leaked session header %q", got)
	}
}

func TestForProviderWithHeadersInjectsOnEveryRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Gateway"); got != "gateway" {
			t.Errorf("custom header = %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := &http.Client{
		Transport: ForProviderWithHeaders(http.DefaultTransport, "provider-1", server.URL, "key", "model", 30,
			[]config.AIProviderHeader{{Name: "X-Gateway", Value: "gateway"}}),
		Timeout: 5 * time.Second,
	}
	req, err := http.NewRequest(http.MethodPost, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}
