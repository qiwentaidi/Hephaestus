package crack

import (
	"net"
	"testing"
)

func TestZookeeperUnauth(t *testing.T) {
	address := startFakeZookeeperServer(t)

	flag, response, err := ZookeeperUnauth(address)
	if err != nil {
		t.Fatalf("expected zookeeper unauthorized check to succeed, got error: %v", err)
	}
	if !flag {
		t.Fatal("expected zookeeper unauthorized check to return true")
	}
	if response != "imok" {
		t.Fatalf("expected imok response, got %q", response)
	}
}

func startFakeZookeeperServer(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen fake zookeeper: %v", err)
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
				buffer := make([]byte, 4)
				n, err := conn.Read(buffer)
				if err != nil {
					return
				}
				switch string(buffer[:n]) {
				case "ruok":
					_, _ = conn.Write([]byte("imok"))
				case "stat":
					_, _ = conn.Write([]byte("Zookeeper version: 3.8.0"))
				}
			}(conn)
		}
	}()

	return listener.Addr().String()
}
