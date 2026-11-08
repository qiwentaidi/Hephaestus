package bevigil

import (
	"fmt"
	"testing"
)

func TestXxx(t *testing.T) {
	bh := FetchHosts("baidu.com", "")
	fmt.Printf("bh.Subdomains: %v\n", bh.Subdomains)
}
