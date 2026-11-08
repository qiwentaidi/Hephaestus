package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
)

func TestPentestStreamResumeCursor(t *testing.T) {
	for _, tt := range []struct {
		name, query, header string
		want                int64
	}{
		{name: "initial subscription"},
		{name: "browser resume", query: "lastEventId=42", want: 42},
		{name: "header compatibility", header: "43", want: 43},
		{name: "header takes precedence", query: "lastEventId=42", header: "44", want: 44},
		{name: "negative query", query: "lastEventId=-1"},
		{name: "negative header", header: "-1"},
		{name: "invalid query", query: "lastEventId=invalid"},
		{name: "invalid header", header: "invalid"},
		{name: "overflow", query: "lastEventId=9223372036854775808"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/stream?"+tt.query, nil)
			c.Request.Header.Set("Last-Event-ID", tt.header)
			if got := pentestStreamResumeCursor(c); got != tt.want {
				t.Fatalf("resume cursor = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDistributedPentestStreamResumesAfterBrowserCursor(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	store := pentest.NewSQLStore(db)
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := store.UpsertProject(ctx, pentest.Project{Name: "resume fixture"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Mode: pentest.SoloPentestMode, Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	for sequence := 1; sequence <= 3; sequence++ {
		payload, _ := json.Marshal(pentestProcessUpdate{SessionID: conv.ID, BlockID: "block", Sequence: sequence})
		if _, err := store.SaveWorkerEvent(ctx, conv.ID, "turn", 1, string(payload)); err != nil {
			t.Fatal(err)
		}
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/stream?lastEventId=2", nil)
	streamDistributedPentestSession(c, store, conv.ID)
	body := recorder.Body.String()
	if !strings.Contains(body, "event: session_snapshot\n") || !strings.Contains(body, "id: 3\n") || !strings.Contains(body, "event: done\n") {
		t.Fatalf("missing snapshot, resumed event or terminal frame: %s", body)
	}
	if strings.Contains(body, "id: 1\n") || strings.Contains(body, "id: 2\n") {
		t.Fatal("replayed events that the browser had already received")
	}
}
