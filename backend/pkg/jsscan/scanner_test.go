package jsscan

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qiwentaidi/trailblazer/pkg/core/crawl"
	tbdb "github.com/qiwentaidi/trailblazer/pkg/core/database"
	sdk "github.com/qiwentaidi/trailblazer/pkg/sdk"
	"hephaestus-backend/pkg/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCrawlTrailblazerAPIAssetsReleasesGateWhenContextEnds(t *testing.T) {
	original := trailblazerCrawlAPIAssets
	trailblazerCrawlAPIAssets = func(_ string, options *sdk.APICrawlOptions) (*sdk.APIAssetResult, error) {
		<-options.Context.Done()
		return nil, options.Context.Err()
	}
	t.Cleanup(func() { trailblazerCrawlAPIAssets = original })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := crawlTrailblazerAPIAssets(ctx, "https://example.test", &sdk.APICrawlOptions{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("crawl error = %v, want context deadline exceeded", err)
	}

	select {
	case trailblazerScanGate <- struct{}{}:
		<-trailblazerScanGate
	case <-time.After(100 * time.Millisecond):
		t.Fatal("trailblazer scan gate was not released after context timeout")
	}
}

func newTestJSScanDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.VulnScanJSVulnerability{},
		&database.VulnScanJSAPIResource{},
		&database.VulnScanJSResource{},
		&database.VulnScanJSAsset{},
		&database.VulnScanJSOperation{},
		&database.VulnScanJSOpenAPIDocument{},
		&database.VulnScanJSSiteTree{},
	); err != nil {
		t.Fatalf("migrate vulnerability table: %v", err)
	}
	return db
}

func TestSaveJSScanAPIAssetsForVulnScanTask(t *testing.T) {
	db := newTestJSScanDB(t)
	assets := &sdk.APIAssetResult{
		Store: sdk.NewAPIStore(),
		JSResources: []sdk.JSResource{{
			URL:          "https://example.test/assets/app.js",
			Content:      `fetch('/api/profile'); const password = "example-password"`,
			ResponseCode: 200,
			Size:         21,
			FetchedAt:    time.Now(),
		}},
		RuntimeRecords: []crawl.NetworkRecord{{
			URL:             "https://example.test/api/profile",
			Method:          "GET",
			ResponseCode:    200,
			ResponseHeaders: map[string]string{"Content-Type": "application/json"},
			ResponseBody:    `{"id":1}`,
			FetchedAt:       time.Now(),
		}},
		OperationSpecs: []crawl.OperationSpec{{Method: "GET", PathTemplate: "/api/profile"}},
		APIRootCandidates: []string{
			"/api/",
		},
	}

	if err := saveJSScanAPIAssetsForVulnScanTask(context.Background(), db, "task-1", 1, "https://example.test", assets, nil); err != nil {
		t.Fatalf("save API assets: %v", err)
	}

	for model, want := range map[any]int64{
		&database.VulnScanJSResource{}:        1,
		&database.VulnScanJSAPIResource{}:     1,
		&database.VulnScanJSOperation{}:       1,
		&database.VulnScanJSOpenAPIDocument{}: 1,
		&database.VulnScanJSAsset{}:           3,
	} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil || count != want {
			t.Fatalf("%T count=%d err=%v, want %d", model, count, err, want)
		}
	}

	var roots, sensitive int64
	if err := db.Model(&database.VulnScanJSAsset{}).Where("type = ?", "apiRoot").Count(&roots).Error; err != nil || roots != 1 {
		t.Fatalf("API Root count=%d err=%v, want 1", roots, err)
	}
	if err := db.Model(&database.VulnScanJSAsset{}).Where("type = ?", "sensitive").Count(&sensitive).Error; err != nil || sensitive != 1 {
		t.Fatalf("sensitive asset count=%d err=%v, want 1", sensitive, err)
	}
	var stored database.VulnScanJSAsset
	if err := db.Where("type = ?", "sensitive").First(&stored).Error; err != nil {
		t.Fatalf("load sensitive asset: %v", err)
	}
	if !strings.Contains(stored.Value, "example-password") {
		t.Fatalf("sensitive asset must retain original value, got %q", stored.Value)
	}
}

func TestSaveJSScanAPIAssetsPersistsSiteTree(t *testing.T) {
	db := newTestJSScanDB(t)
	assets := &sdk.APIAssetResult{
		Store:    sdk.NewAPIStore(),
		SiteTree: []crawl.ElTreeNode{{ID: "root", Label: "https://example.test", Children: []crawl.ElTreeNode{{ID: "api", Label: "api"}}}},
	}
	for _, label := range []string{"api", "v2"} {
		assets.SiteTree[0].Children[0].Label = label
		if err := saveJSScanAPIAssetsForVulnScanTask(context.Background(), db, "tree-task", 2, "https://example.test", assets, nil); err != nil {
			t.Fatalf("save site tree: %v", err)
		}
	}
	var trees []database.VulnScanJSSiteTree
	if err := db.Where("task_id = ? AND version = ?", "tree-task", 2).Find(&trees).Error; err != nil {
		t.Fatalf("load site tree: %v", err)
	}
	if len(trees) != 1 || !strings.Contains(trees[0].TreeData, `"label":"v2"`) {
		t.Fatalf("expected one updated site tree, got %+v", trees)
	}
}

