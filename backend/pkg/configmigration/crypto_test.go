package configmigration

import (
	"reflect"
	"strings"
	"testing"
)

func TestEncryptSecretsRoundTrip(t *testing.T) {
	input := map[string]any{
		"fofa": map[string]any{
			"api": "https://fofa.info",
			"key": "fofa-secret",
		},
		"providers": []any{
			map[string]any{
				"id":    "provider-1",
				"model": "qwen-plus",
				"key":   "sk-test",
			},
		},
		"enabled": true,
		"note":    "keep me",
	}

	encrypted, err := encryptSecrets(input)
	if err != nil {
		t.Fatalf("encryptSecrets() error = %v", err)
	}
	encryptedMap := encrypted.(map[string]any)
	fofa := encryptedMap["fofa"].(map[string]any)
	if fofa["api"] != "https://fofa.info" {
		t.Fatalf("non-secret field changed: %v", fofa["api"])
	}
	fofaKey, _ := fofa["key"].(string)
	if !strings.HasPrefix(fofaKey, encryptedValuePrefix) {
		t.Fatalf("fofa key was not encrypted: %q", fofaKey)
	}
	if fofaKey == "fofa-secret" {
		t.Fatal("fofa key still contains plaintext")
	}

	decrypted, err := decryptSecrets(encrypted)
	if err != nil {
		t.Fatalf("decryptSecrets() error = %v", err)
	}
	if !reflect.DeepEqual(decrypted, input) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", decrypted, input)
	}
}

func TestEncryptSecretsIsIdempotent(t *testing.T) {
	input := map[string]any{"key": "secret"}
	encrypted, err := encryptSecrets(input)
	if err != nil {
		t.Fatalf("encryptSecrets() error = %v", err)
	}
	first := encrypted.(map[string]any)["key"].(string)

	again, err := encryptSecrets(encrypted)
	if err != nil {
		t.Fatalf("encryptSecrets() second error = %v", err)
	}
	second := again.(map[string]any)["key"].(string)
	if first != second {
		t.Fatalf("already encrypted value was re-encrypted: %q != %q", first, second)
	}
}

func TestDecryptSecretsLeavesPlaintextUntouched(t *testing.T) {
	input := map[string]any{"key": "plain-text"}
	decrypted, err := decryptSecrets(input)
	if err != nil {
		t.Fatalf("decryptSecrets() error = %v", err)
	}
	if decrypted.(map[string]any)["key"] != "plain-text" {
		t.Fatalf("plaintext was modified: %#v", decrypted)
	}
}

func TestDecryptSecretsRejectsInvalidPayload(t *testing.T) {
	for _, value := range []string{
		encryptedValuePrefix + "not-base64!!",
		encryptedValuePrefix + "AAAA",
	} {
		_, err := decryptSecrets(map[string]any{"key": value})
		if err == nil {
			t.Fatalf("decryptSecrets(%q) expected error", value)
		}
	}
}

func TestEmptySensitiveValuesAreNotEncrypted(t *testing.T) {
	input := map[string]any{"key": "", "token": "   "}
	encrypted, err := encryptSecrets(input)
	if err != nil {
		t.Fatalf("encryptSecrets() error = %v", err)
	}
	result := encrypted.(map[string]any)
	if result["key"] != "" {
		t.Fatalf("empty key changed: %#v", result["key"])
	}
	if result["token"] != "   " {
		t.Fatalf("whitespace token changed: %#v", result["token"])
	}
}
