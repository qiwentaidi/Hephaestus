package webscan

import (
	"io"
	"strings"
	"sync"
	"time"
)

// 扫描阶段常量（checkpoint meta 中 stageEvents[].stage 的取值）
const (
	// StageInit 引擎初始化（指纹库加载完成）
	StageInit = "init"
	// StageProbe 主机存活探测阶段（SDK finger 阶段，含基础指纹匹配）
	StageProbe = "probe"
	// StageFingerprint 指纹扫描阶段（含目录探测、漏洞指纹匹配等子步骤）
	StageFingerprint = "fingerprint"
	// StageNuclei 漏洞扫描阶段（nuclei PoC 检测）
	StageNuclei = "nuclei"
	// StageComplete 扫描完成
	StageComplete = "complete"
)

// StageEvent 单条阶段事件，随 checkpoint meta 持久化，供前端悬停展示扫描时间线
type StageEvent struct {
	Time  time.Time `json:"time"`
	Stage string    `json:"stage"`
	Label string    `json:"label"`
}

// stageTracker 记录一次扫描任务的阶段事件流。
// SDK（fingers）内部的前置阶段（存活探测/目录探测/漏洞指纹匹配）不触发指纹回调，
// 此期间任务进度完全静止；通过拦截 SDK 日志识别阶段切换，实现两件事：
//  1. 阶段事件驱动进度下限推进（存活性探测 3% → 目录探测 8% → 漏洞指纹匹配 9%）
//  2. 阶段事件写入 checkpoint meta（stageEvents 字段），前端悬停进度条可查看时间线
type stageTracker struct {
	mu      sync.Mutex
	events  []StageEvent
	serial  uint64 // 事件版本号，每次变更递增，供刷新协程判断是否有新事件
	floor   int    // 进度下限（单调递增，阶段事件驱动）
	lastIdx map[string]int
}

func newStageTracker() *stageTracker {
	return &stageTracker{lastIdx: make(map[string]int)}
}

// maxStageEvents 时间线最多保留的事件条数，防止超长扫描任务的 checkpoint 膨胀
const maxStageEvents = 30

// Record 记录一条阶段事件：同阶段去重（刷新时间戳，位置不变），并单调提升进度下限
func (t *stageTracker) Record(stage, label string, minProgress int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	if idx, ok := t.lastIdx[stage]; ok {
		t.events[idx].Time = now
		t.events[idx].Label = label
	} else {
		t.events = append(t.events, StageEvent{Time: now, Stage: stage, Label: label})
		t.lastIdx[stage] = len(t.events) - 1
		if len(t.events) > maxStageEvents {
			t.events = t.events[len(t.events)-maxStageEvents:]
			t.rebuildIndex()
		}
	}
	t.serial++
	if minProgress > t.floor {
		t.floor = minProgress
	}
}

func (t *stageTracker) rebuildIndex() {
	t.lastIdx = make(map[string]int, len(t.events))
	for i, ev := range t.events {
		t.lastIdx[ev.Stage] = i
	}
}

// Events 返回当前事件快照（浅拷贝，调用方可安全序列化）
func (t *stageTracker) Events() []StageEvent {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]StageEvent, len(t.events))
	copy(out, t.events)
	return out
}

// Status 返回事件版本号与进度下限，供刷新协程做变更检测
func (t *stageTracker) Status() (serial uint64, floor int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.serial, t.floor
}

// stageLogWriter 包装 fingers SDK 的日志输出：
// 逐块匹配关键日志行识别阶段切换，其余内容原样转发给平台日志。
// 解决 SDK 前置阶段（回调未触发）期间进度条静止在初始值的问题。
//
// FIX(阶段事件失效): 原实现匹配 "Starting live detection" 等三条日志文本，
// 但 fingers SDK v0.2.6 源码中并不存在这些文本（实际是 cmd 命令行工具的输出），
// 导致 stageEvents 恒为空。现按 SDK 真实的阶段进度日志匹配：
//
//	[INF] progress stage=<stage> status=<running|done> processed=N total=M ...
//
// stage 取值（见 SDK api.go Scan 流程，deepScan 开启时依次执行）：
//
//	finger           存活探测 + 基础指纹（FingerScan，最先执行）
//	dynamic/headless 深度页面发现（discoverDynamicContextPaths，需浏览器）
//	discovered-page  发现页面指纹（scanDiscoveredPages）
//	host-token(-active) HostToken 路径探测（HostTokenPathProbe）
//	active           主动指纹扫描（ActiveFingerScan，即目录探测，最后执行）
type stageLogWriter struct {
	tracker *stageTracker
	sink    io.Writer
}

func (w *stageLogWriter) Write(p []byte) (int, error) {
	line := strings.ToLower(string(p))
	switch {
	// 引擎创建完成（NewScanner 加载指纹库后立即输出），时间线首事件，进度下限 2%
	case strings.Contains(line, "fingerprint repository fingersized"):
		w.tracker.Record(StageInit, "引擎初始化 · 指纹库加载", 2)
	case strings.Contains(line, "progress stage="):
		w.recordStage(line)
	}
	return w.sink.Write(p)
}

// recordStage 解析 stage 进度日志行并记录对应阶段事件。
// 匹配顺序无冲突：stage=host-token-active 需先于 stage=host-token 判断
// （不过两者归并为同一事件，顺序实际无影响）；status=done 与 running
// 记录同一 stage 事件（Record 同 stage 去重，仅刷新时间戳为最近活动时间）。
func (w *stageLogWriter) recordStage(line string) {
	switch {
	case strings.Contains(line, "stage=finger"):
		// SDK 第一步：存活探测 + 基础指纹库匹配，进度下限 3%
		w.tracker.Record(StageProbe, "存活探测 · 基础指纹", 3)
	case strings.Contains(line, "stage=dynamic"), strings.Contains(line, "stage=headless"):
		// 深扫：浏览器加载页面发现动态路径，进度下限 5%
		w.tracker.Record(StageFingerprint+":discovery", "指纹扫描 · 深度页面发现", 5)
	case strings.Contains(line, "stage=discovered-page"):
		// 深扫：对发现的页面做指纹匹配，进度下限 6%
		w.tracker.Record(StageFingerprint+":page", "指纹扫描 · 发现页面指纹", 6)
	case strings.Contains(line, "stage=host-token"):
		// 深扫：HostToken 路径探测，进度下限 7%
		w.tracker.Record(StageFingerprint+":hosttoken", "指纹扫描 · 路径探测", 7)
	case strings.Contains(line, "stage=active"):
		// 深扫最后一步：主动目录探测（敏感路径指纹），进度下限 8%
		w.tracker.Record(StageFingerprint+":active", "指纹扫描 · 主动目录探测", 8)
	}
}
