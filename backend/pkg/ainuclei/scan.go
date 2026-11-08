// Package ainuclei exposes the embedded Nuclei SDK to the dialogue agent without
// invoking the website scanner, fingerprint discovery or platform task scheduler.
package ainuclei

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/gologger/formatter"
	"github.com/projectdiscovery/gologger/levels"
	nuclei "github.com/projectdiscovery/nuclei/v3/lib"
	"github.com/projectdiscovery/nuclei/v3/pkg/output"
	"github.com/projectdiscovery/nuclei/v3/pkg/protocols/common/interactsh"
	"github.com/projectdiscovery/nuclei/v3/pkg/types"
)

// Options deliberately exposes typed SDK settings rather than shell flags.
type Options struct {
	Target          string   `json:"target,omitempty"`
	Targets         []string `json:"targets,omitempty"`
	Template        string   `json:"template,omitempty"`
	Templates       []string `json:"templates,omitempty"`
	Workflows       []string `json:"workflows,omitempty"`
	Tags            string   `json:"tags,omitempty"`
	ExcludeTags     string   `json:"exclude_tags,omitempty"`
	Severity        string   `json:"severity,omitempty"`
	IDs             []string `json:"template_ids,omitempty"`
	Protocols       string   `json:"protocols,omitempty"`
	Headers         []string `json:"headers,omitempty"`
	Variables       []string `json:"variables,omitempty"`
	Proxy           *string  `json:"proxy,omitempty"`
	Concurrency     int      `json:"concurrency,omitempty"`
	RateLimit       int      `json:"rate_limit,omitempty"`
	Timeout         int      `json:"timeout,omitempty"`
	Retries         *int     `json:"retries,omitempty"`
	Headless        bool     `json:"headless,omitempty"`
	Code            bool     `json:"code,omitempty"`
	DAST            bool     `json:"dast,omitempty"`
	OOB             *bool    `json:"oob,omitempty"`
	InteractshURL   string   `json:"interactsh_url,omitempty"`
	InteractshToken string   `json:"interactsh_token,omitempty"`
	ScanStrategy    string   `json:"scan_strategy,omitempty"`
}

type Result struct {
	Status          string    `json:"status"`
	Engine          string    `json:"engine"`
	Targets         []string  `json:"targets"`
	LoadedTemplates int       `json:"loaded_templates"`
	LoadedWorkflows int       `json:"loaded_workflows"`
	Requests        int64     `json:"requests"`
	RequestErrors   int64     `json:"request_errors"`
	Matches         int       `json:"matches"`
	Diagnostics     int       `json:"diagnostics"`
	OutputFile      string    `json:"output_file"`
	StartedAt       time.Time `json:"started_at"`
	DurationMS      int64     `json:"duration_ms"`
	Note            string    `json:"note"`
}

