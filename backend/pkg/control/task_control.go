// Package control 用于控制任务的启停模块
package control

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// TaskController 管理任务的上下文控制
type TaskController struct {
	ctxMap     map[string]context.CancelFunc // 任务ID -> 取消函数
	ctxMapLock sync.RWMutex
}

const subTaskKeySep = "::"

var (
	globalController *TaskController
	once             sync.Once
)

// GetTaskController 获取全局任务控制器（单例）
func GetTaskController() *TaskController {
	once.Do(func() {
		globalController = &TaskController{
			ctxMap: make(map[string]context.CancelFunc),
		}
	})
	return globalController
}

// CreateTaskContext 为任务创建上下文
// taskID: 任务的唯一标识符
// 返回: context 和 cancel 函数
func (tc *TaskController) CreateTaskContext(taskID string) (context.Context, context.CancelFunc) {
	tc.ctxMapLock.Lock()
	// 如果任务已存在，先摘除旧的 cancel，解锁后再执行取消（避免死锁）
	oldCancel, exists := tc.ctxMap[taskID]
	if exists {
		delete(tc.ctxMap, taskID)
	}
	tc.ctxMapLock.Unlock()

	if exists {
		oldCancel()
	}

	// 创建新的 context
	ctx, cancel := context.WithCancel(context.Background())

	// 包装 cancel 函数，确保从 map 中删除
	wrappedCancel := func() {
		cancel()
		tc.ctxMapLock.Lock()
		delete(tc.ctxMap, taskID)
		tc.ctxMapLock.Unlock()
	}

	tc.ctxMapLock.Lock()
	tc.ctxMap[taskID] = wrappedCancel
	tc.ctxMapLock.Unlock()
	return ctx, wrappedCancel
}

// CreateSubTaskContext 为同一父任务下的子任务创建独立上下文，不会取消同父任务的其他子任务
// taskID: 父任务ID
// subTaskID: 子任务唯一标识（如模块名/阶段）
func (tc *TaskController) CreateSubTaskContext(taskID, subTaskID string) (context.Context, context.CancelFunc) {
	key := taskID + subTaskKeySep + subTaskID

	tc.ctxMapLock.Lock()
	// 仅替换同一子任务的旧 context，不影响同父任务的其他子任务
	oldCancel, exists := tc.ctxMap[key]
	if exists {
		delete(tc.ctxMap, key)
	}
	tc.ctxMapLock.Unlock()

	if exists {
		oldCancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	wrappedCancel := func() {
		cancel()
		tc.ctxMapLock.Lock()
		delete(tc.ctxMap, key)
		tc.ctxMapLock.Unlock()
	}

	tc.ctxMapLock.Lock()
	tc.ctxMap[key] = wrappedCancel
	tc.ctxMapLock.Unlock()
	return ctx, wrappedCancel
}

// CancelTask 取消指定任务
func (tc *TaskController) CancelTask(taskID string) error {
	tc.ctxMapLock.Lock()
	cancels := make([]context.CancelFunc, 0, 4)

	// 取消主任务 context（如存在）
	if cancel, exists := tc.ctxMap[taskID]; exists {
		cancels = append(cancels, cancel)
	}

	// 取消该父任务下的所有子任务 context
	prefix := taskID + subTaskKeySep
	for key, cancel := range tc.ctxMap {
		if strings.HasPrefix(key, prefix) {
			cancels = append(cancels, cancel)
		}
	}
	tc.ctxMapLock.Unlock()

	if len(cancels) == 0 {
		return fmt.Errorf("任务 %s 不存在或已完成", taskID)
	}

	for _, cancel := range cancels {
		cancel()
	}
	return nil
}

// IsTaskRunning 检查任务是否正在运行
func (tc *TaskController) IsTaskRunning(taskID string) bool {
	tc.ctxMapLock.RLock()
	defer tc.ctxMapLock.RUnlock()

	if _, exists := tc.ctxMap[taskID]; exists {
		return true
	}

	prefix := taskID + subTaskKeySep
	for key := range tc.ctxMap {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// GetRunningTaskCount 获取正在运行的任务数量
func (tc *TaskController) GetRunningTaskCount() int {
	tc.ctxMapLock.RLock()
	defer tc.ctxMapLock.RUnlock()
	return len(tc.ctxMap)
}

// CancelAllTasks 取消所有正在运行的任务
func (tc *TaskController) CancelAllTasks() {
	tc.ctxMapLock.Lock()
	cancels := make([]context.CancelFunc, 0, len(tc.ctxMap))
	for taskID, cancel := range tc.ctxMap {
		cancels = append(cancels, cancel)
		delete(tc.ctxMap, taskID)
	}
	tc.ctxMapLock.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
}
