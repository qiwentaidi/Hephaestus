package ipmetadata

import "testing"

func TestMergeLocation(t *testing.T) {
	t.Parallel()

	if got := MergeLocation("中国", "北京市", "北京市", "朝阳区"); got != "中国/北京市/朝阳区" {
		t.Fatalf("MergeLocation() = %q", got)
	}
}

func TestNormalizeISP(t *testing.T) {
	t.Parallel()

	for input, want := range map[string]string{
		"移动":  "中国移动",
		"联通":  "中国联通",
		"电信":  "中国电信",
		"教育网": "教育网",
	} {
		if got := NormalizeISP(input); got != want {
			t.Errorf("NormalizeISP(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLookupSkipsNonPublicIP(t *testing.T) {
	t.Parallel()

	resolver := NewResolver(t.TempDir())
	if got := resolver.Lookup("192.168.1.1"); got != (Metadata{}) {
		t.Fatalf("Lookup() = %#v, want an empty result", got)
	}
}

func TestResolverStatusIncludesManualInstallDetails(t *testing.T) {
	t.Parallel()

	status := NewResolver(t.TempDir()).Status()
	if status.State != StateIdle || status.Ready {
		t.Fatalf("unexpected initial status: %#v", status)
	}
	if status.DatabaseDir == "" || status.QQWryURL == "" || status.GeoLiteASNURL == "" {
		t.Fatalf("manual-install details are incomplete: %#v", status)
	}
}
