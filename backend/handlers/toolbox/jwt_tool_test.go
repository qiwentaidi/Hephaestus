package toolbox

import "testing"

func TestBuildJWTKeyVariantsMarksBase64DecodedSecret(t *testing.T) {
	variants := buildJWTKeyVariants("c2VjcmV0", true)

	var decoded *jwtKeyVariant
	for index := range variants {
		if variants[index].Label == "Base64/Base64URL 还原" {
			decoded = &variants[index]
			break
		}
	}

	if decoded == nil {
		t.Fatal("expected base64 decoded variant")
	}
	if !decoded.Base64URLDecoded {
		t.Fatal("expected base64 decoded variant to be marked")
	}
	if decoded.DisplayValue != "c2VjcmV0" {
		t.Fatalf("display value mismatch: got %q", decoded.DisplayValue)
	}
	if string(decoded.Bytes) != "secret" {
		t.Fatalf("decoded bytes mismatch: got %q", string(decoded.Bytes))
	}
}

func TestBuildJWTKeyVariantsDoesNotMarkRawSecretAsBase64Decoded(t *testing.T) {
	variants := buildJWTKeyVariants("secret", true)
	if len(variants) == 0 {
		t.Fatal("expected raw variant")
	}
	if variants[0].Label != "原始文本" {
		t.Fatalf("first variant label mismatch: got %q", variants[0].Label)
	}
	if variants[0].Base64URLDecoded {
		t.Fatal("raw variant should not be marked as base64 decoded")
	}
}
