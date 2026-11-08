package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/config"
)

// Opt in with a disposable PostgreSQL instance; config/DB singletons live only
// in the child process and cannot affect the rest of the handler suite.
func TestSideChatCompletedOrchestrationPostgres(t *testing.T) {
	port := os.Getenv("HEPHAESTUS_SIDE_CHAT_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable PostgreSQL fixture")
	}
	const childEnv = "HEPHAESTUS_SIDE_CHAT_TEST_CHILD"
	if os.Getenv(childEnv) != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSideChatCompletedOrchestrationPostgres$", "-test.v")
		cmd.Env = append(os.Environ(), childEnv+"=1", "HEPHAESTUS_CONFIG_DIR="+t.TempDir())
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("side chat PostgreSQL regression: %v\n%s", err, output)
		}
		return
	}
	config.ForceSetConfigBaseDir(os.Getenv("HEPHAESTUS_CONFIG_DIR"))
	appConfig := config.GetDefaultConfig()
	appConfig.Database = config.DatabaseConfig{Type: config.DatabaseTypePostgres, Host: "127.0.0.1", Port: port, User: "postgres", Password: "sidechat-fixture", DBName: "sidechat_fixture", SSLMode: "disable"}
	if err := config.SaveConfig(appConfig); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model     string              `json:"model"`
			Messages  []aipkg.ChatMessage `json:"messages"`
			MaxTokens *int                `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.Model != "same-as-main" || payload.MaxTokens != nil {
			t.Error("side chat did not retain the main model and provider output default")
		}
		if len(payload.Messages) < 3 || !strings.Contains(payload.Messages[1].Content, "completed fixture evidence") {
			t.Error("completed conversation context was not supplied")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if payload.Messages[len(payload.Messages)-1].Content == "exhaust budget" {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking\"},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"completed task answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	config.InitAIConfigDB(func(string) (any, error) {
		return config.AIConfig{Enabled: true, DefaultProviderID: "global", Providers: []config.AIProviderConfig{
			{ID: "global", Enabled: true, API: "http://127.0.0.1:1/v1", Key: "fixture", Model: "wrong-model"},
			{ID: "main", Enabled: true, API: server.URL + "/v1", Key: "fixture", Model: "main-default"},
		}}, nil
	}, nil, "side-chat-fixture")
	ctx := context.Background()
	store, err := aipkg.StoreFromAppDB(ctx)
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "completed side chat fixture"})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/side-chat", StreamAIWorkbenchSideChatHandler)
	for _, role := range []string{pentest.AgentRoleWorker, pentest.AgentRoleController} {
		conversation, err := store.UpsertConversation(ctx, pentest.Conversation{ID: "orchestration:fixture:" + role, ProjectID: project.ID, Mode: pentest.SoloPentestMode, Status: "completed"})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SetAgentRole(ctx, conversation.ID, role, "", "orchestration:fixture"); err != nil {
			t.Fatal(err)
		}
		if _, err := store.AddMessage(ctx, pentest.Message{ConversationID: conversation.ID, Role: "assistant", Content: "completed fixture evidence"}); err != nil {
			t.Fatal(err)
		}
		for _, prompt := range []string{"Explain the result", "exhaust budget"} {
			body, _ := json.Marshal(sideChatRequest{SessionID: conversation.ID, AgentProviderID: "main", AgentModel: "same-as-main", Messages: []aipkg.ChatMessage{{Role: "user", Content: prompt}}})
			request := httptest.NewRequest("POST", "/side-chat", strings.NewReader(string(body)))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			text := response.Body.String()
			if response.Code != 200 {
				t.Fatalf("%s: status=%d body=%s", role, response.Code, text)
			}
			if prompt == "exhaust budget" {
				if !strings.Contains(text, "event: error") || !strings.Contains(text, "输出 Token 上限") || strings.Contains(text, "event: done") {
					t.Fatalf("%s: truncated answer misreported: %s", role, text)
				}
			} else if !strings.Contains(text, "event: done") || !strings.Contains(text, "completed task answer") || strings.Contains(text, "event: error") {
				t.Fatalf("%s: side chat failed: %s", role, text)
			}
		}
		after, err := store.GetConversation(ctx, conversation.ID)
		if err != nil || after.Status != "completed" {
			t.Fatalf("side chat changed main status: %+v %v", after, err)
		}
		messages, err := store.ListMessages(ctx, conversation.ID, -1)
		if err != nil || len(messages) != 1 || messages[0].Content != "completed fixture evidence" {
			t.Fatalf("side chat changed main transcript: %+v %v", messages, err)
		}
	}
}

func TestSideChatSendsSelectedProviderModelAndBudget(t *testing.T) {
	for _, tc := range []struct {
		name, providerID, model, protocol string
		budget                            int
	}{
		{"explicit", "selected", "chat-model", config.AIProtocolOpenAICompatible, 8192},
		{"workbench-route", "", "", config.AIProtocolOpenAICompatible, 0},
		{"anthropic", "selected", "chat-model", config.AIProtocolAnthropic, 8192},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				wantModel := tc.model
				if wantModel == "" {
					wantModel = "provider-model"
				}
				if payload["model"] != wantModel {
					t.Errorf("model = %v, want %s", payload["model"], wantModel)
				}
				if tc.budget == 0 {
					if _, ok := payload["max_tokens"]; ok {
						t.Errorf("provider default must not be replaced by a fixed cap: %v", payload["max_tokens"])
					}
				} else if payload["max_tokens"] != float64(tc.budget) {
					t.Errorf("max_tokens = %v, want %d", payload["max_tokens"], tc.budget)
				}
				if _, ok := payload["tools"]; ok {
					t.Error("side chat must remain tool-free")
				}
				if tc.protocol == config.AIProtocolAnthropic {
					if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "selected-key" {
						t.Error("selected Anthropic transport was replaced")
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn"}`)
				} else {
					if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer selected-key" {
						t.Error("selected OpenAI transport was replaced")
					}
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				}
			}))
			defer server.Close()
			original := config.AIConfig{
				Enabled: true, DefaultProviderID: "global",
				Routing: config.AIRoutingConfig{Workbench: "selected"},
				Providers: []config.AIProviderConfig{
					{ID: "global", Enabled: true, API: "http://127.0.0.1:1/v1", Key: "global-key", Model: "global-model"},
					{ID: "selected", Enabled: true, Protocol: tc.protocol, API: server.URL + "/v1", Key: "selected-key", Model: "provider-model", MaxOutputTokens: tc.budget},
				},
			}
			cfg, budget := resolveSideChatAIConfig(original, tc.providerID, tc.model)
			var deltas strings.Builder
			result, err := aipkg.GenerateChatCompletionStreamWithTools(context.Background(), cfg, []aipkg.ChatMessage{{Role: "user", Content: "Explain the completed task"}}, nil, budget, 0.2, func(delta aipkg.ChatStreamDelta) error {
				deltas.WriteString(delta.Delta)
				return nil
			})
			if err != nil || result.Content != "answer" || deltas.String() != "answer" || requests != 1 {
				t.Fatalf("result=%+v err=%v deltas=%q requests=%d", result, err, deltas.String(), requests)
			}
			if original.Providers[1].Model != "provider-model" || original.DefaultProviderID != "global" {
				t.Fatal("per-chat model override mutated saved configuration")
			}
		})
	}
}
