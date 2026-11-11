package toolruntime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testRuntime(t *testing.T) *Runtime {
	t.Helper()
	r, err := New(t.TempDir(), []string{"HEPH_NORMA_FIXTURE=shared-runtime"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := r.Close(); err != nil {
			t.Error(err)
		}
	})
	return r
}
func callRuntime(t *testing.T, r *Runtime, name string, args map[string]any) Outcome {
	t.Helper()
	b, _ := json.Marshal(args)
	o, err := r.Call(WithIdentity(context.Background(), "call-1", "execution-1"), name, b)
	if err != nil {
		t.Fatal(err)
	}
	return o
}
func awaitUpdate(t *testing.T, r *Runtime) Outcome {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		updates, err := r.DrainUpdates()
		if err != nil {
			t.Fatal(err)
		}
		if len(updates) > 0 {
			return updates[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("background task never acknowledged exit")
	return Outcome{}
}
func TestCatalogAndNativeBash(t *testing.T) {
	names := map[string]bool{}
	for _, tool := range Tools() {
		if names[tool.Name()] {
			t.Fatalf("duplicate %s", tool.Name())
		}
		names[tool.Name()] = true
	}
	if len(names) != 18 || names["exec"] {
		t.Fatalf("unexpected catalog: %v", names)
	}
	r := testRuntime(t)
	o := callRuntime(t, r, "Bash", map[string]any{"command": "printf '%s' \"$HEPH_NORMA_FIXTURE\""})
	if o.Status != "completed" || o.Output != "shared-runtime" {
		t.Fatalf("%+v", o)
	}
	o = callRuntime(t, r, "Bash", map[string]any{"command": "printf evidence; exit 7"})
	if o.Status != "failed" || o.ExitCode == nil || *o.ExitCode != 7 {
		t.Fatalf("%+v", o)
	}
	if _, err := r.Call(context.Background(), "exec", json.RawMessage(`{"command":"echo obsolete"}`)); err == nil {
		t.Fatal("retired exec dispatched")
	}
}
func TestBackgroundOwnershipAndDurableOutput(t *testing.T) {
	for _, name := range []string{"Bash", "Monitor"} {
		t.Run(name, func(t *testing.T) {
			r := testRuntime(t)
			args := map[string]any{"command": "printf '%s' \"$HEPH_NORMA_FIXTURE\"; sleep 0.08; printf complete"}
			if name == "Bash" {
				args["run_in_background"] = true
			}
			started := callRuntime(t, r, name, args)
			if started.Status != "running" || started.CompletedAt != nil || started.TaskID == "" {
				t.Fatalf("background launch misreported: %+v", started)
			}
			other := testRuntime(t)
			foreign := callRuntime(t, other, "TaskOutput", map[string]any{"task_id": started.TaskID})
			if foreign.Status != "failed" {
				t.Fatal("task escaped its runtime")
			}
			done := awaitUpdate(t, r)
			if done.Status != "completed" || done.ExecutionID != "execution-1" || done.ToolUseID != "call-1" || done.CompletedAt == nil || !strings.Contains(done.Output, "shared-runtimecomplete") {
				t.Fatalf("%+v", done)
			}
			if _, err := r.Close(); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(done.OutputFile)
			if err != nil || string(b) != "shared-runtimecomplete" {
				t.Fatalf("archive lost after close: %s %v", b, err)
			}
			updates, err := r.DrainUpdates()
			if err != nil || len(updates) != 0 {
				t.Fatalf("duplicate completion: %+v %v", updates, err)
			}
		})
	}
}
func TestTimeoutBackgroundAndStopAcknowledgement(t *testing.T) {
	r := testRuntime(t)
	o := callRuntime(t, r, "Bash", map[string]any{"command": "printf start; sleep 30", "timeout_ms": 10})
	if o.Status != "running" {
		t.Fatalf("timeout must yield background work: %+v", o)
	}
	callRuntime(t, r, "TaskStop", map[string]any{"task_id": o.TaskID})
	done := awaitUpdate(t, r)
	if done.Status != "cancelled" || done.CompletedAt == nil {
		t.Fatalf("%+v", done)
	}
}
func TestCloseCancelsDescendantsAndArchives(t *testing.T) {
	r := testRuntime(t)
	marker := filepath.Join(r.workdir, "escaped")
	callRuntime(t, r, "Bash", map[string]any{"command": "printf before; (sleep 0.3; touch '" + marker + "') & wait", "run_in_background": true})
	updates, err := r.Close()
	if err != nil || len(updates) != 1 || updates[0].Status != "cancelled" {
		t.Fatalf("%+v %v", updates, err)
	}
	time.Sleep(400 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("descendant survived runtime close")
	}
	if _, err := os.Stat(updates[0].OutputFile); err != nil {
		t.Fatal(err)
	}
}
