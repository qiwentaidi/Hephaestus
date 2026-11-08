package toolguard

import (
	"encoding/json"
	"net/url"
	"reflect"
	"testing"
)

func TestSQLUpdatePresetBlocksExecutableInputs(t *testing.T) {
	policy, err := Compile(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		"UPDATE users SET password='changed' WHERE id=2",
		"update users set active=false",
		"UPDATE public.users SET active=false WHERE id=2",
		`UPDATE "public"."users" AS u SET active=false WHERE u.id=2`,
		"UPDATE `app`.`users` u SET active=0",
		"UPDATE [dbo].[users] SET active=0",
		"UPDATE ONLY users SET active=false",
		"UPDATE LOW_PRIORITY IGNORE users SET active=0",
		"UPDATE/**/users/**/SET active=0",
		"UPDATE -- comment\n users\nSET active=0",
		"UPDATE 用户 SET active=0",
	}
	for _, sql := range statements {
		t.Run(sql, func(t *testing.T) {
			cases := []struct {
				tool string
				args any
			}{
				{"external_mcp_sql_execute", map[string]any{"query": sql}},
				{"exec", map[string]any{"command": "psql", "args": []string{"-c", sql}}},
				{"shell_send", map[string]any{"text": "psql -c \"" + sql + "\""}},
				{"execute-python-script", map[string]any{"script": "cursor.execute(" + string(mustJSON(t, sql)) + ")"}},
				{"http-framework-test", map[string]any{"url": "https://example.test/search?q=" + url.QueryEscape(sql)}},
				{"http-framework-test", map[string]any{"url": "https://example.test/search?q=" + url.QueryEscape(url.QueryEscape(sql))}},
				{"http-framework-test", map[string]any{"url": "https://example.test/search", "method": "POST", "data": "q=" + sql}},
				{"attack_surface_create_webscan_task", map[string]any{"config": map[string]any{"payload": sql}}},
			}
			for _, tc := range cases {
				match := policy.Check(tc.tool, tc.args)
				if match == nil || match.RuleID != "preset-sql-update" {
					t.Errorf("%s: SQL UPDATE was not intercepted: %#v", tc.tool, match)
				}
			}
		})
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSQLUpdatePresetAllowsOrdinaryRequestsAndReports(t *testing.T) {
	policy, err := Compile(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		tool string
		args any
	}{
		{"http-framework-test", map[string]any{"url": "https://example.test/profile/edit", "method": "POST", "data": `{"name":"Alice"}`}},
		{"http-framework-test", map[string]any{"url": "https://example.test/config/save", "method": "PATCH", "data": `{"theme":"dark"}`}},
		{"http-framework-test", map[string]any{"url": "https://example.test/search", "data": "SELECT id FROM users FOR UPDATE"}},
		{"exec", map[string]any{"command": "apt update"}},
		{"python_execute", map[string]any{"code": `config.update({"theme":"dark"})`}},
		{"record_vulnerability", map[string]any{"evidence": "UPDATE users SET password='changed' WHERE id=2"}},
		{"upsert_project_fact", map[string]any{"body": "UPDATE users SET active=false"}},
	}
	for _, tc := range cases {
		if match := policy.Check(tc.tool, tc.args); match != nil {
			t.Errorf("ordinary request/report blocked: %s %#v: %#v", tc.tool, tc.args, match)
		}
	}
}

func TestSQLUpdatePresetMigrationPreservesEdits(t *testing.T) {
	custom := Rule{ID: "custom", Name: "custom", Enabled: false, Pattern: "custom"}
	original := Config{Enabled: true, PresetVersion: 5, Rules: []Rule{custom}}
	upgraded, changed := MigratePresetRules(original)
	if !changed || upgraded.PresetVersion != CurrentPresetVersion || len(upgraded.Rules) != 2 || !reflect.DeepEqual(upgraded.Rules[0], custom) || upgraded.Rules[1].ID != "preset-sql-update" || !upgraded.Rules[1].Enabled {
		t.Fatalf("v5 migration did not append SQL UPDATE only: %#v", upgraded)
	}
	if !reflect.DeepEqual(original.Rules, []Rule{custom}) {
		t.Fatal("migration modified original configuration")
	}
	for _, edited := range []Config{
		{Enabled: true, PresetVersion: 5, Rules: []Rule{{ID: "preset-sql-update", Name: "edited", Enabled: false, Pattern: "edited"}}},
		{Enabled: true, PresetVersion: 5, Rules: []Rule{{ID: "custom-update", Name: "[内置] SQL UPDATE", Enabled: false, Pattern: "edited"}}},
	} {
		migrated, changed := MigratePresetRules(edited)
		if !changed || !reflect.DeepEqual(migrated.Rules, edited.Rules) {
			t.Fatalf("migration duplicated or overwrote existing rule: %#v", migrated)
		}
	}
	upgraded.Rules = upgraded.Rules[:1] // Removing a migrated preset is permanent.
	again, changed := MigratePresetRules(upgraded)
	if changed || !reflect.DeepEqual(again, upgraded) {
		t.Fatalf("migration restored a removed SQL UPDATE rule: %#v", again)
	}
}
