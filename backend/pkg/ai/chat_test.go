package ai

import (
	"encoding/json"
	"testing"
)

func TestNormalizeChatToolCallArgumentsUnwrapsJSONString(t *testing.T) {
	raw := json.RawMessage(`"{\"scanType\":\"portscan\",\"targets\":[\"192.168.2.101/24\"]}"`)

	normalized := normalizeChatToolCallArguments(raw)

	var args struct {
		ScanType string   `json:"scanType"`
		Targets  []string `json:"targets"`
	}
	if err := json.Unmarshal(normalized, &args); err != nil {
		t.Fatalf("normalized arguments should be valid JSON object: %v", err)
	}
	if args.ScanType != "portscan" {
		t.Fatalf("expected scanType portscan, got %q", args.ScanType)
	}
	if len(args.Targets) != 1 || args.Targets[0] != "192.168.2.101/24" {
		t.Fatalf("unexpected targets: %#v", args.Targets)
	}
}

func TestNormalizeChatToolCallArgumentsWrapsPlainString(t *testing.T) {
	normalized := normalizeChatToolCallArguments(json.RawMessage(`"start scanning"`))

	var args map[string]string
	if err := json.Unmarshal(normalized, &args); err != nil {
		t.Fatalf("normalized arguments should be valid JSON object: %v", err)
	}
	if args["raw"] != "start scanning" {
		t.Fatalf("expected raw argument wrapper, got %#v", args)
	}
}

func TestConvertAnthropicMessagesSplitsSystemPrompt(t *testing.T) {
	system, messages := convertAnthropicMessages([]ChatMessage{
		{Role: "system", Content: "You are a scanner."},
		{Role: "user", Content: "Plan"},
		{Role: "user", Content: "Continue"},
		{Role: "assistant", Content: "OK"},
	})

	if system != "You are a scanner." {
		t.Fatalf("system prompt = %q", system)
	}
	if len(messages) != 2 {
		t.Fatalf("expected merged user message plus assistant message, got %#v", messages)
	}
	if messages[0]["role"] != "user" || messages[0]["content"] != "Plan\n\nContinue" {
		t.Fatalf("unexpected first Claude message: %#v", messages[0])
	}
}

func TestParseAnthropicMessageResponseMapsToolUse(t *testing.T) {
	result, err := parseAnthropicMessageResponse([]byte(`{
		"stop_reason": "tool_use",
		"content": [
			{"type": "text", "text": "I will run a tool."},
			{"type": "tool_use", "id": "toolu_1", "name": "run_scan", "input": {"scanType":"portscan"}}
		]
	}`))
	if err != nil {
		t.Fatalf("parseAnthropicMessageResponse() error = %v", err)
	}
	if result.Content != "I will run a tool." {
		t.Fatalf("content = %q", result.Content)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %#v", result.ToolCalls)
	}
	if result.ToolCalls[0].Name != "run_scan" {
		t.Fatalf("tool call name = %q", result.ToolCalls[0].Name)
	}
	var args map[string]string
	if err := json.Unmarshal(result.ToolCalls[0].Arguments, &args); err != nil {
		t.Fatalf("tool arguments should be JSON: %v", err)
	}
	if args["scanType"] != "portscan" {
		t.Fatalf("unexpected tool arguments: %#v", args)
	}
}
