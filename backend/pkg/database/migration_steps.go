package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"reflect"
	"strings"
	"time"

	"hephaestus-backend/pkg/database/migrations"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/version"

	"golang.org/x/mod/semver"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BeforeAutoMigrate lets packages upgrade legacy storage before schema sync.
// Register it before startup migration; the upgrade must be idempotent.
var BeforeAutoMigrate func(*gorm.DB) error

func AutoMigrate(db *gorm.DB) error {
	ctx := context.Background()
	if db != nil && db.Statement != nil && db.Statement.Context != nil {
		ctx = db.Statement.Context
	}
	return migrations.Run(ctx, db, migrationSteps())
}

// migrationSteps is the ordered upgrade list. Order matters: legacy structures
// and data are upgraded before GORM rebuilds tables, and backfills run after the
// schema is current. All releases register the same historical steps; the
// database completion ledger, not the running application version, selects work.
func migrationSteps() []migrations.Step {
	schema := migrations.Step{ID: schemaSyncStepID(), Name: "同步数据库表结构", Up: syncSchema}
	steps := []migrations.Step{
		{ID: "0001_vuln_scan_task_primary_key", Name: "升级 vul_scan_task 主键结构", Up: once(migrateVulnScanTaskPrimaryKey)},
		// Backfill before schema sync can enforce the new primary key.
		{ID: "0012_web_vulnerability_ids", Name: "补齐 Web 漏洞独立 ID", Up: once(migrateWebVulnerabilityIDs)},
	}
	// Do not record completion if the entry point did not register the upgrader.
	if BeforeAutoMigrate != nil {
		steps = append(steps, migrations.Step{ID: "0013_legacy_pre_schema", Name: "升级旧计划任务结构", Up: once(BeforeAutoMigrate)})
	}
	steps = append(steps, []migrations.Step{
		// SQLite's schema rebuild must not retain the obsolete task foreign key.
		{ID: "0014_subdomain_pre_schema", Name: "解除旧子域名结果外键", Up: once(prepareLegacySubdomainResults), OutsideTx: true},
		schema,
		{ID: "0002_legacy_subdomain_tasks", Name: "升级旧子域名任务数据", Up: once(migrateAndDropLegacySubdomainTasks), OutsideTx: true},
		{ID: "0003_subdomain_result_columns", Name: "补齐子域名结果表结构", Up: once(migrateSubdomainResultColumns)},
		{ID: "0004_drop_legacy_tables", Name: "清理已废弃的旧表", Up: once(dropLegacyTables)},
		{ID: "0006_port_scan_result_columns", Name: "补齐端口扫描结果列", Up: once(ensurePortScanResultColumns)},
		{ID: "0007_repeater_tab_columns", Name: "补齐重放标签页列", Up: once(ensureRepeaterTabColumns)},
		{ID: "0008_scan_agent_node_columns", Name: "补齐扫描节点列", Up: once(ensureScanAgentNodeColumns)},
		{ID: "0009_pentest_scan_task_origin", Name: "回填 AI 扫描任务来源", Up: once(backfillPentestScanTaskOrigins)},
		{ID: "0010_unified_asset_ledger", Name: "回填统一资产台账字段", Up: once(backfillUnifiedAssetLedgerFields)},
		{ID: "0011_collect_data", Name: "合并采集结果为结构化数据", Up: migrateCollectDataStep},
		{ID: "0015_cancelled_combination_status", Name: "同步已取消组合任务状态", Up: once(syncCancelledCombinationStatus)},
	}...)
	return steps
}

func prepareLegacySubdomainResults(db *gorm.DB) error {
	if db.Migrator().HasTable("subdomain_scan_task") {
		return dropLegacySubdomainResultForeignKey(db)
	}
	if db.Dialector.Name() == "sqlite" {
		// Previous SQLite rebuilds used tab-indented DDL. The SQLite migrator
		// treats tabs as quote delimiters, losing columns during a later rebuild.
		var ddl string
		if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", "subdomain_scan_result").Scan(&ddl).Error; err != nil {
			return err
		}
		if strings.Contains(ddl, "\t") {
			return dropLegacySubdomainResultForeignKey(db)
		}
	}
	return nil
}

