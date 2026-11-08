package toolbox

import (
	"strings"
	"testing"

	"hephaestus-backend/pkg/database"
)

func TestSanitizeRepeaterTabForPersistence(t *testing.T) {
	tab := database.RepeaterTab{
		Name:       "req\x00name",
		Method:     "PO\x00ST",
		URL:        "http://example.com/\x00test",
		RawRequest: "GET /\x00 HTTP/1.1",
		Response:   "PK\x00\x03\x04",
	}

	sanitizeRepeaterTabForPersistence(&tab)

	for fieldName, value := range map[string]string{
		"name":       tab.Name,
		"method":     tab.Method,
		"url":        tab.URL,
		"rawRequest": tab.RawRequest,
		"response":   tab.Response,
	} {
		if strings.Contains(value, "\x00") {
			t.Fatalf("%s still contains null byte after sanitization", fieldName)
		}
	}
}
