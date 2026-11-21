package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"hephaestus-backend/pkg/database"
)

type fakeRepeaterActions struct {
	storage   map[string]string
	route     string
	openCalls int
}

func (f *fakeRepeaterActions) SetDesktopStorage(_ context.Context, items map[string]string) error {
	f.storage = items
	return nil
}
func (f *fakeRepeaterActions) OpenDesktopRoute(_ context.Context, route string) error {
	f.route = route
	f.openCalls++
	return nil
}

func repeaterTestContext(t *testing.T) *gin.Context {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/mcp/hephaestus", nil)
	return ctx
}

func rawPOCRequest(server *httptest.Server, method, path, body string) string {
	parsed, _ := url.Parse(server.URL)
	return fmt.Sprintf("%s %s HTTP/1.1\r\nHost: %s\r\nContent-Type: text/plain\r\n\r\n%s", method, path, parsed.Host, body)
}

func TestRepeaterSendReturnsResponseAndStoresTab(t *testing.T) {
	marker := "hephaestus-poc-marker-2026"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/poc" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "poc-response:%s", marker)
	}))
	defer server.Close()

	oldSave := repeaterSaveTab
	defer func() { repeaterSaveTab = oldSave }()
	var saved *database.RepeaterTab
	repeaterSaveTab = func(tab *database.RepeaterTab) error {
		copied := *tab
		saved = &copied
		return nil
	}

	args, _ := json.Marshal(map[string]any{
		"rawRequest": rawPOCRequest(server, http.MethodPost, "/poc", "request-body"),
		"name":       "POC screenshot test",
	})
	result, err := mcpCallRepeaterSend(repeaterTestContext(t), args)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("send failed: %#v", result.Content)
	}
	if saved == nil {
		t.Fatal("repeater tab was not stored")
	}
	if saved.Method != http.MethodPost || !strings.HasSuffix(saved.URL, "/poc") {
		t.Fatalf("stored tab target = %s %s", saved.Method, saved.URL)
	}
	if !strings.Contains(saved.Response, marker) || !strings.Contains(saved.RawRequest, "request-body") {
		t.Fatalf("stored tab missing request/response: %#v", saved)
	}
	text := result.Content[0].Text
	if !strings.Contains(text, marker) || !strings.Contains(text, saved.ID) {
		t.Fatalf("reply does not contain response/tabId: %q", text)
	}
	structured, _ := result.StructuredContent.(map[string]any)
	if structured["saved"] != true || structured["statusCode"] != 200 {
		t.Fatalf("structured = %#v", structured)
	}
}

