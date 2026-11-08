package airun

import (
	"context"
	"sync"
)

// ActiveRun 表示一次进程内的 pentest 对话执行，供 HTTP 层取消并等待退出。
type ActiveRun struct {
	Cancel context.CancelFunc
	Done   chan struct{}
}

var activeRuns sync.Map

// RegisterActiveRun 注册一次执行；会话已有执行时返回 false。
func RegisterActiveRun(conversationID string, run *ActiveRun) bool {
	_, loaded := activeRuns.LoadOrStore(conversationID, run)
	return !loaded
}

// LoadActiveRun 读取会话当前的执行句柄。
func LoadActiveRun(conversationID string) (*ActiveRun, bool) {
	value, ok := activeRuns.Load(conversationID)
	if !ok {
		return nil, false
	}
	run, ok := value.(*ActiveRun)
	return run, ok
}

// HasActiveRun 判断会话是否有执行中的 run（不触发取消）。
func HasActiveRun(conversationID string) bool {
	_, ok := activeRuns.Load(conversationID)
	return ok
}

// RemoveActiveRunIf 只在当前句柄仍是 run 时删除，避免误删后续注册的执行。
func RemoveActiveRunIf(conversationID string, run *ActiveRun) {
	activeRuns.CompareAndDelete(conversationID, run)
}

// StopActiveRun 取消并移除会话当前的执行；没有执行时返回 false。
func StopActiveRun(conversationID string) bool {
	run, ok := LoadActiveRun(conversationID)
	if !ok {
		return false
	}
	run.Cancel()
	RemoveActiveRunIf(conversationID, run)
	return true
}
