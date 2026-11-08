package toolguard

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestDefaultConfigBlocksDangerousRecursiveDelete(t *testing.T) {
	policy, err := Compile(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	tests := []any{
		map[string]any{"command": "rm -rf /tmp/example"},
		map[string]any{"command": "rm", "args": []string{"-fr", "/tmp/example"}},
		map[string]any{"command": "rm -r -f /tmp/example"},
		map[string]any{"command": "rm --recursive --force /tmp/example"},
		map[string]any{"code": `os.system("rm -rf /tmp/example")`},
	}
	for _, arguments := range tests {
		if match := policy.Check("exec", arguments); match == nil {
			t.Fatalf("expected dangerous delete to be blocked for %#v, got %#v", arguments, match)
		} else if match.RuleID != "preset-recursive-delete" {
			t.Fatalf("expected recursive delete preset, got %q", match.RuleID)
		}
	}
}

func TestPresetRulesCoverExecutableInputs(t *testing.T) {
	policy, err := Compile(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		tool string
		args any
		id   string
	}{
		{"exec", map[string]any{"command": "rm", "args": []string{"-rf", "/tmp/example"}}, "preset-recursive-delete"},
		{"exec", map[string]any{"command": "mkfs.ext4 /dev/sdb"}, "preset-disk-format"},
		{"exec", map[string]any{"command": "dd if=/dev/zero of=/dev/sdb"}, "preset-disk-overwrite"},
		{"execute-python-script", map[string]any{"script": `cursor.execute("DROP TABLE users")`}, "preset-sql-drop"},
		{"python_execute", map[string]any{"code": `db.execute("DELETE FROM accounts")`}, "preset-sql-delete"},
		{"http-framework-test", json.RawMessage(`{"url":"https://example.test/api?q=DELETE%20FROM%20users","method":"GET"}`), "preset-sql-delete"},
		{"http-framework-test", map[string]any{"url": "https://example.test/api", "data": `x=1; TRUNCATE TABLE users`}, "preset-sql-truncate"},
		{"http-framework-test", map[string]any{"url": "https://example.test/api", "data": `q=DELETE+FROM+users`}, "preset-sql-delete"},
		{"external_mcp_sql_execute", map[string]any{"query": "DROP DATABASE app"}, "preset-sql-drop"},
		{"attack_surface_create_webscan_task", map[string]any{"config": map[string]any{"payload": "DELETE FROM users"}}, "preset-sql-delete"},
	}
	for _, tc := range cases {
		if match := policy.Check(tc.tool, tc.args); match == nil || match.RuleID != tc.id {
			t.Fatalf("%s: expected %s for %#v, got %#v", tc.tool, tc.id, tc.args, match)
		}
	}
}

func TestARTEXAdditionalPresets(t *testing.T) {
	policy, err := Compile(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		tool    string
		command string
		id      string
	}{
		{"exec", "rm --recursive /tmp/example", "preset-recursive-delete"},
		{"exec", "rm /etc/profile", "preset-critical-system-delete"},
		{"exec", "mkfs", "preset-disk-format"},
		{"exec", "wipe /dev/sdb", "preset-disk-format"},
		{"exec", "dd if=/dev/zero of= /dev/sdb", "preset-disk-overwrite"},
		{"shell_send", ":(){ :|:& };:", "preset-fork-bomb"},
		{"exec", "shutdown -h now", "preset-shutdown-reboot"},
		{"exec", "kill -9 -1", "preset-kill-all-processes"},
		{"exec", "iptables -F", "preset-firewall-flush"},
		{"python_execute", "db.dropDatabase()", "preset-mongodb-drop"},
		{"execute-python-script", "cursor.execute(\"DROP ROLE analyst\")", "preset-sql-drop"},
		{"exec", "redis-cli FLUSHALL", "preset-redis-flush"},
		{"http-framework-test", "https://example.test/api/clear", "preset-bulk-clear-route"},
		{"exec", "dd if=/dev/zero of=./image", "preset-destructive-system-command"},
		{"exec", "echo x > /dev/sda", "preset-destructive-system-command"},
	}
	for _, tc := range cases {
		args := map[string]any{"command": tc.command}
		if tc.tool == "shell_send" {
			args = map[string]any{"text": tc.command}
		} else if tc.tool == "python_execute" {
			args = map[string]any{"code": tc.command}
		} else if tc.tool == "http-framework-test" {
			args = map[string]any{"method": "GET", "url": tc.command}
		}
		if match := policy.Check(tc.tool, args); match == nil || match.RuleID != tc.id {
			t.Errorf("%s %q: expected %s, got %#v", tc.tool, tc.command, tc.id, match)
		}
	}
	for _, command := range []string{"rm /tmp/example", "echo PATH=$PATH", "curl https://example.test/api/list"} {
		if match := policy.Check("exec", map[string]any{"command": command}); match != nil {
			t.Errorf("benign command %q blocked by %#v", command, match)
		}
	}
	if match := policy.Check("record_vulnerability", map[string]any{"evidence": "observed shutdown and FLUSHALL"}); match != nil {
		t.Errorf("non-executable evidence blocked by %#v", match)
	}
}

func TestARTEXPresetCoverageAndUpgradeDeduplication(t *testing.T) {
	seenIDs := map[string]bool{}
	seenNames := map[string]bool{}
	for _, rule := range PresetRules() {
		if seenIDs[rule.ID] || seenNames[rule.Name] {
			t.Fatalf("duplicate built-in rule: %s (%s)", rule.Name, rule.ID)
		}
		if rule.Name == "[内置] 数据外泄管道" {
			t.Fatal("data-exfiltration pipe must not be seeded")
		}
		seenIDs[rule.ID], seenNames[rule.Name] = true, true
	}
	for _, id := range artexAdditionalPresetIDs {
		if !seenIDs[id] {
			t.Fatalf("missing ARTEX preset %s", id)
		}
	}

	previouslyAdded := Rule{ID: "custom-critical", Name: "[内置] 删除系统关键目录", Enabled: false, Pattern: "custom", Target: "tool_input"}
	config := Config{Enabled: true, PresetVersion: 4, Rules: []Rule{previouslyAdded}}
	upgraded, changed := MigratePresetRules(config)
	if !changed || upgraded.PresetVersion != CurrentPresetVersion || len(upgraded.Rules) != len(artexAdditionalPresetIDs)+1 {
		t.Fatalf("v4 migration duplicated an existing rule or missed a new one: %#v", upgraded)
	}
	if upgraded.Rules[0].ID != previouslyAdded.ID || upgraded.Rules[0].Enabled != previouslyAdded.Enabled || upgraded.Rules[0].Pattern != previouslyAdded.Pattern || upgraded.Rules[0].Target != previouslyAdded.Target {
		t.Fatalf("existing rule was changed: %#v", upgraded.Rules[0])
	}
	again, changed := MigratePresetRules(upgraded)
	if changed || len(again.Rules) != len(upgraded.Rules) {
		t.Fatalf("migration reseeded rules: %#v", again)
	}

	legacyRules := []Rule{
		{ID: "preset-recursive-delete", Pattern: defaultDangerousDeletePattern, Enabled: false},
		{ID: "preset-disk-format", Pattern: previousDiskFormatPattern, Enabled: true},
		{ID: "preset-disk-overwrite", Pattern: "custom-disk-pattern", Enabled: true},
		{ID: "preset-sql-drop", Pattern: previousSQLDropPattern, Enabled: true},
	}
	upgraded, changed = MigratePresetRules(Config{Enabled: true, PresetVersion: 4, Rules: legacyRules})
	if !changed || upgraded.Rules[0].Pattern != artexRecursiveDeletePattern || upgraded.Rules[0].Enabled || upgraded.Rules[1].Pattern != artexDiskFormatPattern || upgraded.Rules[2].Pattern != "custom-disk-pattern" || upgraded.Rules[3].Pattern != artexSQLDropPattern {
		t.Fatalf("default patterns were not upgraded without preserving edits: %#v", upgraded.Rules)
	}
}

func TestPresetRulesAvoidNonExecutableText(t *testing.T) {
	policy, err := Compile(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		tool string
		args any
	}{
		{"http-framework-test", map[string]any{"url": "https://example.test/api/items/1", "method": "GET"}},
		{"http-framework-test", map[string]any{"url": "https://example.test/api", "data": "SELECT id FROM users"}},
		{"record_vulnerability", map[string]any{"evidence": "An attacker attempted DROP TABLE users"}},
		{"record_vulnerability", map[string]any{"url": "https://example.test/items/1", "method": "DELETE", "evidence": "request was observed"}},
		{"space_engine_search", map[string]any{"query": "DELETE FROM users"}},
	}
	for _, tc := range cases {
		if match := policy.Check(tc.tool, tc.args); match != nil {
			t.Fatalf("%s: false block for %#v: %#v", tc.tool, tc.args, match)
		}
	}
}

func TestPresetHTTPDeleteRulesBlockRequestForms(t *testing.T) {
	policy, err := Compile(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		tool string
		args any
	}{
		{"http-framework-test", map[string]any{"url": "https://example.test/items/1", "method": "DELETE"}},
		{"http-framework-test", map[string]any{"url": "https://example.test/items/1", "method": "POST", "headers": map[string]string{"X-HTTP-Method-Override": "DELETE"}}},
		{"external_mcp_web_call", json.RawMessage(`{"url":"https://example.test/items/1","method":"delete"}`)},
		{"exec", map[string]any{"command": "curl -X DELETE https://example.test/items/1"}},
		{"exec", map[string]any{"command": "curl", "args": []string{"--request=DELETE", "https://example.test/items/1"}}},
		{"exec", map[string]any{"command": "wget --method=DELETE https://example.test/items/1"}},
		{"execute-python-script", map[string]any{"script": `requests.delete("https://example.test/items/1")`}},
		{"execute-python-script", map[string]any{"script": `session.request("DELETE", url)`}},
		{"execute-python-script", map[string]any{"script": `pool.urlopen("DELETE", url)`}},
		{"execute-python-script", map[string]any{"script": `client.build_request("DELETE", url)`}},
		{"execute-python-script", map[string]any{"script": `request = Request("DELETE", url)`}},
		{"exec", map[string]any{"command": `curl -X POST -H 'X-HTTP-Method-Override: DELETE' https://example.test/items/1`}},
		{"execute-python-script", map[string]any{"script": `urllib.request.Request(url, method="DELETE")`}},
		{"execute-python-script", map[string]any{"script": `client.delete(url)`}},
		{"external_mcp_http_execute", map[string]any{"request": map[string]any{"url": "https://example.test/items/1", "method": "DELETE"}}},
	}
	for _, tc := range cases {
		if match := policy.Check(tc.tool, tc.args); match == nil || match.RuleID == "" {
			t.Fatalf("%s: expected HTTP DELETE block for %#v, got %#v", tc.tool, tc.args, match)
		}
	}
}

func TestPresetRulesAreEditableAndNotReseeded(t *testing.T) {
	config := DefaultConfig()
	config.Rules[0].Enabled = false
	config.Rules[0].Pattern = "changed-pattern"
	config.Rules = config.Rules[:len(config.Rules)-1]
	migrated, changed := MigratePresetRules(config)
	if changed || len(migrated.Rules) != len(config.Rules) || migrated.Rules[0].Enabled || migrated.Rules[0].Pattern != "changed-pattern" {
		t.Fatalf("preset edits were overwritten: %#v", migrated)
	}
	policy, err := Compile(Config{Enabled: true, PresetVersion: CurrentPresetVersion, Rules: nil})
	if err != nil {
		t.Fatal(err)
	}
	if match := policy.Check("exec", map[string]any{"command": "rm -rf /tmp/example"}); match != nil {
		t.Fatalf("empty editable rule list unexpectedly blocked: %#v", match)
	}
	if match := policy.Check("http-framework-test", map[string]any{"url": "https://example.test/items/1", "method": "DELETE"}); match != nil {
		t.Fatalf("empty editable rule list unexpectedly blocked HTTP DELETE: %#v", match)
	}
}

func TestWriteDeleteRouteUsesEditableInterceptionRule(t *testing.T) {
	policy, err := Compile(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []map[string]any{
		{"url": "https://example.test/adminapi/article.article/delete", "method": "POST"},
		{"request": map[string]any{"url": "https://example.test/adminapi/article.article/delete", "method": "POST"}},
		{"url": "https://example.test/api/delete/123", "method": "POST"},
	} {
		if match := policy.Check("http-framework-test", args); match == nil || (match.RuleID != "preset-http-post-delete-route" && match.RuleID != "preset-high-risk-api-operation") {
			t.Fatalf("write delete route was not blocked: args=%#v match=%#v", args, match)
		}
	}
	if match := policy.Check("http-framework-test", map[string]any{"url": "https://example.test/adminapi/article.article/delete", "method": "GET"}); match == nil || match.RuleID != "preset-high-risk-api-operation" {
		t.Fatalf("existing high-risk API rule did not block delete route: %#v", match)
	}
	legacy := Config{Enabled: true, PresetVersion: 1, Rules: []Rule{{ID: "custom", Name: "custom", Enabled: true, Pattern: "custom"}}}
	upgraded, changed := MigratePresetRules(legacy)
	if !changed || upgraded.PresetVersion != CurrentPresetVersion || len(upgraded.Rules) != 4+len(artexAdditionalPresetIDs) || upgraded.Rules[0].ID != "custom" || upgraded.Rules[1].ID != "preset-http-post-delete-route" || upgraded.Rules[2].ID != "preset-high-risk-api-operation" {
		t.Fatalf("v1 migration restored removed rules or missed new rule: %+v", upgraded)
	}
}

func TestPresetMigrationReusesExactLegacyDefaultOnly(t *testing.T) {
	legacy := Rule{
		ID: "old-rule-id", Name: "高危递归删除命令", Enabled: false,
		Tools:   []string{"exec", "exec_run", "python_execute", "execute-python-script"},
		Pattern: defaultDangerousDeletePattern,
		Message: "检测到高危递归删除命令 {match}，工具调用已被安全策略拦截。请检查命令影响范围后再试。",
	}
	custom := Rule{ID: "custom", Name: "custom", Enabled: false, Pattern: "custom"}
	migrated, changed := MigratePresetRules(Config{Enabled: true, Rules: []Rule{legacy, custom}})
	if !changed || migrated.PresetVersion != CurrentPresetVersion || len(migrated.Rules) != len(PresetRules())+1 {
		t.Fatalf("unexpected migration: %#v", migrated)
	}
	last := migrated.Rules[len(migrated.Rules)-1]
	var restoredLegacy *Rule
	for i := range migrated.Rules {
		if migrated.Rules[i].ID == legacy.ID {
			restoredLegacy = &migrated.Rules[i]
			break
		}
	}
	if restoredLegacy == nil || restoredLegacy.Name != "[内置] 递归强制删除 rm -rf" || restoredLegacy.Origin != "preset" || restoredLegacy.Enabled || last.ID != custom.ID || last.Enabled != custom.Enabled || last.Pattern != custom.Pattern {
		t.Fatalf("legacy/default rule migration lost user data: %#v", migrated.Rules)
	}
	again, changed := MigratePresetRules(migrated)
	if changed || len(again.Rules) != len(migrated.Rules) {
		t.Fatalf("migration repeated: %#v", again)
	}
}

func TestEnsureRuleIDsAssignsAndMigratesOpaqueIDs(t *testing.T) {
	config := Config{Enabled: true, Rules: []Rule{
		{Name: "new", Enabled: true, Pattern: "new"},
		{ID: legacyDangerousDeleteRuleID, Name: "legacy", Enabled: true, Pattern: "legacy"},
		{ID: "existing-audit-id", Name: "existing", Enabled: true, Pattern: "existing"},
	}}
	normalized := EnsureRuleIDs(config)
	for _, index := range []int{0, 1} {
		if _, err := uuid.Parse(normalized.Rules[index].ID); err != nil {
			t.Fatalf("rule %d ID = %q, expected UUID", index, normalized.Rules[index].ID)
		}
	}
	if normalized.Rules[0].ID == normalized.Rules[1].ID {
		t.Fatal("expected unique generated rule IDs")
	}
	if normalized.Rules[2].ID != "existing-audit-id" {
		t.Fatalf("existing ID changed to %q", normalized.Rules[2].ID)
	}
	if config.Rules[0].ID != "" || config.Rules[1].ID != legacyDangerousDeleteRuleID {
		t.Fatal("EnsureRuleIDs mutated the input config")
	}
}

func TestDefaultConfigScopesRuleToExecutionTools(t *testing.T) {
	policy, err := Compile(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if match := policy.Check("workbench_fact_upsert", map[string]any{"body": "example: rm -rf /tmp/demo"}); match != nil {
		t.Fatalf("expected non-execution tool to be allowed, got %#v", match)
	}
	if match := policy.Check("exec", map[string]any{"command": "rm -r /tmp/demo"}); match != nil {
		t.Fatalf("expected recursive delete without force to be allowed, got %#v", match)
	}
}

func TestPolicyUsesFirstEnabledMatchingRule(t *testing.T) {
	policy, err := Compile(Config{Enabled: true, Rules: []Rule{
		{ID: "disabled", Name: "disabled", Enabled: false, Pattern: "secret"},
		{ID: "first", Name: "first", Enabled: true, Pattern: `(?P<match>secret)`, Message: "命中 {match} / {tool} / {rule}"},
		{ID: "second", Name: "second", Enabled: true, Pattern: "secret"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	match := policy.Check("demo", map[string]any{"value": "secret"})
	if match == nil || match.RuleID != "first" || match.Message != "命中 secret / demo / first" {
		t.Fatalf("unexpected match: %#v", match)
	}
}

func TestCompileRejectsInvalidRule(t *testing.T) {
	_, err := Compile(Config{Enabled: true, Rules: []Rule{{ID: "broken", Name: "broken", Enabled: true, Pattern: "("}}})
	if err == nil {
		t.Fatal("expected invalid regular expression to fail")
	}
}

func TestCheckToolCallScopesRuntimeGuard(t *testing.T) {
	policy, err := Compile(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	dangerousURL := "https://example.invalid/admin/system/user/remove"

	nonExecution := []struct {
		name string
		args map[string]any
	}{
		{"http_request", map[string]any{"url": dangerousURL, "method": "DELETE"}},
		{"attack_surface_create_webscan_task", map[string]any{"targets": []string{dangerousURL}}},
		{"external_mcp_web_call", map[string]any{"url": dangerousURL}},
		{"webscan_create_task", map[string]any{"targets": []string{dangerousURL}}},
		{"space_engine_search", map[string]any{"query": "DELETE FROM users"}},
	}
	for _, tc := range nonExecution {
		if match := policy.CheckToolCall(tc.name, tc.args); match != nil {
			t.Fatalf("%s must not be intercepted by the call guard: %#v", tc.name, match)
		}
	}

	execution := []struct {
		name string
		args map[string]any
	}{
		{"exec", map[string]any{"command": "curl -X DELETE " + dangerousURL}},
		{"exec_run", map[string]any{"command": "curl", "args": []string{"-X", "DELETE", dangerousURL}}},
		{"python_execute", map[string]any{"code": "requests.delete('" + dangerousURL + "')"}},
		{"execute-python-script", map[string]any{"script": "requests.delete('" + dangerousURL + "')"}},
		{"shell_open", map[string]any{"command": "curl -X DELETE " + dangerousURL}},
		{"shell_send", map[string]any{"text": "curl -X DELETE " + dangerousURL}},
		{"http-framework-test", map[string]any{"url": dangerousURL}},
		{"http-framework-test", map[string]any{"url": "https://example.invalid/article.article/delete", "method": "POST"}},
		{"http-framework-test", map[string]any{"url": "https://example.invalid/user/action", "method": "POST", "data": "action=remove&id=1"}},
		{"http-framework-test", map[string]any{"url": "https://example.invalid/items/1", "method": "POST", "headers": map[string]any{"X-HTTP-Method-Override": "DELETE"}}},
	}
	for _, tc := range execution {
		if match := policy.CheckToolCall(tc.name, tc.args); match == nil {
			t.Fatalf("%s must remain guarded: %#v", tc.name, tc.args)
		}
	}
}

func TestHTTPStyleToolCoversPythonHighRiskPatterns(t *testing.T) {
	policy, err := Compile(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	const base = "https://example.invalid"
	cases := []struct {
		name     string
		httpArgs map[string]any
		pyArgs   map[string]any
	}{
		{"path remove", map[string]any{"url": base + "/admin/system/user/remove"}, map[string]any{"script": "requests.get('" + base + "/admin/system/user/remove')"}},
		{"post delete route", map[string]any{"url": base + "/article.article/delete", "method": "POST"}, map[string]any{"script": "requests.post('" + base + "/article.article/delete')"}},
		{"body operation", map[string]any{"url": base + "/user/action", "method": "POST", "data": "action=remove&id=1"}, map[string]any{"script": "requests.post('" + base + "/user/action', data={'action': 'remove'})"}},
		{"encoded path", map[string]any{"url": base + "/users/%2572emove"}, map[string]any{"script": "requests.get('" + base + "/users/%2572emove')"}},
		{"explicit delete method", map[string]any{"url": base + "/items/1", "method": "DELETE"}, map[string]any{"script": "requests.delete('" + base + "/items/1')"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			httpMatch := policy.CheckToolCall("http-framework-test", tc.httpArgs)
			pyMatch := policy.CheckToolCall("execute-python-script", tc.pyArgs)
			if httpMatch == nil || pyMatch == nil {
				t.Fatalf("high-risk coverage mismatch: http=%#v python=%#v", httpMatch, pyMatch)
			}
		})
	}
}
