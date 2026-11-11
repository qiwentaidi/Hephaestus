package scantask

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/subdomain"
)

func TestCreateSubdomainTaskPersistsEffectiveConfig(t *testing.T) {
	// Use isolated configuration; no external APIs or scan workers are started.
	getSetting := func(key string) (interface{}, error) {
		if key == "test-space" {
			return config.SpaceEngineConfig{FOFA: config.FOFAConfig{Key: "private-test-key"}}, nil
		}
		return config.AIConfig{}, nil
	}
	config.InitSpaceEngineConfigDB(getSetting, nil, "test-space")
	config.InitAIConfigDB(getSetting, nil, "test-ai")
	t.Cleanup(func() {
		config.InitSpaceEngineConfigDB(nil, nil, "")
		config.InitAIConfigDB(nil, nil, "")
	})

	for _, tc := range []struct {
		name            string
		input           map[string]interface{}
		thread, timeout float64
		engines, dns    []string
	}{
		{"omitted", nil, 50, 5, []string{"FOFA", "Digitorus", "HackerTarget", "THC", "Anubis", "IP138"}, subdomain.DefaultDnsServers},
		{"empty", map[string]interface{}{"thread": 0, "timeout": 0, "appendEngines": []string{}, "dnsServers": []string{}}, 50, 5, []string{"FOFA", "Digitorus", "HackerTarget", "THC", "Anubis", "IP138"}, subdomain.DefaultDnsServers},
		{"explicit", map[string]interface{}{"thread": 12, "timeout": 9, "appendEngines": []string{"hackertarget"}, "dnsServers": []string{"1.1.1.1"}, "customOption": true}, 12, 9, []string{"HackerTarget"}, []string{"1.1.1.1:53"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openSQLite(t, t.TempDir()+"/tasks.db")
			if err := db.AutoMigrate(&database.VulnScanTask{}, &database.VulnScanTaskItem{}); err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(tc.input)
			id, _, _, _, err := CreateAIScanTaskWithDB(db, "subdomain", "default config test", "agent", "", []string{"example.test"}, tc.input)
			if err != nil {
				t.Fatal(err)
			}
			var task database.VulnScanTask
			if err := db.Where("id = ?", id).First(&task).Error; err != nil {
				t.Fatal(err)
			}
			var saved map[string]interface{}
			if err := json.Unmarshal([]byte(task.Config), &saved); err != nil {
				t.Fatal(err)
			}
			if saved["thread"] != tc.thread || saved["timeout"] != tc.timeout || saved["mode"] != float64(1) {
				t.Fatalf("missing effective numeric defaults: %s", task.Config)
			}
			options, err := subdomain.BuildScanOption([]string{"example.test"}, saved)
			if err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string][]string{"dnsServers": tc.dns, "appendEngines": tc.engines} {
				wantJSON, _ := json.Marshal(want)
				gotJSON, _ := json.Marshal(saved[key])
				if string(gotJSON) != string(wantJSON) {
					t.Fatalf("%s: got %s want %s", key, gotJSON, wantJSON)
				}
			}
			if !reflect.DeepEqual(options.DnsServers, tc.dns) || !reflect.DeepEqual(options.AppendEngines, tc.engines) {
				t.Fatal("execution changed persisted defaults")
			}
			if !task.CreatedByAI || task.Status != "pending" || strings.Contains(task.Config, "private-test-key") {
				t.Fatalf("incorrect metadata or persisted credentials: %s", task.Config)
			}
			if tc.input["customOption"] != nil && saved["customOption"] != true {
				t.Fatal("lost unrelated option")
			}
			after, _ := json.Marshal(tc.input)
			if string(before) != string(after) {
				t.Fatal("mutated caller configuration")
			}
		})
	}
}