func once(fn func(*gorm.DB) error) func(context.Context, *gorm.DB) (bool, error) {
	return func(_ context.Context, db *gorm.DB) (bool, error) {
		return true, fn(db)
	}
}

// syncSchema lets GORM synchronise the current model set at any runtime version.
// The step ID embeds the release and a fingerprint of the model
// definitions, so the schema sync runs once per release (or whenever a model
// changes) instead of on every startup.
func syncSchema(_ context.Context, db *gorm.DB) (bool, error) {
	// Keep exactly one schema snapshot row: older fingerprints are meaningless
	// once the current model set has been synchronised.
	if err := db.Exec("DELETE FROM "+migrations.TableName+" WHERE version LIKE ?", "schema_sync:%").Error; err != nil {
		return false, fmt.Errorf("清理旧表结构版本失败: %w", err)
	}
	if err := db.AutoMigrate(schemaModels()...); err != nil {
		return false, err
	}
	return true, nil
}

func schemaSyncStepID() string {
	sum := sha256.Sum256([]byte(version.Version + "|" + version.DesktopVersion + "|" + schemaModelFingerprint()))
	return "schema_sync:" + hex.EncodeToString(sum[:8])
}

func schemaModelFingerprint() string {
	digest := sha256.New()
	for _, model := range schemaModels() {
		writeTypeFingerprint(digest, reflect.TypeOf(model))
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func writeTypeFingerprint(digest hash.Hash, modelType reflect.Type) {
	if modelType == nil {
		return
	}
	for modelType.Kind() == reflect.Pointer || modelType.Kind() == reflect.Slice {
		modelType = modelType.Elem()
	}
	if modelType.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			writeTypeFingerprint(digest, field.Type)
			continue
		}
		fmt.Fprintf(digest, "%s %s %s\n", field.Name, field.Type.String(), field.Tag.Get("gorm"))
	}
	fmt.Fprint(digest, "---\n")
}

// schemaModels is the single source of truth for GORM schema synchronisation.
func schemaModels() []any {
	return []any{

		&DBConnectionManager{},
		&SpaceEngineSyntax{},
		&SpaceEngineSearchHistory{},
		&MCPSpaceEngineAuditLog{},
		&Setting{},
		&CustomTheme{},
		&DirsearchPathHistory{},
		&VulnScanTask{},
		&VulnScanTaskItem{},
		&ScanSchedule{},
		&ScanScheduleOccurrence{},
		&FingerprintInfo{},
		&VulnerabilityInfo{},
		&PortScanResult{},
		&PortScanResultAsset{},
		&SubdomainScanResult{},
		&DBCollectResult{},
		&DBCollectCustomRule{},
		&ScanTemplate{},
		&CompanyInfoTask{},
		&CompanyInfoTaskResult{},
		&APIDocTestTask{},
		&APIDocTestResult{},
		&RepeaterTab{},
		&PoCDebugTab{},
		&User{},
		&KeyManagerEntry{},
		// 新的 VulnScan 相关的 JS 扫描结果表
		&VulnScanJSVulnerability{},
		&VulnScanJSAsset{},
		&VulnScanJSSiteTree{},
		&VulnScanJSResource{},
		&VulnScanJSAPIResource{},
		&VulnScanJSOperation{},
		&VulnScanJSOpenAPIDocument{},
		// 新的 VulnScan 相关的目录扫描结果表
		&VulnScanDirResult{},
		&JSScanConfig{},
		&SpaceExportTask{},
		&SpaceSearchHistory{},
		&CombinationTask{},
		&ScanAgentNode{},
		&AgentUpdate{},
		&AgentPythonConfig{},
		&ScanAgentCommand{},
		&ScanAssetFile{},
		&SystemStatsSample{},
		&UnifiedAsset{},
		&UnifiedAssetObservation{},
		&UnifiedAssetRelation{},
		&UnifiedAssetChange{},
		&SecurityFinding{},
		&FindingAssetLink{},
		&AssetRisk{},
		&AIAssetReference{},
		&AIWorkbenchTask{},
		&AIWorkbenchTaskStep{},
		&AIWorkbenchPlanTask{},
		&AIWorkbenchApproval{},
		&AIWorkbenchObservation{},
		&AIWorkbenchEvidenceItem{},
		&AIWorkbenchHypothesis{},
		&AIWorkbenchToolCall{},
		&AIWorkbenchToolExecution{},
		&AIWorkbenchAgentRun{},
		&AIWorkbenchFinding{},
		&AIWorkbenchMessage{},
		&AIWorkbenchSideChat{},
		&AIWorkbenchEvent{},
		&AIWorkbenchAssetBucket{},
		&AIWorkbenchGoalEvaluation{},
		&AIWorkbenchAPIEndpoint{},
		&AIWorkbenchAPIRelation{},
		&AIWorkbenchAuthProfile{},
		&AIWorkbenchExtractedVariable{},
		&AIWorkbenchHTTPRequestEvidence{},
		&AIWorkbenchCustomStrategyPack{}}
}