func TestTrailblazerVulnerabilityFindingIsPersisted(t *testing.T) {
	db := newTestJSScanDB(t)
	handleTrailblazerVulnFindingForVulnScanTask(db, "task-1", 3, sdk.ScanEvent{
		Type: sdk.EventTypeVulnerability,
		Data: tbdb.VulnRecord{
			Title:            "SQL注入漏洞 (boolean-based)",
			Level:            "high",
			Type:             "SQL_INJECTION",
			URL:              "https://example.test/api/users?id=1",
			Method:           "GET",
			Request:          "GET /api/users?id=1' HTTP/1.1",
			Response:         "SQL syntax error",
			ResponseLength:   16,
			Confidence:       "high",
			ConfidenceReason: "error signature",
			Description:      "发现SQL注入漏洞",
			CreatedAt:        time.Now(),
		},
	})

	var stored database.VulnScanJSVulnerability
	if err := db.Where("task_id = ? AND version = ?", "task-1", 3).First(&stored).Error; err != nil {
		t.Fatalf("load persisted Trailblazer finding: %v", err)
	}
	if stored.Type != "SQL_INJECTION" || stored.Request == "" || stored.Confidence != "high" {
		t.Fatalf("unexpected persisted finding: %#v", stored)
	}
}

func TestNewTrailblazerOperationFuzzersUsesGlobalSwitch(t *testing.T) {
	if got := newTrailblazerOperationFuzzers(VulnDetection{}, nil); len(got) != 0 {
		t.Fatalf("disabled global switch created %d fuzzers", len(got))
	}

	config := GetDefaultConfig().VulnDetection
	fuzzers := newTrailblazerOperationFuzzers(config, nil)
	if len(fuzzers) != 5 {
		t.Fatalf("enabled global switch created %d fuzzers, want 5 without upload checker", len(fuzzers))
	}
	want := []string{"sqli", "lfi", "ssrf", "redirect", "xss"}
	for index, fuzzer := range fuzzers {
		if fuzzer == nil || fuzzer.Name() != want[index] {
			t.Fatalf("fuzzer %d = %v, want %s", index, fuzzer, want[index])
		}
	}
	config.SSRF.Enabled = false
	fuzzers = newTrailblazerOperationFuzzers(config, nil)
	if len(fuzzers) != 4 {
		t.Fatalf("disabled SSRF created %d fuzzers, want 4", len(fuzzers))
	}
	for _, fuzzer := range fuzzers {
		if fuzzer.Name() == "ssrf" {
			t.Fatal("disabled SSRF fuzzer was registered")
		}
	}
	fuzzers = newTrailblazerOperationFuzzers(GetDefaultConfig().VulnDetection, testUploadChecker{})
	if len(fuzzers) != 6 || fuzzers[5].Name() != "file-upload" {
		t.Fatalf("upload reviewer created %d fuzzers, want file-upload as sixth", len(fuzzers))
	}
}

type testUploadChecker struct{}

func (testUploadChecker) CheckFileUpload(_, _, _ string) (bool, string, error) {
	return false, "", nil
}

func TestJSScanUploadCheckerVerifiesUploadedContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/uploads/test.html" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("<h1>uploadtest</h1>"))
	}))
	defer server.Close()
	checker := newJSScanUploadChecker(context.Background(), server.URL, "")
	ok, uploadedURL, err := checker.CheckFileUpload(`{"data":{"fileUrl":"/uploads/test.html"}}`, "test.html", "<h1>uploadtest</h1>")
	if err != nil || !ok || uploadedURL != server.URL+"/uploads/test.html" {
		t.Fatalf("upload verification = %t %q %v", ok, uploadedURL, err)
	}
	ok, _, err = checker.CheckFileUpload(`{"data":{"fileUrl":"/uploads/test.html"}}`, "test.html", "different content")
	if err != nil || ok {
		t.Fatalf("unmatched upload content = %t %v", ok, err)
	}
}

func TestIsUnauthorizedTrailblazerFinding(t *testing.T) {
	for _, record := range []tbdb.VulnRecord{
		{Type: "authorization"},
		{Type: "未授权访问"},
		{Title: "未授权访问（认证对照）"},
	} {
		if !isUnauthorizedTrailblazerFinding(record) {
			t.Fatalf("expected unauthorized finding: %#v", record)
		}
	}
	if isUnauthorizedTrailblazerFinding(tbdb.VulnRecord{Type: "SQL_INJECTION", Title: "SQL 注入"}) {
		t.Fatal("SQL injection must not enter unauthorized AI review")
	}
}

