package webscan

import (
	"bytes"
	"testing"
	"time"
)

// 验证 stageLogWriter 能匹配 fingers SDK v0.2.6 真实输出的阶段日志
// （格式见 SDK progress.go: [INF] progress stage=<x> status=<y> ...）
func TestStageLogWriterMatchesSDKProgressLogs(t *testing.T) {
	cases := []struct {
		name      string
		logLine   string
		wantStage string
		wantFloor int
	}{
		{
			name:      "引擎初始化日志",
			logLine:   "[INF] fingerprint repository fingersized\n",
			wantStage: StageInit,
			wantFloor: 2,
		},
		{
			name:      "基础指纹阶段运行中",
			logLine:   "[INF] progress stage=finger status=running processed=10 total=100 requested=10 failed=0 matched=3 skipped=0 elapsed=30s rate=0.3/s\n",
			wantStage: StageProbe,
			wantFloor: 3,
		},
		{
			name:      "基础指纹阶段完成",
			logLine:   "[INF] progress stage=finger status=done processed=100 total=100 requested=100 failed=1 matched=20 skipped=0 elapsed=1m10s rate=1.4/s\n",
			wantStage: StageProbe,
			wantFloor: 3,
		},
		{
			name:      "深度页面发现(dynamic)",
			logLine:   "[INF] progress stage=dynamic status=running processed=5 total=50 requested=5 failed=0 matched=0 skipped=0 elapsed=30s rate=0.2/s\n",
			wantStage: StageFingerprint + ":discovery",
			wantFloor: 5,
		},
		{
			name:      "深度页面发现(headless)",
			logLine:   "[INF] progress stage=headless status=done processed=50 total=50 requested=50 failed=2 matched=5 skipped=0 elapsed=1m0s rate=0.8/s\n",
			wantStage: StageFingerprint + ":discovery",
			wantFloor: 5,
		},
		{
			name:      "发现页面指纹",
			logLine:   "[INF] progress stage=discovered-page status=running processed=3 total=30 requested=3 failed=0 matched=1 skipped=0 elapsed=30s rate=0.1/s\n",
			wantStage: StageFingerprint + ":page",
			wantFloor: 6,
		},
		{
			name:      "HostToken路径探测",
			logLine:   "[INF] progress stage=host-token status=running processed=2 total=20 requested=2 failed=0 matched=0 skipped=0 elapsed=30s rate=0.1/s\n",
			wantStage: StageFingerprint + ":hosttoken",
			wantFloor: 7,
		},
		{
			name:      "HostToken主动探测(host-token-active)",
			logLine:   "[INF] progress stage=host-token-active status=done processed=20 total=20 requested=20 failed=1 matched=2 skipped=0 elapsed=45s rate=0.4/s\n",
			wantStage: StageFingerprint + ":hosttoken",
			wantFloor: 7,
		},
		{
			name:      "主动目录探测",
			logLine:   "[INF] progress stage=active status=running processed=100 total=500 requested=120 failed=3 matched=8 skipped=0 elapsed=30s rate=3.3/s\n",
			wantStage: StageFingerprint + ":active",
			wantFloor: 8,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tracker := newStageTracker()
			var sink bytes.Buffer
			w := &stageLogWriter{tracker: tracker, sink: &sink}

			if _, err := w.Write([]byte(tc.logLine)); err != nil {
				t.Fatalf("Write() error = %v", err)
			}

			events := tracker.Events()
			if len(events) == 0 {
				t.Fatalf("期望记录阶段事件 %q，实际无事件", tc.wantStage)
			}
			last := events[len(events)-1]
			if last.Stage != tc.wantStage {
				t.Fatalf("期望阶段 %q，实际 %q", tc.wantStage, last.Stage)
			}
			if last.Time.IsZero() {
				t.Fatal("阶段事件时间戳不应为零值")
			}
			_, floor := tracker.Status()
			if floor != tc.wantFloor {
				t.Fatalf("期望进度下限 %d，实际 %d", tc.wantFloor, floor)
			}
			// 日志应原样转发到 sink，不吞内容
			if sink.String() != tc.logLine {
				t.Fatalf("日志应原样转发，实际 sink=%q", sink.String())
			}
		})
	}
}

// 验证普通结果输出（非阶段日志）不产生事件、不影响进度下限
func TestStageLogWriterIgnoresNonStageLogs(t *testing.T) {
	tracker := newStageTracker()
	var sink bytes.Buffer
	w := &stageLogWriter{tracker: tracker, sink: &sink}

	noise := []string{
		"[Finger] http://example.com [200] [1234] [Example] [Title]\n",
		"[WRN] No surviving target found, active fingerprint scanning has been skipped\n",
		"[INF] 主动指纹扫描已取消\n",
	}
	for _, line := range noise {
		if _, err := w.Write([]byte(line)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	if events := tracker.Events(); len(events) != 0 {
		t.Fatalf("普通日志不应产生阶段事件，实际 %d 条", len(events))
	}
	if _, floor := tracker.Status(); floor != 0 {
		t.Fatalf("普通日志不应推进进度下限，实际 %d", floor)
	}
}

// 验证多阶段顺序推进：事件按发生顺序累积、进度下限单调递增
func TestStageLogWriterSequentialStages(t *testing.T) {
	tracker := newStageTracker()
	w := &stageLogWriter{tracker: tracker, sink: &bytes.Buffer{}}

	sequence := []string{
		"[INF] fingerprint repository fingersized\n",
		"[INF] progress stage=finger status=done processed=10 total=10 requested=10 failed=0 matched=5 skipped=0 elapsed=5s rate=2.0/s\n",
		"[INF] progress stage=dynamic status=done processed=5 total=5 requested=5 failed=0 matched=0 skipped=0 elapsed=10s rate=0.5/s\n",
		"[INF] progress stage=active status=done processed=50 total=50 requested=50 failed=2 matched=6 skipped=0 elapsed=20s rate=2.5/s\n",
	}
	for _, line := range sequence {
		if _, err := w.Write([]byte(line)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	events := tracker.Events()
	if len(events) != 4 {
		t.Fatalf("期望 4 条阶段事件，实际 %d", len(events))
	}
	// 时间线按发生顺序
	wantOrder := []string{StageInit, StageProbe, StageFingerprint + ":discovery", StageFingerprint + ":active"}
	for i, want := range wantOrder {
		if events[i].Stage != want {
			t.Fatalf("事件[%d] 期望阶段 %q，实际 %q", i, want, events[i].Stage)
		}
	}
	// 时间戳单调不减
	for i := 1; i < len(events); i++ {
		if events[i].Time.Before(events[i-1].Time) {
			t.Fatalf("事件[%d] 时间戳早于事件[%d]", i, i-1)
		}
	}
	if _, floor := tracker.Status(); floor != 8 {
		t.Fatalf("期望最终进度下限 8，实际 %d", floor)
	}
	_ = time.Now
}
