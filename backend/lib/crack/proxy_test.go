package crack

import (
	"bufio"
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func startEchoServer(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen echo: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return listener.Addr().String()
}

func startConnectProxy(t *testing.T, requiredAuth string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen proxy: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				reader := bufio.NewReader(conn)
				request, err := http.ReadRequest(reader)
				if err != nil {
					return
				}
				if request.Method != http.MethodConnect {
					_, _ = io.WriteString(conn, "HTTP/1.1 405 Method Not Allowed\r\nContent-Length: 0\r\n\r\n")
					return
				}
				if requiredAuth != "" && request.Header.Get("Proxy-Authorization") != requiredAuth {
					_, _ = io.WriteString(conn, "HTTP/1.1 407 Proxy Authentication Required\r\nContent-Length: 0\r\n\r\n")
					return
				}
				target, err := net.DialTimeout("tcp", request.Host, time.Second)
				if err != nil {
					_, _ = io.WriteString(conn, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
					return
				}
				defer target.Close()
				_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
				if reader.Buffered() > 0 {
					_, _ = io.Copy(target, reader)
				}
				go func() { _, _ = io.Copy(target, conn) }()
				_, _ = io.Copy(conn, target)
			}()
		}
	}()
	return listener.Addr().String()
}

func TestHTTPConnectDialer(t *testing.T) {
	echoAddress := startEchoServer(t)
	proxyAddress := startConnectProxy(t, "")
	dial, err := NewProxyDialContext("http://" + proxyAddress)
	if err != nil {
		t.Fatalf("new proxy dialer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := dial(ctx, "tcp", echoAddress)
	if err != nil {
		t.Fatalf("dial through proxy: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	response := make([]byte, 4)
	if _, err := io.ReadFull(conn, response); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(response) != "ping" {
		t.Fatalf("unexpected echo: %q", response)
	}
}

func TestHTTPConnectDialerWithBasicAuth(t *testing.T) {
	echoAddress := startEchoServer(t)
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
	proxyAddress := startConnectProxy(t, auth)
	dial, err := NewProxyDialContext("http://user:pass@" + proxyAddress)
	if err != nil {
		t.Fatalf("new proxy dialer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := dial(ctx, "tcp", echoAddress)
	if err != nil {
		t.Fatalf("dial through authenticated proxy: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ok")); err != nil {
		t.Fatalf("write: %v", err)
	}
	response := make([]byte, 2)
	if _, err := io.ReadFull(conn, response); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(response) != "ok" {
		t.Fatalf("unexpected echo: %q", response)
	}
}

func TestTCPRelayTunnelsConnections(t *testing.T) {
	echoAddress := startEchoServer(t)
	proxyAddress := startConnectProxy(t, "")
	dial, err := NewProxyDialContext("http://" + proxyAddress)
	if err != nil {
		t.Fatalf("new proxy dialer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	relay, err := newTCPRelay(ctx, echoAddress, dial)
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}
	defer relay.Close()
	if relay.Address() == "" {
		t.Fatal("relay address is empty")
	}

	conn, err := net.DialTimeout("tcp", relay.Address(), time.Second)
	if err != nil {
		t.Fatalf("dial relay: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	payload := []byte("relay-check")
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write relay: %v", err)
	}
	response := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, response); err != nil {
		t.Fatalf("read relay: %v", err)
	}
	if string(response) != string(payload) {
		t.Fatalf("unexpected relay echo: %q", response)
	}
}

func TestNewProxyDialContextRejectsUnsupportedScheme(t *testing.T) {
	if _, err := NewProxyDialContext("ftp://127.0.0.1:21"); err == nil {
		t.Fatal("expected unsupported scheme error")
	}
	if _, err := NewProxyDialContext(""); err == nil {
		t.Fatal("expected empty proxy error")
	}
}

func TestWrapProxyRelayCallbackRestoresTargetURL(t *testing.T) {
	var got VulnerabilityInfo
	callback := wrapProxyRelayCallback(func(v VulnerabilityInfo) { got = v }, "10.0.0.5:22", "127.0.0.1:45678")
	callback(VulnerabilityInfo{URL: "127.0.0.1:45678"})
	if got.URL != "10.0.0.5:22" {
		t.Fatalf("URL = %q", got.URL)
	}
}

func startSOCKS5Proxy(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen socks5 proxy: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				greeting := make([]byte, 2)
				if _, err := io.ReadFull(conn, greeting); err != nil {
					return
				}
				methods := make([]byte, int(greeting[1]))
				if _, err := io.ReadFull(conn, methods); err != nil {
					return
				}
				if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
					return
				}
				request := make([]byte, 4)
				if _, err := io.ReadFull(conn, request); err != nil {
					return
				}
				if request[0] != 0x05 || request[1] != 0x01 {
					return
				}
				var host string
				switch request[3] {
				case 0x01:
					address := make([]byte, 4)
					if _, err := io.ReadFull(conn, address); err != nil {
						return
					}
					host = net.IP(address).String()
				case 0x03:
					length := make([]byte, 1)
					if _, err := io.ReadFull(conn, length); err != nil {
						return
					}
					address := make([]byte, int(length[0]))
					if _, err := io.ReadFull(conn, address); err != nil {
						return
					}
					host = string(address)
				case 0x04:
					address := make([]byte, 16)
					if _, err := io.ReadFull(conn, address); err != nil {
						return
					}
					host = net.IP(address).String()
				default:
					return
				}
				portBytes := make([]byte, 2)
				if _, err := io.ReadFull(conn, portBytes); err != nil {
					return
				}
				port := int(portBytes[0])<<8 | int(portBytes[1])
				target, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), time.Second)
				if err != nil {
					_, _ = conn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
					return
				}
				defer target.Close()
				if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
					return
				}
				go func() { _, _ = io.Copy(target, conn) }()
				_, _ = io.Copy(conn, target)
			}()
		}
	}()
	return listener.Addr().String()
}

func TestSOCKS5Dialer(t *testing.T) {
	echoAddress := startEchoServer(t)
	proxyAddress := startSOCKS5Proxy(t)
	dial, err := NewProxyDialContext("socks5://" + proxyAddress)
	if err != nil {
		t.Fatalf("new socks5 dialer: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := dial(ctx, "tcp", echoAddress)
	if err != nil {
		t.Fatalf("dial through socks5 proxy: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("socks")); err != nil {
		t.Fatalf("write: %v", err)
	}
	response := make([]byte, 5)
	if _, err := io.ReadFull(conn, response); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(response) != "socks" {
		t.Fatalf("unexpected echo: %q", response)
	}
}
