package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
)

func TestProjectTokenUsageResponse(t *testing.T) {
	s := openAIPentestHandlerTestStore(t)
	ctx := context.Background()
	p, err := s.UpsertProject(ctx, pentest.Project{Name: "Tokens"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := s.UpsertConversation(ctx, pentest.Conversation{ProjectID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"totalTokens": 120, "inputTokens": 100, "outputTokens": 20, "cacheReadTokens": 80}
	if err = s.RecordTurnTokenUsageDetail(ctx, "request", conv.ID, "", pentest.TokenUsage{TotalTokens: 120, InputTokens: 100, OutputTokens: 20, CacheReadTokens: 80}); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteConversation(ctx, conv.ID); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/projects/:id/token-usage", func(c *gin.Context) { writeAIPentestProjectTokenUsage(c, s) })
	for _, id := range []string{p.ID, "missing"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/projects/"+id+"/token-usage", nil))
		if id == "missing" {
			if w.Code != 404 {
				t.Fatal(w.Code, w.Body.String())
			}
			continue
		}
		var body struct {
			Success bool
			Data    map[string]int64
		}
		if err = json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || !body.Success || len(body.Data) != 4 {
			t.Fatal(w.Code, w.Body.String())
		}
		for key, value := range want {
			if body.Data[key] != value {
				t.Fatalf("%s: %v", key, body.Data)
			}
		}
	}
}
