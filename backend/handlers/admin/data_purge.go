package admin

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"hephaestus-backend/lib/pentest/taskengine"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/datapurge"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/taskstate"
)

const (
	dataPurgeConfirmZH = "清空数据"
	dataPurgeConfirmEN = "CLEAR DATA"

	dataPurgeCodeInvalidConfirmation = "invalid_confirmation"
	dataPurgeCodeInProgress          = "purge_in_progress"
	dataPurgeCodeActiveTasks         = "active_tasks"
	dataPurgeCodeFailed              = "purge_failed"
)

var dataPurgeMu sync.Mutex

var purgeDataDirectories = []string{
	"ai-pentest",
	"exports",
	"favicon",
	"screenshot",
	"scan-evidence",
	"mini-program-decompile",
	"mini-program-info-cache",
	"scan-egress",
}

type dataPurgeRequest struct {
	Confirm     string `json:"confirm"`
	IncludeLogs bool   `json:"includeLogs"`
}

type dataPurgeActiveTask struct {
	Kind  string `json:"kind"`
	Count int64  `json:"count"`
}

type dataPurgeResponse struct {
	ClearedTables              int      `json:"clearedTables"`
	ClearedOrchestrationTables int      `json:"clearedOrchestrationTables"`
	DeletedDirectories         int      `json:"deletedDirectories"`
	LogsCleared                bool     `json:"logsCleared"`
	Warnings                   []string `json:"warnings"`
}

// PurgeDataHandler removes every business-data row and generated artifact while
// preserving the settings, identity, themes, templates, dictionaries, agent
// nodes and AI prompt configuration that make the instance usable.
func PurgeDataHandler(c *gin.Context) {
	var req dataPurgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
			"code":    dataPurgeCodeInvalidConfirmation,
		})
		return
	}
	if !validDataPurgeConfirmation(req.Confirm) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "确认词不正确",
			"code":    dataPurgeCodeInvalidConfirmation,
		})
		return
	}
	if !dataPurgeMu.TryLock() {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "已有数据清理任务正在执行",
			"code":    dataPurgeCodeInProgress,
		})
		return
	}
	defer dataPurgeMu.Unlock()

	datapurge.SetRunning(true)
	defer datapurge.SetRunning(false)

	db, err := database.GetDB()
	if err != nil {
		writeDataPurgeError(c, http.StatusInternalServerError, "获取数据库连接失败: "+err.Error())
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		writeDataPurgeError(c, http.StatusInternalServerError, "获取底层数据库连接失败: "+err.Error())
		return
	}

	activeTasks, err := collectActivePurgeTasks(db)
	if err != nil {
		writeDataPurgeError(c, http.StatusInternalServerError, "检查运行中任务失败: "+err.Error())
		return
	}
	if len(activeTasks) > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "存在运行中的任务，请先停止后再清理",
			"code":    dataPurgeCodeActiveTasks,
			"data": gin.H{
				"activeTasks": activeTasks,
			},
		})
		return
	}

	dataDir := config.GetDataDir()
	cleanupStalePurgeStaging(dataDir)
	stage, err := stagePurgeDirectories(dataDir)
	if err != nil {
		writeDataPurgeError(c, http.StatusInternalServerError, "暂存待删除文件失败: "+err.Error())
		return
	}
	restoreOnFailure := true
	defer func() {
		if !restoreOnFailure {
			return
		}
		if errs := stage.Restore(); len(errs) > 0 {
			logger.Warning("数据清理失败后恢复暂存文件失败: %v", errors.Join(errs...))
		}
	}()

	mainResult, err := datapurge.Purge(c.Request.Context(), datapurge.Target{
		DB:      sqlDB,
		Dialect: db.Dialector.Name(),
		Schema:  "public",
		Keep:    datapurge.MainKeepTables,
	})
	if err != nil {
		writeDataPurgeError(c, http.StatusInternalServerError, "清空业务数据失败: "+err.Error())
		return
	}
	restoreOnFailure = false

	warnings := make([]string, 0, 3)
	orchestrationResult, err := taskengine.PurgeBusinessData(c.Request.Context(), sqlDB)
	if err != nil {
		logger.Warning("清理 AI 编排数据失败: %v", err)
		warnings = append(warnings, "orchestration_purge_failed")
	}

	if errs := stage.Discard(); len(errs) > 0 {
		logger.Warning("删除暂存业务文件失败: %v", errors.Join(errs...))
		warnings = append(warnings, "staged_files_cleanup_failed")
	}

	logsCleared := false
	if req.IncludeLogs {
		var logWarnings []string
		logsCleared, logWarnings = clearRuntimeLogsIn(config.GetLogsDir())
		if len(logWarnings) > 0 {
			logger.Warning("清空运行日志时出现问题: %s", strings.Join(logWarnings, "; "))
			warnings = append(warnings, "logs_cleanup_failed")
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": dataPurgeResponse{
			ClearedTables:              len(mainResult.Tables),
			ClearedOrchestrationTables: len(orchestrationResult.Tables),
			DeletedDirectories:         stage.DiscardedCount(),
			LogsCleared:                logsCleared,
			Warnings:                   warnings,
		},
	})
}

