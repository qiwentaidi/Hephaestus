package crack

import "testing"

func TestParseRdpUser(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantDomain string
		wantUser   string
	}{
		{name: "plain user", raw: "administrator", wantUser: "administrator"},
		{name: "backslash domain", raw: `DOMAIN\administrator`, wantDomain: "DOMAIN", wantUser: "administrator"},
		{name: "slash domain", raw: "DOMAIN/administrator", wantDomain: "DOMAIN", wantUser: "administrator"},
		{name: "upn", raw: "administrator@example.local", wantDomain: "example.local", wantUser: "administrator"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDomain, gotUser := parseRdpUser(tt.raw)
			if gotDomain != tt.wantDomain || gotUser != tt.wantUser {
				t.Fatalf("parseRdpUser(%q) = (%q, %q), want (%q, %q)", tt.raw, gotDomain, gotUser, tt.wantDomain, tt.wantUser)
			}
		})
	}
}
