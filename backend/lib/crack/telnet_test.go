package crack

import (
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
)

func TestTelnetScanReportsUnauthBeforeCredentials(t *testing.T) {
	address := startFakeUnauthTelnetServer(t)
	host, port := splitTestAddress(t, address)

	flag, err := TelnetUnauth(host, port)
	if err != nil {
		t.Fatalf("expected telnet unauth check to complete, got error: %v", err)
	}
	if !flag {
		t.Fatal("expected telnet unauth check to return true")
	}

	var got *VulnerabilityInfo
	TelnetScan(context.Background(), address, []string{"admin:password"}, func(vuln VulnerabilityInfo) {
		got = &vuln
	})

	if got == nil {
		t.Fatal("expected unauthorized finding")
	}
	if got.ID != "telnet-unauthorized" {
		t.Fatalf("expected telnet unauthorized finding, got %q", got.ID)
	}
	if got.Extract != "" {
		t.Fatalf("expected unauthorized finding without credential extract, got %q", got.Extract)
	}
}

func startFakeUnauthTelnetServer(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen fake telnet: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = conn.Write([]byte("# "))
				_, _ = io.Copy(io.Discard, conn)
			}(conn)
		}
	}()

	return listener.Addr().String()
}

func splitTestAddress(t *testing.T, address string) (string, int) {
	t.Helper()

	parts := strings.Split(address, ":")
	if len(parts) != 2 {
		t.Fatalf("expected host:port address, got %q", address)
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatalf("parse test port: %v", err)
	}
	return parts[0], port
}
