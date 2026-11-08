package machineidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func codeForTest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func TestMachineIdentitySurvivesIdentifierChanges(t *testing.T) {
	dataDir := t.TempDir()
	first := MachineInfo{
		MachineCode: codeForTest("host-one"),
		CodeSource:  "host_id",
		HostID:      "host-one",
		Hostname:    "workstation",
		MACs:        []string{"aa:bb:cc:dd:ee:01"},
	}
	stabilizeMachineInfo(&first, dataDir)

	// HostID can temporarily disappear, and virtual interfaces can appear.
	second := MachineInfo{
		MachineCode: codeForTest("aa:bb:cc:dd:ee:01,aa:bb:cc:dd:ee:02"),
		CodeSource:  "mac",
		Hostname:    "workstation",
		MACs:        []string{"aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02"},
	}
	stabilizeMachineInfo(&second, dataDir)
	if second.MachineCode != first.MachineCode || second.CodeSource != "persisted" {
		t.Fatalf("machine code changed after HostID loss or NIC addition")
	}

	// Once an identifier has been seen, it also survives removal of the old NIC.
	third := MachineInfo{
		MachineCode: codeForTest("aa:bb:cc:dd:ee:02"),
		CodeSource:  "mac",
		Hostname:    "workstation",
		MACs:        []string{"aa:bb:cc:dd:ee:02"},
	}
	stabilizeMachineInfo(&third, dataDir)
	if third.MachineCode != first.MachineCode {
		t.Fatalf("machine code changed after NIC removal")
	}

	mode, err := os.Stat(filepath.Join(dataDir, machineIdentityFile))
	if err != nil {
		t.Fatal(err)
	}
	if mode.Mode().Perm() != 0600 {
		t.Fatalf("identity file permissions = %o, want 600", mode.Mode().Perm())
	}
}

func TestMachineIdentityRejectsDifferentStrongIdentifiers(t *testing.T) {
	dataDir := t.TempDir()
	first := MachineInfo{
		MachineCode: codeForTest("host-one"),
		HostID:      "host-one",
		Hostname:    "same-hostname",
		MACs:        []string{"aa:bb:cc:dd:ee:01"},
	}
	stabilizeMachineInfo(&first, dataDir)
	second := MachineInfo{
		MachineCode: codeForTest("host-two"),
		HostID:      "host-two",
		Hostname:    "same-hostname",
		MACs:        []string{"aa:bb:cc:dd:ee:02"},
	}
	stabilizeMachineInfo(&second, dataDir)
	if second.MachineCode == first.MachineCode {
		t.Fatalf("different machine with same hostname reused prior code")
	}
}

func TestMachineIdentityPreservesHostnameFallback(t *testing.T) {
	dataDir := t.TempDir()
	first := MachineInfo{
		MachineCode: codeForTest("workstation"),
		Hostname:    "workstation",
	}
	stabilizeMachineInfo(&first, dataDir)
	second := MachineInfo{
		MachineCode: codeForTest("host-one"),
		HostID:      "host-one",
		Hostname:    "workstation",
	}
	stabilizeMachineInfo(&second, dataDir)
	if second.MachineCode != first.MachineCode {
		t.Fatalf("machine code changed when stronger identifier appeared")
	}
}
