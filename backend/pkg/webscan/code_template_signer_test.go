package webscan

import (
	"testing"

	nucleitemplates "github.com/projectdiscovery/nuclei/v3/pkg/templates"
)

func TestEnsureCodeTemplateSignatureStats(t *testing.T) {
	verifier := "test-hephaestus-autosign"
	delete(nucleitemplates.SignatureStats, verifier)
	t.Cleanup(func() {
		delete(nucleitemplates.SignatureStats, verifier)
	})

	ensureCodeTemplateSignatureStats(verifier)

	if nucleitemplates.SignatureStats[verifier] == nil {
		t.Fatalf("expected signature stats counter for verifier %q", verifier)
	}
}

func TestEnsureCodeTemplateSignatureStatsIgnoresEmptyVerifier(t *testing.T) {
	ensureCodeTemplateSignatureStats(" ")

	if _, ok := nucleitemplates.SignatureStats[""]; ok {
		t.Fatal("expected empty verifier to be ignored")
	}
}
