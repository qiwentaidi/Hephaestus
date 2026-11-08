package ai

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
)

func TestAIProcessMessageStreamPublishesBeforeModelFinishes(t *testing.T) {
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
	project, _ := store.UpsertProject(ctx, pentest.Project{Name: "POST fixture"})
	conv, _ := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Status: "running"})
	_, _ = store.AddMessage(ctx, pentest.Message{ConversationID: conv.ID, Role: "user", Content: "Explain fixture"})
	finish := make(chan struct{})
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"即时分析文本\"}}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-finish:
		case <-r.Context().Done():
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer model.Close()
	r := gin.New()
	r.POST("/message", func(c *gin.Context) {
		streamPentestAgentRun(c, pentest.AgentLoopConfig{BaseURL: model.URL, APIKey: "fixture", Model: "fixture", Store: store, Project: project, Conversation: conv, MaxIterations: 1, Progress: recordPentestAgentProgress(store, conv.ID)}, []pentest.AgentMessage{{Role: "user", Content: "Explain fixture"}}, "", "fixture")
	})
	server := httptest.NewServer(r)
	defer server.Close()
	client := server.Client()
	client.Timeout = 3 * time.Second
	response, err := client.Post(server.URL+"/message", "application/json", nil)
	if err != nil {
		close(finish)
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		close(finish)
		t.Fatal(err)
	}
	if strings.TrimSpace(line) != "event: process_update" {
		close(finish)
		t.Fatalf("first event was a delayed snapshot: %s", line)
	}
	line, err = reader.ReadString('\n')
	close(finish)
	if err != nil || !strings.Contains(line, "即时分析文本") {
		t.Fatalf("missing live content: %s %v", line, err)
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		t.Fatal(err)
	}
}

