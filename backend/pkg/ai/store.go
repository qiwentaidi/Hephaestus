package ai

import (
	"context"
	"database/sql"
	"sync"

	"hephaestus-backend/lib/pentest"
	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
)

var (
	storeMigrationMu sync.Mutex
	storeMigratedDBs = make(map[*sql.DB]bool)
)

// StoreFromAppDB 基于应用数据库构造 Pentest 存储，并保证同一连接只迁移一次。
//
// 迁移在 Agent 运行时模式下跳过：Agent 不负责业务库的表结构。
func StoreFromAppDB(ctx context.Context) (*pentest.SQLStore, error) {
	gdb, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, err
	}
	dialect := ""
	if gdb.Dialector != nil {
		dialect = gdb.Dialector.Name()
	}
	store := pentest.NewSQLStoreWithDialect(sqlDB, dialect)

	storeMigrationMu.Lock()
	defer storeMigrationMu.Unlock()
	if !storeMigratedDBs[sqlDB] {
		if config.GetRuntimeMode() != config.RuntimeModeAgent {
			if err := store.Migrate(ctx); err != nil {
				return nil, err
			}
			if _, err := (assetstore.Store{DB: gdb}).BackfillAIFindingURLs(ctx, true); err != nil {
				return nil, err
			}
		}
		storeMigratedDBs[sqlDB] = true
	}
	return store, nil
}
