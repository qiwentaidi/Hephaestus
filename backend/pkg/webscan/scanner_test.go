package webscan

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"hephaestus-backend/pkg/types"
)

func TestParseConfigToWebscanOptionsSeparatesWebAndNonWebTargets(t *testing.T) {
	targets := []string{
		"http://example.com",
		"https://example.org",
		"ftp://127.0.0.1:21",
		"ssh://127.0.0.1:22",
		"192.168.2.1/24",
		"192.168.2.10",
	}

	options := parseConfigToWebscanOptions(map[string]interface{}{}, targets)

	expectedWebTargets := []string{
		"http://example.com",
		"https://example.org",
	}
	if !reflect.DeepEqual(options.Target, expectedWebTargets) {
		t.Fatalf("expected web targets %v, got %v", expectedWebTargets, options.Target)
	}

	expectedTCP := map[string][]string{
		"ftp": {"ftp://127.0.0.1:21"},
		"ssh": {"ssh://127.0.0.1:22"},
	}
	if !reflect.DeepEqual(options.TcpTarget, expectedTCP) {
		t.Fatalf("expected tcp targets %v, got %v", expectedTCP, options.TcpTarget)
	}

	if options.NucleiThreads != defaultNucleiThreads {
		t.Fatalf("expected default nuclei threads %d, got %d", defaultNucleiThreads, options.NucleiThreads)
	}

	if !options.DisableMaxHostErr {
		t.Fatalf("expected DisableMaxHostErr to default to true")
	}

	if !options.SkipNucleiWithWAF {
		t.Fatalf("expected SkipNucleiWithWAF to default to true")
	}
}

func TestParseConfigToWebscanOptionsAllowsStableDefaultsOverride(t *testing.T) {
	options := parseConfigToWebscanOptions(map[string]interface{}{
		"nucleiThreads":     float64(6),
		"disableMaxHostErr": false,
		"skipNucleiWithWAF": false,
	}, []string{"http://example.com"})

	if options.NucleiThreads != 6 {
		t.Fatalf("expected nuclei threads override 6, got %d", options.NucleiThreads)
	}

	if options.DisableMaxHostErr {
		t.Fatalf("expected DisableMaxHostErr override to false")
	}

	if options.SkipNucleiWithWAF {
		t.Fatalf("expected SkipNucleiWithWAF override to false")
	}
}

func TestBuildNucleiJobsIncludesProtocolTagsForNonWebTargets(t *testing.T) {
	options := types.WebscanOptions{
		TemplateFiles: []string{"custom-template.yaml"},
		TcpTarget: map[string][]string{
			"ftp": {"ftp://127.0.0.1:21"},
			"ssh": {"ssh://127.0.0.1:22"},
		},
	}

	fingerprintTags := map[string][]string{
		"http://example.com": {"nginx", "wordpress"},
	}

	jobs, skippedTargets := buildNucleiJobs(options, fingerprintTags, nil)

	expected := []types.NucleiScanJob{
		{
			URL:      "http://example.com",
			Template: []string{"custom-template.yaml"},
			Tags:     []string{"nginx", "wordpress"},
		},
		{
			URL:      "ftp://127.0.0.1:21",
			Template: []string{"custom-template.yaml"},
			Tags:     []string{"ftp"},
		},
		{
			URL:      "ssh://127.0.0.1:22",
			Template: []string{"custom-template.yaml"},
			Tags:     []string{"ssh"},
		},
	}

	if !reflect.DeepEqual(jobs, expected) {
		t.Fatalf("expected jobs %#v, got %#v", expected, jobs)
	}

	if len(skippedTargets) != 0 {
		t.Fatalf("expected no skipped targets, got %v", skippedTargets)
	}
}

func TestBuildNucleiJobsSkipsWAFTargetsWhenEnabled(t *testing.T) {
	options := types.WebscanOptions{
		SkipNucleiWithWAF: true,
	}

	jobs, skippedTargets := buildNucleiJobs(
		options,
		map[string][]string{
			"http://example.com": {"nginx"},
			"http://waf.example": {"nginx"},
		},
		map[string]types.Tag{
			"http://waf.example": {AssetType: "waf", ProductName: "cloudflare"},
		},
	)

	expectedJobs := []types.NucleiScanJob{
		{
			URL:  "http://example.com",
			Tags: []string{"nginx"},
		},
	}
	expectedSkipped := []string{"http://waf.example"}

	if !reflect.DeepEqual(jobs, expectedJobs) {
		t.Fatalf("expected jobs %#v, got %#v", expectedJobs, jobs)
	}

	if !reflect.DeepEqual(skippedTargets, expectedSkipped) {
		t.Fatalf("expected skipped targets %#v, got %#v", expectedSkipped, skippedTargets)
	}
}

func TestBuildNucleiJobsDoesNotSkipWAFTargetsForSpecialScan(t *testing.T) {
	options := types.WebscanOptions{
		SkipNucleiWithWAF: true,
		WebscanOption:     4,
	}

	jobs, skippedTargets := buildNucleiJobs(
		options,
		map[string][]string{
			"http://waf.example": {"nginx"},
		},
		map[string]types.Tag{
			"http://waf.example": {AssetType: "waf", ProductName: "cloudflare"},
		},
	)

	expectedJobs := []types.NucleiScanJob{
		{
			URL:  "http://waf.example",
			Tags: []string{"nginx"},
		},
	}

	if !reflect.DeepEqual(jobs, expectedJobs) {
		t.Fatalf("expected jobs %#v, got %#v", expectedJobs, jobs)
	}

	if len(skippedTargets) != 0 {
		t.Fatalf("expected no skipped targets, got %#v", skippedTargets)
	}
}

