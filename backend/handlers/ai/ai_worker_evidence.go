package ai

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/config"
)

func prepareWorkerScanEvidence(ctx context.Context, store *pentest.SQLStore, turn pentest.WorkerTurn, executionID string, call *pentest.AgentToolCall) error {
	// Clone arguments: the execution record retains the exact model request.
	encoded, err := json.Marshal(call.Function.Arguments)
	if err != nil {
		return err
	}
	var args map[string]any
	if err = json.Unmarshal(encoded, &args); err != nil {
		return err
	}
	conf, ok := args["config"].(map[string]any)
	if !ok {
		conf = map[string]any{}
		args["config"] = conf
	}
	delete(conf, "workerEvidenceFiles") // References can only be minted by the primary.
	refs := map[string]pentest.WorkerEvidenceReference{}
	dir := filepath.Join(config.GetDataDir(), "ai-pentest", "scan-material", executionID)
	prepare := func(path string) (string, error) {
		ref, e := store.FreezeWorkerEvidence(ctx, turn.ConversationID, strings.TrimPrefix(path, "evidence:"))
		if e != nil {
			return "", e
		}
		restored, e := store.RestoreWorkerEvidenceReference(ctx, ref, dir)
		if e != nil {
			return "", e
		}
		refs[restored] = ref
		return restored, nil
	}
	for _, container := range []map[string]any{args, conf} {
		for _, key := range []string{"dictPaths", "templateFiles"} {
			if paths, ok := container[key].([]any); ok {
				for i, value := range paths {
					path, ok := value.(string)
					if !ok {
						continue
					}
					restored, e := prepare(path)
					if e != nil {
						return e
					}
					paths[i] = restored
				}
			}
		}
		for _, key := range []string{"username", "password"} {
			if path, ok := container[key].(string); ok && (filepath.IsAbs(path) || strings.HasPrefix(path, "evidence:")) {
				restored, e := prepare(path)
				if e != nil {
					return e
				}
				container[key] = restored
			}
		}
	}
	if len(refs) > 0 {
		conf["workerEvidenceFiles"] = refs
	}
	call.Function.Arguments = args
	return nil
}
