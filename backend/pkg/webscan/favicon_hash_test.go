package webscan

import "testing"

func TestFaviconIconHashRejectsNonImageResponses(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte("<!doctype html><html><body>not found</body></html>"),
		[]byte(`{"error":"favicon not found"}`),
		[]byte(""),
	} {
		if got := faviconIconHash(raw); got != "" {
			t.Fatalf("expected non-image response to have no icon hash, got %q", got)
		}
	}
}

func TestFaviconIconHashAcceptsImageSignatures(t *testing.T) {
	validImages := [][]byte{
		{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a},
		{0x00, 0x00, 0x01, 0x00, 0x01, 0x00},
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
	}
	for _, raw := range validImages {
		if got := faviconIconHash(raw); got == "" {
			t.Fatalf("expected image bytes to have an icon hash: %x", raw)
		}
	}
}
