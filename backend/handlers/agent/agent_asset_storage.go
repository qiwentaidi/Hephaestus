package agent

import (
	"hephaestus-backend/pkg/agentasset"
	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
)

func UpsertAgentScanAssetRecord(db *gorm.DB, record *database.ScanAssetFile) error {
	return agentasset.UpsertScanAssetRecord(db, record)
}

func NormalizeAgentScanAssetType(assetType string) string {
	return agentasset.NormalizeScanAssetType(assetType)
}

func normalizeAgentScanAssetFilename(raw string) string {
	return agentasset.NormalizeScanAssetFilename(raw)
}

func DetectAgentScanAssetContentType(assetType, filename string) string {
	return agentasset.DetectScanAssetContentType(assetType, filename)
}

func syncAgentTaskAssetFilesToStorage(db *gorm.DB, taskID string, version int) error {
	return agentasset.SyncTaskAssetFilesToStorage(db, taskID, version)
}

func DeleteUnusedAgentScanAsset(db *gorm.DB, taskID, assetType, rawPath string) error {
	return agentasset.DeleteUnusedScanAsset(db, taskID, assetType, rawPath)
}
