package ai

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/lib/pentest"
	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/airun"
)

// Each subscriber keeps only the latest accumulated text per block. Slow readers
// cannot block the model, and a bounded overflow requests a durable resync.
type pentestProcessUpdate struct {
	SessionID string `json:"sessionId"`
	BlockID   string `json:"blockId"`
	Sequence  int    `json:"sequence"`
	RunID     string `json:"runId"`
	Item      gin.H  `json:"item"`
}
type pentestProcessSubscription struct {
	ready   chan struct{}
	pending map[string]pentestProcessUpdate
	resync  bool
}

var pentestProcessBus = struct {
	sync.Mutex
	subs map[string]map[*pentestProcessSubscription]struct{}
}{subs: make(map[string]map[*pentestProcessSubscription]struct{})}

func subscribePentestProcess(id string) (*pentestProcessSubscription, func()) {
	s := &pentestProcessSubscription{ready: make(chan struct{}, 1), pending: make(map[string]pentestProcessUpdate)}
	pentestProcessBus.Lock()
	if pentestProcessBus.subs[id] == nil {
		pentestProcessBus.subs[id] = make(map[*pentestProcessSubscription]struct{})
	}
	pentestProcessBus.subs[id][s] = struct{}{}
	pentestProcessBus.Unlock()
	return s, func() {
		pentestProcessBus.Lock()
		defer pentestProcessBus.Unlock()
		delete(pentestProcessBus.subs[id], s)
		if len(pentestProcessBus.subs[id]) == 0 {
			delete(pentestProcessBus.subs, id)
		}
	}
}
func publishPentestProcess(update pentestProcessUpdate) {
	pentestProcessBus.Lock()
	defer pentestProcessBus.Unlock()
	for s := range pentestProcessBus.subs[update.SessionID] {
		if len(s.pending) >= 128 {
			s.resync = true
			clear(s.pending)
		}
		s.pending[update.BlockID] = update
		select {
		case s.ready <- struct{}{}:
		default:
		}
	}
}
func (s *pentestProcessSubscription) drain() ([]pentestProcessUpdate, bool) {
	pentestProcessBus.Lock()
	defer pentestProcessBus.Unlock()
	updates := make([]pentestProcessUpdate, 0, len(s.pending))
	for _, update := range s.pending {
		updates = append(updates, update)
	}
	clear(s.pending)
	resync := s.resync
	s.resync = false
	sort.SliceStable(updates, func(i, j int) bool { return updates[i].Sequence < updates[j].Sequence })
	return updates, resync
}
func writePentestProcessUpdates(c *gin.Context, s *pentestProcessSubscription) {
	updates, resync := s.drain()
	if resync {
		writeSSE(c, "process_resync", gin.H{})
	}
	for _, update := range updates {
		writeSSE(c, "process_update", gin.H{"process": update})
	}
}

// Text delivery precedes persistence. Checkpoint the accumulated block at most
// every 300ms/1KiB; always persist its first update and terminal state.
func recordPentestAgentProgress(store *pentest.SQLStore, conversationID string) pentest.AgentProgressFunc {
	if store == nil || strings.TrimSpace(conversationID) == "" {
		return nil
	}
	type checkpoint struct {
		at        time.Time
		size      int
		createdAt time.Time
	}
	checkpoints := make(map[string]checkpoint)
	pending := make(map[string]pentest.Message)
	timers := make(map[string]*time.Timer)
	var mu sync.Mutex
	save := func(msg pentest.Message) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := store.SaveProcessMessage(ctx, msg); err != nil {
			log.Printf("[ai-process] save block %s: %v", msg.ID, err)
			return
		}
		if t := store.WorkerLease(); t != nil {
			var payload pentestProgressPayload
			if json.Unmarshal([]byte(msg.Content), &payload) == nil {
				update := pentestProcessUpdate{SessionID: conversationID, BlockID: msg.ID, RunID: payload.RunID, Sequence: payload.Sequence, Item: pentestProgressChatItem(msg)}
				body, _ := json.Marshal(update)
				if _, err := store.SaveWorkerEvent(ctx, conversationID, t.ID, t.Generation, string(body)); err != nil {
					log.Printf("[ai-process] save event: %v", err)
					return
				}
				publishPentestProcess(update)
			}
		}
		checkpoints[msg.ID] = checkpoint{at: time.Now(), size: len(msg.Content), createdAt: msg.CreatedAt}
	}
	return func(event pentest.AgentLoopEvent) {
		mu.Lock()
		defer mu.Unlock()
		payload := pentestProgressPayloadFromEvent(event)
		if strings.TrimSpace(payload.Label) == "" {
			return
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return
		}
		now := time.Now()
		cp := checkpoints[event.BlockID]
		if cp.createdAt.IsZero() {
			cp.createdAt = now
		}
		msg := pentest.Message{ID: event.BlockID, ConversationID: conversationID, Role: "progress", Content: string(body), CreatedAt: cp.createdAt}
		if event.BlockID != "" {
			if store.WorkerLease() == nil {
				publishPentestProcess(pentestProcessUpdate{SessionID: conversationID, BlockID: event.BlockID, RunID: event.RunID, Sequence: event.Sequence, Item: pentestProgressChatItem(msg)})
			}
			if event.Status == "running" && !cp.at.IsZero() && now.Sub(cp.at) < 300*time.Millisecond && len(body)-cp.size < 1024 {
				pending[event.BlockID] = msg
				if timers[event.BlockID] == nil {
					id := event.BlockID
					timers[id] = time.AfterFunc(300*time.Millisecond-now.Sub(cp.at), func() {
						mu.Lock()
						defer mu.Unlock()
						if latest, ok := pending[id]; ok {
							save(latest)
							delete(pending, id)
						}
						delete(timers, id)
					})
				}
				return
			}
			if timer := timers[event.BlockID]; timer != nil {
				timer.Stop()
				delete(timers, event.BlockID)
			}
			delete(pending, event.BlockID)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if event.BlockID != "" {
			save(msg)
		} else {
			_, _ = store.AddMessage(ctx, msg)
		}
	}
}

