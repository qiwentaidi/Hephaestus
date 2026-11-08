package toolguard

import (
	"fmt"
	"net/url"
	"testing"
)

func TestHighRiskRequestInputsAcrossTools(t *testing.T) {
	p, err := Compile(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	const path = "/yzt/jycyyzt-interface/admin/system/user/remove"
	const target = "https://example.invalid" + path
	cases := []struct {
		name, tool string
		args       map[string]any
	}{
		{"http default method", "http-framework-test", map[string]any{"url": target}},
		{"http blank method", "http-framework-test", map[string]any{"url": target, "method": "  "}},
		{"http get", "http-framework-test", map[string]any{"url": target, "method": "GET"}},
		{"http post", "http-framework-test", map[string]any{"url": target, "method": "POST"}},
		{"encoded path", "http-framework-test", map[string]any{"url": "https://example.invalid/users/%2572emove"}},
		{"mixed case", "http-framework-test", map[string]any{"url": "https://example.invalid/users/ReMoVe"}},
		{"url aliases do not hide each other", "external_mcp_web_call", map[string]any{"url": "https://example.invalid/list", "endpoint": target}},
		{"nested request", "external_mcp_web_call", map[string]any{"request": map[string]any{"url": target}}},
		{"request array", "external_mcp_web_call", map[string]any{"requests": []any{map[string]any{"uri": target}}}},
		{"scan targets", "attack_surface_create_webscan_task", map[string]any{"targets": []string{target}}},
		{"exec post", "exec", map[string]any{"command": "curl -X POST '" + target + "'"}},
		{"exec argv", "exec_run", map[string]any{"command": "curl", "args": []string{"-X", "POST", target}}},
		{"python code", "python_execute", map[string]any{"code": "requests.post('" + target + "')"}},
		{"python script", "execute-python-script", map[string]any{"script": "requests.post('" + target + "')"}},
		{"relative path literal", "execute-python-script", map[string]any{"script": "session.post(base + '" + path + "')"}},
		{"shell open", "shell_open", map[string]any{"command": "curl -X POST " + target}},
		{"shell send", "shell_send", map[string]any{"text": "curl -X POST " + target}},
		{"raw request", "http-framework-test", map[string]any{"raw_request": "POST " + path + " HTTP/1.1\r\nHost: example.invalid\r\n\r\n"}},
		{"form operation", "http-framework-test", map[string]any{"url": "https://example.invalid/user/action", "method": "POST", "data": "action=remove&id=1"}},
		{"json operation", "http-framework-test", map[string]any{"url": "https://example.invalid/user/action", "data": `{"operation":"remove","id":1}`}},
		{"nested json operation", "http-framework-test", map[string]any{"url": "https://example.invalid/user/action", "body": map[string]any{"input": map[string]any{"op": "remove"}}}},
		{"encoded operation", "http-framework-test", map[string]any{"url": "https://example.invalid/user/action", "data": "%61ction=%72emove"}},
		{"query operation", "http-framework-test", map[string]any{"url": "https://example.invalid/user?action=remove"}},
		{"curl form operation", "exec", map[string]any{"command": "curl https://example.invalid/user -d 'action=remove'"}},
		{"python json operation", "python_execute", map[string]any{"code": `requests.post('https://example.invalid/user', json={'action': 'remove'})`}},
		{"shell form operation", "shell_send", map[string]any{"text": "curl https://example.invalid/user -d 'action=remove'"}},
		// Explicit operation selectors now follow the existing high-risk API
		// rule, including update; ordinary SQL/prose mentioning update does not.
		{"update operation", "http-framework-test", map[string]any{"url": "https://example.invalid/profile", "data": `{"action":"update","name":"Alice"}`}},
		{"effective header values", "http-framework-test", map[string]any{"url": "https://example.invalid/user", "headers": map[string][]string{"X-HTTP-Method-Override": {"DELETE"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if match := p.Check(tc.tool, tc.args); match == nil || match.RuleID != "preset-high-risk-api-operation" {
				t.Fatalf("dangerous request passed high-risk rule: %#v", match)
			}
		})
	}
}

func TestRequestExtractionPreservesBenignInputsAndEditableRules(t *testing.T) {
	p, err := Compile(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"exec", map[string]any{"command": "apt update"}},
		{"python_execute", map[string]any{"code": `config.update({'theme':'dark'})`}},
		{"exec", map[string]any{"command": "curl https://example.invalid/users/list"}},
		{"shell_send", map[string]any{"text": "curl https://example.invalid/users/list"}},
		{"http-framework-test", map[string]any{"url": "https://example.invalid/users/list", "data": `{"description":"action=remove", "name":"remove"}`}},
		{"record_vulnerability", map[string]any{"url": "https://example.invalid/users/remove", "method": "DELETE", "evidence": "requests.post('https://example.invalid/users/remove')"}},
		{"upsert_project_fact", map[string]any{"body": "POST /users/remove HTTP/1.1"}},
		{"exec", map[string]any{"command": "echo hello", "description": "https://example.invalid/users/remove"}},
	} {
		if match := p.Check(tc.tool, tc.args); match != nil {
			t.Errorf("benign %s blocked: %#v", tc.tool, match)
		}
	}
	config := DefaultConfig()
	for i := range config.Rules {
		if config.Rules[i].ID == "preset-high-risk-api-operation" {
			config.Rules[i].Enabled = false
		}
	}
	p, err = Compile(config)
	if err != nil {
		t.Fatal(err)
	}
	if match := p.Check("exec", map[string]any{"command": "curl https://example.invalid/users/remove"}); match != nil {
		t.Fatalf("disabled high-risk rule still applied: %#v", match)
	}
	// Custom method/path rules retain the original METHOD + path candidate.
	p, err = Compile(Config{Enabled: true, Rules: []Rule{{ID: "custom", Name: "custom", Enabled: true, Target: "http_method", Pattern: `^POST /private$`}}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Check("http-framework-test", map[string]any{"url": "https://example.invalid/private", "method": "POST"}) == nil {
		t.Fatal("custom rule no longer matches")
	}
}

func TestAsBlockedUnwrapsHTTPTransportErrors(t *testing.T) {
	want := &Match{RuleID: "test", RuleName: "test", MatchedText: "remove"}
	err := &url.Error{Op: "Post", URL: "https://example.invalid/remove", Err: fmt.Errorf("transport: %w", NewBlockedError(want))}
	got, ok := AsBlocked(err)
	if !ok || got.RuleID != want.RuleID {
		t.Fatalf("wrapped block lost: %#v, %v", got, ok)
	}
	if _, ok := AsBlocked(fmt.Errorf("ordinary network failure")); ok {
		t.Fatal("network error reported as blocked")
	}
}
