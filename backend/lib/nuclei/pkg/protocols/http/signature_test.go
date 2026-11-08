package http

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

func TestRemovedAWSRequestSignatureIsRejected(t *testing.T) {
	var request Request
	err := yaml.Unmarshal([]byte("method: GET\npath: ['{{BaseURL}}']\nsignature: AWS\n"), &request)
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("signed requests must fail during template loading: %v", err)
	}
	request.Signature.Value = AWSSignature
	if err := request.Compile(nil); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("programmatic signed requests must also be rejected: %v", err)
	}
}
