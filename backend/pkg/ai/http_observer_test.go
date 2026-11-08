package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestObserveWorkbenchHTTPTargetsExtractsPageSignals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Server", "test-server")
		_, _ = w.Write([]byte(`
			<html>
				<head><title>Example Login</title><script src="/static/app.js"></script></head>
				<body>
					<form action="/auth/login"></form>
					<a href="/api/users">users</a>
					<script>fetch("/api/profile")</script>
				</body>
			</html>
		`))
	}))
	defer server.Close()

	result := ObserveWorkbenchHTTPTargetsWithClient(context.Background(), server.URL, []string{"not-a-url"}, server.Client())
	if len(result.Targets) != 1 {
		t.Fatalf("expected one observed target, got %#v", result.Targets)
	}
	observed := result.Targets[0]
	if observed.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d, want 200", observed.StatusCode)
	}
	if observed.Title != "Example Login" {
		t.Fatalf("Title = %q", observed.Title)
	}
	if observed.Forms != 1 || observed.Scripts != 2 {
		t.Fatalf("forms/scripts = %d/%d", observed.Forms, observed.Scripts)
	}
	for _, want := range []string{"/auth/login", "/api/profile", "/api/users"} {
		if !containsString(observed.APIRoutes, want) {
			t.Fatalf("api routes %#v missing %q", observed.APIRoutes, want)
		}
	}
	if !strings.Contains(result.Summary(), "接口线索 3 条") {
		t.Fatalf("unexpected summary: %s", result.Summary())
	}
}

func TestObserveWorkbenchHTTPTargetsSkipsNonHTTPTargets(t *testing.T) {
	result := ObserveWorkbenchHTTPTargetsWithClient(context.Background(), "example.com", []string{"10.0.0.0/24"}, http.DefaultClient)
	if len(result.Targets) != 0 {
		t.Fatalf("expected no observed targets, got %#v", result.Targets)
	}
	if len(result.Skipped) != 2 {
		t.Fatalf("expected skipped non-http targets, got %#v", result.Skipped)
	}
}
