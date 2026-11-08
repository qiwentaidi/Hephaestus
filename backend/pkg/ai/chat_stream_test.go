package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hephaestus-backend/pkg/config"
)

func TestChatStreamResponses(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body, wantContent, wantError string
	}{
		{"reasoning-then-answer", "text/event-stream", "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"private reasoning\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", "answer", ""},
		{"reasoning-exhausted", "text/event-stream", "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"private reasoning\"},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n", "", "输出 Token 上限"},
		{"empty", "text/event-stream", "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", "", "finish_reason=stop"},
		{"json-gateway", "application/json; charset=utf-8", `{"choices":[{"message":{"content":"answer"},"finish_reason":"stop"}]}`, "answer", ""},
		{"json-exhausted", "application/json", `{"choices":[{"message":{"content":null},"finish_reason":"length"}]}`, "", "输出 Token 上限"},
		{"invalid-json", "application/json", `{"choices":`, "", "unexpected end"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if payload["max_tokens"] != float64(6000) || payload["stream"] != true {
					t.Error("explicit token budget or stream mode was lost")
				}
				w.Header().Set("Content-Type", tc.contentType)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			var deltas strings.Builder
			result, err := GenerateChatCompletionStreamWithTools(context.Background(), config.AIConfig{
				Enabled: true, API: server.URL + "/v1", Key: "fixture", Model: "fixture",
			}, []ChatMessage{{Role: "user", Content: "hello"}}, nil, 6000, 0.2, func(delta ChatStreamDelta) error {
				deltas.WriteString(delta.Delta)
				return nil
			})
			if tc.wantError == "" && err != nil || tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("err = %v, want %q", err, tc.wantError)
			}
			if result.Content != tc.wantContent || deltas.String() != tc.wantContent || requests != 1 {
				t.Fatalf("content=%q deltas=%q requests=%d", result.Content, deltas.String(), requests)
			}
		})
	}
}

func TestChatStreamCallbackCancellation(t *testing.T) {
	for _, contentType := range []string{"application/json", "text/event-stream"} {
		t.Run(contentType, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", contentType)
				if contentType == "application/json" {
					fmt.Fprint(w, `{"choices":[{"message":{"content":"answer"},"finish_reason":"stop"}]}`)
				} else {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"}}]}\n\ndata: [DONE]\n\n")
				}
			}))
			defer server.Close()
			_, err := GenerateChatCompletionStreamWithTools(context.Background(), config.AIConfig{
				Enabled: true, API: server.URL + "/v1", Key: "fixture", Model: "fixture",
			}, []ChatMessage{{Role: "user", Content: "hello"}}, nil, 0, 0.2, func(ChatStreamDelta) error { return context.Canceled })
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation lost: %v", err)
			}
		})
	}
}
