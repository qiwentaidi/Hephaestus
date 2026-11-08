// Package aibatch schedules durable, independent natural-language tasks.
package aibatch

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrUnacknowledged = errors.New("child execution has not acknowledged exit")

type Item struct {
	Generation            int        `json:"generation"`
	PreviousConversations []string   `json:"previousConversations,omitempty" gorm:"serializer:json;type:text"`
	ID                    string     `json:"id" gorm:"primaryKey"`
	QueueID               string     `json:"queueId" gorm:"index"`
	Position              int        `json:"position"`
	Instruction           string     `json:"instruction"`
	ConversationID        string     `json:"conversationId"`
	Status                string     `json:"status"`
	Attempts              int        `json:"attempts"`
	Error                 string     `json:"error"`
	Result                string     `json:"result"`
	Modules               []string   `json:"modules,omitempty" gorm:"serializer:json;type:text"`
	StartedAt             *time.Time `json:"startedAt"`
	CompletedAt           *time.Time `json:"completedAt"`
}

func (Item) TableName() string { return "ai_batch_items" }

type Queue struct {
	ID             string    `json:"id" gorm:"primaryKey"`
	ProjectID      string    `json:"projectId" gorm:"index"`
	Name           string    `json:"name"`
	ProviderID     string    `json:"providerId"`
	Model          string    `json:"model"`
	Concurrency    int       `json:"concurrency"`
	Status         string    `json:"status" gorm:"index"`
	PauseRequested bool      `json:"pauseRequested"`
	Error          string    `json:"error"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	Items          []Item    `json:"items" gorm:"foreignKey:QueueID"`
}

func (Queue) TableName() string { return "ai_batch_queues" }

// Checkpoint must persist external IDs before execution proceeds. A runner must
// return only after all its children exit; otherwise return ErrUnacknowledged.
type Checkpoint func(Item) error
type Runner func(context.Context, Queue, Item, Checkpoint) (string, error)
type execution struct {
	cancel context.CancelFunc
	done   chan struct{}
}
type Manager struct {
	mu     sync.Mutex
	db     *gorm.DB
	run    Runner
	active map[string]*execution
}

func New(db *gorm.DB, run Runner) (*Manager, error) {
	if err := db.AutoMigrate(&Queue{}, &Item{}); err != nil {
		return nil, err
	}
	return &Manager{db: db, run: run, active: map[string]*execution{}}, nil
}
func (m *Manager) load(id string) (Queue, error) {
	var q Queue
	err := m.db.Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("position ASC") }).First(&q, "id = ?", id).Error
	return q, err
}
func (m *Manager) save(q *Queue) error {
	q.UpdatedAt = time.Now()
	return m.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Items").Save(q).Error; err != nil {
			return err
		}
		for i := range q.Items {
			if err := tx.Save(&q.Items[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (m *Manager) Get(id string) (Queue, error) { m.mu.Lock(); defer m.mu.Unlock(); return m.load(id) }
func (m *Manager) List() ([]Queue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var qs []Queue
	err := m.db.Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("position ASC") }).Order("created_at DESC").Find(&qs).Error
	return qs, err
}
func (m *Manager) Create(q Queue, instructions []string) (Queue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.TrimSpace(q.Name) == "" || q.ProjectID == "" {
		return q, fmt.Errorf("name and project are required")
	}
	if q.Concurrency < 1 || q.Concurrency > 8 {
		return q, fmt.Errorf("concurrency must be 1–8")
	}
	q.ID = uuid.NewString()
	q.Status = "pending"
	q.PauseRequested = false
	q.CreatedAt = time.Now()
	q.Items = nil
	for _, s := range instructions {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if len(s) > 32000 {
			return q, fmt.Errorf("instruction too long")
		}
		q.Items = append(q.Items, Item{ID: uuid.NewString(), QueueID: q.ID, Position: len(q.Items), Instruction: s, Status: "pending", Generation: 1})
	}
	if len(q.Items) == 0 || len(q.Items) > 1000 {
		return q, fmt.Errorf("provide 1–1000 instructions")
	}
	return q, m.save(&q)
}
func (m *Manager) Start(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.active[id]; ok {
		return fmt.Errorf("queue is still executing or pausing")
	}
	q, err := m.load(id)
	if err != nil {
		return err
	}
	if q.Status == "pausing" {
		return fmt.Errorf("waiting for child exit: %s", q.Error)
	}
	ready := false
	for i := range q.Items {
		if q.Items[i].Status == "paused" {
			q.Items[i].Status = "pending"
		}
		ready = ready || q.Items[i].Status == "pending"
	}
	if !ready {
		return fmt.Errorf("no pending tasks; retry failed items separately")
	}
	return m.startLocked(q)
}

// startLocked persists the runnable queue before dispatch. The caller holds m.mu.
func (m *Manager) startLocked(q Queue) error {
	q.Status = "running"
	q.PauseRequested = false
	q.Error = ""
	if err := m.save(&q); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	e := &execution{cancel: cancel, done: make(chan struct{})}
	m.active[q.ID] = e
	go m.execute(ctx, q, e)
	return nil
}
func (m *Manager) Pause(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	q, err := m.load(id)
	if err != nil {
		return err
	}
	if q.Status == "paused" {
		return nil
	}
	if q.Status != "pending" && q.Status != "running" && q.Status != "pausing" {
		return fmt.Errorf("queue cannot be paused in %s", q.Status)
	}
	q.PauseRequested = true
	q.Status = "pausing"
	if m.active[id] == nil && q.Status == "pausing" {
		unresolved := false
		for _, i := range q.Items {
			unresolved = unresolved || i.Status == "running" || i.Status == "pausing"
		}
		if !unresolved {
			q.Status = "paused"
		}
	}
	if err = m.save(&q); err != nil {
		return err
	}
	if e := m.active[id]; e != nil {
		e.cancel()
	}
	return nil
}
func (m *Manager) checkpoint(id string, item Item) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	q, err := m.load(id)
	if err != nil {
		return err
	}
	for i := range q.Items {
		if q.Items[i].ID == item.ID {
			q.Items[i].ConversationID = item.ConversationID
			q.Items[i].Modules = append([]string(nil), item.Modules...)
			return m.save(&q)
		}
	}
	return fmt.Errorf("item not found")
}
func (m *Manager) claim(id string) (Queue, Item, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	q, err := m.load(id)
	if err != nil || q.PauseRequested || q.Status != "running" {
		return q, Item{}, false
	}
	for i := range q.Items {
		if q.Items[i].Status == "pending" {
			now := time.Now()
			q.Items[i].Status = "running"
			q.Items[i].StartedAt = &now
			q.Items[i].CompletedAt = nil
			q.Items[i].Attempts++
			q.Items[i].Error = ""
			if err = m.save(&q); err != nil {
				return q, Item{}, false
			}
			return q, q.Items[i], true
		}
	}
	return q, Item{}, false
}
func (m *Manager) execute(ctx context.Context, q Queue, e *execution) {
	var wg sync.WaitGroup
	for n := 0; n < q.Concurrency; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if ctx.Err() != nil {
					return
				}
				current, item, ok := m.claim(q.ID)
				if !ok {
					return
				}
				result, err := m.runSafely(ctx, current, item, func(next Item) error { return m.checkpoint(q.ID, next) })
				m.mu.Lock()
				latest, loadErr := m.load(q.ID)
				if loadErr == nil {
					for i := range latest.Items {
						if latest.Items[i].ID == item.ID {
							it := &latest.Items[i]
							now := time.Now()
							it.CompletedAt = &now
							it.Result = result
							it.Status = "completed"
							if err != nil {
								it.Error = err.Error()
								it.Status = "failed"
							}
							if errors.Is(err, ErrUnacknowledged) {
								it.Status = "pausing"
								it.CompletedAt = nil
								latest.PauseRequested = true
								latest.Status = "pausing"
								latest.Error = err.Error()
								e.cancel()
							} else if ctx.Err() != nil && latest.PauseRequested {
								it.Status = "paused"
								it.Error = ""
								it.CompletedAt = nil
							}
						}
					}
					if saveErr := m.save(&latest); saveErr != nil {
						e.cancel()
					}
				}
				m.mu.Unlock()
				if loadErr != nil {
					return
				}
			}
		}()
	}
	wg.Wait()
	e.cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	defer close(e.done)
	defer delete(m.active, q.ID)
	latest, err := m.load(q.ID)
	if err != nil {
		return
	}
	unresolved := false
	failed := false
	pending := false
	paused := false
	for _, i := range latest.Items {
		unresolved = unresolved || i.Status == "pausing" || i.Status == "running"
		failed = failed || i.Status == "failed"
		pending = pending || i.Status == "pending"
		paused = paused || i.Status == "paused"
	}
	switch {
	case unresolved:
		latest.Status = "pausing"
		latest.PauseRequested = true
	case latest.PauseRequested:
		latest.Status = "paused"
	case pending:
		latest.Status = "paused"
		latest.PauseRequested = true
		latest.Error = "Dispatch interrupted; continue to retry pending tasks"
	case failed:
		latest.Status = "completed_with_errors"
	case paused:
		latest.Status = "paused"
		latest.PauseRequested = true
	default:
		latest.Status = "completed"
	}
	if err = m.save(&latest); err != nil {
		log.Printf("AI batch queue %s final state could not be persisted: %v", q.ID, err)
	}
}
func (m *Manager) runSafely(ctx context.Context, q Queue, i Item, save Checkpoint) (result string, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("%w: runner panic: %v", ErrUnacknowledged, p)
		}
	}()
	return m.run(ctx, q, i, save)
}

// Recover runs only at process startup, before interrupted scan recovery. It
// never auto-enqueues work. The callback reconciles persisted external modules.
func (m *Manager) Recover(reconcile func(Item) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var qs []Queue
	if err := m.db.Preload("Items").Where("pause_requested = ? OR status IN ?", true, []string{"running", "pausing"}).Find(&qs).Error; err != nil {
		return err
	}
	return m.reconcileQueues(qs, reconcile)
}

// ReconcilePaused finishes only previously requested pauses whose runner has
// exited. It never stops a running queue or launches new work.
func (m *Manager) ReconcilePaused(reconcile func(Item) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var queues []Queue
	if err := m.db.Preload("Items").Where("pause_requested=true AND status='pausing'").Find(&queues).Error; err != nil {
		return err
	}
	ready := queues[:0]
	for _, q := range queues {
		if m.active[q.ID] == nil {
			ready = append(ready, q)
		}
	}
	return m.reconcileQueues(ready, reconcile)
}

func (m *Manager) reconcileQueues(qs []Queue, reconcile func(Item) error) error {
	for n := range qs {
		q := &qs[n]
		q.PauseRequested = true
		q.Status = "pausing"
		if err := m.save(q); err != nil {
			return err
		}
		unresolved := false
		for i := range q.Items {
			it := &q.Items[i]
			if it.Status == "running" || it.Status == "pausing" || it.Status == "paused" {
				if reconcile != nil {
					if err := reconcile(*it); err != nil {
						unresolved = true
						it.Status = "pausing"
						it.Error = err.Error()
						q.Error = err.Error()
						continue
					}
				}
				it.Status = "paused"
				it.Error = ""
				it.CompletedAt = nil
			}
		}
		if !unresolved {
			q.Status = "paused"
			q.Error = ""
		}
		if err := m.save(q); err != nil {
			return err
		}
	}
	return nil
}

// RetryFailed atomically requeues failed items and starts execution. Completed
// and paused items retain their state, conversations and evidence.
func (m *Manager) RetryFailed(id string) (int, error) {
	return m.retry(id, "", true)
}
func (m *Manager) Retry(id, itemID string) error {
	_, err := m.retry(id, itemID, false)
	return err
}
func (m *Manager) retry(id, itemID string, all bool) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active[id] != nil {
		return 0, fmt.Errorf("pause queue before retrying")
	}
	q, err := m.load(id)
	if err != nil {
		return 0, err
	}
	if q.Status == "pausing" || q.Status == "running" {
		return 0, fmt.Errorf("waiting for child exit")
	}
	for _, item := range q.Items {
		if item.Status == "running" || item.Status == "pausing" {
			return 0, fmt.Errorf("waiting for child exit")
		}
	}
	count := 0
	for i := range q.Items {
		item := &q.Items[i]
		if !all && item.ID != itemID {
			continue
		}
		if item.Status != "failed" {
			if !all {
				return 0, fmt.Errorf("only failed items can be retried")
			}
			continue
		}
		item.Status = "pending"
		// Retry continues the same conversation and native scan ownership. Keep
		// the generation marker so scans created before a checkpoint are still
		// discovered; claim increments Attempts when execution actually resumes.
		item.Error = ""
		count++
	}
	if count == 0 {
		if !all {
			return 0, fmt.Errorf("item not found")
		}
		return 0, nil
	}
	if all {
		if err := m.startLocked(q); err != nil {
			return 0, err
		}
		return count, nil
	}
	q.Status = "paused"
	q.PauseRequested = true
	q.Error = ""
	if err := m.save(&q); err != nil {
		return 0, err
	}
	return count, nil
}

// RemoveConversation deletes every queue item that references the given
// conversation so the conversation itself can be deleted. Affected active
// queues are cancelled first so in-flight runners stop writing to the
// conversation; queues left without items are removed as well. Runners
// finish asynchronously: every queue load+save critical section holds m.mu,
// so a finishing worker cannot resurrect rows deleted here.
func (m *Manager) RemoveConversation(conversationID string) error {
	m.mu.Lock()
	var queueIDs []string
	err := m.db.Model(&Item{}).Where("conversation_id = ?", conversationID).Distinct().Pluck("queue_id", &queueIDs).Error
	m.mu.Unlock()
	if err != nil {
		return err
	}
	if len(queueIDs) == 0 {
		return nil
	}
	for _, id := range queueIDs {
		// Pause cancels the execution context of an active queue; terminal
		// queues simply report an ignorable error.
		_ = m.Pause(id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("conversation_id = ?", conversationID).Delete(&Item{}).Error; err != nil {
			return err
		}
		for _, id := range queueIDs {
			var remaining int64
			if err := tx.Model(&Item{}).Where("queue_id = ?", id).Count(&remaining).Error; err != nil {
				return err
			}
			if remaining > 0 {
				continue
			}
			if err := tx.Where("id = ?", id).Delete(&Queue{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active[id] != nil {
		return fmt.Errorf("pause queue before deleting")
	}
	q, err := m.load(id)
	if err != nil {
		return err
	}
	if q.Status == "pausing" || q.Status == "running" {
		return fmt.Errorf("queue is still stopping")
	}
	return m.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("queue_id = ?", id).Delete(&Item{}).Error; err != nil {
			return err
		}
		return tx.Delete(&q).Error
	})
}

// Shutdown persists every active pause before waiting; the caller's timeout
// leaves pausing records intact for startup reconciliation.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	ids := make([]string, 0, len(m.active))
	waits := make([]<-chan struct{}, 0, len(m.active))
	for id, e := range m.active {
		ids = append(ids, id)
		waits = append(waits, e.done)
	}
	m.mu.Unlock()
	for _, id := range ids {
		if err := m.Pause(id); err != nil {
			return err
		}
	}
	for _, done := range waits {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
