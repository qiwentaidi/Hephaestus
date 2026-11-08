package compiler

import (
	"context"
	"fmt"
	"testing"

	"github.com/projectdiscovery/nuclei/v3/pkg/types"
)

func TestSupportedPOCNativeModules(t *testing.T) {
	for _, name := range []string{"bytes", "fs", "mysql", "net", "postgres", "redis", "smb", "ssh", "structs"} {
		t.Run(name, func(t *testing.T) {
			result, err := requireNativeModule("nuclei/" + name)
			if err != nil || result["success"] != true {
				t.Fatalf("current POC module unavailable: result=%v error=%v", result, err)
			}
		})
	}
}

func TestDefaultRuntimeModulesRemainUsable(t *testing.T) {
	runtime := createNewRuntime()
	result, err := runtime.RunString(`structs.Unpack("B", structs.Pack("B", [42]))[0] === 42 && typeof bytes.Buffer === "function"`)
	if err != nil || result.ToBoolean() != true {
		t.Fatalf("implicit runtime modules unavailable: result=%v error=%v", result, err)
	}
}

func TestRemovedNativeModuleFailsExplicitly(t *testing.T) {
	if _, err := requireNativeModule("nuclei/kerberos"); err == nil {
		t.Fatal("removed native module must fail rather than silently execute an empty scan")
	}
}

func requireNativeModule(name string) (ExecuteResult, error) {
	program, err := SourceAutoMode(fmt.Sprintf("var module = require(%q); typeof module === 'object';", name), false)
	if err != nil {
		return nil, err
	}
	return New().ExecuteWithOptions(program, NewExecuteArgs(), &ExecuteOptions{
		Context: context.Background(), TimeoutVariants: types.NewTimeoutVariant(2),
	})
}
