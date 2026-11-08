package collect

import (
	"fmt"
	"sync"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/types"

	"gorm.io/gorm"
)

var (
	dbMu sync.RWMutex
)

// LoadConnections 加载所有数据库连接
func LoadConnections() ([]types.DatabaseConnection, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var managers []database.DBConnectionManager
	if err := db.Find(&managers).Error; err != nil {
		return nil, fmt.Errorf("查询数据库连接失败: %w", err)
	}

	connections := make([]types.DatabaseConnection, len(managers))
	for i, m := range managers {
		connections[i] = types.DatabaseConnection{
			Nanoid:        m.Nanoid,
			Scheme:        m.Scheme,
			Host:          m.Host,
			Port:          m.Port,
			Username:      m.Username,
			Password:      m.Password,
			ServerName:    m.ServerName,
			Notes:         m.Notes,
			DatabaseCount: m.DatabaseCount,
			TableCount:    m.TableCount,
			UpdatedAt:     m.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	return connections, nil
}

// SaveConnections 保存所有数据库连接（已废弃，使用 Add/Update/Remove 方法）
func SaveConnections(connections []types.DatabaseConnection) error {
	// 这个方法保留用于向后兼容，但建议使用 Add/Update/Remove 方法
	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	// 使用事务
	return db.Transaction(func(tx *gorm.DB) error {
		// 清空现有数据
		if err := tx.Exec("DELETE FROM db_connection_manager").Error; err != nil {
			return fmt.Errorf("清空数据库连接失败: %w", err)
		}

		// 插入新数据
		managers := make([]database.DBConnectionManager, len(connections))
		for i, conn := range connections {
			managers[i] = database.DBConnectionManager{
				Nanoid:     conn.Nanoid,
				Scheme:     conn.Scheme,
				Host:       conn.Host,
				Port:       conn.Port,
				Username:   conn.Username,
				Password:   conn.Password,
				ServerName: conn.ServerName,
				Notes:      conn.Notes,
			}
		}

		if err := tx.Create(&managers).Error; err != nil {
			return fmt.Errorf("插入数据库连接失败: %w", err)
		}

		return nil
	})
}

// AddConnection 添加数据库连接
func AddConnection(conn types.DatabaseConnection) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	manager := database.DBConnectionManager{
		Nanoid:     conn.Nanoid,
		Scheme:     conn.Scheme,
		Host:       conn.Host,
		Port:       conn.Port,
		Username:   conn.Username,
		Password:   conn.Password,
		ServerName: conn.ServerName,
		Notes:      conn.Notes,
	}

	if err := db.Create(&manager).Error; err != nil {
		return fmt.Errorf("添加数据库连接失败: %w", err)
	}

	return nil
}

// UpdateConnection 更新数据库连接
func UpdateConnection(conn types.DatabaseConnection) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	result := db.Model(&database.DBConnectionManager{}).
		Where("nanoid = ?", conn.Nanoid).
		Updates(database.DBConnectionManager{
			Scheme:     conn.Scheme,
			Host:       conn.Host,
			Port:       conn.Port,
			Username:   conn.Username,
			Password:   conn.Password,
			ServerName: conn.ServerName,
			Notes:      conn.Notes,
		})

	if result.Error != nil {
		return fmt.Errorf("更新数据库连接失败: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("未找到连接: %s", conn.Nanoid)
	}

	return nil
}

// UpdateConnectionStats 更新连接统计信息（数据库数量、表数量、更新时间）
func UpdateConnectionStats(nanoid string, databaseCount, tableCount int) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	result := db.Model(&database.DBConnectionManager{}).
		Where("nanoid = ?", nanoid).
		Updates(map[string]interface{}{
			"database_count": databaseCount,
			"table_count":    tableCount,
			"updated_at":     time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("更新连接统计信息失败: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("未找到连接: %s", nanoid)
	}

	return nil
}

// RemoveConnection 删除数据库连接
func RemoveConnection(nanoid string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	result := db.Where("nanoid = ?", nanoid).Delete(&database.DBConnectionManager{})
	if result.Error != nil {
		return fmt.Errorf("删除数据库连接失败: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("未找到连接: %s", nanoid)
	}

	return nil
}

// GetConnection 获取指定连接
func GetConnection(nanoid string) (*types.DatabaseConnection, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var manager database.DBConnectionManager
	if err := db.Where("nanoid = ?", nanoid).First(&manager).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("未找到连接: %s", nanoid)
		}
		return nil, fmt.Errorf("查询数据库连接失败: %w", err)
	}

	conn := &types.DatabaseConnection{
		Nanoid:        manager.Nanoid,
		Scheme:        manager.Scheme,
		Host:          manager.Host,
		Port:          manager.Port,
		Username:      manager.Username,
		Password:      manager.Password,
		ServerName:    manager.ServerName,
		Notes:         manager.Notes,
		DatabaseCount: manager.DatabaseCount,
		TableCount:    manager.TableCount,
		UpdatedAt:     manager.UpdatedAt.Format("2006-01-02 15:04:05"),
	}

	return conn, nil
}
