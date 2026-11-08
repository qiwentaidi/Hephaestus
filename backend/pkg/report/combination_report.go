package report

import (
	"fmt"
	"hephaestus-backend/pkg/database"
	"strings"
)

func loadCombinationChildrenByCombinationID(combinationID string, version int) ([]database.VulnScanTask, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}
	var children []database.VulnScanTask
	if err := db.Where("parent_combination_id = ? AND version = ? AND scan_type <> ?", combinationID, version, "combination").
		Order("step_index ASC, created_at ASC").
		Find(&children).Error; err != nil {
		return nil, fmt.Errorf("查询子任务失败: %w", err)
	}
	return children, nil
}

func loadCombinationChildren(taskID string, version int) ([]database.VulnScanTask, error) {
	task, err := loadTaskByIDAndVersion(taskID, version)
	if err != nil {
		return nil, err
	}

	combinationID := strings.TrimSpace(task.ParentCombinationID)
	if combinationID == "" {
		combinationID = taskID
	}

	return loadCombinationChildrenByCombinationID(combinationID, version)
}

func GenerateCombinationWordReportFromDB(taskID string, version int, taskName string) ([]byte, error) {
	task, err := loadTaskByIDAndVersion(taskID, version)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(taskName) == "" {
		taskName = task.TaskName
	}
	return GenerateMergedWordReportFromDB([]MergeTaskRef{{ID: taskID, Version: version}}, taskName+" - 组合任务报告")
}

func GenerateCombinationHTMLReportFromDB(taskID string, version int, taskName string) ([]byte, error) {
	task, err := loadTaskByIDAndVersion(taskID, version)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(taskName) == "" {
		taskName = task.TaskName
	}
	return GenerateMergedHTMLReportFromDB([]MergeTaskRef{{ID: taskID, Version: version}}, taskName+" - 组合任务报告")
}

func GenerateCombinationXLSXReportFromDB(taskID string, version int, taskName string) ([]byte, error) {
	task, err := loadTaskByIDAndVersion(taskID, version)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(taskName) == "" {
		taskName = task.TaskName
	}
	return GenerateMergedXLSXReportFromDB([]MergeTaskRef{{ID: taskID, Version: version}}, taskName+" - 组合任务报告")
}
