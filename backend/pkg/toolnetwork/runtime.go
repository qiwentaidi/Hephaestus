package toolnetwork

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"hephaestus-backend/pkg/toolguard"
)

const DefaultImage = "hephaestus-egress:local"

type Runtime struct {
	ctx                    context.Context
	Root, SessionID, Image string
	Rules                  func() (toolguard.Config, error)
	mu                     sync.Mutex
	executions             map[*Execution]bool
	closed                 bool
}
type Execution struct {
	runtime                                       *Runtime
	ID, tool, dir, volume, gateway, worker, token string
	Proxy                                         *url.URL
	Roots                                         *x509.CertPool
	stop                                          chan struct{}
	done                                          chan struct{}
	mu                                            sync.Mutex
	closed                                        bool
	blockedOffset                                 int
	closeErr                                      error
}

func NewRuntime(ctx context.Context, root, session string, rules func() (toolguard.Config, error)) *Runtime {
	image := os.Getenv("HEPHAESTUS_EGRESS_IMAGE")
	if image == "" {
		image = DefaultImage
	}
	return &Runtime{ctx: ctx, Root: root, SessionID: session, Image: image, Rules: rules, executions: map[*Execution]bool{}}
}
func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func docker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("egress Docker operation failed: %w: %s", err, strings.TrimSpace(string(b)))
	}
	return strings.TrimSpace(string(b)), nil
}
func (r *Runtime) Begin(ctx context.Context, tool, id string) (e *Execution, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.ctx.Err() != nil {
		return nil, fmt.Errorf("egress execution stopped")
	}
	if r.Rules == nil {
		return nil, fmt.Errorf("egress rules unavailable")
	}
	if id == "" {
		id = randomID()
	}
	if len(id) > 128 || strings.ContainsAny(id, "/\\") || id == "." || id == ".." {
		return nil, fmt.Errorf("invalid execution identity")
	}
	if err = os.MkdirAll(r.Root, 0700); err != nil {
		return nil, err
	}
	archive := filepath.Join(r.Root, "cmd-output")
	if err = os.MkdirAll(archive, 0700); err != nil {
		return nil, err
	}
	if info, statErr := os.Lstat(archive); statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("invalid audit archive directory")
	}

	if _, err = docker(ctx, "image", "inspect", r.Image); err != nil {
		return nil, fmt.Errorf("受控出口镜像不可用；请构建 %s，不会回退直连: %w", r.Image, err)
	}
	dir, err := os.MkdirTemp("", "hephaestus-egress-")
	if err != nil {
		return nil, err
	}
	uid := randomID()
	e = &Execution{runtime: r, ID: id, tool: tool, dir: dir, volume: "heph-egress-" + uid, gateway: "heph-gateway-" + uid, worker: "heph-tool-" + uid, token: randomID(), stop: make(chan struct{}), done: make(chan struct{})}
	// The policy and audit mounts are never exposed to tool containers.
	for _, name := range []string{"policy", "audit"} {
		if err = os.Mkdir(filepath.Join(dir, name), 0755); err != nil {
			os.RemoveAll(dir)
			return nil, err
		}
	}
	if err = os.Chmod(filepath.Join(dir, "audit"), 0777); err != nil {
		return nil, err
	}
	if err = e.renew(); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	go e.heartbeat()
	cleanup := e
	defer func() {
		if err != nil {
			cleanupErr := cleanup.closeResources()
			err = errors.Join(err, cleanupErr)
			if cleanupErr != nil {
				r.executions[cleanup] = true
			}
		}
	}()
	if _, err = docker(ctx, "volume", "create", e.volume); err != nil {
		return nil, err
	}
	_, err = docker(ctx, "run", "-d", "--rm", "--pull=never", "--name", e.gateway, "--add-host", "host.docker.internal:host-gateway", "--label", "hephaestus.egress=1", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=64", "--memory=256m", "--mount", "type=volume,src="+e.volume+",dst=/channel", "--mount", "type=bind,src="+filepath.Join(dir, "policy")+",dst=/policy,readonly", "--mount", "type=bind,src="+filepath.Join(dir, "audit")+",dst=/audit", "-p", "127.0.0.1::8080", "--entrypoint", "/usr/local/bin/hephaestus-egress", r.Image, "serve")
	if err != nil {
		return nil, err
	}
	ready := filepath.Join(dir, "audit", "ready")
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if b, readErr := os.ReadFile(ready); readErr == nil {
			e.Roots = x509.NewCertPool()
			if !e.Roots.AppendCertsFromPEM(b) {
				return nil, fmt.Errorf("invalid gateway CA")
			}
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			logs, _ := docker(ctx, "logs", e.gateway)
			return nil, fmt.Errorf("gateway did not become ready: %s", logs)
		case <-ticker.C:
		}
	}
	address, err := docker(ctx, "port", e.gateway, "8080/tcp")
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(address, "127.0.0.1:") {
		return nil, fmt.Errorf("gateway publication is not loopback-only")
	}
	e.Proxy, err = url.Parse("http://" + address)
	if err != nil {
		return nil, err
	}
	r.executions[e] = true
	return e, nil
}
func (e *Execution) renew() error {
	rules, err := e.runtime.Rules()
	if err != nil {
		return err
	}
	select {
	case <-e.stop:
		return context.Canceled
	case <-e.runtime.ctx.Done():
		return e.runtime.ctx.Err()
	default:
	}

	return WriteState(filepath.Join(e.dir, "policy", "state.json"), State{Version: ProtocolVersion, SessionID: e.runtime.SessionID, ExecutionID: e.ID, Tool: e.tool, Token: e.token, Expires: time.Now().Add(5 * time.Second), Rules: rules})
}
func (e *Execution) heartbeat() {
	defer close(e.done)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-e.runtime.ctx.Done():
			_ = os.Remove(filepath.Join(e.dir, "policy", "state.json"))
			return
		case <-e.stop:
			return
		case <-ticker.C:
			if e.renew() != nil {
				_ = os.Remove(filepath.Join(e.dir, "policy", "state.json"))
				return
			}
		}
	}
}
func (e *Execution) Close() error {
	err := e.closeResources()
	e.runtime.mu.Lock()
	if err == nil {
		delete(e.runtime.executions, e)
	}
	e.runtime.mu.Unlock()
	return err
}
func (e *Execution) closeResources() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return e.closeErr
	}
	e.closed = true
	close(e.stop)
	_ = os.Remove(filepath.Join(e.dir, "policy", "state.json"))
	select {
	case <-e.done:
	case <-time.After(6 * time.Second):
		e.closeErr = errors.New("policy heartbeat did not acknowledge shutdown")
	}
	_ = os.Remove(filepath.Join(e.dir, "policy", "state.json"))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	// Remove/acknowledge execution processes before the parent can complete.
	for _, name := range []string{e.worker, e.gateway} {
		e.closeErr = errors.Join(e.closeErr, removeContainer(ctx, name))
	}

	if _, err := docker(ctx, "volume", "rm", e.volume); err != nil && !strings.Contains(err.Error(), "no such volume") {
		e.closeErr = errors.Join(e.closeErr, err)
	}
	// Audit belongs to the trusted parent, outside the writable tool workspace.
	if b, err := os.ReadFile(filepath.Join(e.dir, "audit", "requests.jsonl")); err == nil {
		dst := filepath.Join(e.runtime.Root, "cmd-output", "egress-"+e.ID+".jsonl")
		if err = os.WriteFile(dst, b, 0600); err != nil {
			e.closeErr = errors.Join(e.closeErr, err)
		}
	}
	if e.closeErr == nil {
		_ = os.RemoveAll(e.dir)
	}
	if e.closeErr != nil {
		e.closeErr = errors.Join(ErrStopUnconfirmed, e.closeErr)
	}
	return e.closeErr
}
func (r *Runtime) Close() error {
	r.mu.Lock()
	r.closed = true
	list := make([]*Execution, 0, len(r.executions))
	for e := range r.executions {
		list = append(list, e)
	}
	r.mu.Unlock()
	var err error
	for _, e := range list {
		err = errors.Join(err, e.Close())
	}
	return err
}
func (e *Execution) Command(ctx context.Context, command string, args []string, cwd string, interactive bool) (*exec.Cmd, error) {
	root, err := filepath.EvalSymlinks(e.runtime.Root)
	if err != nil {
		return nil, err
	}
	if cwd == "" {
		cwd = root
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, cwd)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("tool cwd escapes session")
	}
	for _, sub := range []string{"cmd-output", "tmp"} {
		p := filepath.Join(root, sub)
		if err = os.MkdirAll(p, 0700); err != nil {
			return nil, err
		}
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("invalid session runtime directory")
		}
	}
	argv := []string{"run", "--rm", "--pull=never", "--name", e.worker, "--label", "hephaestus.egress=1", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=256", "--memory=1g", "--user", strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()), "--mount", "type=volume,src=" + e.volume + ",dst=/channel,readonly", "--mount", "type=bind,src=" + root + ",dst=" + root, "--mount", "type=bind,src=" + filepath.Join(root, "cmd-output") + ",dst=" + filepath.Join(root, "cmd-output") + ",readonly", "--workdir", cwd, "--env", "HOME=" + root, "--env", "HEPHAESTUS_SESSION_WORKDIR=" + root, "--env", "PYTHONDONTWRITEBYTECODE=1", "--entrypoint", "/usr/local/bin/hephaestus-egress"}
	if interactive {
		argv = append(argv, "-i")
	}
	argv = append(argv, e.runtime.Image, "relay", "--", command)
	argv = append(argv, args...)
	cmd := exec.CommandContext(ctx, "docker", argv...)
	// Never forward the parent's database/model/Agent environment into tools.
	cmd.Env = os.Environ()
	cmd.Cancel = func() error {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := docker(c, "rm", "-f", e.worker)
		if err != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return err
	}
	return cmd, nil
}
func (e *Execution) Transport() http.RoundTripper {
	transport := &http.Transport{Proxy: http.ProxyURL(e.Proxy), TLSClientConfig: &tls.Config{RootCAs: e.Roots, MinVersion: tls.VersionTLS12}, ProxyConnectHeader: http.Header{"Proxy-Authorization": {"Bearer " + e.token}}, DisableKeepAlives: true, ResponseHeaderTimeout: 60 * time.Second}
	return &authenticatedTransport{base: transport, token: e.token}
}