// migrateSubdomainResultColumns creates or upgrades subdomain_scan_result. The
// table is created by schema sync, so this only repairs databases that predate
// the DNS error column.
func migrateSubdomainResultColumns(db *gorm.DB) error {
	if !db.Migrator().HasTable(&SubdomainScanResult{}) {
		return db.AutoMigrate(&SubdomainScanResult{})
	}
	if !db.Migrator().HasColumn(&SubdomainScanResult{}, "ResolutionError") {
		if err := db.Migrator().AddColumn(&SubdomainScanResult{}, "ResolutionError"); err != nil {
			return fmt.Errorf("添加子域名DNS错误列失败: %w", err)
		}
	}
	return nil
}

// dropLegacyTables removes storage that no released version reads any more.
func dropLegacyTables(db *gorm.DB) error {
	for _, table := range []string{
		"subdomain_history_result",
		"subdomain_history",
		"ai_assistant_session_task_link",
		"ai_assistant_session_message",
		"ai_assistant_session",
	} {
		if !db.Migrator().HasTable(table) {
			continue
		}
		if err := db.Migrator().DropTable(table); err != nil {
			return fmt.Errorf("删除 %s 表失败: %w", table, err)
		}
	}
	return nil
}

// migrateCollectDataStep converts legacy HTML collection payloads into the
// structured columns/rows format. Concurrent changes defer the step to the next
// startup instead of failing it, matching the previous retry behaviour.
func migrateCollectDataStep(_ context.Context, db *gorm.DB) (bool, error) {
	return runCollectDataMigration(db)
}

