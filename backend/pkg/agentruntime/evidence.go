package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"gorm.io/gorm"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/config"
)

func materializeScanEvidence(ctx context.Context, db *gorm.DB, payload *TaskPayload) (map[string]interface{}, error) {
	encoded, err := json.Marshal(payload.Config)
	if err != nil {
		return nil, err
	}
	var conf map[string]interface{}
	if err = json.Unmarshal(encoded, &conf); err != nil {
		return nil, err
	}
	raw, ok := conf["workerEvidenceFiles"]
	if !ok {
		return conf, nil
	}
	encoded, err = json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var refs map[string]pentest.WorkerEvidenceReference
	if err = json.Unmarshal(encoded, &refs); err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	store := pentest.NewSQLStoreWithDialect(sqlDB, db.Dialector.Name())
	dir := filepath.Join(config.GetDataDir(), "scan-evidence", payload.ID, fmt.Sprint(payload.Version), fmt.Sprintf("%d-%d", payload.ItemID, payload.Generation))
	replacements := map[string]string{}
	for old, ref := range refs {
		if ref.ConversationID != conf["aiWorkerConversationId"] {
			return nil, fmt.Errorf("scan evidence conversation mismatch")
		}
		path, e := store.RestoreWorkerEvidenceReference(ctx, ref, dir)
		if e != nil {
			return nil, e
		}
		replacements[old] = path
	}
	var replace func(any) any
	replace = func(value any) any {
		switch v := value.(type) {
		case string:
			if path, ok := replacements[v]; ok {
				return path
			}
		case []any:
			for i, item := range v {
				v[i] = replace(item)
			}
		case map[string]any:
			for key, item := range v {
				if key != "workerEvidenceFiles" {
					v[key] = replace(item)
				}
			}
		}
		return value
	}
	replace(conf)
	return conf, nil
}