type authenticatedTransport struct {
	base  *http.Transport
	token string
}

func (t *authenticatedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header.Set("Proxy-Authorization", "Bearer "+t.token)
	response, err := t.base.RoundTrip(clone)
	if err == nil && response.Header.Get("X-Hephaestus-Egress-Blocked") == "1" {
		match := &toolguard.Match{
			RuleID:      response.Header.Get("X-Hephaestus-Egress-Rule-ID"),
			RuleName:    response.Header.Get("X-Hephaestus-Egress-Rule-Name"),
			MatchedText: response.Header.Get("X-Hephaestus-Egress-Matched-Text"),
			Message:     response.Header.Get("X-Hephaestus-Egress-Rule-Message"),
		}
		response.Body.Close()
		if match.RuleID != "" || match.RuleName != "" || match.MatchedText != "" || match.Message != "" {
			return nil, BlockedWithMatch("actual request rejected by egress gateway", match)
		}
		return nil, Blocked("actual request rejected by egress gateway")
	}
	return response, err
}
func Blocked(reason string) error {
	return toolguard.NewBlockedError(&toolguard.Match{RuleID: "enforced-network-egress", RuleName: "强制受控网络出口", Message: reason})
}
func BlockedWithMatch(reason string, match *toolguard.Match) error {
	if match == nil {
		return Blocked(reason)
	}
	copy := *match
	if strings.TrimSpace(copy.Message) == "" {
		copy.Message = reason
	}
	return toolguard.NewBlockedError(&copy)
}

