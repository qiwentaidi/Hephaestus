package collect

import (
	"context"

	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
)

type CollectMigrationIssue = database.CollectMigrationIssue
type CollectMigrationReport = database.CollectMigrationReport

// MigrateCollectData shares the startup implementation with the diagnostic CLI.
func MigrateCollectData(ctx context.Context, db *gorm.DB, apply bool) (CollectMigrationReport, error) {
	return database.MigrateCollectData(ctx, db, apply)
}
