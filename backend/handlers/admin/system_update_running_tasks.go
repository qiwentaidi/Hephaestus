package admin

import (
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/taskstate"
)

// updateInterruptibleScanStatuses 是重启后会真正中断（可能丢失进度）的扫描任务状态。
var updateInterruptibleScanStatuses = []string{
	taskstate.StatusRunning,
	taskstate.StatusRecovering,
	"pausing",
	"cancelling",
}

// updateInterruptibleAIStatuses 是 AI 对话/工作台仍在执行的会话状态。
var updateInterruptibleAIStatuses = []string{
	"running",
	"draining",
	"waiting_tool",
	"waiting_approval",
}

// updateRunningTaskOrder 固定前端展示顺序。
var updateRunningTaskOrder = []string{
	"webscan",
	"portscan",
	"dirsearch",
	"jsscan",
	"crack",
	"combination",
	"company_info",
	"api_doc_test",
	"space_export",
	"ai_dialogue",
}

// UpdateRunningTaskGroup 表示某一模块正在运行的任务数量。
type UpdateRunningTaskGroup struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

// UpdateRunningTaskSummary 是自动重启前用于判断是否需要人工确认的概况。
type UpdateRunningTaskSummary struct {
	Total  int64                    `json:"total"`
	Groups []UpdateRunningTaskGroup `json:"groups"`
}

// GetUpdateRunningTasks 返回当前正在运行、会被重启中断的任务概况。
func GetUpdateRunningTasks(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败"})
		return
	}

	summary := collectRunningUpdateTasks(db)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

func collectRunningUpdateTasks(db *gorm.DB) UpdateRunningTaskSummary {
	counts := map[string]int64{}

	var scanRows []struct {
		Key   string
		Count int64
	}
	if err := db.Model(&database.VulnScanTask{}).
		Select("scan_type AS key, COUNT(*) AS count").
		Where("status IN ?", updateInterruptibleScanStatuses).
		Group("scan_type").
		Scan(&scanRows).Error; err == nil {
		for _, row := range scanRows {
			key := strings.TrimSpace(row.Key)
			if key == "" {
				key = "webscan"
			}
			counts[key] += row.Count
		}
	}

	addRunningCount(db, counts, &database.CompanyInfoTask{}, "company_info")
	addRunningCount(db, counts, &database.APIDocTestTask{}, "api_doc_test")
	addRunningCount(db, counts, &database.SpaceExportTask{}, "space_export")

	var aiCount int64
	if err := db.Raw(
		`SELECT COUNT(*) FROM pentest_conversations WHERE status IN ?`,
		updateInterruptibleAIStatuses,
	).Scan(&aiCount).Error; err == nil && aiCount > 0 {
		counts["ai_dialogue"] += aiCount
	}

	groups := make([]UpdateRunningTaskGroup, 0, len(counts))
	var total int64
	for key, count := range counts {
		if count <= 0 {
			continue
		}
		groups = append(groups, UpdateRunningTaskGroup{Key: key, Count: count})
		total += count
	}

	order := make(map[string]int, len(updateRunningTaskOrder))
	for index, key := range updateRunningTaskOrder {
		order[key] = index
	}
	sort.SliceStable(groups, func(i, j int) bool {
		left, leftOK := order[groups[i].Key]
		right, rightOK := order[groups[j].Key]
		if leftOK && rightOK {
			return left < right
		}
		if leftOK != rightOK {
			return leftOK
		}
		return groups[i].Key < groups[j].Key
	})

	return UpdateRunningTaskSummary{Total: total, Groups: groups}
}

func addRunningCount(db *gorm.DB, counts map[string]int64, model any, key string) {
	var count int64
	if err := db.Model(model).Where("status IN ?", updateInterruptibleScanStatuses).Count(&count).Error; err != nil {
		return
	}
	if count > 0 {
		counts[key] += count
	}
}
