package jsscan

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type secretLeakFinding struct {
	Provider string
	Kind     string
	Value    string
	Level    string
}

var (
	slackWebhookPattern  = regexp.MustCompile(`(?i)https://hooks\.slack\.com/services/[a-z0-9]+/[a-z0-9]+/[a-z0-9_-]{12,}`)
	slackTokenPattern    = regexp.MustCompile(`\b(?:xox[abprs]-|xapp-)[A-Za-z0-9-]{10,}\b`)
	feishuWebhookPattern = regexp.MustCompile(`(?i)https://open\.(?:feishu\.cn|larksuite\.com)/open-apis/bot/v2/hook/[a-z0-9_-]{16,}`)
	feishuSecretPattern  = regexp.MustCompile(`(?i)(?:app[_-]?secret|tenant[_-]?access[_-]?token|app[_-]?access[_-]?token)\s*["']?\s*[:=]\s*["']([A-Za-z0-9_-]{16,})`)
)

func findKnownSecretLeaks(content string) []secretLeakFinding {
	if strings.TrimSpace(content) == "" {
		return nil
	}

	findings := make([]secretLeakFinding, 0)
	appendMatches := func(pattern *regexp.Regexp, provider, kind, level string) {
		for _, value := range pattern.FindAllString(content, -1) {
			findings = append(findings, secretLeakFinding{Provider: provider, Kind: kind, Value: strings.TrimSpace(value), Level: level})
		}
	}
	appendMatches(slackWebhookPattern, "Slack", "Incoming Webhook", "high")
	appendMatches(slackTokenPattern, "Slack", "Access Token", "high")
	appendMatches(feishuWebhookPattern, "飞书 / Lark", "Bot Webhook", "high")
	for _, match := range feishuSecretPattern.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			findings = append(findings, secretLeakFinding{Provider: "飞书 / Lark", Kind: "App Secret 或 Access Token", Value: strings.TrimSpace(match[1]), Level: "high"})
		}
	}

	seen := make(map[string]struct{}, len(findings))
	unique := make([]secretLeakFinding, 0, len(findings))
	for _, finding := range findings {
		key := finding.Provider + "|" + finding.Kind + "|" + finding.Value
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, finding)
	}
	sort.Slice(unique, func(i, j int) bool {
		return unique[i].Provider+unique[i].Kind+unique[i].Value < unique[j].Provider+unique[j].Kind+unique[j].Value
	})
	return unique
}

func saveSecretLeakFindings(db *gorm.DB, taskID string, version int, source, content string) {
	for _, finding := range findKnownSecretLeaks(content) {
		record := &database.VulnScanJSVulnerability{
			ID:          uuid.NewString(),
			TaskID:      taskID,
			Version:     version,
			Title:       finding.Provider + " 密钥泄露",
			Level:       finding.Level,
			Type:        "api-key-exposure",
			URL:         source,
			Method:      "GET",
			Confidence:  "high",
			Description: "检测到 " + finding.Provider + " " + finding.Kind + "：" + finding.Value,
			CreatedAt:   time.Now(),
		}
		_ = upsertJSScanVulnerabilityRecord(db, record)
	}
}
