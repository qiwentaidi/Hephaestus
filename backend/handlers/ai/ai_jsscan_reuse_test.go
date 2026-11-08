package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"hephaestus-backend/internal/testfixture"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scantask"
)

func jsTestCreator(t *testing.T, count *int) jsCollectionCreator {
	return func(db *gorm.DB, kind, name, mode, agent string, targets []string, config map[string]interface{}) (string, string, string, string, error) {
		*count++
		id := fmt.Sprintf("js-%d", *count)
		mode, err := scantask.NormalizeMode(mode)
		if err != nil {
			return "", "", "", "", err
		}
		if mode == "local" {
			agent = ""
		}
		config["targets"] = targets
		c, _ := json.Marshal(config)
		urls, _ := json.Marshal(targets)
		task := database.VulnScanTask{ID: id, Version: 1, ScanType: kind, TaskName: name, ScanMode: mode, AssignedAgentID: agent, Targets: string(urls), Config: string(c), Status: "pending", CreatedAt: time.Now()}
		err = db.Create(&task).Error
		return id, kind, mode, agent, err
	}
}

func TestJSCollectionReuseAcrossTurnsAndConditions(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	calls := 0
	create := jsTestCreator(t, &calls)
	ctx := context.Background()
	args := pentestNativeJsscanArgs{Targets: []string{"https://example.test/app"}}
	first, _, err := createOrReuseJSScanWithDB(ctx, db, "conversation-a", nil, args, create)
	if err != nil {
		t.Fatal(err)
	}
	args.TaskName = "js-scan-sm2-check"
	again, _, err := createOrReuseJSScanWithDB(ctx, db, "conversation-a", nil, args, create)
	if err != nil || again["taskId"] != first["taskId"] || again["reused"] != true || calls != 1 {
		t.Fatalf("duplicate created: %#v %v count=%d", again, err, calls)
	}
	for _, status := range []string{"running", "completed", "paused"} {
		if err := db.Model(&database.VulnScanTask{}).Where("id = ?", first["taskId"]).Update("status", status).Error; err != nil {
			t.Fatal(err)
		}
		got, _, err := createOrReuseJSScanWithDB(ctx, db, "conversation-a", nil, args, create)
		if err != nil || got["status"] != status || calls != 1 {
			t.Fatalf("status=%s result=%v err=%v", status, got, err)
		}
	}
	// Different conversations must never inherit another conversation's scan.
	other, _, err := createOrReuseJSScanWithDB(ctx, db, "conversation-b", nil, args, create)
	if err != nil || other["reused"] != false || calls != 2 {
		t.Fatal(other, err, calls)
	}
	args.Config = map[string]interface{}{"headers": map[string]interface{}{"Cookie": "different-session"}}
	changed, _, err := createOrReuseJSScanWithDB(ctx, db, "conversation-a", nil, args, create)
	if err != nil || changed["reused"] != false || calls != 3 {
		t.Fatal(changed, err, calls)
	}
	args.ForceRescan = true
	if _, _, err := createOrReuseJSScanWithDB(ctx, db, "conversation-a", nil, args, create); err == nil || calls != 3 {
		t.Fatal("rescan without reason allowed")
	}
	args.RescanReason = "source bundle changed"
	forced, _, err := createOrReuseJSScanWithDB(ctx, db, "conversation-a", nil, args, create)
	if err != nil || forced["reused"] != false || calls != 4 {
		t.Fatal(forced, err, calls)
	}
	if err := db.Model(&database.VulnScanTask{}).Where("id = ?", forced["taskId"]).Update("status", "failed").Error; err != nil {
		t.Fatal(err)
	}
	args.ForceRescan = false
	retried, _, err := createOrReuseJSScanWithDB(ctx, db, "conversation-a", nil, args, create)
	if err != nil || retried["reused"] != false || calls != 5 {
		t.Fatal("reused stale snapshot after failed rescan", retried, err, calls)
	}
}

func TestJSCollectionLegacyHistoryAndLatestVersion(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	calls := 0
	create := jsTestCreator(t, &calls)
	ctx := context.Background()
	args := pentestNativeJsscanArgs{Targets: []string{"https://example.test"}}
	id, _, _, _, err := create(db, "jsscan", "legacy", "local", "", args.Targets, map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	store := openAIPentestHandlerTestStore(t)
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "legacy"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Title: "legacy"})
	if err != nil {
		t.Fatal(err)
	}
	result := fmt.Sprintf(`{"ok":true,"result":{"taskId":%q}}`, id)
	if _, err := store.SaveToolExecution(ctx, pentest.ToolExecution{ID: "old", ConversationID: conv.ID, ToolName: "attack_surface_create_jsscan_task", Status: pentest.ToolExecutionStatusCompleted, ResultJSON: result}); err != nil {
		t.Fatal(err)
	}
	history, err := store.CompletedToolResults(ctx, conv.ID, "attack_surface_create_jsscan_task")
	if err != nil {
		t.Fatal(err)
	}
	ids := jsCollectionHistoryIDs(history)
	got, _, err := createOrReuseJSScanWithDB(ctx, db, conv.ID, ids, args, create)
	if err != nil || got["taskId"] != id || got["reused"] != true || calls != 1 {
		t.Fatal(got, err, calls)
	}
	// Never fall back to an old completed version when the latest was cancelled.
	var old database.VulnScanTask
	if err := db.Where("id = ?", id).First(&old).Error; err != nil {
		t.Fatal(err)
	}
	old.Version = 2
	old.Status = "cancelled"
	old.CreatedAt = time.Now()
	if err := db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	got, _, err = createOrReuseJSScanWithDB(ctx, db, conv.ID, ids, args, create)
	if err != nil || got["reused"] != false || calls != 2 {
		t.Fatal(got, err, calls)
	}
}

