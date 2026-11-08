// Package toolnetwork enforces AI target traffic through a per-execution gateway.
package toolnetwork

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"hephaestus-backend/pkg/toolguard"
)

const ProtocolVersion = 1
const MaxBody = 4 << 20

type State struct {
	Version     int              `json:"version"`
	SessionID   string           `json:"sessionId"`
	ExecutionID string           `json:"executionId"`
	Tool        string           `json:"tool"`
	Expires     time.Time        `json:"expires"`
	Rules       toolguard.Config `json:"rules"`
	Token       string           `json:"token"`
}

func ReadState(path string) (State, error) {
	var state State
	b, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	if len(b) > 2<<20 {
		return state, fmt.Errorf("egress policy too large")
	}
	if err = json.Unmarshal(b, &state); err != nil {
		return state, err
	}
	if state.Version != ProtocolVersion || state.SessionID == "" || state.ExecutionID == "" || state.Tool == "" || state.Token == "" || !time.Now().Before(state.Expires) {
		return state, fmt.Errorf("egress policy missing, expired or incompatible")
	}
	if _, err = toolguard.Compile(state.Rules); err != nil {
		return state, err
	}
	return state, nil
}

func WriteState(path string, state State) error {
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Chmod(0644); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
