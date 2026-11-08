package mcpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

func streamTestResult(method string) any {
	switch method {
	case "initialize":
		return map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{"tools": map[string]any{}}}
	case "tools/list":
		return map[string]any{"tools": []Tool{{Name: "echo", InputSchema: map[string]any{"type": "object"}}}}
	default:
		return map[string]any{"content": []any{map[string]any{"type": "text", "text": "verified"}}}
	}
}

func TestLocalMCPProcess(t *testing.T) {
	if os.Getenv("HEPHAESTUS_MCP_TEST_CHILD") == "1" {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			var req struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
			}
			if json.Unmarshal(scanner.Bytes(), &req) != nil {
				os.Exit(2)
			}
			if req.ID == 0 {
				continue
			}
			if os.Getenv("HEPHAESTUS_MCP_TEST_STALL") == "1" {
				time.Sleep(time.Minute)
			}
			json.NewEncoder(os.Stdout).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": streamTestResult(req.Method)})
		}
		os.Exit(0)
	}
	server := Server{Name: "local", Transport: "stdio", Command: os.Args[0], Args: []string{"-test.run=^TestLocalMCPProcess$"}, Env: map[string]string{"HEPHAESTUS_MCP_TEST_CHILD": "1"}}
	tools, err := Discover(context.Background(), server)
	if err != nil || len(tools) != 1 {
		t.Fatalf("discovery: %v %v", tools, err)
	}
	result, err := Call(context.Background(), server, "echo", map[string]any{})
	if err != nil || result["content"] == nil {
		t.Fatalf("call: %v %v", result, err)
	}
	server.Env["HEPHAESTUS_MCP_TEST_STALL"] = "1"
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = Discover(ctx, server)
	if err == nil || time.Since(started) > 3*time.Second {
		t.Fatalf("process cancellation did not complete: %v", err)
	}
}

func TestLegacySSEDiscoveryAndCall(t *testing.T) {
	var mu sync.Mutex
	streams := map[string]chan []byte{}
	next := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test" {
			t.Error("missing auth")
		}
		if r.Method == http.MethodGet {
			mu.Lock()
			next++
			id := fmt.Sprint(next)
			messages := make(chan []byte, 8)
			streams[id] = messages
			mu.Unlock()
			defer func() { mu.Lock(); delete(streams, id); mu.Unlock() }()
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "event: endpoint\ndata: /message?session=%s\n\n", id)
			w.(http.Flusher).Flush()
			for {
				select {
				case data := <-messages:
					fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
					w.(http.Flusher).Flush()
				case <-r.Context().Done():
					return
				}
			}
		}
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.ID != 0 {
			raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": streamTestResult(req.Method)})
			mu.Lock()
			messages := streams[r.URL.Query().Get("session")]
			mu.Unlock()
			if messages == nil {
				t.Error("missing session")
				w.WriteHeader(400)
				return
			}
			messages <- raw
		}
		w.WriteHeader(202)
	}))
	defer remote.Close()
	server := Server{Name: "legacy", Transport: "sse", URL: remote.URL + "/sse", Headers: map[string]string{"Authorization": "Bearer test"}}
	tools, err := Discover(context.Background(), server)
	if err != nil || len(tools) != 1 {
		t.Fatalf("discover: %v %v", tools, err)
	}
	result, err := Call(context.Background(), server, "echo", nil)
	if err != nil || result["content"] == nil {
		t.Fatalf("call: %v %v", result, err)
	}
}

func TestLegacySSERejectsOtherOrigin(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: endpoint\ndata: https://other.invalid/message\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer remote.Close()
	_, err := Discover(context.Background(), Server{Name: "legacy", Transport: "sse", URL: remote.URL})
	if err == nil {
		t.Fatal("accepted cross-origin endpoint")
	}
}
