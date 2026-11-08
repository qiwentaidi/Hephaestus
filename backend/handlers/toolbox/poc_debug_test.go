package toolbox

import (
	"strings"
	"testing"

	"hephaestus-backend/pkg/database"
)

func TestSanitizePoCDebugTabForPersistence(t *testing.T) {
	tab := database.PoCDebugTab{
		ID:         "tab\x00id",
		Name:       "debug\x00tab",
		TemplateID: "template\x00id",
		YAML:       "id: test\x00-template",
		Target:     "https://example.com/\x00",
		Source:     "invalid",
		SourceName: "source\x00name",
		ActiveView: "invalid",
		LastStatus: "invalid",
	}

	sanitizePoCDebugTabForPersistence(&tab)

	for fieldName, value := range map[string]string{
		"id":         tab.ID,
		"name":       tab.Name,
		"templateID": tab.TemplateID,
		"yaml":       tab.YAML,
		"target":     tab.Target,
		"sourceName": tab.SourceName,
	} {
		if strings.Contains(value, "\x00") {
			t.Fatalf("%s still contains null byte after sanitization", fieldName)
		}
	}
	if tab.Source != "manual" {
		t.Fatalf("source = %q, want manual", tab.Source)
	}
	if tab.ActiveView != "template" {
		t.Fatalf("activeView = %q, want template", tab.ActiveView)
	}
	if tab.LastStatus != "" {
		t.Fatalf("lastStatus = %q, want empty", tab.LastStatus)
	}
}
