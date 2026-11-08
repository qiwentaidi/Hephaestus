package dirsearch

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"hephaestus-backend/pkg/database"
	"gorm.io/gorm"
)

// PathHistory 路径历史记录
type PathHistory struct {
	Path  string `json:"path"`  // 路径
	Times int    `json:"times"` // 命中次数
}

// IncrementPathTimes 增加路径的命中次数
func IncrementPathTimes(resultURL string) error {
	if resultURL == "" {
		return errors.New("URL is empty")
	}

	// 从完整URL中提取路径部分
	path := extractPathFromURL(resultURL)
	if path == "" {
		return nil // 如果无法提取路径，跳过
	}

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("get database failed: %w", err)
	}

	// 使用 GORM 的 Clauses 实现 UPSERT
	pathHistory := database.DirsearchPathHistory{
		Path:  path,
		Times: 1,
	}

	// 先尝试查找是否存在
	var existing database.DirsearchPathHistory
	result := db.Where("path = ?", path).First(&existing)
	if result.Error == gorm.ErrRecordNotFound {
		// 不存在，创建新记录
		if err := db.Create(&pathHistory).Error; err != nil {
			return fmt.Errorf("increment path times failed: %w", err)
		}
	} else if result.Error != nil {
		return fmt.Errorf("increment path times failed: %w", result.Error)
	} else {
		// 存在，更新次数
		if err := db.Model(&existing).Update("times", gorm.Expr("times + ?", 1)).Error; err != nil {
			return fmt.Errorf("increment path times failed: %w", err)
		}
	}

	return nil
}

// BatchIncrementPathTimes 批量增加路径的命中次数
func BatchIncrementPathTimes(resultURLs []string) error {
	if len(resultURLs) == 0 {
		return nil
	}

	// 统计每个路径出现的次数
	pathCounts := make(map[string]int)
	for _, resultURL := range resultURLs {
		if resultURL == "" {
			continue
		}
		path := extractPathFromURL(resultURL)
		if path != "" {
			pathCounts[path]++
		}
	}

	if len(pathCounts) == 0 {
		return nil
	}

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("get database failed: %w", err)
	}

	// 使用 GORM 事务
	return db.Transaction(func(tx *gorm.DB) error {
		for path, count := range pathCounts {
			var existing database.DirsearchPathHistory
			result := tx.Where("path = ?", path).First(&existing)
			if result.Error == gorm.ErrRecordNotFound {
				// 不存在，创建新记录
				pathHistory := database.DirsearchPathHistory{
					Path:  path,
					Times: count,
				}
				if err := tx.Create(&pathHistory).Error; err != nil {
					return fmt.Errorf("create path history failed: %w", err)
				}
			} else if result.Error != nil {
				return fmt.Errorf("query path history failed: %w", result.Error)
			} else {
				// 存在，更新次数
				if err := tx.Model(&existing).Update("times", gorm.Expr("times + ?", count)).Error; err != nil {
					return fmt.Errorf("update path times failed: %w", err)
				}
			}
		}
		return nil
	})
}

// ListPathHistory 获取路径历史记录列表，按命中次数降序排列
func ListPathHistory(limit int, offset int) ([]PathHistory, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("get database failed: %w", err)
	}

	var pathHistories []database.DirsearchPathHistory
	if err := db.Order("times DESC").
		Limit(limit).
		Offset(offset).
		Find(&pathHistories).Error; err != nil {
		return nil, fmt.Errorf("query path history failed: %w", err)
	}

	histories := make([]PathHistory, len(pathHistories))
	for i, ph := range pathHistories {
		histories[i] = PathHistory{
			Path:  ph.Path,
			Times: ph.Times,
		}
	}

	return histories, nil
}

// GetPathHistory 根据路径获取历史记录
func GetPathHistory(path string) (*PathHistory, error) {
	if path == "" {
		return nil, errors.New("path is required")
	}

	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("get database failed: %w", err)
	}

	var ph database.DirsearchPathHistory
	if err := db.Where("path = ?", path).First(&ph).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // 未找到记录，返回 nil
		}
		return nil, fmt.Errorf("query path history failed: %w", err)
	}

	return &PathHistory{
		Path:  ph.Path,
		Times: ph.Times,
	}, nil
}

