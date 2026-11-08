package asset

import "testing"

func TestIsJSScanAssetCandidate(t *testing.T) {
	cases := map[string]bool{
		"https://api.example.com/v1": true,
		"http://192.0.2.10:8080":     true,
		"api.example.com":            true,
		"192.0.2.10":                 true,
		"/api/users":                 false,
		"user@example.com":           false,
		"1.2.3.4:8080":               false,
		"not a host":                 false,
	}
	for input, want := range cases {
		if got := isJSScanAssetCandidate(input); got != want {
			t.Fatalf("isJSScanAssetCandidate(%q) = %v, want %v", input, got, want)
		}
	}
}
