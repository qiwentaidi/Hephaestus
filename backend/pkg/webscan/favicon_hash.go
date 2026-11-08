package webscan

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"hephaestus-backend/pkg/agentasset"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	fofa "hephaestus-backend/pkg/space-engine/fofa"

	"gorm.io/gorm"
)

const faviconHashBackfillBatchSize = 200

// FaviconIconHash calculates the FOFA-compatible MMH3 hash for a locally
// stored favicon. Non-image responses saved from favicon probes deliberately
// return empty: an HTML fallback page is not a site icon.
func FaviconIconHash(favicon string) string {
	filename := agentasset.NormalizeScanAssetFilename(favicon)
	if filename == "" {
		return ""
	}

	raw, err := os.ReadFile(filepath.Join(config.GetScanAssetDir("favicon"), filename))
	if err != nil {
		return ""
	}
	return faviconIconHash(raw)
}

func faviconIconHash(raw []byte) string {
	if !isFaviconImage(raw) {
		return ""
	}
	return fofa.Mmh3Hash32(raw)
}

// isFaviconImage accepts the formats commonly served as website favicons and
// rejects HTML, JSON, and error bodies that can otherwise produce a hash.
func isFaviconImage(raw []byte) bool {
	if len(raw) < 4 {
		return false
	}
	if bytes.HasPrefix(raw, []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}) || // PNG
		bytes.HasPrefix(raw, []byte{0xff, 0xd8, 0xff}) || // JPEG
		bytes.HasPrefix(raw, []byte("GIF87a")) || bytes.HasPrefix(raw, []byte("GIF89a")) || // GIF
		bytes.HasPrefix(raw, []byte("BM")) { // BMP
		return true
	}
	if len(raw) >= 12 && bytes.Equal(raw[:4], []byte("RIFF")) && bytes.Equal(raw[8:12], []byte("WEBP")) {
		return true
	}
	if len(raw) >= 6 && raw[0] == 0 && raw[1] == 0 && raw[2] == 1 && raw[3] == 0 {
		return true // ICO
	}
	// SVG favicons may start with an XML declaration, so look for the element
	// near the beginning instead of requiring it as the first bytes.
	prefix := bytes.ToLower(bytes.TrimSpace(raw))
	if len(prefix) > 4096 {
		prefix = prefix[:4096]
	}
	return bytes.Contains(prefix, []byte("<svg"))
}

func storedFaviconIconHash(db *gorm.DB, favicon string) (hash string, available bool) {
	filename := agentasset.NormalizeScanAssetFilename(favicon)
	if filename == "" {
		return "", false
	}

	raw, err := os.ReadFile(filepath.Join(config.GetScanAssetDir("favicon"), filename))
	if err == nil {
		return faviconIconHash(raw), true
	}
	if db == nil {
		return "", false
	}

	var asset database.ScanAssetFile
	if err := db.Select("data").Where("asset_type = ? AND file_name = ?", "favicon", filename).First(&asset).Error; err != nil {
		return "", false
	}
	return faviconIconHash(asset.Data), true
}

// BackfillFaviconIconHashes revalidates historical scan fingerprints. A
// missing source file is left untouched so a later restored asset can be
// processed on a subsequent startup; readable non-images have their stale
// hash cleared.
func BackfillFaviconIconHashes(db *gorm.DB) error {
	if db == nil {
		return nil
	}

	offset := 0
	for {
		var rows []database.FingerprintInfo
		if err := db.Where("favicon IS NOT NULL AND TRIM(favicon) <> ''").
			Order("created_at ASC, task_id ASC, url ASC").
			Offset(offset).
			Limit(faviconHashBackfillBatchSize).
			Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}

		for _, row := range rows {
			hash, available := storedFaviconIconHash(db, row.Favicon)
			if !available || strings.TrimSpace(row.IconHash) == hash {
				continue
			}
			result := db.Model(&database.FingerprintInfo{}).
				Where("task_id = ? AND version = ? AND url = ?", row.TaskID, row.Version, row.URL).
				Update("icon_hash", hash)
			if result.Error != nil {
				return result.Error
			}
		}
		offset += len(rows)
	}

	return nil
}
