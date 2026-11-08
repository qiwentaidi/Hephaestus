package toolnetwork

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"hephaestus-backend/pkg/toolguard"
)

// Gateway only speaks HTTP. CONNECT is terminated here, never copied to a
// target socket. A fresh authority lives only for this execution.
type Gateway struct {
	State     func() (State, error)
	CA        *Authority
	Audit     func(map[string]any) error
	Transport http.RoundTripper
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	tunnels   map[net.Conn]bool
}

func NewGateway(ctx context.Context, state func() (State, error), audit func(map[string]any) error) (*Gateway, error) {
	a, err := NewAuthority()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 15 * time.Second}).DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 60 * time.Second, DisableKeepAlives: true, ForceAttemptHTTP2: false}
	return &Gateway{State: state, CA: a, Audit: audit, Transport: tr, ctx: ctx, cancel: cancel, tunnels: map[net.Conn]bool{}}, nil
}
func (g *Gateway) Close() {
	g.cancel()
	g.mu.Lock()
	defer g.mu.Unlock()
	for c := range g.tunnels {
		c.Close()
	}
	if t, ok := g.Transport.(*http.Transport); ok {
		t.CloseIdleConnections()
	}
}
func (g *Gateway) Handler(auth bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { g.serve(w, r, auth, "") })
}
func (g *Gateway) deny(w http.ResponseWriter, message string) {
	if g.Audit != nil {
		state, _ := g.State()
		_ = g.Audit(map[string]any{"time": time.Now().UTC(), "sessionId": state.SessionID, "executionId": state.ExecutionID, "tool": state.Tool, "blocked": true, "reason": "inspection_rejected"})
	}

	g.respondDenied(w, message)
}
func (g *Gateway) respondDenied(w http.ResponseWriter, message string) {
	w.Header().Set("X-Hephaestus-Egress-Blocked", "1")
	http.Error(w, "egress blocked: "+message, http.StatusForbidden)
}
func (g *Gateway) respondDeniedWithMatch(w http.ResponseWriter, message string, match *toolguard.Match) {
	if match != nil {
		w.Header().Set("X-Hephaestus-Egress-Rule-ID", match.RuleID)
		w.Header().Set("X-Hephaestus-Egress-Rule-Name", match.RuleName)
		w.Header().Set("X-Hephaestus-Egress-Matched-Text", match.MatchedText)
		w.Header().Set("X-Hephaestus-Egress-Rule-Message", match.Message)
	}
	g.respondDenied(w, message)
}
func (g *Gateway) serve(w http.ResponseWriter, r *http.Request, auth bool, tunnel string) {
	state, err := g.State()
	if err != nil || g.ctx.Err() != nil {
		g.deny(w, "policy unavailable or execution stopped")
		return
	}
	if auth && !validProxyCredential(r.Header.Get("Proxy-Authorization"), state.Token) {
		g.deny(w, "invalid gateway credential")
		return
	}
	if r.Method == http.MethodConnect {
		if tunnel != "" {
			g.deny(w, "nested CONNECT is disabled")
			return
		}
		g.connect(w, r)
		return
	}
	if r.ProtoMajor != 1 || r.Header.Get("Upgrade") != "" || len(r.Trailer) > 0 || len(r.Header.Values("Content-Type")) > 1 || len(r.Header.Values("Content-Encoding")) > 1 {
		g.deny(w, "unsupported protocol or upgrade")
		return
	}
	if tunnel != "" {
		if r.URL.IsAbs() || !sameAuthority(r.Host, tunnel, "443") {
			g.deny(w, "CONNECT authority mismatch")
			return
		}
		r.URL.Scheme = "https"
		r.URL.Host = tunnel
	}
	if r.URL.Scheme != "http" && r.URL.Scheme != "https" || r.URL.Hostname() == "" || r.URL.User != nil || !sameAuthority(r.Host, r.URL.Host, defaultPort(r.URL.Scheme)) {
		g.deny(w, "invalid request authority")
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stop := context.AfterFunc(g.ctx, cancel)
	defer stop()
	if r.Body == nil {
		r.Body = http.NoBody
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxBody+1))
	if err != nil || len(body) > MaxBody {
		g.deny(w, "request body exceeds inspection limit or cannot be read")
		return
	}
	inspected, err := inspectBody(body, r.Header)
	if err != nil {
		g.deny(w, err.Error())
		return
	}
	policy, err := toolguard.Compile(state.Rules)
	if err != nil {
		g.deny(w, "invalid policy")
		return
	}
	args := map[string]any{"url": r.URL.String(), "method": r.Method, "headers": r.Header, "data": inspected, "raw_request": r.Method + " " + r.URL.RequestURI() + " HTTP/1.1\n" + string(body)}
	match := policy.CheckRequest(state.Tool, args)
	if match == nil && state.Tool != "http-framework-test" {
		match = policy.CheckRequest("http-framework-test", args)
	}
	if state.Tool == "shell_open" || state.Tool == "shell_send" {
		for _, scope := range []string{"shell_open", "shell_send", "exec"} {
			if match == nil {
				match = policy.CheckRequest(scope, args)
			}
		}
	}
	sum := sha256.Sum256(body)
	query := sha256.Sum256([]byte(r.URL.RawQuery))
	event := map[string]any{"time": time.Now().UTC(), "sessionId": state.SessionID, "executionId": state.ExecutionID, "tool": state.Tool, "method": r.Method, "origin": r.URL.Scheme + "://" + r.URL.Host, "path": r.URL.EscapedPath(), "queryHash": hex.EncodeToString(query[:]), "bodyHash": hex.EncodeToString(sum[:]), "blocked": match != nil}
	if match != nil {
		event["ruleId"] = match.RuleID
		event["ruleName"] = match.RuleName
		event["matchedText"] = match.MatchedText
		event["ruleMessage"] = match.Message
	}
	if g.Audit == nil || g.Audit(event) != nil {
		g.deny(w, "audit unavailable")
		return
	}
	if match != nil {
		g.respondDeniedWithMatch(w, toolguard.FormatBlockedMessage(match), match)
		return
	}
	// Re-check the execution lease immediately before forwarding.
	if _, err = g.State(); err != nil {
		g.deny(w, "execution expired")
		return
	}
	out := r.Clone(ctx)
	out.RequestURI = ""
	out.Body = io.NopCloser(bytes.NewReader(body))
	out.ContentLength = int64(len(body))
	out.TransferEncoding = nil
	stripHopHeaders(out.Header)
	out.Header.Del("Proxy-Authorization")
	response, err := g.Transport.RoundTrip(out)
	if err != nil {
		http.Error(w, "egress upstream request failed", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusSwitchingProtocols {
		g.deny(w, "protocol switch is disabled")
		return
	}
	stripHopHeaders(response.Header)
	response.Header.Del("X-Hephaestus-Egress-Blocked")
	for key, values := range response.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func validProxyCredential(value, token string) bool {
	value = strings.TrimSpace(value)
	if subtle.ConstantTimeCompare([]byte(value), []byte("Bearer "+token)) == 1 {
		return true
	}
	const prefix = "Basic "
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(value, prefix)))
	if err != nil {
		return false
	}
	parts := strings.SplitN(string(raw), ":", 2)
	return len(parts) == 2 && subtle.ConstantTimeCompare([]byte(parts[0]), []byte("Bearer")) == 1 && subtle.ConstantTimeCompare([]byte(parts[1]), []byte(token)) == 1
}
func stripHopHeaders(h http.Header) {
	for _, v := range h.Values("Connection") {
		for _, key := range strings.Split(v, ",") {
			h.Del(strings.TrimSpace(key))
		}
	}
	for _, key := range []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "TE", "Trailer", "Transfer-Encoding", "Upgrade"} {
		h.Del(key)
	}
}
func defaultPort(scheme string) string {
	if scheme == "https" {
		return "443"
	}
	return "80"
}
func sameAuthority(a, b, port string) bool { return authority(a, port) == authority(b, port) }
func authority(v, port string) string {
	u, err := url.Parse("http://" + v)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" {
		return "!" + v
	}
	p := u.Port()
	if p == "" {
		p = port
	}
	return net.JoinHostPort(strings.ToLower(u.Hostname()), p)
}

