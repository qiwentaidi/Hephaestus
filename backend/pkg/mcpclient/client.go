// Package mcpclient connects the internal AI to user-configured MCP servers.
// It is independent of the platform's outward-facing MCP server registry.
package mcpclient

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const ProtocolVersion = "2025-06-18"
const maxResponse = 8 << 20

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type Server struct {
	Transport     string            `json:"transport"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	WorkDir       string            `json:"workDir,omitempty"`
	DisabledTools []string          `json:"disabledTools,omitempty"`
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	URL           string            `json:"url"`
	Enabled       bool              `json:"enabled"`
	Insecure      bool              `json:"insecure,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	Tools         []Tool            `json:"tools"`
	CheckedAt     string            `json:"checkedAt,omitempty"`
	LastError     string            `json:"lastError,omitempty"`
}

func (s Server) Mode() string {
	if s.Transport == "" {
		return "http"
	}
	return s.Transport
}

func (s Server) ToolEnabled(name string) bool {
	for _, disabled := range s.DisabledTools {
		if disabled == name {
			return false
		}
	}
	return true
}

func Validate(s Server) error {
	if strings.TrimSpace(s.Name) == "" || len(s.Name) > 100 {
		return fmt.Errorf("service name is required (maximum 100 characters)")
	}
	if s.Mode() == "stdio" {
		if strings.TrimSpace(s.Command) == "" || strings.ContainsRune(s.Command, 0) {
			return fmt.Errorf("local MCP command is required")
		}
		for _, arg := range s.Args {
			if strings.ContainsRune(arg, 0) {
				return fmt.Errorf("invalid command argument")
			}
		}
		for k, v := range s.Env {
			if k == "" || strings.ContainsAny(k, "=\x00") || strings.ContainsRune(v, 0) {
				return fmt.Errorf("invalid environment variable")
			}
		}
		return nil
	}
	if s.Mode() != "http" && s.Mode() != "sse" {
		return fmt.Errorf("unsupported MCP transport")
	}
	u, err := url.Parse(s.URL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("provide an HTTP(S) MCP endpoint without embedded credentials or fragment")
	}
	for k, v := range s.Headers {
		if !validHeaderName(k) || strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("invalid HTTP header")
		}
		switch strings.ToLower(k) {
		case "host", "content-length", "content-type", "accept", "mcp-session-id", "mcp-protocol-version", "connection", "transfer-encoding":
			return fmt.Errorf("header %s is managed by the MCP client", k)
		}
	}
	return nil
}

func validHeaderName(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c)) {
			return false
		}
	}
	return true
}

// Alias keeps arbitrary remote names within model function-name limits and
// prevents collisions with native tools or another external service.
func Alias(serverID, toolName string) string {
	sum := sha256.Sum256([]byte(serverID + "\x00" + toolName))
	var label strings.Builder
	for _, c := range toolName {
		if label.Len() >= 24 {
			break
		}
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' {
			label.WriteRune(c)
		} else {
			label.WriteByte('_')
		}
	}
	return fmt.Sprintf("external_mcp_%s_%x", label.String(), sum[:10])
}

