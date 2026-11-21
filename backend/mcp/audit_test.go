package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func auditTestContext(t *testing.T) *gin.Context {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/mcp/hephaestus", nil)
	startMCPAudit(ctx, "hephaestus")
	return ctx
}

func TestMCPAuditArgumentsRedactSecrets(t *testing.T) {
	ctx := auditTestContext(t)
	raw := json.RawMessage(`{
		"token": "super-secret-token",
		"nested": {"password": "hunter2", "safe": "visible"},
		"rawRequest": "POST /poc HTTP/1.1\r\nHost: example.test\r\nAuthorization: Bearer leaked\r\nCookie: sid=leaked\r\n\r\nbody"
	}`)
	markMCPAuditArguments(ctx, raw)
	stored := currentMCPAudit(ctx).ArgumentsJSON
	for _, leaked := range []string{"super-secret-token", "hunter2", "Bearer leaked", "sid=leaked"} {
		if strings.Contains(stored, leaked) {
			t.Fatalf("audit arguments leaked %q: %s", leaked, stored)
		}
	}
	for _, visible := range []string{"[redacted]", "visible", "POST /poc"} {
		if !strings.Contains(stored, visible) {
			t.Fatalf("audit arguments missing %q: %s", visible, stored)
		}
	}
}

func TestMCPAuditArgumentsAndResponsesAreTruncated(t *testing.T) {
	ctx := auditTestContext(t)
	large := strings.Repeat("a", mcpAuditArgumentLimit+1024)
	raw, _ := json.Marshal(map[string]string{"query": large})
	markMCPAuditArguments(ctx, raw)
	stored := currentMCPAudit(ctx).ArgumentsJSON
	if !strings.HasSuffix(stored, "…[truncated]") || len(stored) > mcpAuditArgumentLimit+64 {
		t.Fatalf("arguments were not truncated: len=%d suffix=%q", len(stored), stored[len(stored)-20:])
	}
}

func TestMCPAuditToolResultSummarizesContent(t *testing.T) {
	ctx := auditTestContext(t)
	base64Image := strings.Repeat("QUJD", 2000)
	markMCPAuditResult(ctx, mcpToolCallResult{
		Content: []mcpToolContent{
			{Type: "text", Text: "POC response marker"},
			{Type: "image", MimeType: "image/png", Data: base64Image},
		},
		StructuredContent: map[string]any{"tabId": "tab-1", "saved": true},
	})
	audit := currentMCPAudit(ctx)
	if !strings.Contains(audit.ResponseText, "POC response marker") {
		t.Fatalf("response text missing: %q", audit.ResponseText)
	}
	if !strings.Contains(audit.ResponseText, "[image image/png") {
		t.Fatalf("image placeholder missing: %q", audit.ResponseText)
	}
	if strings.Contains(audit.ResponseText, base64Image[:64]) {
		t.Fatal("raw image data must not be stored in the audit")
	}
	if !strings.Contains(audit.ResponseJSON, `"tabId":"tab-1"`) {
		t.Fatalf("structured result missing: %q", audit.ResponseJSON)
	}
}

func TestMCPAuditResultWithoutResponse(t *testing.T) {
	ctx := auditTestContext(t)
	markMCPAuditResult(ctx, mcpToolCallResult{Content: []mcpToolContent{{Type: "text", Text: "only text"}}})
	audit := currentMCPAudit(ctx)
	if audit.ResponseText != "only text" || audit.ResponseJSON != "" {
		t.Fatalf("audit result = %#v", audit)
	}
}
