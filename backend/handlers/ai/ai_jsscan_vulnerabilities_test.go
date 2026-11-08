package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"hephaestus-backend/internal/testfixture"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/mcptools"
)

func TestJSVulnerabilitySearchAndEvidence(t *testing.T) {
	testJSVulnerabilitySearchAndEvidence(t, testfixture.SQLiteDB(t))
}

func TestJSVulnerabilitySearchAndEvidencePostgres(t *testing.T) {
	dsn := os.Getenv("HEPHAESTUS_AGENT_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL fixture")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "js_vulnerability_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	sqlDB, err := sql.Open("pgx", dsn+" search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	testJSVulnerabilitySearchAndEvidence(t, db)
}

func testJSVulnerabilitySearchAndEvidence(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.AutoMigrate(&database.VulnScanTask{}, &database.VulnScanJSVulnerability{}); err != nil {
		t.Fatal(err)
	}
	task := database.VulnScanTask{ID: "js-task", Version: 1, ScanType: "jsscan"}
	for _, row := range []database.VulnScanTask{task, {ID: task.ID, Version: 2, ScanType: "jsscan"}, {ID: "other-task", Version: 1, ScanType: "jsscan"}} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC().Truncate(time.Second)
	rows := []database.VulnScanJSVulnerability{
		{ID: "real-uuid-a", TaskID: task.ID, Version: 1, Title: "未授权访问 USER_INFO 100%", Level: "high", Type: "unauthorized", Method: "GET", URL: "https://example.test/user", Request: "GET /user HTTP/1.1", Response: `HTTP/1.1 200 OK` + "\n" + `{"name":"测试"}`, Description: "真实描述", CreatedAt: now},
		{ID: "real-uuid-b", TaskID: task.ID, Version: 1, Title: "静态线索", Level: "medium", Type: "sensitive", Method: "POST", URL: "https://example.test/static", StaticContexts: []database.VulnStaticContext{{SourceURL: "https://example.test/app.js", Snippet: "USERxINFO 100x"}}, CreatedAt: now},
		{ID: "wrong-version", TaskID: task.ID, Version: 2, Title: "USER_INFO", Level: "high", CreatedAt: now},
		{ID: "wrong-task", TaskID: "other-task", Version: 1, Title: "USER_INFO", Level: "high", CreatedAt: now},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	search := func(args pentestJSScanVulnerabilitySearchArgs) map[string]any {
		t.Helper()
		result, handled, err := searchPentestJSScanVulnerabilitiesWithDB(db, task, args)
		if err != nil || !handled {
			t.Fatalf("search: handled=%v err=%v", handled, err)
		}
		return result.(map[string]any)
	}
	page1 := search(pentestJSScanVulnerabilitySearchArgs{PageSize: 1})
	first := page1["vulnerabilities"].([]map[string]any)[0]
	if page1["total"] != int64(2) || page1["hasMore"] != true || first["id"] != rows[0].ID || first["taskId"] != task.ID || first["version"] != 1 || first["hasRequest"] != true || first["hasResponse"] != true {
		t.Fatalf("unexpected first page: %v", page1)
	}
	if _, ok := first["response"]; ok {
		t.Fatal("list included raw response")
	}
	page2 := search(pentestJSScanVulnerabilitySearchArgs{Page: 2, PageSize: 1})
	second := page2["vulnerabilities"].([]map[string]any)[0]
	if page2["hasMore"] != false || second["id"] != rows[1].ID || second["hasRequest"] != false || second["hasResponse"] != false {
		t.Fatalf("unexpected second page: %v", page2)
	}
	for _, args := range []pentestJSScanVulnerabilitySearchArgs{
		{Query: "user_info"}, {Query: "100%"}, {Query: "真实描述"},
		{Query: rows[0].ID}, {Level: " HIGH ", Type: "UNAUTHORIZED", Method: "get"},
	} {
		result := search(args)
		if result["total"] != int64(1) || result["vulnerabilities"].([]map[string]any)[0]["id"] != rows[0].ID {
			t.Fatalf("filter=%+v result=%v", args, result)
		}
	}
	empty := search(pentestJSScanVulnerabilitySearchArgs{Query: "' OR 1=1 --"})
	if empty["total"] != int64(0) || len(empty["vulnerabilities"].([]map[string]any)) != 0 || empty["hasMore"] != false {
		t.Fatalf("query was not treated literally: %v", empty)
	}
	capped := search(pentestJSScanVulnerabilitySearchArgs{Page: -1, PageSize: 999})
	if capped["page"] != 1 || capped["pageSize"] != 50 {
		t.Fatalf("pagination bounds: %v", capped)
	}
	evidence, handled, err := getPentestJSScanEvidenceWithDB(db, task, pentestJSScanEvidenceArgs{EvidenceID: first["id"].(string), EvidenceType: "vulnerability", DetailLevel: "full"})
	if err != nil || !handled {
		t.Fatalf("read searched ID: handled=%v err=%v", handled, err)
	}
	detail := evidence.(map[string]any)
	if detail["request"] != rows[0].Request || detail["response"] != rows[0].Response || detail["taskId"] != task.ID || detail["version"] != 1 || detail["truncated"] != false {
		t.Fatalf("wrong evidence: %v", detail)
	}
	staticEvidence, _, err := getPentestJSScanEvidenceWithDB(db, task, pentestJSScanEvidenceArgs{EvidenceID: second["id"].(string), EvidenceType: "vulnerability"})
	if err != nil || staticEvidence.(map[string]any)["staticContexts"].([]database.VulnStaticContext)[0].Snippet != rows[1].StaticContexts[0].Snippet {
		t.Fatalf("static evidence: %v err=%v", staticEvidence, err)
	}
	for _, id := range []string{task.ID, "1", "vul-1", "wrong-task", "wrong-version"} {
		_, _, err := getPentestJSScanEvidenceWithDB(db, task, pentestJSScanEvidenceArgs{EvidenceID: id, EvidenceType: "vulnerability"})
		if err == nil || !strings.Contains(err.Error(), jsScanVulnerabilitySearchTool) {
			t.Fatalf("invalid ID=%s error=%v", id, err)
		}
	}
	// Read a long raw HTTP response without whitespace changes or hidden truncation.
	longResponse := "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\n" + strings.Repeat("第一行\n  第二行\t\n", 2000)
	if err := db.Model(&database.VulnScanJSVulnerability{}).Where("id = ?", rows[0].ID).Update("response", longResponse).Error; err != nil {
		t.Fatal(err)
	}
	args := pentestJSScanEvidenceArgs{EvidenceID: rows[0].ID, EvidenceType: "vulnerability", Field: "response", Length: 1024}
	var assembled strings.Builder
	for {
		part, _, err := getPentestJSScanEvidenceWithDB(db, task, args)
		if err != nil {
			t.Fatal(err)
		}
		window := part.(map[string]any)
		assembled.WriteString(window["content"].(string))
		args.Offset, args.ContentHash = window["nextOffset"].(int), window["contentHash"].(string)
		if window["hasMore"] == false {
			break
		}
	}
	if assembled.String() != longResponse {
		t.Fatal("raw HTTP response changed during segmented reads")
	}
	args.ContentHash = "stale"
	if _, _, err := getPentestJSScanEvidenceWithDB(db, task, args); err == nil {
		t.Fatal("stale payload hash accepted")
	}
	args.Field = "unknown"
	if _, _, err := getPentestJSScanEvidenceWithDB(db, task, args); err == nil {
		t.Fatal("invalid evidence field accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := searchPentestJSScanVulnerabilitiesWithDB(db.WithContext(ctx), task, pentestJSScanVulnerabilitySearchArgs{}); err == nil {
		t.Fatal("cancelled search succeeded")
	}
	if err := db.Migrator().DropTable(&database.VulnScanJSVulnerability{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := getPentestJSScanEvidenceWithDB(db, task, pentestJSScanEvidenceArgs{EvidenceID: rows[0].ID, EvidenceType: "vulnerability"}); err == nil || strings.Contains(err.Error(), "漏洞证据不存在") {
		t.Fatalf("database failure was reported as missing evidence: %v", err)
	}
}

func TestJSVulnerabilitySearchAvailableToAgent(t *testing.T) {
	allowed := false
	for _, name := range pentest.DefaultPentestToolAllowlist() {
		allowed = allowed || name == jsScanVulnerabilitySearchTool
	}
	if !allowed || !isPentestTaskReadTool(jsScanVulnerabilitySearchTool) {
		t.Fatal("vulnerability search is not an allowed task reader")
	}
	if err := validatePentestTaskReadType(jsScanVulnerabilitySearchTool, database.VulnScanTask{ScanType: "portscan"}); err == nil {
		t.Fatal("search accepted non-JS task")
	}
	// Invalid input reaches the native dispatch and is rejected before database access.
	_, handled, err := mcptools.RunNative(context.Background(), pentest.AgentLoopConfig{}, pentest.AgentToolCall{Function: pentest.AgentFunctionCall{Name: jsScanVulnerabilitySearchTool, Arguments: map[string]any{}}})
	if !handled || err == nil || !strings.Contains(err.Error(), "taskId") {
		t.Fatalf("native dispatch: handled=%v err=%v", handled, err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Tools []pentest.AgentToolSchema `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		found := false
		for _, tool := range body.Tools {
			found = found || tool.Function.Name == jsScanVulnerabilitySearchTool
		}
		if !found {
			t.Error("model did not receive vulnerability search schema")
		}
		requests++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"可读取漏洞列表"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	_, err = pentest.RunAgentLoop(context.Background(), pentest.AgentLoopConfig{
		BaseURL: server.URL, APIKey: "test", Model: "test", DisableModelStreaming: true,
		WorkDir: t.TempDir(), MaxIterations: 1,
		Conversation: pentest.Conversation{Mode: pentest.SoloPentestMode},
		NativeTools:  mcptools.NativeSchemas(), ToolAllowlist: pentest.DefaultPentestToolAllowlist(),
	}, "读取 JS 任务的漏洞", nil)
	if err != nil || requests != 1 {
		t.Fatalf("model request count=%d err=%v", requests, err)
	}
}