func streamPentestSession(c *gin.Context, store *pentest.SQLStore, conversationID string) {
	if !strings.HasPrefix(conversationID, pentest.OrchestrationRunPrefix) && (store.Distributed() || store.LocalWorkerEnabled()) {
		streamDistributedPentestSession(c, store, conversationID)
		return
	}

	sub, unsubscribe := subscribePentestProcess(conversationID)
	defer unsubscribe()
	snapshot, err := buildPentestLiveSessionSnapshot(c.Request.Context(), store, conversationID, "", "")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}
	interval := time.Duration(queryInt(c, "intervalMs", 2500)) * time.Millisecond
	if interval < 500*time.Millisecond {
		interval = 500 * time.Millisecond
	}
	if interval > 10*time.Second {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	writeSSE(c, "session_snapshot", gin.H{"session": snapshot, "updatedAt": time.Now().Format(time.RFC3339)})
	for {
		task, _ := snapshot["task"].(gin.H)
		status, _ := task["status"].(string)
		if !airun.ConversationStatusIsActive(status) {
			return
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-sub.ready:
			writePentestProcessUpdates(c, sub)
		case <-ticker.C:
			next, err := buildPentestLiveSessionSnapshot(c.Request.Context(), store, conversationID, "", "")
			if err != nil {
				writeSSE(c, "error", gin.H{"error": err.Error()})
				return
			}
			snapshot = next
			writeSSE(c, "session_snapshot", gin.H{"session": snapshot, "updatedAt": time.Now().Format(time.RFC3339)})
		}
	}
}

func StreamAIPentestSessionHandler(c *gin.Context) {
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	streamPentestSession(c, store, c.Param("id"))
}

type pentestAgentRunResult struct {
	snapshot gin.H
	err      error
}

func streamPentestAgentRun(c *gin.Context, cfg pentest.AgentLoopConfig, history []pentest.AgentMessage, providerID, model string) {
	if cfg.Store != nil && cfg.Store.Distributed() {
		streamDistributedPentestSession(c, cfg.Store, cfg.Conversation.ID)
		return
	}

	runCtx, cancel := context.WithCancel(context.Background())

	sub, unsubscribe := subscribePentestProcess(cfg.Conversation.ID)
	defer unsubscribe()

	resultCh := make(chan pentestAgentRunResult, 1)
	go func() {
		defer cancel()
		snapshot, err := runPentestAgentLoopAndSnapshot(runCtx, cfg, history, providerID, model)
		resultCh <- pentestAgentRunResult{snapshot: snapshot, err: err}
	}()

	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case result := <-resultCh:
			if result.snapshot != nil {
				writeSSE(c, "session_snapshot", gin.H{"session": result.snapshot, "updatedAt": time.Now().Format(time.RFC3339)})
				writeSSE(c, "done", gin.H{"session": result.snapshot, "updatedAt": time.Now().Format(time.RFC3339)})
				return
			}
			if result.err != nil {
				writeSSE(c, "error", gin.H{"error": result.err.Error()})
			}
			writeSSE(c, "done", gin.H{"session": nil, "updatedAt": time.Now().Format(time.RFC3339)})
			return
		case <-sub.ready:
			writePentestProcessUpdates(c, sub)
		case <-ticker.C:
			if cfg.Store == nil || cfg.Conversation.ID == "" {
				continue
			}
			snapshotCtx, snapshotCancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			snapshot, err := buildPentestLiveSessionSnapshot(snapshotCtx, cfg.Store, cfg.Conversation.ID, firstNonEmptyPentest(model, cfg.Model), providerID)
			snapshotCancel()
			if err == nil {
				writeSSE(c, "session_snapshot", gin.H{"session": snapshot, "updatedAt": time.Now().Format(time.RFC3339)})
			}
		}
	}
}
