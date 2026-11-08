package database

import (
	"context"
	"fmt"
	"strings"

	"hephaestus-backend/pkg/types"

	"gorm.io/gorm"
)

type CollectMigrationIssue struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type CollectMigrationReport struct {
	Scanned     int                     `json:"scanned"`
	Structured  int                     `json:"structured"`
	Convertible int                     `json:"convertible"`
	Migrated    int                     `json:"migrated"`
	Failed      int                     `json:"failed"`
	Conflicted  int                     `json:"conflicted"`
	Issues      []CollectMigrationIssue `json:"issues"`
}

// MigrateCollectData converts legacy HTML to text-only columns/rows JSON.
// With apply=false it performs no writes or schema migrations. Failed records
// remain intact and cannot be served as HTML by GetCollectResults. Re-running
// after interruption is safe: valid structured records are never rewritten.
func MigrateCollectData(ctx context.Context, db *gorm.DB, apply bool) (CollectMigrationReport, error) {
	report := CollectMigrationReport{Issues: []CollectMigrationIssue{}}
	lastID := ""
	firstBatch := true
	for {
		var records []DBCollectResult
		query := db.WithContext(ctx).Select("id", "content", "rows_count").Order("id ASC").Limit(100)
		if !firstBatch {
			query = query.Where("id > ?", lastID)
		}
		if err := query.Find(&records).Error; err != nil {
			return report, fmt.Errorf("读取采集数据失败: %w", err)
		}
		if len(records) == 0 {
			return report, nil
		}
		for _, record := range records {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			report.Scanned++
			data, err := DecodeCollectData(record.Content, record.RowsCount)
			if err != nil {
				report.Failed++
				report.Issues = append(report.Issues, CollectMigrationIssue{record.ID, err.Error()})
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(record.Content), "{") {
				report.Structured++
				continue
			}
			content, err := EncodeCollectData(types.CollectResult{Columns: data.Columns, Rows: data.Rows, RowsCount: data.RowsCount})
			if err != nil {
				return report, fmt.Errorf("编码采集记录 %s 失败: %w", record.ID, err)
			}
			report.Convertible++
			if !apply {
				continue
			}
			// Optimistic compare-and-swap also protects records deleted or changed
			// by another process after the batch was read. Only content changes.
			updated := db.WithContext(ctx).Model(&DBCollectResult{}).
				Where("id = ? AND content = ? AND rows_count = ?", record.ID, record.Content, record.RowsCount).
				UpdateColumn("content", content)
			if updated.Error != nil {
				return report, fmt.Errorf("迁移采集记录 %s 失败: %w", record.ID, updated.Error)
			}
			if updated.RowsAffected == 0 {
				report.Conflicted++
				report.Issues = append(report.Issues, CollectMigrationIssue{record.ID, "记录已变化或删除，请重新预检"})
			} else {
				report.Migrated++
			}
		}
		lastID = records[len(records)-1].ID
		firstBatch = false
	}
}
