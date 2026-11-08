package main

import "testing"

func TestComputeDesktopWindowMetricsStaysInsideWorkArea(t *testing.T) {
	cases := []struct {
		name           string
		workAreaWidth  int
		workAreaHeight int
	}{
		{name: "standard", workAreaWidth: 1920, workAreaHeight: 1080},
		{name: "low resolution", workAreaWidth: 1024, workAreaHeight: 600},
		{name: "very low resolution", workAreaWidth: 800, workAreaHeight: 480},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			metrics := computeDesktopWindowMetrics(tt.workAreaWidth, tt.workAreaHeight)
			if metrics.width > tt.workAreaWidth || metrics.height > tt.workAreaHeight {
				t.Fatalf("window size = %dx%d, exceeds work area %dx%d", metrics.width, metrics.height, tt.workAreaWidth, tt.workAreaHeight)
			}
			if metrics.minWidth > metrics.width || metrics.minHeight > metrics.height {
				t.Fatalf("minimum size = %dx%d, exceeds window size %dx%d", metrics.minWidth, metrics.minHeight, metrics.width, metrics.height)
			}
		})
	}
}

func TestComputeDesktopWindowMetricsUsesAvailableSizeForTinyWorkArea(t *testing.T) {
	metrics := computeDesktopWindowMetrics(320, 240)

	if metrics.width != 288 || metrics.height != 208 {
		t.Fatalf("window size = %dx%d, want 288x208", metrics.width, metrics.height)
	}
}