func TestJSCollectionConcurrentCalls(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	calls := 0
	create := jsTestCreator(t, &calls)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := createOrReuseJSScanWithDB(context.Background(), db, "same", nil, pentestNativeJsscanArgs{Targets: []string{"https://example.test"}}, create)
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls != 1 {
		t.Fatalf("created %d tasks", calls)
	}
}

func TestJSContentWindowsAndDeepMatches(t *testing.T) {
	content := strings.Repeat("前缀\n", 10000) + `const publicKey="key"; sm2.doEncrypt(body,publicKey,1);` + strings.Repeat("尾部", 10000)
	matches := jsContentMatches(content, "PUBLICKEY")
	if len(matches) == 0 || matches[0]["matchOffset"].(int) < 16000 || !strings.Contains(matches[0]["content"].(string), "sm2.doEncrypt") {
		t.Fatal(matches)
	}
	got, err := jsContentWindow(content, pentestJSScanEvidenceArgs{Query: "SM2", Length: 400}, 3000)
	if err != nil || got["matchFound"] != true || !strings.Contains(got["content"].(string), "sm2.doEncrypt") {
		t.Fatal(got, err)
	}
	hash := got["contentHash"].(string)
	var assembled strings.Builder
	offset := 0
	for {
		chunk, err := jsContentWindow(content, pentestJSScanEvidenceArgs{Offset: offset, Length: 1023, ContentHash: hash}, 3000)
		if err != nil {
			t.Fatal(err)
		}
		assembled.WriteString(chunk["content"].(string))
		offset = chunk["nextOffset"].(int)
		if chunk["hasMore"] == false {
			break
		}
	}
	if assembled.String() != content {
		t.Fatal("Unicode source changed across chunks")
	}
	for _, args := range []pentestJSScanEvidenceArgs{{Offset: -1}, {Offset: len([]rune(content)) + 1}, {Length: -1}, {ContentHash: "stale"}} {
		if _, err := jsContentWindow(content, args, 3000); err == nil {
			t.Fatal("invalid range/hash accepted")
		}
	}
	empty, err := jsContentWindow(content, pentestJSScanEvidenceArgs{Query: "not_present"}, 3000)
	if err != nil || empty["matchFound"] != false || empty["content"] != "" {
		t.Fatal(empty, err)
	}
	capped, err := jsContentWindow(content, pentestJSScanEvidenceArgs{Length: 999999}, 3000)
	if err != nil || len([]rune(capped["content"].(string))) != 16000 {
		t.Fatal("window cap failed", err)
	}
}

func TestOpenAPIDocumentWindowReadsCompleteUnicodeDocument(t *testing.T) {
	document := `{"openapi":"3.1.0","paths":{` + strings.Repeat(`"/订单/详情":{"get":{"responses":{}}},`, 1200) + `"/end":{}}}`
	var assembled strings.Builder
	args := pentestJSScanOpenAPIArgs{Length: 4096}
	for {
		chunk, err := openAPIDocumentWindow(document, args)
		if err != nil {
			t.Fatal(err)
		}
		assembled.WriteString(chunk["content"].(string))
		args.Offset = chunk["nextOffset"].(int)
		args.ContentHash = chunk["contentHash"].(string)
		if chunk["hasMore"] == false {
			break
		}
	}
	if assembled.String() != document {
		t.Fatal("OpenAPI document was truncated or changed across windows")
	}
	args.ContentHash = "stale"
	if _, err := openAPIDocumentWindow(document, args); err == nil {
		t.Fatal("stale OpenAPI content hash accepted")
	}
}

func TestJSResourceSearchLiteralAndVersionIsolation(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	if err := db.AutoMigrate(&database.VulnScanJSResource{}); err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("x", 20000) + "PUBLIC_KEY = 'value';"
	for _, row := range []database.VulnScanJSResource{{ID: "right", TaskID: "t", Version: 1, Content: content}, {ID: "wildcard", TaskID: "t", Version: 1, Content: "PUBLICxKEY"}, {ID: "other-version", TaskID: "t", Version: 2, Content: content}, {ID: "other-task", TaskID: "other", Version: 1, Content: content}} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	result, _, err := searchPentestJSScanResourcesWithDB(db, database.VulnScanTask{ID: "t", Version: 1}, pentestJSScanResourceSearchArgs{Query: "public_key"})
	if err != nil {
		t.Fatal(err)
	}
	payload := result.(map[string]any)
	resources := payload["resources"].([]map[string]any)
	if len(resources) != 1 || resources[0]["id"] != "right" || len(resources[0]["matches"].([]map[string]any)) == 0 {
		t.Fatal(payload)
	}
}

