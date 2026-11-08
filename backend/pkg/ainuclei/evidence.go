package ainuclei

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/logrusorgru/aurora"
	"github.com/projectdiscovery/gologger/levels"
	"github.com/projectdiscovery/nuclei/v3/pkg/output"
	"github.com/projectdiscovery/nuclei/v3/pkg/progress"
)

type evidenceWriter struct {
	mu            sync.Mutex
	file          *os.File
	err           error
	matches       atomic.Int64
	requests      atomic.Int64
	requestErrors atomic.Int64
	diagnostics   atomic.Int64
}

var _ output.Writer = (*evidenceWriter)(nil)
var _ progress.Progress = (*evidenceWriter)(nil)

func newEvidenceWriter(workDir string) (*evidenceWriter, error) {
	dir, err := filepath.Abs(filepath.Join(workDir, "cmd-output"))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(dir, "nuclei-*.jsonl")
	if err != nil {
		return nil, err
	}
	return &evidenceWriter{file: f}, nil
}

func (w *evidenceWriter) record(value any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err == nil {
		w.err = json.NewEncoder(w.file).Encode(value)
	}
}
func (w *evidenceWriter) finish() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return errors.Join(w.err, w.file.Close())
}

// SDK Close is called before finish, allowing late OOB callbacks to be archived.
func (w *evidenceWriter) Close()                   {}
func (w *evidenceWriter) Colorizer() aurora.Aurora { return aurora.NewAurora(false) }
func (w *evidenceWriter) ResultCount() int         { return int(w.matches.Load()) }
func (w *evidenceWriter) Write(event *output.ResultEvent) error {
	if event == nil {
		return nil
	}
	if event.MatcherStatus {
		w.matches.Add(1)
	}
	w.record(map[string]any{"kind": "result", "event": event})
	return nil
}
func (w *evidenceWriter) WriteFailure(event *output.InternalWrappedEvent) error {
	if event != nil {
		w.record(map[string]any{"kind": "non_match", "event": event.InternalEvent})
	}
	return nil
}
func (w *evidenceWriter) Request(templateID, target, protocol string, err error) {
	w.record(map[string]any{"kind": "request", "template_id": templateID, "target": target, "protocol": protocol, "error": errorText(err)})
}
func (w *evidenceWriter) RequestStatsLog(string, string) {}
func (w *evidenceWriter) WriteStoreDebugData(host, templateID, eventType, data string) {
	w.record(map[string]any{"kind": "debug", "host": host, "template_id": templateID, "type": eventType, "data": data})
}
func (w *evidenceWriter) Stop()                     {}
func (w *evidenceWriter) Init(int64, int, int64)    {}
func (w *evidenceWriter) AddToTotal(int64)          {}
func (w *evidenceWriter) IncrementRequests()        { w.requests.Add(1) }
func (w *evidenceWriter) SetRequests(n uint64)      { w.requests.Add(int64(n)) }
func (w *evidenceWriter) IncrementMatched()         {}
func (w *evidenceWriter) IncrementErrorsBy(n int64) { w.requestErrors.Add(n) }
func (w *evidenceWriter) IncrementFailedRequestsBy(n int64) {
	w.requests.Add(n)
	w.requestErrors.Add(n)
}

type evidenceLogWriter struct{ w *evidenceWriter }

func (l evidenceLogWriter) Write(data []byte, level levels.Level) {
	l.w.diagnostics.Add(1)
	l.w.record(map[string]any{"kind": "diagnostic", "level": level, "message": string(data)})
}