type session struct {
	server  Server
	client  *http.Client
	id      string
	version string
	stream  *streamTransport
	nextID  int
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func connect(ctx context.Context, server Server) (*session, error) {
	if err := Validate(server); err != nil {
		return nil, err
	}
	s := &session{server: server, version: ProtocolVersion, client: &http.Client{
		Timeout: 60 * time.Second,
		// Credentials must never follow a redirect to another endpoint.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
	if server.Insecure {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- explicit user opt-in for self-signed MCP endpoints.
		s.client.Transport = transport
	}
	if server.Mode() != "http" {
		stream, err := openStream(ctx, server, s.client)
		if err != nil {
			return nil, err
		}
		s.stream = stream
	}
	var init struct {
		ProtocolVersion string                     `json:"protocolVersion"`
		Capabilities    map[string]json.RawMessage `json:"capabilities"`
	}
	if err := s.rpc(ctx, "initialize", map[string]any{"protocolVersion": ProtocolVersion, "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "hephaestus-ai", "version": "1.0.0"}}, &init); err != nil {
		s.close()
		return nil, err
	}
	switch init.ProtocolVersion {
	case "2024-11-05", "2025-03-26", ProtocolVersion:
		s.version = init.ProtocolVersion
	default:
		s.close()
		return nil, fmt.Errorf("unsupported MCP protocol version %q", init.ProtocolVersion)
	}
	if _, ok := init.Capabilities["tools"]; !ok {
		s.close()
		return nil, fmt.Errorf("MCP server does not advertise tools")
	}
	if err := s.rpc(ctx, "notifications/initialized", nil, nil); err != nil {
		s.close()
		return nil, err
	}
	return s, nil
}

func (s *session) request(ctx context.Context, method string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.server.URL, body)
	if err != nil {
		return nil, fmt.Errorf("invalid MCP request")
	}
	for k, v := range s.server.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", s.version)
	if s.id != "" {
		req.Header.Set("Mcp-Session-Id", s.id)
	}
	res, err := s.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// URL and header credentials must not reach API errors or model messages.
		return nil, fmt.Errorf("MCP connection failed; check endpoint, TLS, and network connectivity")
	}
	return res, nil
}

func (s *session) close() {
	if s.stream != nil {
		s.stream.close()
		return
	}
	if s.id == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if res, err := s.request(ctx, http.MethodDelete, nil); err == nil {
		res.Body.Close()
	}
}

func (s *session) rpc(ctx context.Context, method string, params any, out any) error {
	payload := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		payload["params"] = params
	}
	expectedID := ""
	if out != nil {
		s.nextID++
		payload["id"] = s.nextID
		expectedID = fmt.Sprint(s.nextID)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("invalid MCP arguments: %w", err)
	}
	if s.stream != nil {
		return s.stream.rpc(ctx, data, expectedID, out)
	}
	res, err := s.request(ctx, http.MethodPost, bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("MCP %s returned HTTP %d", method, res.StatusCode)
	}
	if method == "initialize" {
		s.id = res.Header.Get("Mcp-Session-Id")
	}
	if out == nil {
		return nil
	}
	decode := func(raw []byte) (bool, error) {
		var msg rpcResponse
		if err := json.Unmarshal(raw, &msg); err != nil {
			return false, fmt.Errorf("invalid MCP JSON response")
		}
		if string(msg.ID) != expectedID {
			return false, nil
		}
		if msg.JSONRPC != "2.0" {
			return true, fmt.Errorf("invalid MCP JSON-RPC version")
		}
		if msg.Error != nil {
			return true, fmt.Errorf("MCP %s failed (code %d)", method, msg.Error.Code)
		}
		if len(msg.Result) == 0 {
			return true, fmt.Errorf("MCP response has no result")
		}
		return true, json.Unmarshal(msg.Result, out)
	}
	limited := io.LimitReader(res.Body, maxResponse+1)
	if strings.HasPrefix(strings.ToLower(res.Header.Get("Content-Type")), "text/event-stream") {
		scanner := bufio.NewScanner(limited)
		scanner.Buffer(make([]byte, 4096), maxResponse)
		var event strings.Builder
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if event.Len() > 0 {
					done, err := decode([]byte(event.String()))
					if done || err != nil {
						return err
					}
					event.Reset()
				}
			} else if strings.HasPrefix(line, "data:") {
				if event.Len() > 0 {
					event.WriteByte('\n')
				}
				event.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("MCP event stream interrupted")
		}
		return fmt.Errorf("MCP event stream ended before its response or exceeded size limit")
	}
	raw, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("MCP response interrupted")
	}
	if len(raw) > maxResponse {
		return fmt.Errorf("MCP response exceeds size limit")
	}
	done, err := decode(raw)
	if err != nil {
		return err
	}
	if !done {
		return fmt.Errorf("MCP response ID does not match request")
	}
	return nil
}

func Discover(ctx context.Context, server Server) ([]Tool, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	s, err := connect(ctx, server)
	if err != nil {
		return nil, err
	}
	defer s.close()
	tools := []Tool{}
	cursor := ""
	seen := map[string]bool{}
	names := map[string]bool{}
	for page := 0; page < 20; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var result struct {
			Tools      []Tool `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		if err := s.rpc(ctx, "tools/list", params, &result); err != nil {
			return nil, err
		}
		for _, tool := range result.Tools {
			if tool.Name == "" || names[tool.Name] || tool.InputSchema == nil || tool.InputSchema["type"] != "object" {
				return nil, fmt.Errorf("invalid or duplicate MCP tool definition")
			}
			names[tool.Name] = true
			tools = append(tools, tool)
			if len(tools) > 500 {
				return nil, fmt.Errorf("MCP server exceeds 500 tools")
			}
		}
		cursor = result.NextCursor
		if cursor == "" {
			return tools, nil
		}
		if seen[cursor] {
			return nil, fmt.Errorf("MCP tools pagination repeated a cursor")
		}
		seen[cursor] = true
	}
	return nil, fmt.Errorf("MCP tools pagination exceeds limit")
}

// Client is a persistent MCP session. Open it once per agent run and reuse it
// for every tool call; Close it when the run ends. It deliberately does not
// call tools/list, so the caller can keep using the cached tool catalog.
type Client struct {
	server  Server
	session *session
	mu      sync.Mutex
	closed  bool
}

// Open establishes one MCP session. The context must live for the whole run:
// stdio/SSE transports keep a stream whose lifetime is tied to it.
func Open(ctx context.Context, server Server) (*Client, error) {
	s, err := connect(ctx, server)
	if err != nil {
		return nil, err
	}
	return &Client{server: server, session: s}, nil
}

// Server returns the configuration this client was opened with.
func (c *Client) Server() Server { return c.server }

// Call invokes one remote tool on the persistent session. Calls are serialized:
// a stateful MCP server is not expected to handle overlapping requests.
func (c *Client) Call(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.session == nil {
		return nil, fmt.Errorf("MCP client is closed")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	var result map[string]any
	if err := c.session.rpc(ctx, "tools/call", map[string]any{"name": name, "arguments": args}, &result); err != nil {
		return nil, err
	}
	if failed, _ := result["isError"].(bool); failed {
		return result, fmt.Errorf("external MCP tool reported an error")
	}
	return result, nil
}

// Close terminates the persistent session. It is safe to call more than once.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if c.session != nil {
		c.session.close()
	}
	return nil
}

// Call is the one-shot compatibility path. Session-owning callers should use
// Open/Client.Call instead so repeated calls share one MCP session.
func Call(ctx context.Context, server Server, name string, args map[string]any) (map[string]any, error) {
	client, err := Open(ctx, server)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return client.Call(ctx, name, args)
}