// SearchPathHistory 搜索路径历史记录（模糊匹配）
func SearchPathHistory(keyword string, limit int) ([]PathHistory, error) {
	if keyword == "" {
		return ListPathHistory(limit, 0)
	}
	if limit <= 0 {
		limit = 100
	}

	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("get database failed: %w", err)
	}

	var pathHistories []database.DirsearchPathHistory
	if err := db.Where("path LIKE ?", "%"+keyword+"%").
		Order("times DESC").
		Limit(limit).
		Find(&pathHistories).Error; err != nil {
		return nil, fmt.Errorf("query path history failed: %w", err)
	}

	histories := make([]PathHistory, len(pathHistories))
	for i, ph := range pathHistories {
		histories[i] = PathHistory{
			Path:  ph.Path,
			Times: ph.Times,
		}
	}

	return histories, nil
}

// DeletePathHistory 删除路径历史记录
func DeletePathHistory(path string) error {
	if path == "" {
		return errors.New("path is required")
	}

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("get database failed: %w", err)
	}

	if err := db.Where("path = ?", path).Delete(&database.DirsearchPathHistory{}).Error; err != nil {
		return fmt.Errorf("delete path history failed: %w", err)
	}

	return nil
}

// ClearPathHistory 清空所有路径历史记录
func ClearPathHistory() error {
	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("get database failed: %w", err)
	}

	if err := db.Where("1 = 1").Delete(&database.DirsearchPathHistory{}).Error; err != nil {
		return fmt.Errorf("clear path history failed: %w", err)
	}

	return nil
}

// GetPathHistoryStats 获取路径历史统计信息
func GetPathHistoryStats() (map[string]interface{}, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("get database failed: %w", err)
	}

	type statsResult struct {
		TotalPaths int64 `gorm:"column:total_paths"`
		TotalHits  int64 `gorm:"column:total_hits"`
		MaxHits    int   `gorm:"column:max_hits"`
	}

	var result statsResult
	// 统计总数和总和
	if err := db.Model(&database.DirsearchPathHistory{}).
		Select("COUNT(*) as total_paths, COALESCE(SUM(times), 0) as total_hits, COALESCE(MAX(times), 0) as max_hits").
		Scan(&result).Error; err != nil {
		return nil, fmt.Errorf("query stats failed: %w", err)
	}

	totalPaths := result.TotalPaths
	totalHits := result.TotalHits
	maxHits := result.MaxHits

	var maxPath string
	if maxHits > 0 {
		var ph database.DirsearchPathHistory
		if err := db.Where("times = ?", maxHits).First(&ph).Error; err == nil {
			maxPath = ph.Path
		}
	}

	stats := map[string]interface{}{
		"totalPaths": int(totalPaths),
		"totalHits":  int(totalHits),
		"maxHits":    maxHits,
		"maxPath":    maxPath,
	}

	return stats, nil
}

// extractPathFromURL 从完整URL中提取路径部分
func extractPathFromURL(resultURL string) string {
	if resultURL == "" {
		return ""
	}

	// 解析URL
	parsedURL, err := url.Parse(resultURL)
	if err != nil {
		// 如果解析失败，尝试直接提取路径部分
		// 假设格式为 http://example.com/path
		if strings.HasPrefix(resultURL, "http://") || strings.HasPrefix(resultURL, "https://") {
			parts := strings.SplitN(resultURL, "/", 4)
			if len(parts) >= 4 {
				return "/" + parts[3]
			}
		}
		return ""
	}

	// 获取路径部分（包含查询参数前的路径）
	path := parsedURL.Path

	// 规范化路径
	if path == "" || path == "/" {
		return "/"
	}

	// 去除末尾的斜杠（除非是根路径）
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		path = "/"
	}

	return path
}
