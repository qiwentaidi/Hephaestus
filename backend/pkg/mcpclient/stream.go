package mcpclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// A session makes sequential RPC calls; one reader owns the receive stream.
// Cancellation closes the stream and terminates/reaps local server processes.
type streamTransport struct {
	server    Server
	client    *http.Client
	endpoint  string
	writer    io.WriteCloser
	reader    io.ReadCloser
	cancel    context.CancelFunc
	cmd       *exec.Cmd
	done      chan struct{}
	messages  chan []byte
	endpoints chan string
	errors    chan error
	once      sync.Once
}

func openStream(parent context.Context, server Server, client *http.Client) (*streamTransport, error) {
	ctx, cancel := context.WithCancel(parent)
	t := &streamTransport{server: server, client: client, cancel: cancel, messages: make(chan []byte, 16), endpoints: make(chan string, 1), errors: make(chan error, 1), done: make(chan struct{})}
	if server.Mode() == "stdio" {
		cmd := exec.CommandContext(ctx, server.Command, server.Args...)
		configureProcess(cmd)
		cmd.Dir = server.WorkDir
		cmd.Env = os.Environ()
		for k, v := range server.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		cmd.Stderr = io.Discard // Never include environment values or credentials in API errors.
		cmd.WaitDelay = 2 * time.Second
		var err error
		t.writer, err = cmd.StdinPipe()
		if err != nil {
			cancel()
			return nil, fmt.Errorf("cannot open MCP stdin")
		}
		t.reader, err = cmd.StdoutPipe()
		if err != nil {
			t.writer.Close()
			cancel()
			return nil, fmt.Errorf("cannot open MCP stdout")
		}
		if err = cmd.Start(); err != nil {
			t.writer.Close()
			t.reader.Close()
			cancel()
			return nil, fmt.Errorf("cannot start local MCP server; check command and working directory")
		}
		t.cmd = cmd
		go func() { _ = cmd.Wait(); close(t.done) }()
	} else {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("invalid SSE URL")
		}
		for k, v := range server.Headers {
			req.Header.Set(k, v)
		}
		req.Header.Set("Accept", "text/event-stream")
		// SSE is a persistent connection; its lifetime is bounded by the call context.
		streamClient := *client
		streamClient.Timeout = 0
		res, err := streamClient.Do(req)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("cannot connect to MCP SSE stream")
		}
		if res.StatusCode != 200 || !strings.HasPrefix(strings.ToLower(res.Header.Get("Content-Type")), "text/event-stream") {
			res.Body.Close()
			cancel()
			return nil, fmt.Errorf("invalid MCP SSE response (HTTP %d)", res.StatusCode)
		}
		t.reader = res.Body
	}
	go t.read(ctx)
	if server.Mode() == "sse" {
		select {
		case endpoint := <-t.endpoints:
			base, _ := url.Parse(server.URL)
			target, err := url.Parse(endpoint)
			if err != nil {
				t.close()
				return nil, fmt.Errorf("invalid SSE endpoint")
			}
			target = base.ResolveReference(target)
			// Do not forward configured credentials to an arbitrary event-supplied host.
			if target.Scheme != base.Scheme || target.Host != base.Host || target.User != nil || target.Fragment != "" {
				t.close()
				return nil, fmt.Errorf("SSE endpoint must use the configured origin")
			}
			t.endpoint = target.String()
		case err := <-t.errors:
			t.close()
			return nil, err
		case <-ctx.Done():
			t.close()
			return nil, ctx.Err()
		}
	}
	return t, nil
}

func (t *streamTransport) close() {
	t.once.Do(func() {
		t.cancel()
		if t.writer != nil {
			_ = t.writer.Close()
		}
		if t.reader != nil {
			_ = t.reader.Close()
		}
		if t.cmd != nil {
			stopProcess(t.cmd)
			<-t.done
		}
	})
}

func (t *streamTransport) read(ctx context.Context) {
	scanner := bufio.NewScanner(t.reader)
	scanner.Buffer(make([]byte, 4096), maxResponse)
	event := ""
	var data strings.Builder
	send := func() bool {
		if data.Len() == 0 {
			event = ""
			return true
		}
		value := data.String()
		data.Reset()
		if event == "endpoint" {
			select {
			case t.endpoints <- value:
			default:
			}
		} else {
			select {
			case t.messages <- []byte(value):
			case <-ctx.Done():
				return false
			}
		}
		event = ""
		return true
	}
	for scanner.Scan() {
		line := scanner.Text()
		if t.server.Mode() == "stdio" {
			select {
			case t.messages <- append([]byte(nil), scanner.Bytes()...):
			case <-ctx.Done():
				return
			}
			continue
		}
		if line == "" {
			if !send() {
				return
			}
		} else if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			if data.Len() > maxResponse {
				break
			}
		}
	}
	select {
	case t.errors <- fmt.Errorf("MCP stream closed or exceeded message limit"):
	default:
	}
}

func (t *streamTransport) rpc(ctx context.Context, data []byte, expected string, out any) error {
	if t.writer != nil {
		if _, err := t.writer.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("MCP stdin write failed")
		}
	} else {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("invalid SSE message endpoint")
		}
		for k, v := range t.server.Headers {
			req.Header.Set(k, v)
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := t.client.Do(req)
		if err != nil {
			return fmt.Errorf("MCP SSE message request failed")
		}
		res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return fmt.Errorf("MCP SSE message returned HTTP %d", res.StatusCode)
		}
	}
	if out == nil {
		return nil
	}
	for {
		select {
		case raw := <-t.messages:
			var msg rpcResponse
			if json.Unmarshal(raw, &msg) != nil {
				return fmt.Errorf("invalid MCP stream JSON")
			}
			if string(msg.ID) != expected {
				continue
			}
			if msg.JSONRPC != "2.0" {
				return fmt.Errorf("invalid MCP JSON-RPC version")
			}
			if msg.Error != nil {
				return fmt.Errorf("MCP request failed (code %d)", msg.Error.Code)
			}
			if len(msg.Result) == 0 {
				return fmt.Errorf("MCP response has no result")
			}
			return json.Unmarshal(msg.Result, out)
		case err := <-t.errors:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
