package ai

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
)

func TestSaveAIAgentPromptHTTP(t *testing.T) {
	store := openAIPentestHandlerTestStore(t)
	router := gin.New()
	router.PUT("/agent-prompts/:role", func(c *gin.Context) { saveAIAgentPrompt(c, store) })
	request := func(role, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", "/agent-prompts/"+role, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, req)
		if response.Code != status {
			t.Fatalf("status=%d want=%d body=%s", response.Code, status, response.Body.String())
		}
		return response
	}
	request("planner", `{broken`, 400)
	request("planner", `{"prompt":" ","version":1}`, 400)
	request("unknown", `{"prompt":"test","version":1}`, 400)
	request("planner", `{"prompt":"test"}`, 400)
	request("planner", `{"prompt":"`+strings.Repeat("x", 1024*1024)+`","version":1}`, 400)
	request("planner", `{"prompt":"HTTP custom planner","version":1}`, 200)
	conflict := request("planner", `{"prompt":"stale","version":1}`, 409)
	if !strings.Contains(conflict.Body.String(), "version_conflict") {
		t.Fatal(conflict.Body.String())
	}
	response := request("planner", `{"version":2,"reset":true}`, 200)
	var result struct {
		Data pentest.AgentPrompt `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Customized || result.Data.Prompt != pentest.PlannerRoleBlock() || result.Data.Version != 3 {
		t.Fatalf("incorrect reset: %+v", result.Data)
	}
}
