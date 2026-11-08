package jsscan

import (
	"strings"
	"testing"
)

// Keep the synthetic token split in source.
const slackBotTokenFixture = "xoxb" + "-" + "1234567890-abcdefghijklmnopqrst"

func TestFindKnownSecretLeaksRecognizesSlackAndFeishuCredentials(t *testing.T) {
	content := `
const slack = "` + slackBotTokenFixture + `";
const hook = "https://hooks.slack.com/services/T00000000/B00000000/abcdefghijklmnopqrstuv";
const app_secret = "abcdefghijklmnopqrstuvwx";
const larkHook = "https://open.larksuite.com/open-apis/bot/v2/hook/abcdefghijklmnopqrstuvwx";
`

	findings := findKnownSecretLeaks(content)
	if len(findings) != 4 {
		t.Fatalf("expected 4 findings, got %d: %#v", len(findings), findings)
	}
	for _, finding := range findings {
		if !strings.Contains(content, finding.Value) {
			t.Fatalf("finding must retain original value: %#v", finding)
		}
	}
}

func TestSaveSecretLeakFindingsPersistsOriginalSecret(t *testing.T) {
	db := newTestJSScanDB(t)
	secret := slackBotTokenFixture
	saveSecretLeakFindings(db, "task-1", 1, "https://example.test/api", secret)

	var record struct{ Description string }
	if err := db.Table("vuln_scan_js_vulnerability").Select("description").First(&record).Error; err != nil {
		t.Fatalf("load finding: %v", err)
	}
	if !strings.Contains(record.Description, secret) {
		t.Fatalf("stored description must retain the complete secret: %q", record.Description)
	}
}
