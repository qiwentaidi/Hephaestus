package subdomain

import (
	"errors"
	"strings"
	"testing"

	"hephaestus-backend/pkg/config"
)

func TestNormalizeSubdomainCandidate(t *testing.T) {
	tests := []struct {
		name      string
		domain    string
		candidate string
		want      string
		wantOK    bool
	}{
		{
			name:      "trims whitespace and lowercases",
			domain:    "51dong.com.cn",
			candidate: "  RTR.51DONG.COM.CN  ",
			want:      "rtr.51dong.com.cn",
			wantOK:    true,
		},
		{
			name:      "rejects blank candidate",
			domain:    "51dong.com.cn",
			candidate: "   ",
			wantOK:    false,
		},
		{
			name:      "rejects leading dot from blank relative source",
			domain:    "51dong.com.cn",
			candidate: ".51dong.com.cn",
			wantOK:    false,
		},
		{
			name:      "strips concrete wildcard prefix",
			domain:    "51dong.com.cn",
			candidate: "*.api.51dong.com.cn",
			want:      "api.51dong.com.cn",
			wantOK:    true,
		},
		{
			name:      "rejects root wildcard",
			domain:    "51dong.com.cn",
			candidate: "*.51dong.com.cn",
			wantOK:    false,
		},
		{
			name:      "extracts host from url",
			domain:    "51dong.com.cn",
			candidate: "https://www.51dong.com.cn:8443/path?q=1",
			want:      "www.51dong.com.cn",
			wantOK:    true,
		},
		{
			name:      "extracts host from host port",
			domain:    "51dong.com.cn",
			candidate: "app.51dong.com.cn:443",
			want:      "app.51dong.com.cn",
			wantOK:    true,
		},
		{
			name:      "rejects outside domain",
			domain:    "51dong.com.cn",
			candidate: "www.example.com",
			wantOK:    false,
		},
		{
			name:      "rejects invalid label",
			domain:    "51dong.com.cn",
			candidate: "bad_name.51dong.com.cn",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeSubdomainCandidate(tt.domain, tt.candidate)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQualifySubdomain(t *testing.T) {
	tests := []struct {
		name      string
		domain    string
		candidate string
		want      string
	}{
		{
			name:      "adds root domain to relative subdomain",
			domain:    "51dong.com.cn",
			candidate: "api",
			want:      "api.51dong.com.cn",
		},
		{
			name:      "keeps full subdomain",
			domain:    "51dong.com.cn",
			candidate: "api.51dong.com.cn",
			want:      "api.51dong.com.cn",
		},
		{
			name:      "keeps blank candidate blank for filter rejection",
			domain:    "51dong.com.cn",
			candidate: "   ",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := qualifySubdomain(tt.domain, tt.candidate); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeSourcesDeduplicatesAndUsesFallbackOnlyWhenEmpty(t *testing.T) {
	got := normalizeSources([]string{" FOFA ", "Quake", "FOFA"}, "API")
	want := []string{"FOFA", "Quake"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %#v, want %#v", got, want)
	}

	got = normalizeSources(nil, "AI")
	want = []string{"AI"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestAppendEngineEnabledIsCaseInsensitive(t *testing.T) {
	if !appendEngineEnabled([]string{" FOFA ", " CRTName "}, "crt.name", "crtname") {
		t.Fatal("expected crt.name engine alias to be enabled")
	}
	if appendEngineEnabled([]string{"FOFA"}, "crt.name", "crtname") {
		t.Fatal("expected crt.name engine alias to be disabled")
	}
}

func TestCanonicalSubdomainAppendEngines(t *testing.T) {
	got := canonicalSubdomainAppendEngines([]string{"fofa", "HACKERTARGET", "crtname", "fofa", " custom "})
	want := []string{"FOFA", "HackerTarget", "crt.name", "custom"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestDefaultSubdomainAppendEnginesIncludesConfiguredAndPublicSources(t *testing.T) {
	got := defaultSubdomainAppendEngines(config.SpaceEngineConfig{
		FOFA:           config.FOFAConfig{Key: "fofa-key"},
		Hunter:         config.HunterConfig{Key: "hunter-key"},
		Securitytrails: config.SecuritytrailsConfig{Key: "securitytrails-key"},
	})
	want := []string{"FOFA", "Hunter", "Securitytrails", "Digitorus", "HackerTarget", "THC", "Anubis", "IP138"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestResolveSourcesForSubdomainFallsBackToLegacySource(t *testing.T) {
	got := resolveSourcesForSubdomain("api.example.com", "API", map[string][]string{
		"www.example.com": []string{"FOFA"},
	})
	want := []string{"API"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestDetectWildcardReturnsTrueWhenRandomSubdomainResolves(t *testing.T) {
	originalResolver := resolveDNSRecords
	defer func() {
		resolveDNSRecords = originalResolver
	}()

	var queriedHost string
	var queriedServers []string
	var queriedTimeout int
	resolveDNSRecords = func(host string, dnsServers []string, timeout int) ([]string, []string, error) {
		queriedHost = host
		queriedServers = append([]string(nil), dnsServers...)
		queriedTimeout = timeout
		return []string{"203.0.113.10"}, nil, nil
	}

	if !detectWildcard("Example.COM", []string{"8.8.8.8:53"}, 7) {
		t.Fatal("expected wildcard detection to return true")
	}
	if !strings.HasSuffix(queriedHost, ".example.com") {
		t.Fatalf("queried host %q does not use normalized root domain", queriedHost)
	}
	if queriedHost == "example.com" {
		t.Fatal("expected a random child hostname, got root domain")
	}
	if len(queriedServers) != 1 || queriedServers[0] != "8.8.8.8:53" {
		t.Fatalf("queried servers = %#v, want configured DNS server", queriedServers)
	}
	if queriedTimeout != 7 {
		t.Fatalf("timeout = %d, want 7", queriedTimeout)
	}
}

func TestDetectWildcardReturnsTrueWhenRandomSubdomainHasCNAME(t *testing.T) {
	originalResolver := resolveDNSRecords
	defer func() {
		resolveDNSRecords = originalResolver
	}()

	resolveDNSRecords = func(host string, dnsServers []string, timeout int) ([]string, []string, error) {
		return nil, []string{"wildcard.example.net"}, nil
	}

	if !detectWildcard("example.com", []string{"8.8.8.8:53"}, 5) {
		t.Fatal("expected CNAME-only response to be treated as wildcard")
	}
}

func TestDetectWildcardReturnsFalseWithoutUsefulRecords(t *testing.T) {
	originalResolver := resolveDNSRecords
	defer func() {
		resolveDNSRecords = originalResolver
	}()

	resolveDNSRecords = func(host string, dnsServers []string, timeout int) ([]string, []string, error) {
		return nil, nil, nil
	}

	if detectWildcard("example.com", []string{"8.8.8.8:53"}, 5) {
		t.Fatal("expected empty response to return false")
	}
}

func TestDetectWildcardReturnsFalseOnResolveError(t *testing.T) {
	originalResolver := resolveDNSRecords
	defer func() {
		resolveDNSRecords = originalResolver
	}()

	resolveDNSRecords = func(host string, dnsServers []string, timeout int) ([]string, []string, error) {
		return nil, nil, errors.New("lookup failed")
	}

	if detectWildcard("example.com", []string{"8.8.8.8:53"}, 5) {
		t.Fatal("expected resolver error to return false")
	}
}

func TestGenerateRandomSubdomain(t *testing.T) {
	got := generateRandomSubdomain("example.com")
	if !strings.HasSuffix(got, ".example.com") {
		t.Fatalf("got %q, want suffix .example.com", got)
	}
	label := strings.TrimSuffix(got, ".example.com")
	if len(label) != 16 {
		t.Fatalf("random label length = %d, want 16", len(label))
	}
}

func TestNormalizeDNSServerSettings(t *testing.T) {
	settings, err := NormalizeDNSServerSettings(DNSServerSettings{
		Servers: []DNSServerSetting{
			{Address: "223.6.6.6", Name: "阿里DNS", Enabled: true},
			{Address: "223.6.6.6:53", Name: "duplicate", Enabled: false},
			{Address: "8.8.8.8:5353", Name: "  Google DNS  ", Enabled: true},
			{Address: "1.1.1.1:53", Enabled: false},
		},
	}, true)
	if err != nil {
		t.Fatalf("NormalizeDNSServerSettings returned error: %v", err)
	}

	want := []DNSServerSetting{
		{Address: "223.6.6.6:53", Name: "阿里DNS", Enabled: true},
		{Address: "8.8.8.8:5353", Name: "Google DNS", Enabled: true},
		{Address: "1.1.1.1:53", Name: defaultDNSServerName, Enabled: false},
	}
	if len(settings.Servers) != len(want) {
		t.Fatalf("expected %d servers, got %d: %#v", len(want), len(settings.Servers), settings.Servers)
	}
	for i := range want {
		if settings.Servers[i] != want[i] {
			t.Fatalf("server %d = %#v, want %#v", i, settings.Servers[i], want[i])
		}
	}
}

func TestNormalizeDNSServerSettingsRequiresEnabled(t *testing.T) {
	_, err := NormalizeDNSServerSettings(DNSServerSettings{
		Servers: []DNSServerSetting{
			{Address: "223.6.6.6:53", Name: "阿里DNS", Enabled: false},
		},
	}, true)
	if err == nil {
		t.Fatal("expected error when no DNS server is enabled")
	}
}

func TestNormalizeDNSServerSettingsMergesDuplicateEnabledState(t *testing.T) {
	settings, err := NormalizeDNSServerSettings(DNSServerSettings{
		Servers: []DNSServerSetting{
			{Address: "223.6.6.6:53", Name: "阿里DNS", Enabled: false},
			{Address: "223.6.6.6", Name: "duplicate", Enabled: true},
		},
	}, true)
	if err != nil {
		t.Fatalf("NormalizeDNSServerSettings returned error: %v", err)
	}
	if len(settings.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(settings.Servers))
	}
	if !settings.Servers[0].Enabled {
		t.Fatal("expected duplicate enabled state to be merged")
	}
}

func TestNormalizeDNSServerAddressRejectsInvalidInput(t *testing.T) {
	tests := []string{
		"",
		"example.com:53",
		"8.8.8.8:0",
		"8.8.8.8:65536",
		"8.8.8.8:not-a-port",
	}

	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			if _, err := NormalizeDNSServerAddress(tt); err == nil {
				t.Fatalf("expected %q to be rejected", tt)
			}
		})
	}
}
