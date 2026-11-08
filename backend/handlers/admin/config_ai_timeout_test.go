package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/pkg/config"
)

func TestAIRequestTimeoutAPIValidationAndPersistence(t *testing.T) {
	var stored any
	config.InitAIConfigDB(func(string) (any, error) { return stored, nil }, func(_ string, v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, &stored)
	}, "fixture-ai")
	t.Cleanup(func() { config.InitAIConfigDB(nil, nil, "") })
	r := gin.New()
	r.PUT("/config/ai", UpdateAIConfigHandler)
	r.GET("/config/ai", GetAIConfigHandler)
	for _, tc := range []struct {
		value  string
		status int
	}{
		{"30", 200}, {"1", 200}, {"120", 200}, {"121", 400}, {"-1", 400}, {"1.5", 400},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/config/ai", strings.NewReader(`{"providers":[{"id":"local","requestTimeoutMinutes":`+tc.value+`}]}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("value %s: status %d body %s", tc.value, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/config/ai", nil))
	var got struct {
		Data config.AIConfig `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Data.Providers) != 1 || got.Data.Providers[0].RequestTimeoutMinutes != 120 {
		t.Fatalf("invalid updates modified persisted value: %s", w.Body.String())
	}
}
