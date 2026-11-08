// Package nodepython manages node-local Python selection. Probes run only in
// the node heartbeat process, never in an API request on a different machine.
package nodepython

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hephaestus-backend/pkg/database"
)

type Selection struct{ Mode, Path string }

var local struct {
	sync.RWMutex
	selection Selection
}
var probes sync.Map

func Current() Selection { local.RLock(); defer local.RUnlock(); return local.selection }
func Key(node database.ScanAgentNode) string {
	if node.LocalWorker {
		return "local:" + node.LastSeenHost
	}
	return node.AgentID
}

// Resolve checks only interpreter execution and version. It deliberately does
// not import venv, run ensurepip, or create a virtual environment.
func Resolve(ctx context.Context, selection Selection) (string, string, error) {
	requested := strings.TrimSpace(selection.Path)
	if selection.Mode == "manual" {
		if !filepath.IsAbs(requested) {
			return "", "", fmt.Errorf("Python interpreter must be an absolute path")
		}
		return probe(ctx, requested)
	}
	for _, name := range []string{"python3", "python"} {
		if p, err := exec.LookPath(name); err == nil {
			if p, v, err := probe(ctx, p); err == nil {
				return p, v, nil
			}
		}
	}
	return "", "", fmt.Errorf("no runnable Python 3 interpreter found in this node's PATH")
}

func probe(ctx context.Context, path string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "-I", "-c", "import json,sys; print(json.dumps({'major':sys.version_info.major,'version':sys.version.split()[0]}))")
	cmd.WaitDelay = time.Second
	var out limitedOutput
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("cannot run Python %s: %w", path, err)
	}
	var result struct {
		Major   int    `json:"major"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(out.data, &result); err != nil || result.Major != 3 || result.Version == "" {
		return "", "", fmt.Errorf("%s did not return a Python 3 version", path)
	}
	abs, err := filepath.Abs(path)
	return abs, result.Version, err
}

type limitedOutput struct{ data []byte }

func (b *limitedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if left := 4096 - len(b.data); left > 0 {
		if len(p) > left {
			p = p[:left]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}

// Poll is nonblocking so a slow interpreter cannot stall worker heartbeats.
func Poll(db *gorm.DB, node database.ScanAgentNode) {
	key := Key(node)
	if _, loaded := probes.LoadOrStore(key, true); loaded {
		return
	}
	// Load applied configuration before registration returns and work is claimed.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	var state database.AgentPythonConfig
	if db.WithContext(ctx).First(&state, "node_key=?", key).Error == nil {
		local.Lock()
		local.selection = Selection{state.Mode, state.Path}
		local.Unlock()
	}
	cancel()
	go func() { defer probes.Delete(key); _ = Process(context.Background(), db, key) }()
}

func Process(ctx context.Context, db *gorm.DB, key string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	db = db.WithContext(ctx)
	initial := database.AgentPythonConfig{NodeKey: key, Mode: "auto", RequestID: uuid.NewString(), RequestMode: "auto", Save: true, Status: "pending"}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&initial).Error; err != nil {
		return err
	}
	var state database.AgentPythonConfig
	if err := db.First(&state, "node_key = ?", key).Error; err != nil {
		return err
	}
	local.Lock()
	local.selection = Selection{state.Mode, state.Path}
	local.Unlock()
	if state.Status != "pending" {
		return nil
	}
	p, version, err := Resolve(ctx, Selection{state.RequestMode, state.RequestPath})
	now := time.Now()
	updates := map[string]any{"status": "ready", "detected_path": p, "detected_version": version, "error": "", "checked_at": now}
	if err != nil {
		updates["status"], updates["error"] = "error", err.Error()
	} else if state.Save {
		updates["mode"], updates["path"], updates["version"] = state.RequestMode, p, version
	}
	r := db.Model(&database.AgentPythonConfig{}).Where("node_key=? AND request_id=?", key, state.RequestID).Updates(updates)
	if r.Error == nil && r.RowsAffected == 1 && err == nil && state.Save {
		local.Lock()
		local.selection = Selection{state.RequestMode, p}
		local.Unlock()
	}
	return r.Error
}

// InterpreterPath pins selection when a run starts; missing Python is reported
// only if that run actually invokes a Python tool.
func InterpreterPath(s Selection) string {
	if s.Mode == "manual" || s.Path != "" {
		return s.Path
	}
	for _, name := range []string{"python3", "python"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return "python3"
}

func LocalKey() string { host, _ := os.Hostname(); return "local:" + host }
