package asset

import (
	"context"
	"errors"
	"net/http"
	"strings"

	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// maxLedgerBatchItems bounds one batch request. The selection UI works page by
// page, so this is far above any realistic selection while still protecting the
// server from unbounded transactions.
const maxLedgerBatchItems = 5000

type ledgerBatchArchiveFailure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

type ledgerBatchArchiveResult struct {
	Action  string                      `json:"action"`
	Updated int                         `json:"updated"`
	Skipped int                         `json:"skipped"`
	Failed  []ledgerBatchArchiveFailure `json:"failed"`
}

type ledgerBatchArchiveRequest struct {
	Action string                  `json:"action"`
	Items  []ledgerEntitySelection `json:"items"`
}

func normalizeLedgerArchiveAction(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "archive":
		return true, true
	case "restore", "unarchive":
		return false, true
	default:
		return false, false
	}
}

func ledgerArchiveActionLabel(archived bool) string {
	if archived {
		return "archive"
	}
	return "restore"
}

// loadLedgerArchiveAssetTypes returns the asset type for every requested id so
// the archive loop does not issue one lookup per row.
func loadLedgerArchiveAssetTypes(ctx context.Context, db *gorm.DB, ids []string) (map[string]string, error) {
	types := make(map[string]string, len(ids))
	const chunkSize = 500
	for start := 0; start < len(ids); start += chunkSize {
		end := start + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		var rows []database.UnifiedAsset
		if err := db.WithContext(ctx).Model(&database.UnifiedAsset{}).Select("id", "type").Where("id IN ?", ids[start:end]).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			types[row.ID] = row.Type
		}
	}
	return types, nil
}

// applyLedgerEntityArchive archives or restores the standalone asset behind each
// selected ledger row. Websites aggregate URL paths per origin, so a URL asset
// always goes through ArchiveWebsite to keep the whole origin in sync.
func applyLedgerEntityArchive(ctx context.Context, store assetstore.Store, items []ledgerEntitySelection, archived bool) (ledgerBatchArchiveResult, error) {
	result := ledgerBatchArchiveResult{Action: ledgerArchiveActionLabel(archived), Failed: []ledgerBatchArchiveFailure{}}
	assetIDs := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		assetID := strings.TrimSpace(item.AssetID)
		if assetID == "" {
			continue
		}
		if _, exists := seen[assetID]; exists {
			continue
		}
		seen[assetID] = struct{}{}
		assetIDs = append(assetIDs, assetID)
	}
	types, err := loadLedgerArchiveAssetTypes(ctx, store.DB, assetIDs)
	if err != nil {
		return result, err
	}
	processed := make(map[string]struct{}, len(assetIDs))
	for _, item := range items {
		assetID := strings.TrimSpace(item.AssetID)
		if assetID == "" {
			result.Skipped++
			continue
		}
		if _, done := processed[assetID]; done {
			continue
		}
		processed[assetID] = struct{}{}
		assetType, ok := types[assetID]
		if !ok {
			result.Failed = append(result.Failed, ledgerBatchArchiveFailure{ID: ledgerSelectionLabel(item), Error: "资产不存在"})
			continue
		}
		var archiveErr error
		if assetType == assetstore.TypeURL {
			_, archiveErr = store.ArchiveWebsite(ctx, assetID, archived)
		} else {
			_, archiveErr = store.UpdateAsset(ctx, assetID, assetstore.AssetUpdate{Archived: &archived})
		}
		if archiveErr != nil {
			result.Failed = append(result.Failed, ledgerBatchArchiveFailure{ID: ledgerSelectionLabel(item), Error: archiveErr.Error()})
			continue
		}
		result.Updated++
	}
	return result, nil
}

func ledgerSelectionLabel(item ledgerEntitySelection) string {
	if value := strings.TrimSpace(item.Value); value != "" {
		return value
	}
	if id := strings.TrimSpace(item.ID); id != "" {
		return id
	}
	return strings.TrimSpace(item.AssetID)
}

func BatchArchiveAssetLedgerEntitiesHandler(c *gin.Context) {
	var request ledgerBatchArchiveRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	archived, ok := normalizeLedgerArchiveAction(request.Action)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "action must be archive or restore"})
		return
	}
	if len(request.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "请先选择要操作的资产"})
		return
	}
	if len(request.Items) > maxLedgerBatchItems {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "一次最多处理 5000 项资产"})
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	store := assetstore.Store{DB: db.WithContext(c.Request.Context())}
	result, err := applyLedgerEntityArchive(c.Request.Context(), store, request.Items, archived)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "资产不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