func migrateVulnScanTaskPrimaryKey(db *gorm.DB) error {
	if db == nil || db.Dialector == nil || db.Dialector.Name() != "postgres" {
		// 这套历史主键迁移依赖 PostgreSQL 的 information_schema / pg_index / regclass。
		// 桌面端 SQLite 以及其他驱动都直接交给 AutoMigrate 处理当前结构。
		return nil
	}

	// 检查表是否存在
	var tableExists bool
	err := db.Raw(`
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = 'vul_scan_task'
		)
	`).Scan(&tableExists).Error
	if err != nil {
		return fmt.Errorf("检查表是否存在失败: %w", err)
	}

	if !tableExists {
		// 表不存在，AutoMigrate 会创建新表，直接返回
		return nil
	}

	// 检查当前主键约束
	var constraintName string
	err = db.Raw(`
		SELECT constraint_name
		FROM information_schema.table_constraints
		WHERE table_schema = 'public'
		AND table_name = 'vul_scan_task'
		AND constraint_type = 'PRIMARY KEY'
	`).Scan(&constraintName).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("检查主键约束失败: %w", err)
	}

	// 检查 version 列是否存在
	var versionColumnExists bool
	err = db.Raw(`
		SELECT EXISTS (
			SELECT FROM information_schema.columns
			WHERE table_schema = 'public'
			AND table_name = 'vul_scan_task'
			AND column_name = 'version'
		)
	`).Scan(&versionColumnExists).Error
	if err != nil {
		return fmt.Errorf("检查 version 列是否存在失败: %w", err)
	}

	// 如果 version 列不存在，先添加它
	if !versionColumnExists {
		if err := db.Exec(`
			ALTER TABLE vul_scan_task 
			ADD COLUMN version INTEGER NOT NULL DEFAULT 1
		`).Error; err != nil {
			return fmt.Errorf("添加 version 列失败: %w", err)
		}
	} else {
		// 如果 version 列已存在，检查是否有 NULL 值，如果有则设置为 1
		if err := db.Exec(`
			UPDATE vul_scan_task 
			SET version = 1 
			WHERE version IS NULL
		`).Error; err != nil {
			return fmt.Errorf("更新 NULL version 值失败: %w", err)
		}
		// 确保 version 列不允许 NULL
		if err := db.Exec(`
			ALTER TABLE vul_scan_task 
			ALTER COLUMN version SET NOT NULL,
			ALTER COLUMN version SET DEFAULT 1
		`).Error; err != nil {
			// 如果失败，可能是已经设置了，继续执行
		}
	}

	// 检查主键是否已经是联合主键
	var primaryKeyColumns []string
	rows, err := db.Raw(`
		SELECT a.attname
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = 'vul_scan_task'::regclass
		AND i.indisprimary
		ORDER BY a.attnum
	`).Rows()
	if err != nil {
		// 如果没有找到主键约束，说明表可能还没有主键，继续执行迁移
		if err == gorm.ErrRecordNotFound {
			// 继续执行，添加联合主键
		} else {
			return fmt.Errorf("检查主键列失败: %w", err)
		}
	} else if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var colName string
			if scanErr := rows.Scan(&colName); scanErr != nil {
				rows.Close()
				return fmt.Errorf("扫描主键列名失败: %w", scanErr)
			}
			primaryKeyColumns = append(primaryKeyColumns, colName)
		}
		// 检查是否有扫描错误
		if err := rows.Err(); err != nil {
			return fmt.Errorf("遍历主键列时出错: %w", err)
		}
	}

	// 如果主键已经是联合主键 (id, version)，不需要迁移
	if len(primaryKeyColumns) == 2 {
		hasID := false
		hasVersion := false
		for _, col := range primaryKeyColumns {
			if col == "id" {
				hasID = true
			}
			if col == "version" {
				hasVersion = true
			}
		}
		if hasID && hasVersion {
			// 已经是联合主键，不需要迁移
			return nil
		}
	}

	// 需要迁移：删除旧主键，添加新联合主键
	// 首先检查是否有重复的 (id, version) 组合
	var duplicateCount int64
	if err := db.Raw(`
		SELECT COUNT(*) 
		FROM (
			SELECT id, version, COUNT(*) as cnt
			FROM vul_scan_task
			GROUP BY id, version
			HAVING COUNT(*) > 1
		) duplicates
	`).Scan(&duplicateCount).Error; err != nil {
		return fmt.Errorf("检查重复记录失败: %w", err)
	}

	if duplicateCount > 0 {
		// 如果有重复记录，需要先处理它们
		// 为每个重复的 (id, version) 组合分配新的 version 号
		if err := db.Exec(`
			WITH numbered AS (
				SELECT ctid, id, version,
					ROW_NUMBER() OVER (PARTITION BY id, version ORDER BY created_at) as rn
				FROM vul_scan_task
			),
			max_versions AS (
				SELECT id, MAX(version) as max_version
				FROM vul_scan_task
				GROUP BY id
			)
			UPDATE vul_scan_task t
			SET version = mv.max_version + n.rn
			FROM numbered n
			JOIN max_versions mv ON n.id = mv.id
			WHERE t.ctid = n.ctid AND n.rn > 1
		`).Error; err != nil {
			return fmt.Errorf("处理重复记录失败: %w", err)
		}
	}

	if constraintName != "" {
		// 删除旧的主键约束
		if err := db.Exec(fmt.Sprintf(`
			ALTER TABLE vul_scan_task 
			DROP CONSTRAINT IF EXISTS %s
		`, constraintName)).Error; err != nil {
			return fmt.Errorf("删除旧主键约束失败: %w", err)
		}
	}

	// 添加新的联合主键
	if err := db.Exec(`
		ALTER TABLE vul_scan_task 
		ADD PRIMARY KEY (id, version)
	`).Error; err != nil {
		return fmt.Errorf("添加联合主键失败: %w", err)
	}

	return nil
}

