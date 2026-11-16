package version

import (
	"regexp"
	"testing"
)

var releaseVersionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func TestVersionFormat(t *testing.T) {
	for name, value := range map[string]string{
		"Version":        Version,
		"DesktopVersion": DesktopVersion,
	} {
		if !releaseVersionPattern.MatchString(value) {
			t.Fatalf("%s = %q must match vMAJOR.MINOR.PATCH", name, value)
		}
	}
}

// TestVersionLockstep guards the unified release scheme: Web and Desktop ship
// from one tag, so their version constants must never drift apart.
func TestVersionLockstep(t *testing.T) {
	if Version != DesktopVersion {
		t.Fatalf("Version (%s) and DesktopVersion (%s) must match for unified releases", Version, DesktopVersion)
	}
}
