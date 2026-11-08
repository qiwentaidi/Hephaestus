package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDiscoverAndCall(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint("sse=", stream), func(t *testing.T) {
			var closed atomic.Int32
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-secret" {
					t.Error("missing credential")
				}
				if r.Method == http.MethodDelete {
					closed.Add(1)
					w.WriteHeader(204)
					return
				}
				var req struct {
					ID     int            `json:"id"`
					Method string         `json:"method"`
					Params map[string]any `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					return
				}
				if req.Method != "initialize" && (r.Header.Get("Mcp-Session-Id") != "test-session" || r.Header.Get("MCP-Protocol-Version") != "2025-03-26") {
					t.Error("session or negotiated protocol missing")
				}
				var result any
				switch req.Method {
				case "initialize":
					w.Header().Set("Mcp-Session-Id", "test-session")
					result = map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{"tools": map[string]any{}}}
				case "notifications/initialized":
					w.WriteHeader(202)
					return
				case "tools/list":
					name := "first"
					next := "page2"
					if req.Params["cursor"] == "page2" {
						name = "second"
						next = ""
					}
					result = map[string]any{"tools": []Tool{{Name: name, InputSchema: map[string]any{"type": "object"}}}, "nextCursor": next}
				case "tools/call":
					if req.Params["name"] != "first" {
						t.Error("wrong remote name")
					}
					args, _ := req.Params["arguments"].(map[string]any)
					result = map[string]any{"content": []any{map[string]any{"type": "text", "text": args["value"]}}, "isError": args["fail"] == true}
				default:
					t.Errorf("unexpected method %s", req.Method)
				}
				msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/message\"}\n\n")
					fmt.Fprintf(w, "data: %s\n\n", msg)
				} else {
					w.Header().Set("Content-Type", "application/json")
					w.Write(msg)
				}
			}))
			defer remote.Close()
			server := Server{Name: "test", URL: remote.URL, Headers: map[string]string{"Authorization": "Bearer test-secret"}}
			tools, err := Discover(context.Background(), server)
			if err != nil || len(tools) != 2 {
				t.Fatalf("discover: %+v %v", tools, err)
			}
			result, err := Call(context.Background(), server, "first", map[string]any{"value": "proof"})
			if err != nil || !strings.Contains(fmt.Sprint(result), "proof") {
				t.Fatalf("call: %+v %v", result, err)
			}
			result, err = Call(context.Background(), server, "first", map[string]any{"fail": true})
			if err == nil || result["isError"] != true {
				t.Fatalf("tool error lost: %+v %v", result, err)
			}
			if closed.Load() != 3 {
				t.Fatalf("sessions not closed: %d", closed.Load())
			}
		})
	}
}

func TestNoCredentialRedirectAndNoErrorBodyLeak(t *testing.T) {
	var hits atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", destination.URL)
		w.WriteHeader(302)
		fmt.Fprint(w, "private-secret")
	}))
	defer origin.Close()
	_, err := Discover(context.Background(), Server{Name: "test", URL: origin.URL, Headers: map[string]string{"Authorization": "private-secret"}})
	if err == nil || strings.Contains(err.Error(), "private-secret") || hits.Load() != 0 {
		t.Fatalf("redirect or secret leak: %v hits=%d", err, hits.Load())
	}
}

func TestCancellationAndAliasIsolation(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer remote.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := Discover(ctx, Server{Name: "test", URL: remote.URL}); err == nil {
		t.Fatal("expected cancelled discovery")
	}
	a := Alias("a", "attack_surface_create_dirscan_task")
	if a == Alias("b", "attack_surface_create_dirscan_task") || a == Alias("a", "other") || len(a) > 64 {
		t.Fatal("tool names must be isolated and bounded")
	}
}

func TestValidate(t *testing.T) {
	for _, url := range []string{"file:///etc/passwd", "http://user:pass@localhost/mcp", "https://example.com/#fragment", "localhost:8080"} {
		if Validate(Server{Name: "test", URL: url}) == nil {
			t.Fatalf("accepted %q", url)
		}
	}
	if Validate(Server{Name: "test", URL: "http://localhost/mcp", Headers: map[string]string{"Authorization": "value\r\nx: y"}}) == nil {
		t.Fatal("accepted header injection")
	}
}
