package ipmetadata

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMergeLocation(t *testing.T) {
	t.Parallel()

	if got := MergeLocation("中国", "北京市", "北京市", "朝阳区"); got != "中国/北京市/朝阳区" {
		t.Fatalf("MergeLocation() = %q", got)
	}
}

func TestNormalizeISP(t *testing.T) {
	t.Parallel()

	for input, want := range map[string]string{
		"移动":  "中国移动",
		"联通":  "中国联通",
		"电信":  "中国电信",
		"教育网": "教育网",
	} {
		if got := NormalizeISP(input); got != want {
			t.Errorf("NormalizeISP(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLookupSkipsNonPublicIP(t *testing.T) {
	t.Parallel()

	resolver := NewResolver(t.TempDir())
	if got := resolver.Lookup("192.168.1.1"); got != (Metadata{}) {
		t.Fatalf("Lookup() = %#v, want an empty result", got)
	}
}

func TestResolverStatusIncludesManualInstallDetails(t *testing.T) {
	t.Parallel()

	status := NewResolver(t.TempDir()).Status()
	if status.State != StateIdle || status.Ready {
		t.Fatalf("unexpected initial status: %#v", status)
	}
	if status.DatabaseDir == "" || status.QQWryURL == "" || status.GeoLiteASNURL == "" {
		t.Fatalf("manual-install details are incomplete: %#v", status)
	}
}

func TestStartWarmupDoesNotBlockAndLookupNeverWaits(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseServer := func() { releaseOnce.Do(func() { close(release) }) }

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(func() {
		releaseServer()
		server.Close()
	})

	resolver := NewResolver(t.TempDir())
	resolver.qqwryURL = server.URL
	resolver.asnURL = server.URL
	resolver.client = server.Client()

	started := time.Now()
	resolver.StartWarmup()
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("StartWarmup() took %v, want an immediate return", elapsed)
	}

	started = time.Now()
	if got := resolver.Lookup("8.8.8.8"); got != (Metadata{}) {
		t.Fatalf("Lookup() = %#v, want empty metadata while the databases are missing", got)
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("Lookup() took %v, want an immediate return", elapsed)
	}

	releaseServer()
	waitForState(t, resolver, StateFailed)
}

func TestLookupDoesNotStartDatabaseDownloads(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requests, 1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	resolver := NewResolver(t.TempDir())
	resolver.qqwryURL = server.URL
	resolver.asnURL = server.URL
	resolver.client = server.Client()

	if got := resolver.Lookup("8.8.8.8"); got != (Metadata{}) {
		t.Fatalf("Lookup() = %#v, want an empty result", got)
	}
	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("download attempts = %d after a lookup, want 0", got)
	}
	if got := resolver.Status().State; got != StateIdle {
		t.Fatalf("resolver state = %q after a lookup, want %q", got, StateIdle)
	}
}

func TestStartWarmupLoadsDatabasesInBackground(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requests, 1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	resolver := NewResolver(t.TempDir())
	resolver.qqwryURL = server.URL
	resolver.asnURL = server.URL
	resolver.client = server.Client()

	resolver.StartWarmup()
	waitForState(t, resolver, StateFailed)
	if got := atomic.LoadInt32(&requests); got == 0 {
		t.Fatal("warmup did not attempt to load the databases")
	}
}

func waitForState(t *testing.T, resolver *Resolver, want string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got := resolver.Status().State; got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("resolver state = %q, want %q", resolver.Status().State, want)
}
