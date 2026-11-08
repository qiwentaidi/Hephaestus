package space

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"hephaestus-backend/pkg/database"
)

const maxSpaceEngineSearchHistoryLimit = 50

func normalizeSpaceEngineHistorySource(source string) string {
	return strings.ToLower(strings.TrimSpace(source))
}

func isSupportedSpaceEngineHistorySource(source string) bool {
	switch normalizeSpaceEngineHistorySource(source) {
	case "fofa", "hunter", "quake":
		return true
	default:
		return false
	}
}

func SaveSpaceEngineSearchHistory(source, query, username string) error {
	source = normalizeSpaceEngineHistorySource(source)
	query = strings.TrimSpace(query)
	username = strings.TrimSpace(username)

	if !isSupportedSpaceEngineHistorySource(source) {
		return fmt.Errorf("不支持的空间引擎来源: %s", source)
	}
	if query == "" {
		return fmt.Errorf("搜索语句不能为空")
	}
	if username == "" {
		return fmt.Errorf("用户名不能为空")
	}

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var existing database.SpaceEngineSearchHistory
	now := time.Now()
	err = db.Where("source = ? AND query = ? AND created_by = ?", source, query, username).First(&existing).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("查询搜索历史失败: %w", err)
	}

	if err == nil {
		return db.Model(&existing).Update("updated_at", now).Error
	}

	record := &database.SpaceEngineSearchHistory{
		ID:        database.GenerateNanoid(),
		Source:    source,
		Query:     query,
		CreatedBy: username,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(record).Error; err != nil {
		return fmt.Errorf("保存搜索历史失败: %w", err)
	}

	var idsToDelete []string
	if err := db.Model(&database.SpaceEngineSearchHistory{}).
		Where("source = ? AND created_by = ?", source, username).
		Order("updated_at DESC").
		Offset(maxSpaceEngineSearchHistoryLimit).
		Pluck("id", &idsToDelete).Error; err != nil {
		return fmt.Errorf("清理搜索历史失败: %w", err)
	}
	if len(idsToDelete) > 0 {
		if err := db.Where("id IN ?", idsToDelete).Delete(&database.SpaceEngineSearchHistory{}).Error; err != nil {
			return fmt.Errorf("删除旧搜索历史失败: %w", err)
		}
	}

	return nil
}

func ListSpaceEngineSearchHistory(source, username string, limit int) ([]database.SpaceEngineSearchHistory, error) {
	source = normalizeSpaceEngineHistorySource(source)
	username = strings.TrimSpace(username)

	if !isSupportedSpaceEngineHistorySource(source) {
		return nil, fmt.Errorf("不支持的空间引擎来源: %s", source)
	}
	if username == "" {
		return nil, fmt.Errorf("用户名不能为空")
	}

	if limit <= 0 || limit > maxSpaceEngineSearchHistoryLimit {
		limit = maxSpaceEngineSearchHistoryLimit
	}

	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var records []database.SpaceEngineSearchHistory
	if err := db.Where("source = ? AND created_by = ?", source, username).
		Order("updated_at DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("获取搜索历史失败: %w", err)
	}

	return records, nil
}
