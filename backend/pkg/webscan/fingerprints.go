package webscan

import (
	"embed"

	rootfingers "github.com/qiwentaidi/fingers"
	fingerssdk "github.com/qiwentaidi/fingers/lib"
)

const builtinFingerprintName = "static/finger.yaml"

//go:embed static/finger.yaml
var builtinFingerprintFS embed.FS

func BuiltinFingerprintOption() fingerssdk.FingersSDKOptions {
	return fingerssdk.WithFingerprintFS(builtinFingerprintFS, builtinFingerprintName)
}

func LoadBuiltinFingerprints() ([]rootfingers.FingerEntity, error) {
	return rootfingers.LoadFingerprintFromFS(builtinFingerprintFS, builtinFingerprintName)
}
