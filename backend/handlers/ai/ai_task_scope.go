package ai

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"hephaestus-backend/pkg/database"
)

func isPentestTaskReadTool(name string) bool {
	if isPentestJSScanReadTool(name) {
		return true
	}
	switch name {
	case "attack_surface_get_scan_task", "attack_surface_list_scan_results", "subdomain_get_task", "subdomain_list_results":
		return true
	default:
		return false
	}
}

func validatePentestTaskRead(ctx context.Context, name string, raw map[string]any) error {
	var args pentestJSScanTaskArgs
	if err := decodePentestToolArgs(raw, &args); err != nil {
		return err
	}
	taskID, version, err := normalizePentestTaskReference(args.TaskID, args.Version)
	if err != nil {
		return err
	}
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	query := db.WithContext(ctx).Where("id = ?", taskID)
	if version > 0 {
		query = query.Where("version = ?", version)
	}
	var task database.VulnScanTask
	if err := query.Order("version DESC").First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("扫描任务不存在: %s", taskID)
		}
		return err
	}
	return validatePentestTaskReadType(name, task)
}

func validatePentestTaskReadType(name string, task database.VulnScanTask) error {
	if (isPentestJSScanReadTool(name) && task.ScanType != "jsscan") ||
		((name == "subdomain_get_task" || name == "subdomain_list_results") && task.ScanType != "subdomain") ||
		((name == "attack_surface_get_scan_task" || name == "attack_surface_list_scan_results") && task.ScanType != "webscan" && task.ScanType != "portscan" && task.ScanType != "dirscan") {
		return fmt.Errorf("任务 %s 类型 %s 与读取工具 %s 不匹配", task.ID, task.ScanType, name)
	}
	return nil
}