func TestBuildNucleiJobsUsesCustomTagsForSpecialScan(t *testing.T) {
	options := types.WebscanOptions{
		Target:        []string{"http://example.com", "http://plain.example"},
		TemplateFiles: []string{"custom-template-dir"},
		WebscanOption: 4,
		Tags:          []string{"nacos", " nginx ", "nacos", ""},
	}

	jobs, skippedTargets := buildNucleiJobs(
		options,
		map[string][]string{
			"http://example.com": {"wordpress"},
		},
		nil,
	)

	expectedJobs := []types.NucleiScanJob{
		{
			URL:      "http://example.com",
			Template: []string{"custom-template-dir"},
			Tags:     []string{"nacos", "nginx"},
		},
		{
			URL:      "http://plain.example",
			Template: []string{"custom-template-dir"},
			Tags:     []string{"nacos", "nginx"},
		},
	}

	if !reflect.DeepEqual(jobs, expectedJobs) {
		t.Fatalf("expected jobs %#v, got %#v", expectedJobs, jobs)
	}

	if len(skippedTargets) != 0 {
		t.Fatalf("expected no skipped targets, got %#v", skippedTargets)
	}
}

func TestResolveTemplatesForJobSkipsNonWebFallbackWhenNoMatch(t *testing.T) {
	originalDB := WorkFlowDB
	WorkFlowDB = map[string]Template{
		"web-dast": {
			Id: "web-dast",
			Info: TemplateInfo{
				Metadata: map[string]any{"dast": true},
				Tags:     "http",
			},
			FilePath: "/tmp/web-dast.yaml",
		},
	}
	t.Cleanup(func() {
		WorkFlowDB = originalDB
	})

	templates := resolveTemplatesForJob(types.NucleiScanJob{
		URL:  "ftp://127.0.0.1:21",
		Tags: []string{"ftp"},
	})

	if len(templates) != 0 {
		t.Fatalf("expected no matched templates for non-web target, got %v", templates)
	}
}

func TestResolveTemplatesForJobUsesLoadedAdditionalTemplateFolders(t *testing.T) {
	originalDB := WorkFlowDB
	t.Cleanup(func() {
		mutx.Lock()
		WorkFlowDB = originalDB
		mutx.Unlock()
	})

	templateDir := t.TempDir()
	templatePath := filepath.Join(templateDir, "external-poc.yaml")
	if err := os.WriteFile(templatePath, []byte(`id: external-poc
info:
  name: External PoC
  author: test
  severity: low
  tags: external-product
`), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	if err := GetTagsList([]string{templateDir}); err != nil {
		t.Fatalf("load templates: %v", err)
	}

	templates := resolveTemplatesForJob(types.NucleiScanJob{
		URL:  "http://example.com",
		Tags: []string{"external-product"},
	})

	expected := []string{templatePath}
	if !reflect.DeepEqual(templates, expected) {
		t.Fatalf("expected templates %v, got %v", expected, templates)
	}
}

func TestResolveTemplatesForJobMapsTemplateKeyToExternalFilePath(t *testing.T) {
	originalDB := WorkFlowDB
	t.Cleanup(func() {
		mutx.Lock()
		WorkFlowDB = originalDB
		mutx.Unlock()
	})

	templateDir := t.TempDir()
	templatePath := filepath.Join(templateDir, "selected-poc.yaml")
	if err := os.WriteFile(templatePath, []byte(`id: selected-poc
info:
  name: Selected PoC
  author: test
  severity: low
  tags: selected-product
`), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	if err := GetTagsList([]string{templateDir}); err != nil {
		t.Fatalf("load templates: %v", err)
	}

	templates := resolveTemplatesForJob(types.NucleiScanJob{
		URL:      "http://example.com",
		Tags:     []string{"unrelated"},
		Template: []string{"selected-poc"},
	})

	expected := []string{templatePath}
	if !reflect.DeepEqual(templates, expected) {
		t.Fatalf("expected templates %v, got %v", expected, templates)
	}
}

func TestCalculateNucleiProgress(t *testing.T) {
	tests := []struct {
		name      string
		completed int
		total     int
		expected  int
	}{
		{name: "empty total", completed: 0, total: 0, expected: 60},
		{name: "nothing completed", completed: 0, total: 100, expected: 60},
		{name: "half completed", completed: 50, total: 100, expected: 79},
		{name: "all completed capped before finish", completed: 100, total: 100, expected: 99},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := calculateNucleiProgress(tc.completed, tc.total); got != tc.expected {
				t.Fatalf("expected %d, got %d", tc.expected, got)
			}
		})
	}
}

func TestParseCheckpointInt(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int
	}{
		{name: "int", input: 3, expected: 3},
		{name: "float64", input: float64(4), expected: 4},
		{name: "int64", input: int64(5), expected: 5},
		{name: "unsupported", input: "6", expected: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseCheckpointInt(tc.input); got != tc.expected {
				t.Fatalf("expected %d, got %d", tc.expected, got)
			}
		})
	}
}