func (g *Gateway) connect(w http.ResponseWriter, r *http.Request) {
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil || host == "" || port == "" || r.URL.Host != r.Host || strings.ContainsAny(host, "/@?#") {
		g.deny(w, "invalid CONNECT authority")
		return
	}
	cert, err := g.CA.Certificate(host)
	if err != nil {
		g.deny(w, "TLS interception unavailable")
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		g.deny(w, "TLS interception unavailable")
		return
	}
	c, rw, err := hj.Hijack()
	if err != nil {
		return
	}
	g.mu.Lock()
	g.tunnels[c] = true
	g.mu.Unlock()
	defer func() { c.Close(); g.mu.Lock(); delete(g.tunnels, c); g.mu.Unlock() }()
	_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
	if rw.Flush() != nil {
		return
	}
	// Preserve bytes buffered after CONNECT (clients may pipeline ClientHello).
	buffered := &bufferedConn{Conn: c, r: rw.Reader}
	secure := tls.Server(buffered, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}})
	_ = secure.SetDeadline(time.Now().Add(30 * time.Second))
	if secure.HandshakeContext(g.ctx) != nil {
		return
	}
	reader := bufio.NewReader(secure)
	for {
		_ = secure.SetDeadline(time.Now().Add(2 * time.Minute))
		req, err := http.ReadRequest(reader)
		if err != nil {
			return
		}
		// Parse each inner request before creating any upstream connection.
		writer := &connectionWriter{header: make(http.Header), conn: secure, method: req.Method}
		g.serve(writer, req, false, r.Host)
		req.Body.Close()
		writer.finish()
		return // No persistent tunnels; next request gets a fresh lease/policy check.
	}
}

