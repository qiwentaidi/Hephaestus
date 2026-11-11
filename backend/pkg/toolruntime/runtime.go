package toolruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
	"github.com/google/uuid"
)

type Prepare func(context.Context, string, json.RawMessage, *actool.ToolContext) (json.RawMessage, *actool.ToolContext, error)

type Outcome struct {
	Runtime     string     `json:"runtime"`
	ToolName    string     `json:"tool_name"`
	ToolUseID   string     `json:"tool_call_id,omitempty"`
	ExecutionID string     `json:"execution_id,omitempty"`
	Status      string     `json:"status"`
	TaskID      string     `json:"task_id,omitempty"`
	Output      string     `json:"output"`
	OutputFile  string     `json:"output_file,omitempty"`
	Error       string     `json:"error,omitempty"`
	ExitCode    *int       `json:"exit_code,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func (o Outcome) JSON() string { b, _ := json.Marshal(o); return string(b) }
func ParseOutcome(s string) (Outcome, bool) {
	var o Outcome
	err := json.Unmarshal([]byte(s), &o)
	return o, err == nil && o.Runtime == "norma" && o.ToolName != ""
}

type identityKey struct{}
type identity struct{ call, execution string }

func WithIdentity(ctx context.Context, call, execution string) context.Context {
	return context.WithValue(ctx, identityKey{}, identity{call, execution})
}

type Runtime struct {
	mu      sync.Mutex
	manager *actool.Manager
	prepare Prepare
	archive string
	workdir string
	env     []string
	tracked map[string]Outcome
	closed  bool
	cleaned bool
}

func New(workdir string, env []string, prepare Prepare) (*Runtime, error) {
	if workdir == "" {
		return nil, fmt.Errorf("tool runtime requires a workspace")
	}
	if err := os.MkdirAll(workdir, 0700); err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(workdir)
	if err != nil {
		return nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	archive := filepath.Join(root, "cmd-output")
	if err = os.MkdirAll(archive, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(archive)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("invalid tool output directory")
	}
	id := uuid.NewString()
	archive = filepath.Join(archive, "norma-"+id)
	if err = os.Mkdir(archive, 0700); err != nil {
		return nil, err
	}
	m, err := actool.NewManager(id)
	if err != nil {
		return nil, err
	}
	return &Runtime{manager: m, prepare: prepare, archive: archive, workdir: root, env: append([]string(nil), env...), tracked: map[string]Outcome{}}, nil
}

func (r *Runtime) Wrap(t actool.CoreTool) actool.CoreTool { return &boundTool{CoreTool: t, runtime: r} }

type boundTool struct {
	actool.CoreTool
	runtime *Runtime
}

func (b *boundTool) IsConcurrencySafe(json.RawMessage) bool { return false }
func (b *boundTool) AcceptsRawInput() bool {
	if t, ok := b.CoreTool.(interface{ AcceptsRawInput() bool }); ok {
		return t.AcceptsRawInput()
	}
	return false
}
func (b *boundTool) Call(ctx context.Context, input json.RawMessage, tc *actool.ToolContext) (actool.Result, error) {
	o, result, err := b.runtime.invoke(ctx, b.CoreTool, input, tc)
	if err != nil {
		return actool.Errorf(err.Error()), nil
	}
	if b.Name() == "Bash" || b.Name() == "Monitor" {
		// Keep the metadata envelope parseable through the SDK's global cap.
		for len(o.JSON()) > 24000 && len(o.Output) > 1000 {
			o.Output = o.Output[:len(o.Output)/2]
			o.Output = strings.ToValidUTF8(o.Output, "")
		}
		return actool.Result{Content: actool.Text(o.JSON()).Content, IsError: o.Status == "failed"}, nil
	}
	return result, nil
}

func (r *Runtime) Call(ctx context.Context, name string, input json.RawMessage) (Outcome, error) {
	t, ok := Lookup(name)
	if !ok {
		return Outcome{}, fmt.Errorf("unsupported runtime tool: %s", name)
	}
	o, _, err := r.invoke(ctx, t, input, nil)
	return o, err
}

func (r *Runtime) invoke(ctx context.Context, t actool.CoreTool, input json.RawMessage, tc *actool.ToolContext) (Outcome, actool.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := Outcome{Runtime: "norma", ToolName: t.Name(), Status: "completed", StartedAt: time.Now()}
	id, _ := ctx.Value(identityKey{}).(identity)
	o.ToolUseID, o.ExecutionID = id.call, id.execution
	if r.closed {
		return o, actool.Result{}, fmt.Errorf("tool runtime is closed")
	}
	if !Available(t.Name()) {
		return o, actool.Result{}, fmt.Errorf("tool is unavailable: %s", t.Name())
	}
	if err := ctx.Err(); err != nil {
		return o, actool.Result{}, err
	}
	if err := actool.ValidateInput(t.InputSchema(), input); err != nil {
		return o, actool.Result{}, err
	}
	// Hard-linked archives stay live. On another volume, refresh their snapshot
	// before the model reads them through Read/Bash or inspects task controls.
	for id, pending := range r.tracked {
		if task, ok := r.manager.Get(id); ok {
			if err := copyOutput(task.OutputPath, pending.OutputFile); err != nil {
				return o, actool.Result{}, err
			}
		}
	}
	local := actool.ToolContext{WorkingDir: r.workdir, Env: r.env, OutputDir: r.archive, MaxOutputChars: 30000}
	if tc != nil {
		local = *tc
		o.ToolUseID = tc.ToolUseID
	}
	local.Tasks = r.manager
	local.OutputDir = filepath.Join(r.archive, uuid.NewString())
	if err := os.Mkdir(local.OutputDir, 0700); err != nil {
		return o, actool.Result{}, err
	}
	pc := permission.Context{Mode: permission.ModeBypass, WorkingDir: local.WorkingDir}
	if dec := t.CheckPermissions(ctx, input, pc); dec.Behavior == permission.Deny {
		return o, actool.Result{}, fmt.Errorf("%s", dec.Message)
	}
	var err error
	var commandInput struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(input, &commandInput)
	fields := strings.Fields(commandInput.Command)
	autoBackground := len(fields) > 0 && filepath.Base(fields[0]) != "sleep"
	if r.prepare != nil {
		var prepared *actool.ToolContext
		input, prepared, err = r.prepare(ctx, t.Name(), input, &local)
		if err != nil {
			return o, actool.Result{}, err
		}
		if prepared == nil {
			return o, actool.Result{}, fmt.Errorf("missing prepared tool context")
		}
		local = *prepared
	}
	if t.Name() == "Bash" || t.Name() == "Monitor" {
		return r.command(ctx, t.Name(), input, &local, o, autoBackground)
	}
	res, err := t.Call(ctx, input, &local)
	o.Output = res.Flatten()
	// Capture's files are created in a private per-call directory. Obtain the
	// archive structurally instead of parsing a model-visible output pointer.
	if files, readErr := os.ReadDir(local.OutputDir); readErr == nil {
		for _, file := range files {
			if !file.IsDir() {
				o.OutputFile = filepath.Join(local.OutputDir, file.Name())
				break
			}
		}
	}
	if !utf8.ValidString(o.Output) || strings.ContainsRune(o.Output, 0) {
		if o.OutputFile == "" {
			o.OutputFile = filepath.Join(local.OutputDir, "output.bin")
			if e := os.WriteFile(o.OutputFile, []byte(o.Output), 0600); e != nil {
				return o, res, e
			}
		}
		o.Output = strings.ReplaceAll(strings.ToValidUTF8(o.Output, "�"), "\x00", "�")
	}
	now := time.Now()
	o.CompletedAt = &now
	if err != nil {
		o.Error = err.Error()
	} else if res.IsError {
		o.Error = o.Output
	}
	if o.Error != "" {
		o.Status = "failed"
	}
	if ctx.Err() != nil {
		err = ctx.Err()
		o.Error = err.Error()
		o.Status = "failed"
	}
	return o, res, err
}

// Use Norma's process manager for Bash as well as Monitor. The SDK's foreground
// tool forgets (and deletes) its full output after returning a 30 KB tail. The
// host must archive that file and obtain structured exit metadata BEFORE Forget.
// Spawn, process groups, limits, task control and PTYs remain owned by Norma.
func (r *Runtime) command(ctx context.Context, name string, input json.RawMessage, tc *actool.ToolContext, o Outcome, autoBackground bool) (Outcome, actool.Result, error) {
	var in struct {
		Command     string `json:"command"`
		Timeout     int    `json:"timeout_ms"`
		Background  bool   `json:"run_in_background"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return o, actool.Result{}, err
	}
	if strings.TrimSpace(in.Command) == "" {
		return o, actool.Result{}, fmt.Errorf("command is required")
	}
	if in.Timeout <= 0 {
		in.Timeout = 120000
	}
	if in.Timeout > 600000 {
		in.Timeout = 600000
	}
	bg := in.Background || name == "Monitor"
	if bg && actool.BackgroundTasksDisabled() {
		return o, actool.Result{}, fmt.Errorf("background tasks are disabled")
	}
	kind := actool.KindBash
	if name == "Monitor" {
		kind = actool.KindMonitor
	}
	task, err := r.manager.Spawn(actool.SpawnSpec{Command: in.Command, Description: in.Description, WorkingDir: tc.WorkingDir, Env: tc.Env, Kind: kind})
	if err != nil {
		return o, actool.Result{}, err
	}
	o.TaskID = task.ID
	o.OutputFile = filepath.Join(tc.OutputDir, "output.txt")
	if !bg {
		_, finished := r.manager.Wait(ctx, task.ID, time.Duration(in.Timeout)*time.Millisecond)
		if !finished && ctx.Err() == nil && autoBackground && !actool.BackgroundTasksDisabled() {
			bg = true
		}
		if !finished && !bg {
			r.manager.Kill(task.ID)
			_, ack := r.manager.Wait(context.Background(), task.ID, 10*time.Second)
			if !ack {
				return o, actool.Result{}, fmt.Errorf("process exit unconfirmed: %s", task.ID)
			}
			err = ctx.Err()
			if err == nil {
				err = context.DeadlineExceeded
			}
		}
	}
	if bg {
		r.manager.Background(task.ID)
		o.Status = "running"
		o.CompletedAt = nil
		o.Output = "Command running in background; use TaskOutput, TaskList or TaskStop with task_id " + task.ID
		if e := copyOutput(task.OutputPath, o.OutputFile); e != nil {
			return o, actool.Result{}, e
		}
		r.tracked[task.ID] = o
		return o, actool.Text(o.JSON()), nil
	}
	info, _ := r.manager.Get(task.ID)
	if e := copyOutput(info.OutputPath, o.OutputFile); e != nil {
		return o, actool.Result{}, e
	}
	o.Output, _ = r.manager.Output(task.ID, 30000)
	o.ExitCode = &info.ExitCode
	o.CompletedAt = &info.EndTime
	if info.Status != actool.TaskCompleted {
		o.Status = "failed"
		o.Error = fmt.Sprintf("command failed (exit %d, reason %s)", info.ExitCode, info.Reason)
	}
	if err != nil {
		o.Error = err.Error()
		o.Status = "failed"
	}
	r.manager.Forget(task.ID)
	o.Output = strings.ReplaceAll(strings.ToValidUTF8(o.Output, "�"), "\x00", "�")
	return o, actool.Result{Content: actool.Text(o.Output).Content, IsError: o.Status != "completed"}, err
}

