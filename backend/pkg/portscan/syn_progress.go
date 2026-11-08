package portscan

import (
	"context"
	gonmap "github.com/qiwentaidi/gonmap/lib"
	"hephaestus-backend/pkg/logger"
	"time"
)

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

// SYN batches reorder host/port work and pending results are not durable. Until
// the entire scanning stage completes, replay it on resume (including old unsafe
// checkpoints). Connect retains its existing cursor behavior.
func portScanResumeCompleted(config TCPDiscoveryConfig, completed int32) int32 {
	if config.normalized().Mode == gonmap.PortDiscoveryModeSYN {
		return 0
	}
	return completed
}

func portScanCheckpointMeta(config TCPDiscoveryConfig, displayCompleted int32, total int) map[string]interface{} {
	meta := map[string]interface{}{"completed": portScanResumeCompleted(config, displayCompleted), "total": total}
	if config.normalized().Mode == gonmap.PortDiscoveryModeSYN {
		meta["displayCompleted"] = displayCompleted
		meta["resumePolicy"] = "syn-replay-v1"
	}
	return meta
}

func synDisplayWork(batchTotal, attempted, total int) int32 {
	if batchTotal <= 0 || attempted <= 0 || total <= 0 {
		return 0
	}
	if attempted > total {
		attempted = total
	}
	return int32(float64(batchTotal) * 0.8 * float64(attempted) / float64(total))
}

// This is an observational threshold, NOT a deadline or an automatic retry
// cutoff. A slow round may have packet loss and still benefit from another round.
func synRetryBudget(probes, rate int, timeout time.Duration) time.Duration {
	if probes <= 0 {
		return 0
	}
	if rate <= 0 {
		rate = 1500
	}
	if timeout <= 0 {
		timeout = time.Second
	}
	budget := (time.Duration(float64(probes)/float64(rate)*float64(time.Second)) + timeout + 5*time.Second) * 3
	if budget < time.Minute {
		budget = time.Minute
	}
	return budget
}

// Independent of the sending goroutine: still emits a diagnostic if a native
// driver call or a result callback is blocked. It cannot interrupt a driver call.
func startSYNHeartbeat(ctx context.Context, round int) func() {
	stop, done := make(chan struct{}), make(chan struct{})
	started := time.Now()
	go func() {
		defer close(done)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				logger.Info("SYN 轮次仍在执行: task=%s round=%d elapsed=%s ctxErr=%v", synTaskID(ctx), round, time.Since(started).Round(time.Second), contextError(ctx))
			}
		}
	}()
	return func() { close(stop); <-done }
}

// A scanning checkpoint must stay below 100: startup recovery treats 100 as
// completion. Only the explicit portscan:completed stage may publish 100.
func portScanDisplayPercent(config TCPDiscoveryConfig, completed int32, total, floor int) int {
	if total <= 0 {
		return 0
	}
	progress := int(float64(completed) / float64(total) * 100)
	if progress < floor {
		progress = floor
	}
	limit := 100
	if config.normalized().Mode == gonmap.PortDiscoveryModeSYN {
		limit = 99
	}
	if progress > limit {
		progress = limit
	}
	if progress < 0 {
		progress = 0
	}
	return progress
}

type synTaskIDKey struct{}

func synTaskID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(synTaskIDKey{}).(string)
	return id
}

// The control watcher belongs to the scan's lifetime too. Waiting here prevents
// a query from surviving handler exit and its subsequent pause acknowledgement.
// poll must use ctx for database calls; the stop function cancels before joining.
func startPortScanControlWatcher(ctx context.Context, cancel context.CancelFunc, poll func(context.Context) bool) func() {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if ctx.Err() != nil {
					return
				}
				if poll(ctx) {
					cancel()
					return
				}
			}
		}
	}()
	return func() { cancel(); <-done }
}