func (e *Execution) Blocked() bool {
	_, blocked := e.BlockedMatch()
	return blocked
}
func (e *Execution) BlockedMatch() (*toolguard.Match, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	b, err := os.ReadFile(filepath.Join(e.dir, "audit", "requests.jsonl"))
	if err != nil {
		return nil, false
	}
	if e.blockedOffset > len(b) {
		e.blockedOffset = 0
	}
	fresh := b[e.blockedOffset:]
	// Only consume complete audit lines while the gateway may be appending.
	if end := strings.LastIndexByte(string(fresh), '\n'); end >= 0 {
		fresh = fresh[:end+1]
		e.blockedOffset += len(fresh)
	} else {
		return nil, false
	}
	for _, line := range strings.Split(string(fresh), "\n") {
		var v struct {
			Blocked     bool   `json:"blocked"`
			RuleID      string `json:"ruleId"`
			RuleName    string `json:"ruleName"`
			MatchedText string `json:"matchedText"`
			RuleMessage string `json:"ruleMessage"`
		}
		if json.Unmarshal([]byte(line), &v) == nil && v.Blocked {
			return &toolguard.Match{RuleID: v.RuleID, RuleName: v.RuleName, MatchedText: v.MatchedText, Message: v.RuleMessage}, true
		}
	}
	return nil, false
}