type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// CONNECT serves one HTTP/1.1 request per tunnel. Close-delimited bodies avoid
// retaining target responses in memory and prevent upgrades into opaque streams.
type connectionWriter struct {
	header  http.Header
	conn    net.Conn
	method  string
	written bool
}

func (w *connectionWriter) Header() http.Header { return w.header }
func (w *connectionWriter) WriteHeader(status int) {
	if w.written {
		return
	}
	w.written = true
	w.header.Del("Transfer-Encoding")
	w.header.Set("Connection", "close")
	fmt.Fprintf(w.conn, "HTTP/1.1 %d %s\r\n", status, http.StatusText(status))
	_ = w.header.Write(w.conn)
	_, _ = io.WriteString(w.conn, "\r\n")
}
func (w *connectionWriter) Write(p []byte) (int, error) {
	if !w.written {
		w.WriteHeader(200)
	}
	if w.method == "HEAD" {
		return len(p), nil
	}
	return w.conn.Write(p)
}
func (w *connectionWriter) finish() {
	if !w.written {
		w.WriteHeader(200)
	}
}

func inspectBody(body []byte, headers http.Header) (any, error) {
	if len(body) == 0 {
		return "", nil
	}
	switch strings.ToLower(headers.Get("Content-Encoding")) {
	case "", "identity":
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("invalid compressed body")
		}
		defer reader.Close()
		body, err = io.ReadAll(io.LimitReader(reader, MaxBody+1))
		if err != nil || len(body) > MaxBody {
			return nil, fmt.Errorf("decompressed body exceeds inspection limit")
		}
	default:
		return nil, fmt.Errorf("unsupported body encoding")
	}
	kind, params, err := mime.ParseMediaType(headers.Get("Content-Type"))
	if err != nil && headers.Get("Content-Type") != "" {
		return nil, fmt.Errorf("invalid content type")
	}
	switch {
	case kind == "application/json" || strings.HasSuffix(kind, "+json"):
		if err := uniqueJSON(body); err != nil {
			return nil, err
		}
		var v any
		if err = json.Unmarshal(body, &v); err != nil {
			return nil, fmt.Errorf("invalid JSON body")
		}
		// Preserve raw text for custom rules alongside decoded structured fields.
		return map[string]any{"body": v, "raw_request": string(body)}, nil
	case kind == "application/x-www-form-urlencoded":
		v, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, fmt.Errorf("invalid form body")
		}
		return flattenValues(v), nil
	case kind == "multipart/form-data":
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		fields := map[string]any{}
		for n := 0; ; n++ {
			if n > 1000 {
				return nil, fmt.Errorf("too many multipart fields")
			}
			part, e := reader.NextPart()
			if e == io.EOF {
				break
			}
			if e != nil {
				return nil, fmt.Errorf("invalid multipart body")
			}
			b, e := io.ReadAll(part)
			part.Close()
			if e != nil {
				return nil, e
			}
			key := part.FormName()
			if _, exists := fields[key]; exists {
				return nil, fmt.Errorf("duplicate multipart field")
			}
			fields[key] = string(b)
		}
		return fields, nil
	case kind == "" || kind == "text/plain":
		trimmed := bytes.TrimSpace(body)
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
			if err := uniqueJSON(body); err != nil {
				return nil, err
			}
			var v any
			if err := json.Unmarshal(body, &v); err != nil {
				return nil, err
			}
			return v, nil
		}
		return string(body), nil
	default:
		return nil, fmt.Errorf("unsupported request body type")
	}
}
func flattenValues(v url.Values) map[string]any {
	out := map[string]any{}
	for key, values := range v {
		if len(values) == 1 {
			out[key] = values[0]
		} else {
			for i, value := range values {
				out[fmt.Sprintf("field%d-%s", i, key)] = map[string]any{key: value}
			}
		}
	}
	return out
}

func FileAudit(path string) func(map[string]any) error {
	var mu sync.Mutex
	return func(v map[string]any) error {
		mu.Lock()
		defer mu.Unlock()
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err = f.Write(append(b, '\n')); err != nil {
			return err
		}
		return f.Sync()
	}
}

// Reject ambiguous JSON before the rule engine or upstream parser can disagree.
func uniqueJSON(body []byte) error {
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 100 {
			return fmt.Errorf("JSON nesting limit")
		}
		t, err := d.Token()
		if err != nil {
			return err
		}
		delimiter, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return err
				}
				key, ok := k.(string)
				if !ok || seen[key] {
					return fmt.Errorf("duplicate or invalid JSON key")
				}
				seen[key] = true
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("invalid JSON delimiter")
		}
		_, err = d.Token()
		return err
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	return nil
}
