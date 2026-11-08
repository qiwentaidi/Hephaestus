package crack

import "testing"

func TestSupportedProtocolsIncludesDameng(t *testing.T) {
	protocols := SupportedProtocols()
	for _, protocol := range protocols {
		if protocol == "dameng" {
			return
		}
	}
	t.Fatal("expected dameng to be included in supported protocols")
}
