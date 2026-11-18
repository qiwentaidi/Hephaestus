package ai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"hephaestus-backend/pkg/config"
)

func TestProviderHeadersReachChatStreamAndNonStream(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("X-Gateway"); got != "gateway" {
					t.Errorf("gateway header = %q", got)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
			}))
			defer server.Close()

			cfg := config.AIConfig{
				Enabled: true, API: server.URL + "/v1", Key: "fixture", Model: "fixture",
				Headers: []config.AIProviderHeader{{Name: "X-Gateway", Value: "gateway"}},
			}
			if stream {
				_, err := GenerateChatCompletionStreamWithTools(context.Background(), cfg,
					[]ChatMessage{{Role: "user", Content: "hello"}}, nil, 0, 0, func(ChatStreamDelta) error { return nil })
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if _, err := GenerateChatCompletionWithTools(context.Background(), cfg,
				[]ChatMessage{{Role: "user", Content: "hello"}}, nil, 0, 0); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProviderHeadersReachModelListAndAvailabilityTest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Gateway"); got != "gateway" {
			t.Errorf("gateway header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/models" {
			fmt.Fprint(w, `{"data":[{"id":"fixture-model"}]}`)
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	provider := config.AIProviderConfig{
		ID: "fixture", Name: "Fixture", Protocol: config.AIProtocolOpenAICompatible, Enabled: true,
		API: server.URL + "/v1", Key: "fixture", Model: "fixture-model",
		Headers: []config.AIProviderHeader{{Name: "X-Gateway", Value: "gateway"}},
	}
	models, err := ListModels(context.Background(), provider)
	if err != nil || len(models) != 1 || models[0].ID != "fixture-model" {
		t.Fatalf("models = %#v, err = %v", models, err)
	}
	if err := TestAvailability(context.Background(), config.AIConfig{
		Enabled: true, DefaultProviderID: "fixture", Providers: []config.AIProviderConfig{provider},
	}); err != nil {
		t.Fatal(err)
	}
}
