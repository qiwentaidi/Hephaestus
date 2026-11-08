package notification

import (
	"strings"
	"testing"

	"hephaestus-backend/pkg/config"
)

func TestAIFindingEventReportsOnlyTitleSeverityTarget(t *testing.T) {
	event := AIFindingEvent("  Unauthenticated file read  ", "High", " https://example.test/files/read ")
	if event.EventType != config.NotificationEventVulnerabilityFound {
		t.Fatalf("unexpected event type: %s", event.EventType)
	}
	want := "漏洞标题: Unauthenticated file read\n风险级别: high\n目标: https://example.test/files/read"
	if event.Message != want {
		t.Fatalf("unexpected message:\n%s", event.Message)
	}
	if strings.Count(event.Message, "\n") != 2 {
		t.Fatalf("message contains more than three reported fields:\n%s", event.Message)
	}
	if event.Data["source"] != "ai_workbench" || event.Data["title"] != "Unauthenticated file read" ||
		event.Data["severity"] != "high" || event.Data["target"] != "https://example.test/files/read" {
		t.Fatalf("unexpected data payload: %+v", event.Data)
	}
	if _, ok := event.Data["taskId"]; ok {
		t.Fatalf("AI finding event must not carry a scan task id: %+v", event.Data)
	}
}
