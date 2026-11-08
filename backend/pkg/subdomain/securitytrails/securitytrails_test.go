package securitytrails

import (
	"fmt"
	"testing"
)

func TestXxx(t *testing.T) {
	sh := FetchHosts("element-plus.org", "")
	fmt.Printf("sh.Subdomains: %v\n", sh.Subdomains)
}