func writeDataPurgeError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"success": false,
		"error":   message,
		"code":    dataPurgeCodeFailed,
	})
}

func validDataPurgeConfirmation(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed == dataPurgeConfirmZH || strings.EqualFold(trimmed, dataPurgeConfirmEN)
}

// collectActivePurgeTasks reports work that could still write to the database
// while the purge runs. Paused, completed, failed and cancelled tasks are safe
// to clear.
func collectActivePurgeTasks(db *gorm.DB) ([]dataPurgeActiveTask, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库连接为空")
	}
	counts := map[string]int64{}
	terminal := []string{
		taskstate.StatusCompleted,
		taskstate.StatusFailed,
		taskstate.StatusCancelled,
		taskstate.StatusPaused,
	}

	if db.Migrator().HasTable(&database.VulnScanTask{}) {
		var count int64
		if err := db.Model(&database.VulnScanTask{}).
			Where("status NOT IN ?", terminal).
			Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			counts["scan"] = count
		}
	}
	if db.Migrator().HasTable(&database.CombinationTask{}) {
		var count int64
		if err := db.Model(&database.CombinationTask{}).
			Where("status NOT IN ?", terminal).
			Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			counts["combination"] = count
		}
	}
	for _, check := range []struct {
		key   string
		model any
	}{
		{"company_info", &database.CompanyInfoTask{}},
		{"api_doc_test", &database.APIDocTestTask{}},
		{"space_export", &database.SpaceExportTask{}},
	} {
		if !db.Migrator().HasTable(check.model) {
			continue
		}
		var count int64
		if err := db.Model(check.model).
			Where("status IN ?", []string{taskstate.StatusPending, taskstate.StatusRunning}).
			Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			counts[check.key] = count
		}
	}
	if db.Migrator().HasTable("pentest_worker_turns") {
		var count int64
		if err := db.Table("pentest_worker_turns").
			Where("status IN ?", []string{"queued", "running", "cancelling", "waiting"}).
			Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			counts["ai_worker"] = count
		}
	}

	order := map[string]int{
		"scan": 0, "combination": 1, "company_info": 2, "api_doc_test": 3,
		"space_export": 4, "ai_worker": 5,
	}
	result := make([]dataPurgeActiveTask, 0, len(counts))
	for key, count := range counts {
		result = append(result, dataPurgeActiveTask{Kind: key, Count: count})
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, leftOK := order[result[i].Kind]
		right, rightOK := order[result[j].Kind]
		if leftOK && rightOK {
			return left < right
		}
		if leftOK != rightOK {
			return leftOK
		}
		return result[i].Kind < result[j].Kind
	})
	return result, nil
}

type purgeStageEntry struct {
	source string
	staged string
}

type purgeStage struct {
	root      string
	entries   []purgeStageEntry
	discarded int
}

// stagePurgeDirectories atomically moves generated-data directories out of the
// live tree before the database transaction. If the transaction fails, Restore
// puts them back; otherwise Discard deletes the staged copies.
func stagePurgeDirectories(dataDir string) (*purgeStage, error) {
	root := filepath.Join(dataDir, fmt.Sprintf(".purge-staging-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	stage := &purgeStage{root: root}
	for _, name := range purgeDataDirectories {
		source := filepath.Join(dataDir, name)
		info, err := os.Lstat(source)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, errors.Join(err, errors.Join(stage.Restore()...))
		}
		// Never follow links out of the data directory.
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		staged := filepath.Join(root, name)
		if err := os.Rename(source, staged); err != nil {
			return nil, errors.Join(err, errors.Join(stage.Restore()...))
		}
		stage.entries = append(stage.entries, purgeStageEntry{source: source, staged: staged})
	}
	return stage, nil
}

func (s *purgeStage) Restore() []error {
	if s == nil {
		return nil
	}
	var errs []error
	for i := len(s.entries) - 1; i >= 0; i-- {
		entry := s.entries[i]
		if err := os.Rename(entry.staged, entry.source); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		_ = os.Remove(s.root)
		s.entries = nil
	}
	return errs
}

func (s *purgeStage) Discard() []error {
	if s == nil {
		return nil
	}
	if err := os.RemoveAll(s.root); err != nil {
		return []error{err}
	}
	s.discarded = len(s.entries)
	s.entries = nil
	return nil
}

func (s *purgeStage) DiscardedCount() int {
	if s == nil {
		return 0
	}
	return s.discarded
}

// cleanupStalePurgeStaging removes leftovers from an interrupted earlier purge.
// The handler holds the purge mutex, so no other purge is using them.
func cleanupStalePurgeStaging(dataDir string) {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".purge-staging-") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dataDir, entry.Name())); err != nil {
			logger.Warning("清理历史数据清理暂存目录失败: %v", err)
		}
	}
}

// clearRuntimeLogsIn truncates regular log files instead of unlinking the file
// currently held open by the logger. Directories and symlinks are skipped.
func clearRuntimeLogsIn(dir string) (bool, []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, []string{err.Error()}
	}
	var warnings []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if err := os.Truncate(path, 0); err != nil {
			warnings = append(warnings, err.Error())
		}
	}
	return true, warnings
}
