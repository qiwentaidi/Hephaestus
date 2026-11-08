package ai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"hephaestus-backend/handlers/admin"
)

func TestListAIModelsHandler(t *testing.T) {
	for _, tc := range []struct {
		upstream, status int
		code             string
	}{
		{http.StatusOK, http.StatusOK, ""},
		{http.StatusUnauthorized, http.StatusBadRequest, "authentication_failed"},
	} {
		t.Run(fmt.Sprint(tc.upstream), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.upstream)
				fmt.Fprint(w, `{"data":[{"id":"draft-model"}]}`)
			}))
			defer upstream.Close()
			body, _ := json.Marshal(map[string]string{"protocol": "openai-compatible", "api": upstream.URL + "/v1", "key": "draft-key"})
			router := gin.New()
			router.POST("/models", admin.ListAIModelsHandler)
			req := httptest.NewRequest(http.MethodPost, "/models", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.status || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
			}
			if tc.code != "" && !strings.Contains(response.Body.String(), tc.code) {
				t.Fatal(response.Body.String())
			}
			if tc.code == "" && !strings.Contains(response.Body.String(), "draft-model") {
				t.Fatal(response.Body.String())
			}
		})
	}
}
