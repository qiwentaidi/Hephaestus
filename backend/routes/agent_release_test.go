package routes

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"hephaestus-backend/pkg/agentrelease"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentBootstrapRoutesAndRemovedSSH(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetupRoutes(r)
	for _, path := range []string{"/api/v1/agent/release", "/api/v1/agent/release?format=text", "/api/v1/agent/install.sh", "/api/v1/agent/install.bat", "/api/v1/agent/install.ps1"} {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != 200 {
			t.Fatalf("bootstrap requires auth or is missing: %s: %d", path, response.Code)
		}
		if strings.Contains(response.Body.String(), "__HEPHAESTUS_POWERSHELL_BASE64__") {
			t.Fatal("BAT payload was not expanded")
		}
		if path == "/api/v1/agent/release" {
			var result struct {
				Data agentrelease.Release `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Data.AgentVersion != agentrelease.Current().AgentVersion {
				t.Fatal("wrong bootstrap version")
			}
		}
	}
	for _, route := range r.Routes() {
		if strings.Contains(route.Path, "agent-hosts") {
			t.Fatalf("SSH deployment route remains: %s", route.Path)
		}
	}
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/agent/config.yaml", nil))
	if response.Code != 404 {
		t.Fatal("bootstrap exposes an unapproved file")
	}
}

func TestAgentUpdateRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetupRoutes(r)
	for _, item := range [][2]string{{"GET", "/api/v1/webscan/agent-updates"}, {"POST", "/api/v1/webscan/agent-updates"}, {"POST", "/api/v1/webscan/agent-updates/node/cancel"}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(item[0], item[1], nil))
		if w.Code != 401 {
			t.Fatalf("%s %s requires auth: %d", item[0], item[1], w.Code)
		}
	}
}
