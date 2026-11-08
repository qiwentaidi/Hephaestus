package collect

import (
	"fmt"
	"sync"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/types"
)

var (
	collectMu sync.RWMutex
)

// SaveCollectResult 保存采集结果
func SaveCollectResult(connectionNanoid string, result types.CollectResult) (string, error) {
	content, err := encodeCollectData(result)
	if err != nil {
		return "", err
	}
	collectMu.Lock()
	defer collectMu.Unlock()

	db, err := database.GetDB()
	if err != nil {
		return "", fmt.Errorf("获取数据库连接失败: %w", err)
	}

	// 如果没有ID，生成一个
	if result.ID == "" {
		result.ID = database.GenerateNanoid()
	}

	collectResult := database.DBCollectResult{
		ID:               result.ID,
		ConnectionNanoid: connectionNanoid,
		TableNameField:   result.TableName,
		Content:          content,
		RowsCount:        result.RowsCount,
		MatchedType:      result.MatchedType,
		CreatedAt:        time.Now(),
	}

	if err := db.Create(&collectResult).Error; err != nil {
		return "", fmt.Errorf("保存采集结果失败: %w", err)
	}

	return result.ID, nil
}

// GetCollectResults 获取指定连接的所有采集结果
func GetCollectResults(connectionNanoid string) ([]types.CollectResult, error) {
	collectMu.RLock()
	defer collectMu.RUnlock()

	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var collectResults []database.DBCollectResult
	if err := db.Where("connection_nanoid = ?", connectionNanoid).
		Order("created_at DESC").
		Find(&collectResults).Error; err != nil {
		return nil, fmt.Errorf("查询采集结果失败: %w", err)
	}

	results := make([]types.CollectResult, len(collectResults))
	for i, cr := range collectResults {
		data, dataErr := decodeCollectData(cr.Content, cr.RowsCount)
		results[i] = types.CollectResult{
			ID:               cr.ID,
			ConnectionNanoid: cr.ConnectionNanoid,
			TableName:        cr.TableNameField,
			Columns:          data.Columns,
			Rows:             data.Rows,
			RowsCount:        cr.RowsCount,
			MatchedType:      cr.MatchedType,
			CreatedAt:        cr.CreatedAt.Format("2006-01-02 15:04:05"),
		}
		if dataErr != nil {
			results[i].DataError = dataErr.Error()
		}
	}

	return results, nil
}

// RemoveCollectResult 删除采集结果
func RemoveCollectResult(id string) error {
	collectMu.Lock()
	defer collectMu.Unlock()

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	result := db.Where("id = ?", id).Delete(&database.DBCollectResult{})
	if result.Error != nil {
		return fmt.Errorf("删除采集结果失败: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("未找到采集结果: %s", id)
	}

	return nil
}

// RemoveCollectResultsByConnection 删除指定连接的所有采集结果
func RemoveCollectResultsByConnection(connectionNanoid string) error {
	collectMu.Lock()
	defer collectMu.Unlock()

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	if err := db.Where("connection_nanoid = ?", connectionNanoid).Delete(&database.DBCollectResult{}).Error; err != nil {
		return fmt.Errorf("删除采集结果失败: %w", err)
	}

	return nil
}
