package modelrequest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTimeoutCoversHeadersAndStreamingBody(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprint(streaming), func(t *testing.T) {
			stopped := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(stopped)
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: first\n\n")
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			client := &http.Client{Transport: Transport{Timeout: func() time.Duration { return 100 * time.Millisecond }}}
			resp, err := client.Get(server.URL)
			if streaming && err != nil {
				t.Fatal(err)
			}
			if err == nil {
				defer resp.Body.Close()
				var body []byte
				body, err = io.ReadAll(resp.Body)
				if !strings.Contains(string(body), "first") {
					t.Fatalf("partial output lost: %s", body)
				}
			}
			var timeout *TimeoutError
			if !errors.As(err, &timeout) || !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "高级设置") {
				t.Fatalf("expected actionable model timeout, got %v", err)
			}
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("server request did not cancel")
			}
		})
	}
}

func TestTimeoutSnapshotAndParentCancellation(t *testing.T) {
	started := make(chan struct{}, 4)
	stopped := make(chan struct{}, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		started <- struct{}{}
		<-r.Context().Done()
		stopped <- struct{}{}
	}))
	defer server.Close()
	var duration atomic.Int64
	duration.Store(int64(time.Hour))
	client := &http.Client{Transport: Transport{Timeout: func() time.Duration { return time.Duration(duration.Load()) }}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	active, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer active.Body.Close()
	<-started
	activeErr := make(chan error, 1)
	go func() { _, err := io.Copy(io.Discard, active.Body); activeErr <- err }()
	// Saving a shorter setting affects only the next request (including retries).
	duration.Store(int64(80 * time.Millisecond))
	next, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(io.Discard, next.Body)
	next.Body.Close()
	var timeout *TimeoutError
	if !errors.As(err, &timeout) {
		t.Fatalf("new setting not applied: %v", err)
	}
	<-stopped
	select {
	case err := <-activeErr:
		t.Fatalf("in-flight request changed deadline: %v", err)
	default:
	}
	cancel()
	select {
	case err := <-activeErr:
		if !errors.Is(err, context.Canceled) || errors.As(err, &timeout) {
			t.Fatalf("stop mislabeled as model timeout: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stop did not interrupt the active stream")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("server did not acknowledge cancellation")
	}
}
