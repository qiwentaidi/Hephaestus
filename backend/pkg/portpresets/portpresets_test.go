package portpresets

import "testing"

func TestResolveWorkbenchPortRangePresets(t *testing.T) {
	tests := []struct {
		name       string
		preset     string
		custom     string
		wantRange  string
		wantLabel  string
		wantResult bool
	}{
		{name: "all", preset: "全端口", wantRange: WorkbenchPortRangeFull, wantLabel: "全端口", wantResult: true},
		{name: "top1000", preset: "top 1000", wantRange: WorkbenchPortRangeTop1000, wantLabel: "Top 1000 常见端口", wantResult: true},
		{name: "database", preset: "常见数据库端口", wantRange: WorkbenchPortRangeDatabase, wantLabel: "常见数据库端口", wantResult: true},
		{name: "custom", custom: "80，443，8080-8081", wantRange: "80,443,8080-8081", wantLabel: "自定义端口范围", wantResult: true},
		{name: "invalid", custom: "top ports", wantResult: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRange, gotLabel, gotResult := ResolveWorkbenchPortRange(tt.preset, tt.custom)
			if gotResult != tt.wantResult {
				t.Fatalf("result = %v, want %v", gotResult, tt.wantResult)
			}
			if gotRange != tt.wantRange || gotLabel != tt.wantLabel {
				t.Fatalf("range/label = %q/%q, want %q/%q", gotRange, gotLabel, tt.wantRange, tt.wantLabel)
			}
		})
	}
}

func TestDetectWorkbenchPortPreset(t *testing.T) {
	tests := map[string]string{
		"扫描 top 1000":  WorkbenchPortPresetTop1000,
		"扫常见的数据库端口":    WorkbenchPortPresetDatabase,
		"跑全端口 1-65535": WorkbenchPortPresetAll,
		"只做普通漏洞验证":     "",
	}

	for input, want := range tests {
		if got := DetectWorkbenchPortPreset(input); got != want {
			t.Fatalf("DetectWorkbenchPortPreset(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDetectWorkbenchPortRange(t *testing.T) {
	tests := map[string]string{
		"扫描端口 80，443，8080-8081": "80,443,8080-8081",
		"扫描端口:80":               "80",
		"扫 80,443 端口":           "80,443",
		"扫描 192.168.2.1/24":     "",
		"扫描 192.168.2.1/24 端口":  "",
	}

	for input, want := range tests {
		if got := DetectWorkbenchPortRange(input); got != want {
			t.Fatalf("DetectWorkbenchPortRange(%q) = %q, want %q", input, got, want)
		}
	}
}
