package engineconfig

import (
	"strings"
	"testing"

	"github.com/projectdiscovery/nuclei/v3/pkg/types"
)

func TestRejectRemovedCloudFeatures(t *testing.T) {
	for _, set := range []func(*types.Options){
		func(o *types.Options) { o.EnableCloudUpload = true },
		func(o *types.Options) { o.ScanID = "scan-id" },
		func(o *types.Options) { o.ScanUploadFile = "results.jsonl" },
		func(o *types.Options) { o.AITemplatePrompt = "generate a template" },
	} {
		o := types.DefaultOptions()
		set(o)
		if err := ValidateOptions(o); err == nil || !strings.Contains(err.Error(), "not supported") {
			t.Fatalf("removed feature must fail explicitly before initialization: %v", err)
		}
	}
}

func TestRejectRemovedUncoverDiscovery(t *testing.T) {
	o := types.DefaultOptions()
	o.Uncover = true
	if err := ValidateOptions(o); err == nil || !strings.Contains(err.Error(), "provide resolved targets") {
		t.Fatalf("removed discovery must not produce an empty successful scan: %v", err)
	}
}
