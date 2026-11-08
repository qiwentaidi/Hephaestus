package agentasset

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
)

type uploadConfig struct {
	endpoint  string
	authorize func(*http.Request) error
}

type uploadContextKey struct{}

// WithUpload configures remote asset uploads for a single task.
func WithUpload(ctx context.Context, endpoint string, authorize func(*http.Request) error) context.Context {
	if endpoint == "" || authorize == nil {
		return ctx
	}
	return context.WithValue(ctx, uploadContextKey{}, &uploadConfig{endpoint: endpoint, authorize: authorize})
}

type assetUploadResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Path string `json:"path"`
	} `json:"data"`
	Error string `json:"error"`
}

func isDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint")
}

func NormalizeScanAssetType(assetType string) string {
	switch strings.ToLower(strings.TrimSpace(assetType)) {
	case "screenshot":
		return "screenshot"
	case "favicon":
		return "favicon"
	default:
		return ""
	}
}

func NormalizeScanAssetFilename(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return ""
	}
	name = filepath.Base(name)
	if name == "." || name == "/" || name == `\` {
		return ""
	}
	return name
}

func resolveLocalScanAssetPath(assetType, rawPath string) string {
	cleanType := NormalizeScanAssetType(assetType)
	if cleanType == "" {
		return ""
	}
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	filename := NormalizeScanAssetFilename(path)
	if filename == "" {
		return ""
	}
	return filepath.Join(config.GetScanAssetDir(cleanType), filename)
}

func DetectScanAssetContentType(assetType, filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".ico":
		return "image/x-icon"
	case ".bmp":
		return "image/bmp"
	default:
		if NormalizeScanAssetType(assetType) == "screenshot" {
			return "image/png"
		}
		return "application/octet-stream"
	}
}

func UpsertScanAssetRecord(db *gorm.DB, record *database.ScanAssetFile) error {
	if db == nil || record == nil {
		return fmt.Errorf("invalid upsert args")
	}
	record.AssetType = NormalizeScanAssetType(record.AssetType)
	record.FileName = NormalizeScanAssetFilename(record.FileName)
	if record.AssetType == "" || record.FileName == "" {
		return fmt.Errorf("invalid asset record key")
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = time.Now()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = record.UpdatedAt
	}

	updates := map[string]interface{}{
		"content_type":   record.ContentType,
		"data":           record.Data,
		"size":           record.Size,
		"source_task_id": record.SourceTaskID,
		"source_version": record.SourceVersion,
		"updated_at":     record.UpdatedAt,
	}

	tx := db.Model(&database.ScanAssetFile{}).
		Where("asset_type = ? AND file_name = ?", record.AssetType, record.FileName).
		Updates(updates)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected > 0 {
		return nil
	}

	if err := db.Create(record).Error; err != nil {
		if isDuplicateKeyErr(err) {
			return db.Model(&database.ScanAssetFile{}).
				Where("asset_type = ? AND file_name = ?", record.AssetType, record.FileName).
				Updates(updates).Error
		}
		return err
	}
	return nil
}

func uploadAssetByAPI(ctx context.Context, cfg *uploadConfig, assetType, taskID string, version int, rawPath string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("missing upload config")
	}
	cleanType := NormalizeScanAssetType(assetType)
	if cleanType == "" {
		return "", fmt.Errorf("invalid asset type: %s", assetType)
	}
	localPath := resolveLocalScanAssetPath(cleanType, rawPath)
	if localPath == "" {
		return "", fmt.Errorf("invalid local asset path: %s", rawPath)
	}

	data, err := os.ReadFile(localPath)
	if err != nil {
		return "", err
	}
	filename := NormalizeScanAssetFilename(rawPath)
	if filename == "" {
		filename = fmt.Sprintf("%s_%d.bin", cleanType, time.Now().UnixNano())
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("assetType", cleanType); err != nil {
		return "", err
	}
	if strings.TrimSpace(taskID) != "" {
		_ = writer.WriteField("taskId", taskID)
	}
	if version > 0 {
		_ = writer.WriteField("version", fmt.Sprintf("%d", version))
	}

	fileWriter, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := fileWriter.Write(data); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.endpoint, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := cfg.authorize(req); err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	var result assetUploadResponse
	_ = json.Unmarshal(respBytes, &result)

	if resp.StatusCode != http.StatusOK || !result.Success {
		errMsg := strings.TrimSpace(result.Error)
		if errMsg == "" {
			errMsg = strings.TrimSpace(string(respBytes))
		}
		if errMsg == "" {
			errMsg = resp.Status
		}
		return "", fmt.Errorf("upload api failed: %s", errMsg)
	}
	if strings.TrimSpace(result.Data.Path) == "" {
		return "", fmt.Errorf("upload api returned empty path")
	}
	return strings.TrimSpace(result.Data.Path), nil
}

func upsertScanAssetFile(db *gorm.DB, assetType, taskID string, version int, rawPath string) (string, error) {
	cleanType := NormalizeScanAssetType(assetType)
	if cleanType == "" {
		return "", fmt.Errorf("invalid asset type: %s", assetType)
	}
	filename := NormalizeScanAssetFilename(rawPath)
	if filename == "" {
		return "", nil
	}
	localPath := resolveLocalScanAssetPath(cleanType, rawPath)
	if localPath == "" {
		return "", fmt.Errorf("invalid local path: %s", rawPath)
	}

	data, err := os.ReadFile(localPath)
	if err != nil {
		return "", err
	}
	now := time.Now()
	record := database.ScanAssetFile{
		AssetType:     cleanType,
		FileName:      filename,
		ContentType:   DetectScanAssetContentType(cleanType, filename),
		Data:          data,
		Size:          int64(len(data)),
		SourceTaskID:  taskID,
		SourceVersion: version,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := UpsertScanAssetRecord(db, &record); err != nil {
		return "", err
	}
	return filename, nil
}

func SyncTaskAssetFilesToStorage(db *gorm.DB, taskID string, version int) error {
	return SyncTaskAssetFilesWithContext(context.Background(), db, taskID, version)
}

func SyncTaskAssetFilesWithContext(ctx context.Context, db *gorm.DB, taskID string, version int) error {
	if db == nil || strings.TrimSpace(taskID) == "" || version <= 0 {
		return nil
	}

	var fps []database.FingerprintInfo
	if err := db.Select("screenshot", "favicon").Where("task_id = ? AND version = ?", taskID, version).Find(&fps).Error; err != nil {
		return err
	}
	if len(fps) == 0 {
		return nil
	}

	apiCfg, _ := ctx.Value(uploadContextKey{}).(*uploadConfig)

	screenshots := make(map[string]struct{})
	favicons := make(map[string]struct{})
	for _, fp := range fps {
		if strings.TrimSpace(fp.Screenshot) != "" {
			screenshots[fp.Screenshot] = struct{}{}
		}
		if strings.TrimSpace(fp.Favicon) != "" {
			favicons[fp.Favicon] = struct{}{}
		}
	}

	var firstErr error
	process := func(assetType, column string, values map[string]struct{}) {
		for rawPath := range values {
			var storedPath string
			if apiCfg != nil {
				if p, err := uploadAssetByAPI(ctx, apiCfg, assetType, taskID, version, rawPath); err == nil && strings.TrimSpace(p) != "" {
					storedPath = p
				}
			}
			if storedPath == "" {
				filename, err := upsertScanAssetFile(db, assetType, taskID, version, rawPath)
				if err != nil {
					if firstErr == nil {
						firstErr = fmt.Errorf("%s %s sync failed: %w", assetType, rawPath, err)
					}
					continue
				}
				storedPath = filename
			}

			if strings.TrimSpace(storedPath) == "" || strings.TrimSpace(storedPath) == strings.TrimSpace(rawPath) {
				continue
			}
			if err := db.Model(&database.FingerprintInfo{}).
				Where("task_id = ? AND version = ? AND "+column+" = ?", taskID, version, rawPath).
				Update(column, storedPath).Error; err != nil && firstErr == nil {
				firstErr = fmt.Errorf("update %s path failed: %w", column, err)
			}
		}
	}

	process("screenshot", "screenshot", screenshots)
	process("favicon", "favicon", favicons)
	return firstErr
}

func DeleteUnusedScanAsset(db *gorm.DB, taskID, assetType, rawPath string) error {
	if db == nil || strings.TrimSpace(taskID) == "" {
		return nil
	}
	cleanType := NormalizeScanAssetType(assetType)
	filename := NormalizeScanAssetFilename(rawPath)
	if cleanType == "" || filename == "" {
		return nil
	}

	column := "screenshot"
	if cleanType == "favicon" {
		column = "favicon"
	}

	var refs int64
	cond := fmt.Sprintf("(%s = ? OR %s = ? OR %s = ?)", column, column, column)
	if err := db.Model(&database.FingerprintInfo{}).
		Where("task_id <> ?", taskID).
		Where(cond, filename, cleanType+"/"+filename, "data/"+cleanType+"/"+filename).
		Count(&refs).Error; err != nil {
		return err
	}
	if refs > 0 {
		return nil
	}

	localPath := filepath.Join(config.GetScanAssetDir(cleanType), filename)
	if err := os.Remove(localPath); err != nil && !os.IsNotExist(err) {
	}
	return db.Where("asset_type = ? AND file_name = ?", cleanType, filename).
		Delete(&database.ScanAssetFile{}).Error
}
