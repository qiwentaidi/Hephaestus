package crack

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

const defaultProxyDialTimeout = 10 * time.Second

// DialContextFunc dials a TCP connection through the configured egress path.
type DialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

type dialContextKey struct{}
type proxyURLKey struct{}

// WithDialContext attaches an egress dialer to the crack context.
func WithDialContext(ctx context.Context, dial DialContextFunc) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if dial == nil {
		return ctx
	}
	return context.WithValue(ctx, dialContextKey{}, dial)
}

// WithProxyURL records the resolved proxy URL for HTTP-based crack modules.
func WithProxyURL(ctx context.Context, proxyURL string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return ctx
	}
	return context.WithValue(ctx, proxyURLKey{}, proxyURL)
}

func proxyURLFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(proxyURLKey{}).(string)
	return strings.TrimSpace(value)
}

func dialContextFromContext(ctx context.Context) DialContextFunc {
	if ctx == nil {
		return nil
	}
	dial, _ := ctx.Value(dialContextKey{}).(DialContextFunc)
	return dial
}

// DialContext uses the context egress dialer when present, otherwise a direct
// TCP dial. Callers that must not use a proxy should keep dialing directly.
func DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if dial := dialContextFromContext(ctx); dial != nil {
		return dial(ctx, network, address)
	}
	return (&net.Dialer{Timeout: defaultCrackConnectTimeout}).DialContext(ctx, network, address)
}

// NewProxyDialContext builds a TCP dialer for the application proxy setting.
// Only the explicitly supplied proxy is used; host OS proxy settings are never
// consulted here.
func NewProxyDialContext(proxyURL string) (DialContextFunc, error) {
	raw := strings.TrimSpace(proxyURL)
	if raw == "" {
		return nil, fmt.Errorf("代理地址为空")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("代理地址格式错误")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return (&httpConnectDialer{proxy: parsed, timeout: defaultProxyDialTimeout}).DialContext, nil
	case "socks5":
		return newSOCKS5DialContext(parsed, defaultProxyDialTimeout)
	default:
		return nil, fmt.Errorf("不支持的代理协议: %s", parsed.Scheme)
	}
}

type httpConnectDialer struct {
	proxy   *url.URL
	timeout time.Duration
}

func (d *httpConnectDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, fmt.Errorf("代理仅支持 TCP 连接，收到 %s", network)
	}

	dialer := &net.Dialer{Timeout: d.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", d.proxy.Host)
	if err != nil {
		return nil, fmt.Errorf("连接代理失败: %w", err)
	}
	deadline := time.Now().Add(d.timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)

	if strings.EqualFold(d.proxy.Scheme, "https") {
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: d.proxy.Hostname(),
			MinVersion: tls.VersionTLS12,
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("HTTPS 代理握手失败: %w", err)
		}
		conn = tlsConn
	}

	var request strings.Builder
	fmt.Fprintf(&request, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n", address, address)
	if d.proxy.User != nil {
		username := d.proxy.User.Username()
		password, _ := d.proxy.User.Password()
		credentials := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		fmt.Fprintf(&request, "Proxy-Authorization: Basic %s\r\n", credentials)
	}
	request.WriteString("Proxy-Connection: Keep-Alive\r\n\r\n")
	if _, err := io.WriteString(conn, request.String()); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("发送代理 CONNECT 请求失败: %w", err)
	}

	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("读取代理响应失败: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		if response.Body != nil {
			_ = response.Body.Close()
		}
		_ = conn.Close()
		return nil, fmt.Errorf("代理 CONNECT 失败: %s", response.Status)
	}
	_ = conn.SetDeadline(time.Time{})
	if reader.Buffered() > 0 {
		return &bufferedConn{Conn: conn, reader: reader}, nil
	}
	return conn, nil
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func newSOCKS5DialContext(parsed *url.URL, timeout time.Duration) (DialContextFunc, error) {
	var auth *proxy.Auth
	if parsed.User != nil {
		auth = &proxy.Auth{User: parsed.User.Username()}
		auth.Password, _ = parsed.User.Password()
	}
	forward := &net.Dialer{Timeout: timeout}
	dialer, err := proxy.SOCKS5("tcp", parsed.Host, auth, forward)
	if err != nil {
		return nil, fmt.Errorf("创建 SOCKS5 代理失败: %w", err)
	}
	if contextDialer, ok := dialer.(proxy.ContextDialer); ok {
		return func(ctx context.Context, network, address string) (net.Conn, error) {
			return contextDialer.DialContext(ctx, network, address)
		}, nil
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if ctx == nil {
			ctx = context.Background()
		}
		type dialResult struct {
			conn net.Conn
			err  error
		}
		result := make(chan dialResult, 1)
		go func() {
			conn, err := dialer.Dial(network, address)
			result <- dialResult{conn: conn, err: err}
		}()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case value := <-result:
			return value.conn, value.err
		}
	}, nil
}

type proxyDialerAdapter struct {
	dial DialContextFunc
}

func (a proxyDialerAdapter) Dial(network, address string) (net.Conn, error) {
	return a.dial(context.Background(), network, address)
}

type contextProxyDialer struct {
	dial DialContextFunc
}

func (d contextProxyDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return d.dial(ctx, network, address)
}

// newHTTPClient builds an HTTP client for crack modules. When a proxy was
// configured it is applied explicitly; otherwise the client stays direct and
// ignores process/OS proxy environment variables.
func newHTTPClient(ctx context.Context, timeout time.Duration) *http.Client {
	base, ok := http.DefaultTransport.(*http.Transport)
	var transport *http.Transport
	if ok {
		transport = base.Clone()
	} else {
		transport = &http.Transport{}
	}
	transport.Proxy = nil
	proxyURL := proxyURLFromContext(ctx)
	if proxyURL != "" {
		if parsed, err := url.Parse(proxyURL); err == nil && parsed.Host != "" {
			transport.Proxy = http.ProxyURL(parsed)
		}
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}
