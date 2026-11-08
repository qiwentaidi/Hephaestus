package toolbox

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/toolguard"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestToolGuardConfigHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var stored any
	config.InitToolGuardConfigDB(
		func(string) (any, error) { return stored, nil },
		func(_ string, value any) error { stored = value; return nil },
		"tool_guard_test",
	)
	original := toolguard.Global().Config()
	t.Cleanup(func() { _ = toolguard.UpdateGlobal(original) })

	router := gin.New()
	router.GET("/config", GetToolGuardConfigHandler)
	router.PUT("/config", UpdateToolGuardConfigHandler)
	router.POST("/test", TestToolGuardConfigHandler)

	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/config", nil))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", getResponse.Code, getResponse.Body.String())
	}
	var getPayload struct {
		Success bool             `json:"success"`
		Data    toolguard.Config `json:"data"`
	}
	if err := json.Unmarshal(getResponse.Body.Bytes(), &getPayload); err != nil {
		t.Fatal(err)
	}
	if !getPayload.Success || len(getPayload.Data.Rules) != len(toolguard.PresetRules()) {
		t.Fatalf("unexpected default config: %#v", getPayload)
	}
	defaultRuleID := getPayload.Data.Rules[0].ID
	if defaultRuleID == "" || getPayload.Data.PresetVersion != toolguard.CurrentPresetVersion {
		t.Fatalf("unexpected preset config: %#v", getPayload.Data)
	}
	foundRecursiveDelete := false
	for _, rule := range getPayload.Data.Rules {
		if rule.ID == "preset-recursive-delete" {
			foundRecursiveDelete = true
			break
		}
	}
	if !foundRecursiveDelete {
		t.Fatal("recursive delete preset is missing")
	}
	if stored == nil {
		t.Fatal("expected default config to be persisted")
	}
	secondGetResponse := httptest.NewRecorder()
	router.ServeHTTP(secondGetResponse, httptest.NewRequest(http.MethodGet, "/config", nil))
	var secondGetPayload struct {
		Data toolguard.Config `json:"data"`
	}
	if err := json.Unmarshal(secondGetResponse.Body.Bytes(), &secondGetPayload); err != nil {
		t.Fatal(err)
	}
	if len(secondGetPayload.Data.Rules) != len(toolguard.PresetRules()) || secondGetPayload.Data.Rules[0].ID != defaultRuleID {
		t.Fatalf("default rule ID was not stable: %#v", secondGetPayload.Data)
	}
	stored = toolguard.Config{Enabled: false, Rules: []toolguard.Rule{{
		ID: "dangerous-recursive-delete", Name: "legacy", Enabled: true, Pattern: "secret",
	}}}
	legacyGetResponse := httptest.NewRecorder()
	router.ServeHTTP(legacyGetResponse, httptest.NewRequest(http.MethodGet, "/config", nil))
	var legacyGetPayload struct {
		Data toolguard.Config `json:"data"`
	}
	if err := json.Unmarshal(legacyGetResponse.Body.Bytes(), &legacyGetPayload); err != nil {
		t.Fatal(err)
	}
	if len(legacyGetPayload.Data.Rules) != len(toolguard.PresetRules())+1 || legacyGetPayload.Data.Rules[len(legacyGetPayload.Data.Rules)-1].ID == "dangerous-recursive-delete" {
		t.Fatalf("legacy rule ID was not migrated: %#v", legacyGetPayload.Data)
	}
	legacyID := legacyGetPayload.Data.Rules[len(legacyGetPayload.Data.Rules)-1].ID
	if _, err := uuid.Parse(legacyID); err != nil {
		t.Fatalf("migrated rule ID = %q, expected UUID", legacyID)
	}
	if !legacyGetPayload.Data.Enabled {
		t.Fatal("expected legacy disabled config to be migrated to enabled")
	}

	testBody := []byte(`{"config":{"enabled":false,"rules":[{"name":"高危递归删除命令","enabled":true,"tools":["exec"],"pattern":"(?i)rm\\s+-rf","message":"blocked {match}"}]},"toolName":"exec","arguments":{"command":"rm -rf /tmp/demo"}}`)
	testResponse := httptest.NewRecorder()
	router.ServeHTTP(testResponse, httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(testBody)))
	if testResponse.Code != http.StatusOK || !bytes.Contains(testResponse.Body.Bytes(), []byte(`"blocked":true`)) {
		t.Fatalf("test status = %d, body = %s", testResponse.Code, testResponse.Body.String())
	}

	updateBody := []byte(`{"enabled":false,"rules":[{"name":"new rule","enabled":true,"pattern":"secret","message":"blocked"}]}`)
	updateResponse := httptest.NewRecorder()
	router.ServeHTTP(updateResponse, httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader(updateBody)))
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", updateResponse.Code, updateResponse.Body.String())
	}
	var updatePayload struct {
		Data toolguard.Config `json:"data"`
	}
	if err := json.Unmarshal(updateResponse.Body.Bytes(), &updatePayload); err != nil {
		t.Fatal(err)
	}
	if len(updatePayload.Data.Rules) != 1 {
		t.Fatalf("unexpected updated config: %#v", updatePayload.Data)
	}
	if _, err := uuid.Parse(updatePayload.Data.Rules[0].ID); err != nil {
		t.Fatalf("updated rule ID = %q, expected UUID", updatePayload.Data.Rules[0].ID)
	}
	if !updatePayload.Data.Enabled || !toolguard.Global().Config().Enabled {
		t.Fatal("expected global tool guard to remain enabled")
	}
	updatedGet := httptest.NewRecorder()
	router.ServeHTTP(updatedGet, httptest.NewRequest(http.MethodGet, "/config", nil))
	var updatedPayload struct {
		Data toolguard.Config `json:"data"`
	}
	if err := json.Unmarshal(updatedGet.Body.Bytes(), &updatedPayload); err != nil {
		t.Fatal(err)
	}
	if len(updatedPayload.Data.Rules) != 1 || updatedPayload.Data.Rules[0].ID != updatePayload.Data.Rules[0].ID {
		t.Fatalf("deleted presets were re-seeded: %#v", updatedPayload.Data)
	}
}