func TestRepeaterSendFailureDoesNotStoreTab(t *testing.T) {
	oldSave := repeaterSaveTab
	defer func() { repeaterSaveTab = oldSave }()
	saved := false
	repeaterSaveTab = func(*database.RepeaterTab) error {
		saved = true
		return nil
	}

	raw := "GET / HTTP/1.1\r\nHost: 127.0.0.1:1\r\n\r\n"
	args, _ := json.Marshal(map[string]any{"rawRequest": raw, "timeoutSeconds": 2})
	result, err := mcpCallRepeaterSend(repeaterTestContext(t), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || saved {
		t.Fatalf("failed send must not store a tab: error=%v saved=%v", result.IsError, saved)
	}
	if text := result.Content[0].Text; !strings.Contains(text, "未写入请求转发标签页") {
		t.Fatalf("failure text = %q", text)
	}
}

func TestRepeaterSendRejectsEmptyOrInvalidRequest(t *testing.T) {
	for _, body := range []string{"", "not-a-request"} {
		args, _ := json.Marshal(map[string]any{"rawRequest": body})
		result, err := mcpCallRepeaterSend(repeaterTestContext(t), args)
		if err != nil || !result.IsError {
			t.Fatalf("body %q: err=%v result=%#v", body, err, result)
		}
	}
}

func TestRepeaterOpenSelectsTabInDesktopClient(t *testing.T) {
	oldLoad := repeaterLoadTab
	oldSettle, oldConfirm, oldPending := repeaterOpenSettle, repeaterOpenConfirmWait, repeaterOpenRequestPending
	defer func() {
		repeaterLoadTab = oldLoad
		repeaterOpenSettle, repeaterOpenConfirmWait, repeaterOpenRequestPending = oldSettle, oldConfirm, oldPending
		RegisterDesktopUIActions(nil)
	}()
	repeaterOpenSettle = 5 * time.Millisecond
	repeaterOpenConfirmWait = 5 * time.Millisecond
	repeaterOpenRequestPending = func() bool { return false }
	repeaterLoadTab = func(id string) (database.RepeaterTab, error) {
		return database.RepeaterTab{ID: id, Name: "POC screenshot test"}, nil
	}
	actions := &fakeRepeaterActions{}
	RegisterDesktopUIActions(actions)

	args, _ := json.Marshal(map[string]any{"tabId": "tab-1"})
	result, err := mcpCallRepeaterOpen(repeaterTestContext(t), args)
	if err != nil || result.IsError {
		t.Fatalf("open failed: err=%v result=%#v", err, result)
	}
	if actions.route != repeaterRoute {
		t.Fatalf("route = %q", actions.route)
	}
	payload, ok := actions.storage[repeaterOpenStorageKey]
	if !ok || !strings.Contains(payload, `"tabId":"tab-1"`) {
		t.Fatalf("storage = %#v", actions.storage)
	}
	if text := result.Content[0].Text; !strings.Contains(text, "desktop_screenshot") || !strings.Contains(text, "tab-1") {
		t.Fatalf("open text = %q", text)
	}
}

func TestRepeaterOpenRequiresDesktopClient(t *testing.T) {
	RegisterDesktopUIActions(nil)
	args, _ := json.Marshal(map[string]any{"tabId": "tab-1"})
	result, err := mcpCallRepeaterOpen(repeaterTestContext(t), args)
	if err != nil || !result.IsError || !strings.Contains(result.Content[0].Text, "桌面客户端") {
		t.Fatalf("server-only open must fail with a clear hint: err=%v result=%#v", err, result)
	}
}

func TestRepeaterOpenRetriesWhenFirstLoadWasDropped(t *testing.T) {
	oldLoad, oldSettle, oldConfirm, oldPending := repeaterLoadTab, repeaterOpenSettle, repeaterOpenConfirmWait, repeaterOpenRequestPending
	defer func() {
		repeaterLoadTab, repeaterOpenSettle, repeaterOpenConfirmWait, repeaterOpenRequestPending = oldLoad, oldSettle, oldConfirm, oldPending
		RegisterDesktopUIActions(nil)
	}()
	repeaterOpenSettle = 10 * time.Millisecond
	repeaterOpenConfirmWait = 5 * time.Millisecond
	repeaterOpenRequestPending = func() bool { return true }
	repeaterLoadTab = func(id string) (database.RepeaterTab, error) {
		return database.RepeaterTab{ID: id, Name: "POC"}, nil
	}
	actions := &fakeRepeaterActions{}
	RegisterDesktopUIActions(actions)

	args, _ := json.Marshal(map[string]any{"tabId": "tab-retry"})
	result, err := mcpCallRepeaterOpen(repeaterTestContext(t), args)
	if err != nil || result.IsError {
		t.Fatalf("open failed: err=%v result=%#v", err, result)
	}
	if actions.openCalls != 2 {
		t.Fatalf("open calls = %d, want 2", actions.openCalls)
	}
	structured, _ := result.StructuredContent.(map[string]any)
	if structured["retried"] != true {
		t.Fatalf("structured = %#v", structured)
	}
}

func TestRepeaterToolsCatalogAndProfileVisibility(t *testing.T) {
	catalog := map[string]bool{}
	for _, tool := range mcpCatalogTools() {
		catalog[tool.Name] = true
	}
	if !catalog[repeaterSendToolName] || !catalog[repeaterOpenToolName] || !catalog[repeaterDeleteToolName] {
		t.Fatalf("catalog missing repeater tools: %#v", catalog)
	}
	advertised := func() map[string]bool {
		out := map[string]bool{}
		for _, tool := range mcpToolsForProfile(mcpHephaestusProfile) {
			out[tool.Name] = true
		}
		return out
	}
	RegisterDesktopUIActions(nil)
	if toolList := advertised(); !toolList[repeaterSendToolName] || !toolList[repeaterDeleteToolName] || toolList[repeaterOpenToolName] {
		t.Fatalf("server profile = %#v", toolList)
	}
	RegisterDesktopUIActions(&fakeRepeaterActions{})
	t.Cleanup(func() { RegisterDesktopUIActions(nil) })
	if toolList := advertised(); !toolList[repeaterSendToolName] || !toolList[repeaterOpenToolName] || !toolList[repeaterDeleteToolName] {
		t.Fatalf("desktop profile = %#v", toolList)
	}
}

func TestRepeaterDeleteNotifiesDesktopAndRemovesRow(t *testing.T) {
	oldLoad, oldDelete, oldSettle := repeaterLoadTab, repeaterDeleteTab, repeaterDeleteSettle
	defer func() {
		repeaterLoadTab, repeaterDeleteTab, repeaterDeleteSettle = oldLoad, oldDelete, oldSettle
		RegisterDesktopUIActions(nil)
	}()
	repeaterDeleteSettle = 5 * time.Millisecond
	repeaterLoadTab = func(id string) (database.RepeaterTab, error) {
		return database.RepeaterTab{ID: id, Name: "old POC"}, nil
	}
	deleted := ""
	repeaterDeleteTab = func(id string) error {
		deleted = id
		return nil
	}
	actions := &fakeRepeaterActions{}
	RegisterDesktopUIActions(actions)

	args, _ := json.Marshal(map[string]any{"tabId": "tab-delete"})
	result, err := mcpCallRepeaterDelete(repeaterTestContext(t), args)
	if err != nil || result.IsError {
		t.Fatalf("delete failed: err=%v result=%#v", err, result)
	}
	if deleted != "tab-delete" {
		t.Fatalf("deleted id = %q", deleted)
	}
	if actions.route != repeaterRoute {
		t.Fatalf("route = %q", actions.route)
	}
	payload, ok := actions.storage[repeaterDeleteStorageKey]
	if !ok || !strings.Contains(payload, `"tabId":"tab-delete"`) {
		t.Fatalf("storage = %#v", actions.storage)
	}
	structured, _ := result.StructuredContent.(map[string]any)
	if structured["deleted"] != true || structured["tabId"] != "tab-delete" || structured["uiReloaded"] != true {
		t.Fatalf("structured = %#v", structured)
	}
}

func TestRepeaterDeleteIsIdempotentForMissingTab(t *testing.T) {
	oldLoad, oldDelete := repeaterLoadTab, repeaterDeleteTab
	defer func() { repeaterLoadTab, repeaterDeleteTab = oldLoad, oldDelete }()
	repeaterLoadTab = func(string) (database.RepeaterTab, error) {
		return database.RepeaterTab{}, errRepeaterTabMissing
	}
	deleted := false
	repeaterDeleteTab = func(string) error {
		deleted = true
		return nil
	}
	args, _ := json.Marshal(map[string]any{"tabId": "missing"})
	result, err := mcpCallRepeaterDelete(repeaterTestContext(t), args)
	if err != nil || result.IsError || deleted {
		t.Fatalf("missing delete must be a no-op success: err=%v result=%#v deleted=%v", err, result, deleted)
	}
	structured, _ := result.StructuredContent.(map[string]any)
	if structured["alreadyMissing"] != true {
		t.Fatalf("structured = %#v", structured)
	}
}

func TestRepeaterDeleteWorksWithoutDesktopClient(t *testing.T) {
	oldLoad, oldDelete := repeaterLoadTab, repeaterDeleteTab
	defer func() { repeaterLoadTab, repeaterDeleteTab = oldLoad, oldDelete }()
	RegisterDesktopUIActions(nil)
	repeaterLoadTab = func(id string) (database.RepeaterTab, error) {
		return database.RepeaterTab{ID: id, Name: "server POC"}, nil
	}
	deleted := ""
	repeaterDeleteTab = func(id string) error {
		deleted = id
		return nil
	}
	args, _ := json.Marshal(map[string]any{"tabId": "server-tab"})
	result, err := mcpCallRepeaterDelete(repeaterTestContext(t), args)
	if err != nil || result.IsError || deleted != "server-tab" {
		t.Fatalf("server delete failed: err=%v result=%#v deleted=%q", err, result, deleted)
	}
	structured, _ := result.StructuredContent.(map[string]any)
	if structured["uiReloaded"] != false {
		t.Fatalf("structured = %#v", structured)
	}
}