// Run is synchronous: it returns only after SDK workers and OOB callbacks exit.
// The agent's existing execution record and context own the complete lifecycle.
func Run(ctx context.Context, workDir string, args Options) (result Result, err error) {
	result = Result{Status: "failed", Engine: "embedded-nuclei-sdk", StartedAt: time.Now(),
		Note: "Counts describe this invocation only. No matches does not establish absence of vulnerabilities. Read output_file for the loaded template manifest, evidence and diagnostics; findings require independent verification."}
	defer func() { result.DurationMS = time.Since(result.StartedAt).Milliseconds() }()
	if err = ctx.Err(); err != nil {
		result.Status = "cancelled"
		if errors.Is(err, context.DeadlineExceeded) {
			result.Status = "timed_out"
		}
		return
	}
	if err = args.normalize(); err != nil {
		return
	}
	result.Targets = args.Targets
	if strings.TrimSpace(workDir) == "" {
		return result, errors.New("session workspace is required")
	}
	w, err := newEvidenceWriter(workDir)
	if err != nil {
		return result, err
	}
	result.OutputFile = w.file.Name()
	defer func() {
		if ctx.Err() != nil {
			err = errors.Join(err, ctx.Err())
		}
		result.Requests = w.requests.Load()
		result.RequestErrors = w.requestErrors.Load()
		result.Matches = w.ResultCount()
		result.Diagnostics = int(w.diagnostics.Load())
		if err != nil {
			result.Status = "failed"
			if errors.Is(err, context.Canceled) {
				result.Status = "cancelled"
			} else if errors.Is(err, context.DeadlineExceeded) {
				result.Status = "timed_out"
			} else if errors.Is(err, nuclei.ErrNoTemplatesAvailable) {
				result.Status = "no_templates"
			}
		} else if result.RequestErrors > 0 {
			result.Status = "completed_with_errors"
		} else if result.Diagnostics > 0 {
			result.Status = "completed_with_diagnostics"
		} else {
			result.Status = "completed"
		}
		result.DurationMS = time.Since(result.StartedAt).Milliseconds()
		w.record(map[string]any{"kind": "summary", "result": result, "error": errorText(err)})
		if closeErr := w.finish(); closeErr != nil {
			err = errors.Join(err, closeErr)
			result.Status = "failed"
		}
	}()

	logger := &gologger.Logger{}
	logger.SetWriter(evidenceLogWriter{w})
	logger.SetFormatter(formatter.NewCLI(true))
	logger.SetMaxLevel(levels.LevelWarning)
	options := types.DefaultOptions()
	options.TemplateThreads, options.BulkSize = args.Concurrency, args.Concurrency
	options.Timeout = args.Timeout
	options.Retries = *args.Retries
	options.EnableCodeTemplates, options.Headless, options.DAST = args.Code, args.Headless, args.DAST
	options.OmitTemplate = true
	options.ScanStrategy = args.ScanStrategy
	// HTTP redirects must not implicitly extend the authorized target set.
	options.DisableRedirects = true
	interact := interactsh.DefaultOptions(w, nil, w)
	interact.NoInteractsh = args.OOB != nil && !*args.OOB
	options.NoInteractsh = interact.NoInteractsh
	interact.Logger = logger
	if args.InteractshURL != "" {
		interact.ServerURL = args.InteractshURL
	}
	interact.Authorization = args.InteractshToken
	sdkOptions := []nuclei.NucleiSDKOptions{
		nuclei.WithOptions(options), nuclei.WithLogger(logger),
		nuclei.WithTemplatesOrWorkflows(nuclei.TemplateSources{Templates: args.Templates, Workflows: args.Workflows}),
		nuclei.WithTemplateFilters(nuclei.TemplateFilters{Tags: csv(args.Tags), ExcludeTags: csv(args.ExcludeTags), Severity: args.Severity, IDs: args.IDs, ProtocolTypes: args.Protocols}),
		nuclei.WithHeaders(args.Headers), nuclei.WithVars(args.Variables),
		nuclei.WithGlobalRateLimitCtx(ctx, args.RateLimit, time.Second),
		nuclei.WithInteractshOptions(nuclei.InteractshOpts(*interact)),
		nuclei.UseOutputWriter(w), nuclei.UseStatsWriter(w),
	}
	if args.Proxy != nil && *args.Proxy != "" {
		sdkOptions = append(sdkOptions, nuclei.WithProxy([]string{*args.Proxy}, false))
	}
	engine, err := nuclei.NewNucleiEngineCtx(ctx, sdkOptions...)
	if err != nil {
		return result, err
	}
	// Close drains OOB results too; it must precede the evidence summary/close.
	defer engine.Close()
	if err = engine.LoadAllTemplates(); err != nil {
		return result, err
	}
	result.LoadedTemplates = len(engine.GetTemplates())
	result.LoadedWorkflows = len(engine.GetWorkflows())
	for _, template := range append(engine.GetTemplates(), engine.GetWorkflows()...) {
		data, readErr := os.ReadFile(template.Path)
		if readErr != nil {
			return result, fmt.Errorf("record template provenance: %w", readErr)
		}
		hash := sha256.Sum256(data)
		w.record(map[string]any{"kind": "template", "id": template.ID, "path": template.Path, "sha256": hex.EncodeToString(hash[:])})
	}
	if result.LoadedTemplates+result.LoadedWorkflows == 0 {
		return result, nuclei.ErrNoTemplatesAvailable
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	engine.LoadTargets(args.Targets, false)
	// The writer stores complete results. A no-op callback suppresses SDK stdout.
	err = engine.ExecuteCallbackWithCtx(ctx, func(*output.ResultEvent) {})
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return result, err
}

func (a *Options) normalize() error {
	a.Targets = compact(append(a.Targets, a.Target))
	if len(a.Targets) == 0 {
		return errors.New("target or targets is required")
	}
	for _, target := range a.Targets {
		if strings.ContainsAny(target, "\r\n\t ") {
			return fmt.Errorf("invalid target %q: supply separate targets as an array", target)
		}
	}
	if len(a.Templates)+len(a.Workflows) == 0 {
		return errors.New("no template sources configured; select platform templates or explicit local template/workflow paths")
	}
	for _, path := range append(append([]string{}, a.Templates...), a.Workflows...) {
		if strings.Contains(path, "://") {
			return errors.New("template sources must be local paths; download external templates into the session workspace first")
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("template source %q: %w", path, err)
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("template source %q is not a regular file or directory", path)
		}
	}
	if a.Concurrency == 0 {
		a.Concurrency = 10
	}
	if a.RateLimit == 0 {
		a.RateLimit = 50
	}
	if a.Timeout == 0 {
		a.Timeout = 10
	}
	if a.Retries == nil {
		n := 1
		a.Retries = &n
	}
	if a.Concurrency < 1 || a.RateLimit < 1 || a.Timeout < 1 || *a.Retries < 0 {
		return errors.New("concurrency, rate_limit and timeout must be positive; retries must be non-negative")
	}
	if a.ScanStrategy != "" && a.ScanStrategy != "auto" && a.ScanStrategy != "host-spray" && a.ScanStrategy != "template-spray" {
		return errors.New("scan_strategy must be auto, host-spray or template-spray")
	}
	return nil
}

func csv(s string) []string { return compact(strings.Split(s, ",")) }
func compact(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// ResolvePaths resolves relative files against the session, never the server cwd.
func ResolvePaths(workDir string, values []string) []string {
	paths := compact(values)
	for i, path := range paths {
		if !filepath.IsAbs(path) && !strings.Contains(path, "://") {
			paths[i] = filepath.Join(workDir, path)
		}
	}
	return paths
}
