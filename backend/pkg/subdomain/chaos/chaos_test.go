package chaos

import (
	"fmt"
	"testing"
)

func TestXxx(t *testing.T) {
	domains := FetchHosts("element-plus.org", "")
	fmt.Printf("domains: %v\n", domains)
}