// DrainUpdates reports only acknowledged process exits, never merely a kill
// request. Callers update the original execution ID, not insert a second call.
func (r *Runtime) DrainUpdates() ([]Outcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.updates()
}
func (r *Runtime) updates() ([]Outcome, error) {
	var out []Outcome
	for id, o := range r.tracked {
		t, ok := r.manager.Get(id)
		if !ok || t.EndTime.IsZero() {
			continue
		}
		if err := copyOutput(t.OutputPath, o.OutputFile); err != nil {
			return out, err
		}
		o.CompletedAt, o.ExitCode = &t.EndTime, &t.ExitCode
		o.Status = "completed"
		switch t.Status {
		case actool.TaskFailed:
			o.Status = "failed"
			o.Error = fmt.Sprintf("command failed (exit %d, reason %s)", t.ExitCode, t.Reason)
		case actool.TaskKilled:
			o.Status = "cancelled"
			o.Error = "command stopped"
		}
		o.Output, _ = r.manager.Output(id, 30000)
		o.Output = strings.ReplaceAll(strings.ToValidUTF8(o.Output, "�"), "\x00", "�")
		out = append(out, o)
		delete(r.tracked, id)
	}
	return out, nil
}

// Preserve Norma's interactive-shell/dispatch notifications. Process completions
// use the richer Outcome above, so they must not be delivered twice.
func (r *Runtime) DrainNotifications() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var notes []string
	for _, n := range r.manager.DrainNotifications() {
		if _, isProcess := r.manager.Get(n.TaskID); !isProcess {
			notes = append(notes, n.String())
		}
	}
	return notes
}

func copyOutput(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if info, e := os.Lstat(dst); e == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("invalid output archive")
		}
		if source, err := in.Stat(); err == nil && os.SameFile(source, info) {
			return nil
		}
	} else if os.IsNotExist(e) {
		// Keep the full stream readable while it runs and after Norma removes
		// its own temporary path. Fall back to copies across filesystems.
		if err := os.Link(src, dst); err == nil {
			return nil
		}
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return errors.Join(err, out.Close())
}

func (r *Runtime) Close() ([]Outcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cleaned {
		return nil, nil
	}
	r.closed = true
	r.manager.KillAll()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, t := range r.manager.List() {
		if _, ok := r.manager.Wait(ctx, t.ID, 10*time.Second); !ok {
			return nil, fmt.Errorf("process exit unconfirmed: %s", t.ID)
		}
	}
	out, err := r.updates()
	if err != nil {
		return out, err
	}
	r.manager.Cleanup()
	r.cleaned = true
	return out, nil
}