func TestRawUnauthorizedResponseForAI(t *testing.T) {
	raw := `{"users":[{"email":"alice@example.test","phone":"13800138000","idCard":"11010519491231002X"}],"access_token":"secret-token","role":"admin"}`
	if got := rawUnauthorizedResponseForAI(tbdb.VulnRecord{Response: raw}); got != raw {
		t.Fatalf("response body was changed: got %q want %q", got, raw)
	}
	if got := rawUnauthorizedResponseForAI(tbdb.VulnRecord{Response: "fallback", ResponsePlaintext: "plaintext"}); got != "plaintext" {
		t.Fatalf("plaintext response not preferred: %q", got)
	}
}

func TestAPIRecordSecretLeakPreservesOriginalValue(t *testing.T) {
	db := newTestJSScanDB(t)
	secret := slackBotTokenFixture
	handleJSScanEventForVulnScanTask(db, "task-1", 1, sdk.ScanEvent{
		Type: sdk.EventTypeAPIRecord,
		Data: sdk.APIRecord{
			URL:          "https://example.test/api/config",
			Method:       "GET",
			ResponseBody: `{"slack_token":"` + secret + `"}`,
		},
	})

	var apiCount int64
	if err := db.Model(&database.VulnScanJSAPIResource{}).Count(&apiCount).Error; err != nil || apiCount != 1 {
		t.Fatalf("expected one persisted API record, count=%d err=%v", apiCount, err)
	}
	var leak database.VulnScanJSVulnerability
	if err := db.Where("type = ?", "api-key-exposure").First(&leak).Error; err != nil {
		t.Fatalf("load secret leak: %v", err)
	}
	if !strings.Contains(leak.Description, secret) {
		t.Fatalf("stored leak must preserve the complete secret: %q", leak.Description)
	}
}

func TestUpsertJSScanVulnerabilityRecordKeepsDistinctSensitiveDescriptions(t *testing.T) {
	db := newTestJSScanDB(t)

	base := database.VulnScanJSVulnerability{
		TaskID:      "task-1",
		Version:     1,
		Title:       "敏感关键词泄露",
		Type:        "敏感信息泄露",
		URL:         "http://example.com/app.js",
		Level:       "info",
		AIVerified:  true,
		CreatedAt:   time.Now(),
		Description: "发现敏感关键词泄露: password:\"admin123\"",
	}

	first := base
	first.ID = "vuln-1"
	if err := upsertJSScanVulnerabilityRecord(db, &first); err != nil {
		t.Fatalf("insert first vulnerability: %v", err)
	}

	second := base
	second.ID = "vuln-2"
	second.Description = "发现敏感关键词泄露: password:De(c),rememberMe:S===void"
	if err := upsertJSScanVulnerabilityRecord(db, &second); err != nil {
		t.Fatalf("insert second vulnerability: %v", err)
	}

	var count int64
	if err := db.Model(&database.VulnScanJSVulnerability{}).Count(&count).Error; err != nil {
		t.Fatalf("count vulnerabilities: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 vulnerabilities, got %d", count)
	}
}

func TestUpsertJSScanVulnerabilityRecordUpdatesSameSensitiveDescription(t *testing.T) {
	db := newTestJSScanDB(t)

	record := database.VulnScanJSVulnerability{
		ID:          "vuln-1",
		TaskID:      "task-1",
		Version:     1,
		Title:       "敏感关键词泄露",
		Type:        "敏感信息泄露",
		URL:         "http://example.com/app.js",
		Level:       "info",
		AIVerified:  false,
		CreatedAt:   time.Now(),
		Description: "发现敏感关键词泄露: password:\"admin123\"",
	}
	if err := upsertJSScanVulnerabilityRecord(db, &record); err != nil {
		t.Fatalf("insert vulnerability: %v", err)
	}

	record.Level = "high"
	record.AIVerified = true
	if err := upsertJSScanVulnerabilityRecord(db, &record); err != nil {
		t.Fatalf("update vulnerability: %v", err)
	}

	var count int64
	if err := db.Model(&database.VulnScanJSVulnerability{}).Count(&count).Error; err != nil {
		t.Fatalf("count vulnerabilities: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 vulnerability after update, got %d", count)
	}

	var stored database.VulnScanJSVulnerability
	if err := db.First(&stored, "id = ?", "vuln-1").Error; err != nil {
		t.Fatalf("load updated vulnerability: %v", err)
	}
	if stored.Level != "high" || !stored.AIVerified {
		t.Fatalf("unexpected updated record: level=%s ai_verified=%v", stored.Level, stored.AIVerified)
	}
}