func ensurePortScanResultColumns(db *gorm.DB) error {
	if db.Migrator().HasTable(&PortScanResult{}) {
		if !db.Migrator().HasColumn(&PortScanResult{}, "product") {
			if err := db.Exec(`ALTER TABLE port_scan_result ADD COLUMN IF NOT EXISTS product VARCHAR(255)`).Error; err != nil {
				return err
			}
		}
		if !db.Migrator().HasColumn(&PortScanResult{}, "service_version") {
			if err := db.Exec(`ALTER TABLE port_scan_result ADD COLUMN IF NOT EXISTS service_version VARCHAR(255)`).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureRepeaterTabColumns(db *gorm.DB) error {
	if db.Migrator().HasTable(&RepeaterTab{}) {
		if !db.Migrator().HasColumn(&RepeaterTab{}, "response_length") {
			if err := db.Exec(`ALTER TABLE repeater_tab ADD COLUMN IF NOT EXISTS response_length INTEGER DEFAULT 0`).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureScanAgentNodeColumns(db *gorm.DB) error {
	if db.Migrator().HasTable(&ScanAgentNode{}) {
		for _, field := range []string{"AIWorker", "ScanWorker", "LocalWorker", "AIConcurrency", "ScanConcurrency", "SupportedTypes", "NetworkZones", "PlatformTokenHash", "PlatformSessionPID"} {
			if !db.Migrator().HasColumn(&ScanAgentNode{}, field) {
				if err := db.Migrator().AddColumn(&ScanAgentNode{}, field); err != nil {
					return err
				}
			}
		}
	}

	if db.Migrator().HasTable(&ScanAgentNode{}) {
		if !db.Migrator().HasColumn(&ScanAgentNode{}, "agent_version") {
			if err := db.Exec(`ALTER TABLE scan_agent_node ADD COLUMN IF NOT EXISTS agent_version VARCHAR(50) DEFAULT ''`).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// legacySubdomainScanTask is retained only long enough to migrate installations
// that predate the unified attack-surface task table. Runtime code must not use it.
type legacySubdomainScanTask struct {
	ID             string     `gorm:"primaryKey;column:id"`
	Status         string     `gorm:"column:status"`
	Progress       int        `gorm:"column:progress"`
	TotalCount     int        `gorm:"column:total_count"`
	CurrentCount   int        `gorm:"column:current_count"`
	ResultCount    int        `gorm:"column:result_count"`
	ProgressDetail string     `gorm:"column:progress_detail"`
	Config         string     `gorm:"column:config"`
	ErrorMsg       string     `gorm:"column:error_msg"`
	StartedAt      *time.Time `gorm:"column:started_at"`
	CompletedAt    *time.Time `gorm:"column:completed_at"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
}

func (legacySubdomainScanTask) TableName() string { return "subdomain_scan_task" }

func migrateAndDropLegacySubdomainTasks(db *gorm.DB) error {
	if !db.Migrator().HasTable("subdomain_scan_task") {
		return nil
	}

	var legacyTasks []legacySubdomainScanTask
	if err := db.Table("subdomain_scan_task").Find(&legacyTasks).Error; err != nil {
		return fmt.Errorf("读取旧子域名任务失败: %w", err)
	}
	for _, legacy := range legacyTasks {
		var current VulnScanTask
		lookup := db.Where("id = ?", legacy.ID).Order("version DESC").Limit(1).Find(&current)
		if lookup.Error != nil {
			return fmt.Errorf("查询统一子域名任务 %s 失败: %w", legacy.ID, lookup.Error)
		}
		if lookup.RowsAffected > 0 {
			// 旧版组合任务复用父任务 ID 依次执行各子步骤，子域名步骤也会在
			// subdomain_scan_task 中写入同 ID 的临时记录。组合任务结束后父任务
			// 的 scan_type 是 combination（或中断时停留在某个步骤类型），这类
			// 记录并不是与统一任务冲突的新任务：子域名结果本就挂在父任务 ID
			// 下，保留即可，但不能再把旧进度回写到组合任务上。
			if isCombinationParentTask(current) {
				logger.Warning("旧子域名任务 %s 属于组合任务（当前类型 %s），保留统一任务并跳过进度合并", legacy.ID, current.ScanType)
				continue
			}
			// 其他类型的同 ID 任务无法与旧子域名任务对应。宁可跳过这条旧记录，
			// 也不能让桌面端因为历史数据无法启动；旧结果仍按 task_id 保留。
			if current.ScanType != "subdomain" {
				logger.Warning("旧子域名任务 %s 与统一任务类型 %s 冲突，跳过该条旧任务并保留统一任务", legacy.ID, current.ScanType)
				continue
			}
			if err := db.Model(&VulnScanTask{}).
				Where("id = ? AND version = ?", current.ID, current.Version).
				Updates(map[string]interface{}{
					"current_count":   legacy.CurrentCount,
					"progress_detail": legacy.ProgressDetail,
				}).Error; err != nil {
				return fmt.Errorf("补全统一子域名任务 %s 失败: %w", legacy.ID, err)
			}
			continue
		}

		config := make(map[string]interface{})
		if strings.TrimSpace(legacy.Config) != "" {
			_ = json.Unmarshal([]byte(legacy.Config), &config)
		}
		if config == nil {
			config = make(map[string]interface{})
		}
		targets := legacySubdomainTargets(config)
		if len(targets) == 0 && db.Migrator().HasTable(&SubdomainScanResult{}) {
			if err := db.Model(&SubdomainScanResult{}).
				Where("task_id = ?", legacy.ID).
				Distinct("domain").Order("domain ASC").Pluck("domain", &targets).Error; err != nil {
				return fmt.Errorf("恢复旧子域名任务 %s 的目标失败: %w", legacy.ID, err)
			}
		}
		config["domains"] = targets
		config["targets"] = targets
		targetsJSON, err := json.Marshal(targets)
		if err != nil {
			return err
		}
		configJSON, err := json.Marshal(config)
		if err != nil {
			return err
		}
		createdAt := legacy.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		updatedAt := legacy.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = createdAt
		}
		status := strings.TrimSpace(legacy.Status)
		if status == "" {
			status = "pending"
		}
		taskName, _ := config["taskName"].(string)
		task := VulnScanTask{
			ID: legacy.ID, Version: 1, TaskName: strings.TrimSpace(taskName),
			ScanMode: "local", ScanType: "subdomain",
			Targets: string(targetsJSON), Status: status, Progress: legacy.Progress,
			TotalCount: legacy.TotalCount, CurrentCount: legacy.CurrentCount,
			ProgressDetail: legacy.ProgressDetail, StartedAt: legacy.StartedAt,
			CompletedAt: legacy.CompletedAt, Config: string(configJSON), ErrorMsg: legacy.ErrorMsg,
			CreatedAt: createdAt, UpdatedAt: updatedAt,
		}
		if err := db.Create(&task).Error; err != nil {
			return fmt.Errorf("迁移旧子域名任务 %s 失败: %w", legacy.ID, err)
		}
	}

	if err := dropLegacySubdomainResultForeignKey(db); err != nil {
		return fmt.Errorf("解除旧子域名结果外键失败: %w", err)
	}
	if err := db.Migrator().DropTable("subdomain_scan_task"); err != nil {
		return fmt.Errorf("删除旧子域名任务表失败: %w", err)
	}
	return nil
}

func dropLegacySubdomainResultForeignKey(db *gorm.DB) error {
	if !db.Migrator().HasTable(&SubdomainScanResult{}) {
		return nil
	}
	if db.Dialector.Name() != "sqlite" {
		return db.Exec(`ALTER TABLE subdomain_scan_result DROP CONSTRAINT IF EXISTS fk_subdomain_scan_result_task`).Error
	}

	// Older releases did not have every column in the current result model
	// (notably `sources`). Rebuild the table using only columns present in the
	// legacy schema so that the FK migration also works for those databases.
	columns := []string{"id", "task_id", "domain", "subdomain", "ips", "asset_type", "asset_name", "source", "sources", "is_wildcard", "created_at", "resolution_error"}
	var existingColumns []string
	for _, column := range columns {
		if db.Migrator().HasColumn("subdomain_scan_result", column) {
			existingColumns = append(existingColumns, column)
		}
	}
	if len(existingColumns) == 0 {
		return fmt.Errorf("旧子域名结果表没有可迁移的列")
	}
	columnList := strings.Join(existingColumns, ", ")

	return db.Connection(func(connection *gorm.DB) error {
		if err := connection.Exec(`PRAGMA foreign_keys=OFF`).Error; err != nil {
			return err
		}
		defer connection.Exec(`PRAGMA foreign_keys=ON`)
		return connection.Transaction(func(tx *gorm.DB) error {
			statements := []string{
				`DROP TABLE IF EXISTS subdomain_scan_result_unlinked`,
				`CREATE TABLE subdomain_scan_result_unlinked (
					id varchar(255) PRIMARY KEY,
					task_id varchar(255) NOT NULL,
					domain varchar(255) NOT NULL,
					subdomain varchar(255) NOT NULL,
					ips text NOT NULL,
					asset_type varchar(50),
					asset_name varchar(255),
					source varchar(100),
					sources text,
					is_wildcard numeric DEFAULT false,
					created_at datetime DEFAULT CURRENT_TIMESTAMP,
					resolution_error text
				)`,
				fmt.Sprintf(`INSERT INTO subdomain_scan_result_unlinked (%s)
				 SELECT %s FROM subdomain_scan_result`, columnList, columnList),
				`DROP TABLE subdomain_scan_result`,
				`ALTER TABLE subdomain_scan_result_unlinked RENAME TO subdomain_scan_result`,
				`CREATE INDEX IF NOT EXISTS idx_subdomain_scan_result_task_id ON subdomain_scan_result(task_id)`,
				`CREATE INDEX IF NOT EXISTS idx_subdomain_scan_result_subdomain ON subdomain_scan_result(subdomain)`,
			}
			for _, statement := range statements {
				// Keep stored DDL compatible with GORM's SQLite DDL parser.
				if err := tx.Exec(strings.ReplaceAll(statement, "\t", " ")).Error; err != nil {
					return err
				}
			}
			return nil
		})
	})
}

func legacySubdomainTargets(config map[string]interface{}) []string {
	for _, key := range []string{"domains", "targets"} {
		values, ok := config[key].([]interface{})
		if !ok {
			if stringsList, ok := config[key].([]string); ok {
				return stringsList
			}
			continue
		}
		result := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				result = append(result, strings.TrimSpace(text))
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return nil
}

// backfillUnifiedAssetLedgerFields upgrades the first unified-catalog schema to
// the ledger schema. Existing scan observations become "observed"; everything
// else starts as a declared ledger entry. The statements are idempotent and run
// after AutoMigrate has added the new columns.
func backfillUnifiedAssetLedgerFields(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&UnifiedAsset{}) || !db.Migrator().HasTable(&UnifiedAssetObservation{}) {
		return nil
	}
	if err := db.Exec(`UPDATE unified_assets SET verification = 'observed'
		WHERE (verification IS NULL OR verification = '')
		AND id IN (
			SELECT DISTINCT asset_id FROM unified_asset_observations
			WHERE source NOT IN ('manual', 'import', 'csv', 'excel')
		)`).Error; err != nil {
		return fmt.Errorf("backfill unified asset verification failed: %w", err)
	}
	if err := db.Exec(`UPDATE unified_assets SET verification = 'declared'
		WHERE verification IS NULL OR verification = ''`).Error; err != nil {
		return fmt.Errorf("backfill declared unified assets failed: %w", err)
	}
	if err := db.Exec(`UPDATE unified_asset_observations SET origin = CASE
		WHEN source IN ('manual') THEN 'manual'
		WHEN source IN ('import', 'csv', 'excel') THEN 'import'
		ELSE 'scan' END
		WHERE origin IS NULL OR origin = ''`).Error; err != nil {
		return fmt.Errorf("backfill unified observation origin failed: %w", err)
	}
	return nil
}

// backfillPentestScanTaskOrigins identifies older AI-created scans from saved
// tool results. Task names and scan options are not reliable origin signals.
func backfillPentestScanTaskOrigins(db *gorm.DB) error {
	if !db.Migrator().HasTable("pentest_tool_executions") {
		return nil
	}
	toolNames := []string{
		"attack_surface_create_webscan_task",
		"attack_surface_create_portscan_task",
		"attack_surface_create_jsscan_task",
		"attack_surface_create_dirscan_task",
		"attack_surface_create_crack_task",
		"subdomain_create_task",
	}
	rows, err := db.Table("pentest_tool_executions").
		Select("result_json").
		Where("status = ? AND tool_name IN ?", "completed", toolNames).
		Rows()
	if err != nil {
		return fmt.Errorf("读取 AI 扫描工具记录失败: %w", err)
	}
	defer rows.Close()

	ids := make(map[string]struct{})
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return fmt.Errorf("读取 AI 扫描工具结果失败: %w", err)
		}
		var result struct {
			OK     bool `json:"ok"`
			Result struct {
				TaskID string `json:"taskId"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(raw), &result) == nil && result.OK {
			if id := strings.TrimSpace(result.Result.TaskID); id != "" {
				ids[id] = struct{}{}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历 AI 扫描工具记录失败: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("关闭 AI 扫描工具记录失败: %w", err)
	}
	for id := range ids {
		if err := db.Model(&VulnScanTask{}).Where("id = ? AND created_by_ai = ?", id, false).
			Update("created_by_ai", true).Error; err != nil {
			return fmt.Errorf("标记历史 AI 扫描任务 %s 失败: %w", id, err)
		}
	}
	return nil
}

// --- 采集结果合并迁移（HTML -> columns/rows）---
//
// The setting below is retained only for diagnostics and serialization. It does
// not decide whether to run; schema_migrations tracks step completion.
const collectDataMigrationVersion = "v0.5.5"
const collectDataMigrationSettingKey = "db_collect_data_migration_version"

type collectDataMigrationState struct {
	Version        string                 `json:"version"`
	ServerVersion  string                 `json:"serverVersion,omitempty"`
	DesktopVersion string                 `json:"desktopVersion,omitempty"`
	CompletedAt    time.Time              `json:"completedAt,omitempty"`
	Report         CollectMigrationReport `json:"report"`
}

// Run only on the schema owner after AutoMigrate has created both tables.
// Use the supplied DB, never GetDB/GetSetting: dbOnce is still initializing.
func migrateCollectDataOnStartup(db *gorm.DB) error {
	_, err := runCollectDataMigration(db)
	return err
}

func runCollectDataMigration(db *gorm.DB) (bool, error) {
	var appliedReport *CollectMigrationReport
	if err := db.Transaction(func(tx *gorm.DB) error {
		// Retain the report row lock to serialize concurrent instances. Neither
		// this row nor schema_migrations can suppress an eligible migration.
		initial := Setting{Key: collectDataMigrationSettingKey, Value: `{"version":"v0.0.0"}`}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&initial).Error; err != nil {
			return fmt.Errorf("初始化采集数据迁移版本失败: %w", err)
		}
		var setting Setting
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&setting, "key = ?", collectDataMigrationSettingKey).Error; err != nil {
			return fmt.Errorf("读取采集数据迁移版本失败: %w", err)
		}
		report, err := MigrateCollectData(tx.Statement.Context, tx, true)
		if err != nil {
			return fmt.Errorf("自动迁移数据库采集结果失败: %w", err)
		}
		appliedReport = &report
		state := collectDataMigrationState{
			Version: collectDataMigrationVersion, ServerVersion: version.Version,
			DesktopVersion: version.DesktopVersion, Report: report,
		}
		if report.Conflicted == 0 {
			state.CompletedAt = time.Now().UTC()
		}
		encoded, err := json.Marshal(state)
		if err != nil {
			return err
		}
		// The conversions and diagnostic report commit atomically.
		return tx.Model(&setting).Update("value", string(encoded)).Error
	}); err != nil {
		return false, err
	}
	if appliedReport != nil {
		report := appliedReport
		if report.Migrated > 0 || report.Failed > 0 || report.Conflicted > 0 {
			logger.Info("数据库采集结果自动迁移: 已转换 %d 条，无法转换 %d 条，并发变化 %d 条", report.Migrated, report.Failed, report.Conflicted)
		}
		for _, issue := range report.Issues {
			logger.Warning("数据库采集结果 %s 保留原文: %s", issue.ID, issue.Reason)
		}
	}
	return appliedReport != nil && appliedReport.Conflicted == 0, nil
}

func readCollectMigrationState(value string) (collectDataMigrationState, error) {
	var state collectDataMigrationState
	if err := json.Unmarshal([]byte(value), &state); err != nil || !semver.IsValid(state.Version) {
		return state, fmt.Errorf("采集数据迁移版本记录无效")
	}
	return state, nil
}
