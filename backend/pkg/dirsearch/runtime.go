package dirsearch

import (
	"context"
	"sync"
)

var (
	cancelMu  sync.Mutex
	cancelMap = make(map[string]context.CancelFunc)
)

// RegisterTaskCancel registers a cancel function for a running task.
func RegisterTaskCancel(taskID string, cancel context.CancelFunc) {
	if taskID == "" || cancel == nil {
		return
	}
	cancelMu.Lock()
	cancelMap[taskID] = cancel
	cancelMu.Unlock()
}

// UnregisterTaskCancel removes a cancel function for a task.
func UnregisterTaskCancel(taskID string) {
	if taskID == "" {
		return
	}
	cancelMu.Lock()
	delete(cancelMap, taskID)
	cancelMu.Unlock()
}

// CancelTask signals a running task to stop.
func CancelTask(taskID string) bool {
	if taskID == "" {
		return false
	}
	cancelMu.Lock()
	cancel, ok := cancelMap[taskID]
	cancelMu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}
