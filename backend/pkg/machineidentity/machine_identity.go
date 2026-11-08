package machineidentity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const machineIdentityFile = "machine-identity.json"

var machineIdentityMu sync.Mutex

// Keep the first machine code while a recognizable system identifier remains.
// This also preserves the old machine-code algorithm for existing installations.
type machineIdentityRecord struct {
	MachineCode    string   `json:"machineCode"`
	HostSignals    []string `json:"hostSignals,omitempty"`
	MACSignals     []string `json:"macSignals,omitempty"`
	HostnameSignal string   `json:"hostnameSignal,omitempty"`
}

func machineSignal(kind, value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(kind + ":" + value))
	return hex.EncodeToString(sum[:])
}

func machineIdentityFromInfo(info MachineInfo) machineIdentityRecord {
	record := machineIdentityRecord{
		MachineCode:    info.MachineCode,
		HostnameSignal: machineSignal("hostname", info.Hostname),
	}
	if signal := machineSignal("host", info.HostID); signal != "" {
		record.HostSignals = []string{signal}
	}
	for _, mac := range info.MACs {
		if signal := machineSignal("mac", mac); signal != "" {
			record.MACSignals = append(record.MACSignals, signal)
		}
	}
	record.MACSignals = uniqueSignals(record.MACSignals)
	return record
}

func uniqueSignals(signals []string) []string {
	if len(signals) == 0 {
		return nil
	}
	sort.Strings(signals)
	n := 0
	for _, signal := range signals {
		if n == 0 || signals[n-1] != signal {
			signals[n] = signal
			n++
		}
	}
	return signals[:n]
}

func signalsOverlap(a, b []string) bool {
	for _, left := range a {
		for _, right := range b {
			if left == right {
				return true
			}
		}
	}
	return false
}

func sameMachineIdentity(saved, current machineIdentityRecord) bool {
	if saved.MachineCode == current.MachineCode ||
		signalsOverlap(saved.HostSignals, current.HostSignals) ||
		signalsOverlap(saved.MACSignals, current.MACSignals) {
		return true
	}
	// A hostname is only a fallback when either observation has no stronger ID.
	savedHasStrong := len(saved.HostSignals) > 0 || len(saved.MACSignals) > 0
	currentHasStrong := len(current.HostSignals) > 0 || len(current.MACSignals) > 0
	return (!savedHasStrong || !currentHasStrong) &&
		saved.HostnameSignal != "" && saved.HostnameSignal == current.HostnameSignal
}

func validMachineCode(code string) bool {
	if len(code) != 64 {
		return false
	}
	_, err := hex.DecodeString(code)
	return err == nil
}

func stabilizeMachineInfo(info *MachineInfo, dataDir string) {
	if info == nil || !validMachineCode(info.MachineCode) {
		return
	}
	machineIdentityMu.Lock()
	defer machineIdentityMu.Unlock()

	path := filepath.Join(dataDir, machineIdentityFile)
	current := machineIdentityFromInfo(*info)
	var saved machineIdentityRecord
	previous, readErr := os.ReadFile(path)
	if readErr == nil && json.Unmarshal(previous, &saved) == nil && validMachineCode(saved.MachineCode) {
		if sameMachineIdentity(saved, current) {
			if saved.MachineCode != info.MachineCode {
				info.MachineCode = saved.MachineCode
				info.CodeSource = "persisted"
			}
			current.MachineCode = saved.MachineCode
			current.HostSignals = uniqueSignals(append(saved.HostSignals, current.HostSignals...))
			current.MACSignals = uniqueSignals(append(saved.MACSignals, current.MACSignals...))
			if current.HostnameSignal == "" {
				current.HostnameSignal = saved.HostnameSignal
			}
		}
	}
	raw, err := json.Marshal(current)
	if err == nil && !bytes.Equal(previous, raw) {
		_ = writeMachineIdentity(path, raw)
	}
}

func writeMachineIdentity(path string, raw []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".machine-identity-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(raw); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
