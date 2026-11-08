package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"hephaestus-backend/pkg/config"
)

func TestListModelsDraftProviderAndEndpointFallback(t *testing.T) {
	for _, basePath := range []string{"", "/v1/", "/compatible-mode/v1", "/v1/chat/completions", "/v1/models"} {
		t.Run(basePath, func(t *testing.T) {
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("x-api-key") != "" {
					t.Error("unexpected method or authentication")
				}
				if r.URL.Path == "/models" {
					w.Header().Set("Content-Type", "text/html")
					fmt.Fprint(w, "<html>gateway homepage</html>")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"data":[{"id":"z-model"},{"id":" a-model "},{"id":"z-model"},{"id":""}]}`)
			}))
			defer server.Close()
			// A disabled, unsaved provider with no model must still work.
			models, err := ListModels(context.Background(), config.AIProviderConfig{
				Protocol: config.AIProtocolOpenAICompatible, API: server.URL + basePath, Key: " test-key ",
			})
			if err != nil || !reflect.DeepEqual(models, []ModelOption{{ID: "a-model"}, {ID: "z-model"}}) {
				t.Fatalf("models=%v err=%v", models, err)
			}
			want := "/v1/models"
			if strings.Contains(basePath, "compatible-mode") {
				want = "/compatible-mode/v1/models"
			}
			if paths[len(paths)-1] != want || (basePath == "" && len(paths) != 2) {
				t.Fatalf("unexpected paths: %v", paths)
			}
		})
	}
}

func TestListModelsAnthropicPagination(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/models" || r.Header.Get("x-api-key") != "test-key" || r.Header.Get("anthropic-version") != anthropicVersion || r.Header.Get("Authorization") != "" {
			t.Error("incorrect Anthropic endpoint or authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after_id") == "claude-a" {
			fmt.Fprint(w, `{"data":[{"id":"claude-b","display_name":"Claude B"}],"has_more":false}`)
		} else {
			fmt.Fprint(w, `{"data":[{"id":"claude-a","display_name":"Claude A"}],"has_more":true,"last_id":"claude-a"}`)
		}
	}))
	defer server.Close()
	models, err := ListModels(context.Background(), config.AIProviderConfig{Protocol: config.AIProtocolAnthropic, API: server.URL + "/v1/messages", Key: "test-key"})
	if err != nil || requests != 2 || !reflect.DeepEqual(models, []ModelOption{{ID: "claude-a", Name: "Claude A"}, {ID: "claude-b", Name: "Claude B"}}) {
		t.Fatalf("models=%v requests=%d err=%v", models, requests, err)
	}
}

func TestListModelsFailuresAndEmptyResponse(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body, code string
	}{
		{"unauthorized", 401, `{"error":"secret-test-key"}`, "authentication_failed"},
		{"forbidden", 403, `{}`, "authentication_failed"},
		{"rate limit", 429, `{}`, "rate_limited"},
		{"unsupported", 404, `{}`, "unsupported"},
		{"server error", 500, `{"error":"secret-test-key"}`, "request_failed"},
		{"missing data", 200, `{}`, "invalid_response"},
		{"null data", 200, `{"data":null}`, "invalid_response"},
		{"invalid json", 200, `broken`, "invalid_response"},
		{"bad cursor", 200, `{"data":[],"has_more":true}`, "invalid_response"},
		{"repeated cursor", 200, `{"data":[{"id":"a"}],"has_more":true,"last_id":"a"}`, "invalid_response"},
		{"empty", 200, `{"data":[]}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			models, err := ListModels(context.Background(), config.AIProviderConfig{Protocol: config.AIProtocolOpenAICompatible, API: server.URL, Key: "secret-test-key"})
			if tc.code == "" {
				if err != nil || models == nil || len(models) != 0 {
					t.Fatalf("models=%v err=%v", models, err)
				}
				return
			}
			var listErr *ModelListError
			if !errors.As(err, &listErr) || listErr.Code != tc.code {
				t.Fatalf("expected %s, got %v", tc.code, err)
			}
		})
	}
}

func TestListModelsCancellationAndCrossOriginRedirect(t *testing.T) {
	t.Run("cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
		defer server.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, err := ListModels(ctx, config.AIProviderConfig{Protocol: config.AIProtocolAnthropic, API: server.URL, Key: "test-key"})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	})
	t.Run("redirect", func(t *testing.T) {
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("credentials must not reach a different origin") }))
		defer target.Close()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
		defer server.Close()
		_, err := ListModels(context.Background(), config.AIProviderConfig{Protocol: config.AIProtocolAnthropic, API: server.URL, Key: "test-key"})
		if err == nil {
			t.Fatal("expected cross-origin redirect rejection")
		}
	})
}
