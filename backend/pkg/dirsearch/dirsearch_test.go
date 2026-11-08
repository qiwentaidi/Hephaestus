package dirsearch

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunnerSkipsUnreachableTargetsBeforeScanningPaths(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	closedURL := closedLocalURL(t)
	var totalCount int
	completed := make(chan struct{})
	engine := NewDirsearchEngine(context.Background(), Options{
		Method:                 http.MethodGet,
		URLs:                   []string{server.URL, closedURL},
		Paths:                  []string{"admin", "login"},
		Workers:                2,
		Timeout:                1,
		BodyLengthExcludeTimes: 100,
	}, func(event string, data interface{}) {
		switch event {
		case "dirsearchCounts":
			totalCount, _ = data.(int)
		case "dirsearchComplete":
			select {
			case <-completed:
			default:
				close(completed)
			}
		}
	})

	engine.Runner(context.Background())

	if totalCount != 2 {
		t.Fatalf("expected only reachable target paths to be counted, got %d", totalCount)
	}
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("expected scan completion callback")
	}

	// One probe request plus two path scan requests. The closed target must not
	// receive per-path scan attempts.
	if got := atomic.LoadInt32(&requestCount); got != 3 {
		t.Fatalf("unexpected request count: got %d want 3", got)
	}
}

func TestRunAppendsBackupScanWhenEnabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	counts := make([]int, 0, 2)
	engine := NewDirsearchEngine(context.Background(), Options{
		Method:                 http.MethodGet,
		URLs:                   []string{server.URL},
		Paths:                  []string{"custom-probe"},
		Workers:                16,
		Timeout:                1,
		BodyLengthExcludeTimes: 100,
		Backupscan:             true,
	}, func(event string, data interface{}) {
		if event == "dirsearchCounts" {
			if count, ok := data.(int); ok {
				counts = append(counts, count)
			}
		}
	})

	engine.Run(context.Background())

	if len(counts) != 2 {
		t.Fatalf("expected normal and backup scan count callbacks, got %v", counts)
	}
	if counts[0] != 1 {
		t.Fatalf("expected one custom path in normal phase, got %v", counts)
	}
	if counts[1] <= 1 {
		t.Fatalf("expected backup phase to generate additional candidates, got %v", counts)
	}
}

func closedLocalURL(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	return fmt.Sprintf("http://%s", addr)
}

func TestProbeURLTreatsAnyHTTPResponseAsReachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	engine := NewDirsearchEngine(context.Background(), Options{Timeout: 1}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := engine.probeURL(ctx, server.URL); err != nil {
		t.Fatalf("expected HTTP 404 response to be reachable, got %v", err)
	}
}