func TestAIProcessMessageStreamDisconnectDoesNotCancelRun(t *testing.T) {
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
	project, _ := store.UpsertProject(ctx, pentest.Project{Name: "disconnect-fixture"})
	conv, _ := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Status: "running"})
	_, _ = store.AddMessage(ctx, pentest.Message{ConversationID: conv.ID, Role: "user", Content: "Explain fixture"})
	finish := make(chan struct{})
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"先返回一段\"}}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-finish:
		case <-r.Context().Done():
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer model.Close()
	r := gin.New()
	r.POST("/message", func(c *gin.Context) {
		streamPentestAgentRun(c, pentest.AgentLoopConfig{BaseURL: model.URL, APIKey: "fixture", Model: "fixture", Store: store, Project: project, Conversation: conv, MaxIterations: 1, Progress: recordPentestAgentProgress(store, conv.ID)}, []pentest.AgentMessage{{Role: "user", Content: "Explain fixture"}}, "", "fixture")
	})
	server := httptest.NewServer(r)
	defer server.Close()
	response, err := server.Client().Post(server.URL+"/message", "application/json", nil)
	if err != nil {
		close(finish)
		t.Fatal(err)
	}
	reader := bufio.NewReader(response.Body)
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			close(finish)
			t.Fatal(readErr)
		}
		if strings.Contains(line, "先返回一段") {
			break
		}
	}
	_ = response.Body.Close()
	close(finish)

	deadline := time.Now().Add(2 * time.Second)
	for {
		latest, err := store.GetConversation(ctx, conv.ID)
		if err != nil {
			t.Fatal(err)
		}
		if latest.Status == "completed" {
			return
		}
		if latest.Status == "cancelled" {
			t.Fatal("client stream disconnect cancelled the background run")
		}
		if time.Now().After(deadline) {
			t.Fatalf("conversation status = %q, want completed", latest.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAIProcessLiveDeliveryAndReconnect(t *testing.T) {
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
	project, _ := store.UpsertProject(ctx, pentest.Project{Name: "stream-fixture"})
	conv, _ := store.UpsertConversation(ctx, pentest.Conversation{ProjectID: project.ID, Status: "running"})
	_, _ = store.AddMessage(ctx, pentest.Message{ConversationID: conv.ID, Role: "user", Content: "fixture"})
	record := recordPentestAgentProgress(store, conv.ID)
	r := gin.New()
	r.GET("/stream", func(c *gin.Context) { streamPentestSession(c, store, conv.ID) })
	server := httptest.NewServer(r)
	defer server.Close()
	connect := func() (<-chan string, func()) {
		response, err := server.Client().Get(server.URL + "/stream?intervalMs=10000")
		if err != nil {
			t.Fatal(err)
		}
		lines := make(chan string, 16)
		go func() {
			defer close(lines)
			scanner := bufio.NewScanner(response.Body)
			scanner.Buffer(make([]byte, 4096), 2<<20)
			for scanner.Scan() {
				if strings.HasPrefix(scanner.Text(), "data:") {
					lines <- scanner.Text()
				}
			}
		}()
		return lines, func() { response.Body.Close() }
	}
	read := func(lines <-chan string, contains string) string {
		t.Helper()
		select {
		case line := <-lines:
			if !strings.Contains(line, contains) {
				t.Fatalf("missing %q: %s", contains, line)
			}
			return line
		case <-time.After(time.Second):
			t.Fatal("live text waited for the 10s snapshot ticker")
		}
		return ""
	}
	lines, closeStream := connect()
	read(lines, `"session"`)
	for i := 1; i <= 4; i++ {
		text := strings.Repeat("分片", i)
		record(pentest.AgentLoopEvent{BlockID: "live", RunID: "run", Sequence: i, Type: "assistant_commentary", Message: text, Status: "running"})
		read(lines, text)
	}
	closeStream()
	// No further model chunks arrive: the throttled checkpoint must still flush.
	deadline := time.Now().Add(time.Second)
	for {
		messages, _ := store.ListMessages(ctx, conv.ID, -1)
		found := false
		for _, message := range messages {
			var p pentestProgressPayload
			_ = json.Unmarshal([]byte(message.Content), &p)
			if message.ID == "live" && p.Sequence == 4 {
				found = true
			}
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("idle partial checkpoint was not persisted")
		}
		time.Sleep(10 * time.Millisecond)
	}
	record(pentest.AgentLoopEvent{BlockID: "live", RunID: "run", Sequence: 5, Type: "assistant_commentary", Message: "取消时保留完整文本", Status: "cancelled"})
	lines, closeStream = connect()
	defer closeStream()
	read(lines, "取消时保留完整文本")
	messages, _ := store.ListMessages(ctx, conv.ID, -1)
	var payload pentestProgressPayload
	for _, message := range messages {
		if message.ID == "live" {
			_ = json.Unmarshal([]byte(message.Content), &payload)
		}
	}
	if payload.Sequence != 5 || payload.Status != "cancelled" {
		t.Fatalf("terminal checkpoint lost: %+v", payload)
	}
}

func TestAIProcessSubscribersCoalesceAndIsolate(t *testing.T) {
	a, stopA := subscribePentestProcess("a")
	defer stopA()
	b, stopB := subscribePentestProcess("b")
	defer stopB()
	for i := 1; i <= 1000; i++ {
		publishPentestProcess(pentestProcessUpdate{SessionID: "a", BlockID: "block", Sequence: i})
	}
	updates, resync := a.drain()
	if resync || len(updates) != 1 || updates[0].Sequence != 1000 {
		t.Fatal("latest block not retained")
	}
	if updates, _ := b.drain(); len(updates) != 0 {
		t.Fatal("cross-session leak")
	}
	for i := 0; i < 150; i++ {
		publishPentestProcess(pentestProcessUpdate{SessionID: "a", BlockID: fmt.Sprint(i)})
	}
	if updates, resync := a.drain(); !resync || len(updates) > 128 {
		t.Fatal("unbounded subscriber backlog")
	}
	stopA()
	publishPentestProcess(pentestProcessUpdate{SessionID: "a", BlockID: "after-close"})
	if updates, _ := a.drain(); len(updates) != 0 {
		t.Fatal("subscription leaked")
	}
}
