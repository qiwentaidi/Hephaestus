package cdncheck

import (
	"fmt"
	"testing"
)

func TestXxx(t *testing.T) {
	result, err := Detect("221.178.58.1")
	fmt.Printf("err: %v\n", err)
	fmt.Printf("result: %v\n", result)
}