func TestJSCollectionSignaturePreservesAcquisitionConditions(t *testing.T) {
	base := pentestNativeJsscanArgs{Targets: []string{"https://example.test/#/a", "https://other.test"}}
	original, err := jsCollectionSignature(base)
	if err != nil {
		t.Fatal(err)
	}
	equivalent := pentestNativeJsscanArgs{TaskName: "different", Targets: []string{"https://other.test", "https://example.test/#/a"}, ScanMode: "auto", Config: map[string]interface{}{"vulnDetection": map[string]interface{}{"enabled": false}}}
	same, err := jsCollectionSignature(equivalent)
	if err != nil || same != original {
		t.Fatal("equivalent request differs", err)
	}
	for _, changed := range []pentestNativeJsscanArgs{
		{Targets: []string{"https://example.test/#/b", "https://other.test"}},
		{Targets: base.Targets, Proxy: "http://proxy.test:8080"},
		{Targets: base.Targets, ScanMode: "local"},
		{Targets: base.Targets, ScanMode: "agent", AgentID: "different-node"},
		{Targets: base.Targets, Config: map[string]interface{}{"vulnDetection": map[string]interface{}{"enabled": true}}},
	} {
		signature, err := jsCollectionSignature(changed)
		if err != nil || signature == original {
			t.Fatal("different collection conditions reused", err)
		}
	}
}

func TestNormalizePentestJSCollectionConfigDisablesDefaultVulnerabilityDetection(t *testing.T) {
	for _, detection := range []any{nil, map[string]any{}, map[string]any{"enabled": nil}, map[string]any{"xss": map[string]any{"enabled": true}}} {
		args := pentestNativeJsscanArgs{Config: map[string]any{"vulnDetection": detection}}
		if err := normalizePentestJSCollectionConfig(&args); err != nil {
			t.Fatal(err)
		}
		if args.Config["vulnDetection"].(map[string]any)["enabled"] != false {
			t.Fatalf("partial configuration implicitly enabled vulnerability detection: %#v", args.Config)
		}
	}
	args := pentestNativeJsscanArgs{}
	if err := normalizePentestJSCollectionConfig(&args); err != nil {
		t.Fatal(err)
	}
	detection := args.Config["vulnDetection"].(map[string]interface{})
	if detection["enabled"] != false {
		t.Fatalf("vulnerability detection was not disabled: %#v", args.Config)
	}
	explicit := pentestNativeJsscanArgs{Config: map[string]interface{}{"vulnDetection": map[string]interface{}{"enabled": true}}}
	if err := normalizePentestJSCollectionConfig(&explicit); err != nil {
		t.Fatal(err)
	}
	if explicit.Config["vulnDetection"].(map[string]interface{})["enabled"] != true {
		t.Fatalf("explicit opt-in was overwritten: %#v", explicit.Config)
	}
}

func TestPentestJSCollectionConfigNormalizesBooleanStringsBeforeTaskCreation(t *testing.T) {
	db := testfixture.SQLiteDB(t)
	calls := 0
	create := jsTestCreator(t, &calls)
	for _, tc := range []struct {
		input string
		want  bool
	}{
		{input: "false", want: false},
		{input: "true", want: true},
	} {
		args := pentestNativeJsscanArgs{
			Targets: []string{"https://example.test"},
			Config:  map[string]any{"vulnDetection": map[string]any{"enabled": tc.input}},
		}
		result, _, err := createOrReuseJSScanWithDB(context.Background(), db, "", nil, args, create)
		if err != nil {
			t.Fatal(err)
		}
		var task database.VulnScanTask
		if err := db.First(&task, "id = ?", result["taskId"]).Error; err != nil {
			t.Fatal(err)
		}
		var config struct {
			VulnDetection struct {
				Enabled bool `json:"enabled"`
			} `json:"vulnDetection"`
		}
		if err := json.Unmarshal([]byte(task.Config), &config); err != nil || config.VulnDetection.Enabled != tc.want {
			t.Fatalf("stored config=%s, want enabled=%v: %v", task.Config, tc.want, err)
		}
	}
	for _, config := range []map[string]any{
		{"vulnDetection": map[string]any{"enabled": "invalid"}},
		{"openai": map[string]any{"enabled": "false"}},
	} {
		args := pentestNativeJsscanArgs{Targets: []string{"https://example.test"}, Config: config}
		if _, _, err := createOrReuseJSScanWithDB(context.Background(), db, "", nil, args, create); err == nil {
			t.Fatalf("malformed config created a task: %#v", config)
		}
	}
	if calls != 2 {
		t.Fatalf("malformed config created a task: calls=%d", calls)
	}
}