var ErrStopUnconfirmed = errors.New("egress process stop could not be confirmed")

func (r *Runtime) Context() context.Context { return r.ctx }

type executionIDKey struct{}

func WithExecutionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, executionIDKey{}, id)
}
func ExecutionID(ctx context.Context) string {
	id, _ := ctx.Value(executionIDKey{}).(string)
	return id
}

// TaskNetwork is the task-scoped form of the controlled egress runtime. Unlike
// an AI tool execution, its lifetime covers the asynchronous scan worker.
// Callers must close it when the task (or target shard) reaches a terminal
// state so the audit is archived and the gateway lease is revoked.
type TaskNetwork struct {
	runtime   *Runtime
	execution *Execution
}

func BeginTask(ctx context.Context, root, sessionID, taskID, tool string, rules func() (toolguard.Config, error)) (*TaskNetwork, error) {
	r := NewRuntime(ctx, root, sessionID, rules)
	e, err := r.Begin(ctx, tool, taskID)
	if err != nil {
		return nil, err
	}
	return &TaskNetwork{runtime: r, execution: e}, nil
}

func (n *TaskNetwork) ProxyURL() string {
	if n == nil || n.execution == nil || n.execution.Proxy == nil {
		return ""
	}
	proxy := *n.execution.Proxy
	// Scan engines expose only a proxy URL and cannot install the custom
	// RoundTripper used by the native tool runner. Encode the short-lived
	// gateway credential as standard proxy Basic auth; the gateway validates it
	// against the same execution token before accepting either HTTP or CONNECT.
	proxy.User = url.UserPassword("Bearer", n.execution.token)
	return proxy.String()
}

func (n *TaskNetwork) Transport() http.RoundTripper {
	if n == nil || n.execution == nil {
		return nil
	}
	return n.execution.Transport()
}

func (n *TaskNetwork) Blocked() bool {
	return n != nil && n.execution != nil && n.execution.Blocked()
}

func (n *TaskNetwork) Close() error {
	if n == nil {
		return nil
	}
	if n.runtime != nil {
		return n.runtime.Close()
	}
	if n.execution != nil {
		return n.execution.Close()
	}
	return nil
}

type taskNetworkKey struct{}

func WithTaskNetwork(ctx context.Context, network *TaskNetwork) context.Context {
	return context.WithValue(ctx, taskNetworkKey{}, network)
}

func TaskNetworkFromContext(ctx context.Context) *TaskNetwork {
	if ctx == nil {
		return nil
	}
	network, _ := ctx.Value(taskNetworkKey{}).(*TaskNetwork)
	return network
}

// ProxyURLFromContext is used by scan engines that expose a proxy string but
// construct their own HTTP client internally. An empty result means the task
// is outside the controlled egress path.
func ProxyURLFromContext(ctx context.Context) string {
	if network := TaskNetworkFromContext(ctx); network != nil {
		return network.ProxyURL()
	}
	return ""
}

func removeContainer(ctx context.Context, name string) error {
	_, err := docker(ctx, "rm", "-f", name)
	if err != nil {
		message := strings.ToLower(err.Error())
		if !strings.Contains(message, "no such") && !strings.Contains(message, "already in progress") {
			return err
		}
	}
	// --rm and the watchdog may race with removal. Only absence acknowledges exit.
	for {
		_, err = docker(ctx, "container", "inspect", name)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "no such") {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
