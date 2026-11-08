package runtimeenv

import "testing"

func TestIsSnapChromePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "snap bin", path: "/snap/bin/chromium", want: true},
		{name: "snap package", path: "/snap/chromium/current/usr/lib/chromium-browser/chrome", want: true},
		{name: "non snap chrome", path: "/usr/bin/google-chrome", want: false},
		{name: "similar prefix", path: "/snapshot/bin/chromium", want: false},
		{name: "empty", path: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSnapChromePath(tt.path); got != tt.want {
				t.Fatalf("IsSnapChromePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
