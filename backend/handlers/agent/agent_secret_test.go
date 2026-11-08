package agent

import (
	"os"
	"sync"
	"testing"
)

func resetCredentialKeyForTest() {
	credentialKeyOnce = sync.Once{}
	credentialAESKey = nil
	credentialKeyErr = nil
}

func TestCredentialEncryptDecryptRoundTrip(t *testing.T) {
	_ = os.Setenv("HEPHAESTUS_CREDENTIAL_AES_KEY", "test-key-for-agent-secret")
	defer os.Unsetenv("HEPHAESTUS_CREDENTIAL_AES_KEY")
	resetCredentialKeyForTest()

	plain := "my-super-secret-password"
	enc, err := encryptCredential(plain)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if enc == plain {
		t.Fatalf("cipher text should not equal plain text")
	}
	if !isEncryptedCredential(enc) {
		t.Fatalf("expected encrypted prefix")
	}

	dec, err := decryptCredential(enc)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if dec != plain {
		t.Fatalf("decrypt mismatch: got %q want %q", dec, plain)
	}
}

func TestDecryptPlainTextBackwardCompatible(t *testing.T) {
	plain := "legacy-plain-password"
	dec, err := decryptCredential(plain)
	if err != nil {
		t.Fatalf("decrypt plaintext should not fail: %v", err)
	}
	if dec != plain {
		t.Fatalf("expected plaintext passthrough")
	}
}

func TestMaskedCredentialInput(t *testing.T) {
	if !isMaskedCredentialInput("********") {
		t.Fatalf("expected mask to be recognized")
	}
	if !isMaskedCredentialInput("************") {
		t.Fatalf("expected long star mask to be recognized")
	}
	if isMaskedCredentialInput("abc***") {
		t.Fatalf("invalid mask should not be recognized")
	}
}
