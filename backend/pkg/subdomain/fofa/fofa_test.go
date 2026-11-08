package fofa

import (
	"fmt"
	"hephaestus-backend/pkg/types"
	"testing"
)

func TestXxx(t *testing.T) {
	domains := FetchHosts("baidu.com", types.FofaAuth{
		Address: "https://fofa.info/",
		Key:     "",
		Email:   "",
	})
	fmt.Printf("domains: %v\n", domains)
}
