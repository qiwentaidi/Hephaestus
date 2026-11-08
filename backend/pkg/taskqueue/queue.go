package taskqueue

import (
	"context"
	"sync"

	"hephaestus-backend/pkg/logger"
)

// TaskFunc 任务执行函数类型
type TaskFunc func() error

// Task 任务结构
type Task struct {
	ID     string     // 任务ID
	Name   string     // 任务名称（可选）
	Fn     TaskFunc   // 任务执行函数
	DoneCh chan error // 任务完成通知channel（可选）
}

// Queue 任务队列管理器
type Queue struct {
	name          string
	maxConcurrent int            // 最大并发数
	taskCh        chan *Task     // 任务channel
	wg            sync.WaitGroup // 等待组
	ctx           context.Context
	cancel        context.CancelFunc
	mu            sync.Mutex
	runningCount  int // 当前运行中的任务数量
	useLimiter    bool
}

var (
	globalQueue           *Queue
	companyInfoQueue      *Queue
	spaceExportQueue      *Queue
	globalScanLimiter     *ScanLimiter
	globalQueueOnce       sync.Once
	companyInfoQueueOnce  sync.Once
	spaceExportQueueOnce  sync.Once
	globalScanLimiterOnce sync.Once
)

// ScanLimiter 平台扫描任务共享限流器
type ScanLimiter struct {
	maxConcurrent int
	permits       chan struct{}
}

// InitGlobalScanLimiter 初始化全局扫描限流器
func InitGlobalScanLimiter(maxConcurrent int) {
	globalScanLimiterOnce.Do(func() {
		if maxConcurrent <= 0 {
			maxConcurrent = 3
		}
		globalScanLimiter = &ScanLimiter{
			maxConcurrent: maxConcurrent,
			permits:       make(chan struct{}, maxConcurrent),
		}
	})
}

// GetGlobalScanLimiter 获取全局扫描限流器
func GetGlobalScanLimiter() *ScanLimiter {
	if globalScanLimiter == nil {
		InitGlobalScanLimiter(3)
	}
	return globalScanLimiter
}

// Acquire 获取一个扫描执行名额
func (l *ScanLimiter) Acquire(ctx context.Context) error {
	select {
	case l.permits <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release 释放一个扫描执行名额
func (l *ScanLimiter) Release() {
	select {
	case <-l.permits:
	default:
	}
}

// GetInUseCount 获取当前占用的名额数
func (l *ScanLimiter) GetInUseCount() int {
	return len(l.permits)
}

// GetMaxConcurrent 获取共享并发上限
func (l *ScanLimiter) GetMaxConcurrent() int {
	return l.maxConcurrent
}

// InitGlobalQueue 初始化全局任务队列
func InitGlobalQueue(maxConcurrent int) {
	globalQueueOnce.Do(func() {
		InitGlobalScanLimiter(maxConcurrent)
		globalQueue = NewNamedQueue("扫描任务", maxConcurrent)
		globalQueue.useLimiter = true
		globalQueue.Start()
	})
}

// GetGlobalQueue 获取全局任务队列
func GetGlobalQueue() *Queue {
	if globalQueue == nil {
		// 默认最大并发数为3
		InitGlobalQueue(3)
	}
	return globalQueue
}

// InitCompanyInfoQueue 初始化企业信息收集任务队列（最大并发数为1）
func InitCompanyInfoQueue() {
	companyInfoQueueOnce.Do(func() {
		companyInfoQueue = NewNamedQueue("企业信息收集", 1)
		companyInfoQueue.Start()
	})
}

// GetCompanyInfoQueue 获取企业信息收集任务队列
func GetCompanyInfoQueue() *Queue {
	if companyInfoQueue == nil {
		InitCompanyInfoQueue()
	}
	return companyInfoQueue
}

// InitSpaceExportQueue 初始化空间引擎导出任务队列（严格串行）
func InitSpaceExportQueue() {
	spaceExportQueueOnce.Do(func() {
		spaceExportQueue = NewNamedQueue("空间引擎导出", 1)
		spaceExportQueue.Start()
	})
}

// GetSpaceExportQueue 获取空间引擎导出任务队列
func GetSpaceExportQueue() *Queue {
	if spaceExportQueue == nil {
		InitSpaceExportQueue()
	}
	return spaceExportQueue
}

// NewQueue 创建新的任务队列
func NewQueue(maxConcurrent int) *Queue {
	return NewNamedQueue("任务", maxConcurrent)
}

// NewNamedQueue 创建带名称的新任务队列
func NewNamedQueue(name string, maxConcurrent int) *Queue {
	if maxConcurrent <= 0 {
		maxConcurrent = 3 // 默认值
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Queue{
		name:          name,
		maxConcurrent: maxConcurrent,
		taskCh:        make(chan *Task, 100), // 缓冲100个任务
		ctx:           ctx,
		cancel:        cancel,
		runningCount:  0,
	}
}

// Start 启动队列管理器
func (q *Queue) Start() {
	// 启动worker goroutines
	for i := 0; i < q.maxConcurrent; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}
}

// Stop 停止队列管理器
func (q *Queue) Stop() {
	logger.Info("正在停止%s队列...", q.name)
	q.cancel()
	close(q.taskCh)
	q.wg.Wait()
	logger.Info("%s队列已停止", q.name)
}

// Submit 提交任务到队列
func (q *Queue) Submit(task *Task) error {
	select {
	case q.taskCh <- task:
		logger.Info("%s队列已加入任务: %s (当前队列长度: %d)", q.name, task.ID, len(q.taskCh))
		return nil
	case <-q.ctx.Done():
		return q.ctx.Err()
	}
}

// worker 工作协程
func (q *Queue) worker(id int) {
	defer q.wg.Done()

	for {
		select {
		case task, ok := <-q.taskCh:
			if !ok {
				return
			}

			// 更新运行计数
			q.mu.Lock()
			q.runningCount++
			runningCount := q.runningCount
			q.mu.Unlock()

			logger.Info("%s队列开始执行任务: %s (当前运行中: %d/%d)",
				q.name, task.ID, runningCount, q.maxConcurrent)

			var err error
			func() {
				if q.useLimiter {
					// 全局扫描共享额度：普通任务和组合任务模块共用同一上限
					if acquireErr := GetGlobalScanLimiter().Acquire(q.ctx); acquireErr != nil {
						err = acquireErr
						return
					}
					defer GetGlobalScanLimiter().Release()
				}
				err = task.Fn()
			}()

			// 更新运行计数
			q.mu.Lock()
			q.runningCount--
			q.mu.Unlock()

			// 通知任务完成
			if task.DoneCh != nil {
				select {
				case task.DoneCh <- err:
				default:
				}
			}

			if err != nil {
				logger.Warning("%s队列任务执行失败: %s, 错误: %v", q.name, task.ID, err)
			} else {
				logger.Info("%s队列任务执行完成: %s (当前运行中: %d/%d)",
					q.name, task.ID, q.runningCount, q.maxConcurrent)
			}

		case <-q.ctx.Done():
			return
		}
	}
}

// GetRunningCount 获取当前运行中的任务数量
func (q *Queue) GetRunningCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.runningCount
}

// GetQueueLength 获取队列中等待的任务数量
func (q *Queue) GetQueueLength() int {
	return len(q.taskCh)
}

// GetMaxConcurrent 获取最大并发数
func (q *Queue) GetMaxConcurrent() int {
	return q.maxConcurrent
}
