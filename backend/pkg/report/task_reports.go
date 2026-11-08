package report

import (
	"encoding/json"
	"fmt"
	"hephaestus-backend/pkg/database"
	"html"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

func loadTaskByIDAndVersion(taskID string, version int) (database.VulnScanTask, error) {
	db, err := database.GetDB()
	if err != nil {
		return database.VulnScanTask{}, fmt.Errorf("获取数据库连接失败: %w", err)
	}
	var task database.VulnScanTask
	if err := db.Where("id = ? AND version = ?", taskID, version).First(&task).Error; err != nil {
		return database.VulnScanTask{}, fmt.Errorf("任务不存在: %w", err)
	}
	return task, nil
}

func isReportExportableTaskStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "success", "paused", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func parseTargets(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var targets []string
	if err := json.Unmarshal([]byte(raw), &targets); err == nil {
		return targets
	}
	return []string{raw}
}

func sanitizeSheetName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Sheet"
	}
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		"*", "_",
		"[", "_",
		"]", "_",
		":", "_",
		"?", "_",
	)
	name = replacer.Replace(name)
	r := []rune(name)
	if len(r) > 31 {
		name = string(r[:31])
	}
	return name
}

func writeTaskHeaderRTF(sb *strings.Builder, title string, task database.VulnScanTask, taskName string, version int) {
	reportTitle := title
	if strings.TrimSpace(taskName) != "" {
		reportTitle = fmt.Sprintf("%s - %s", taskName, title)
	}
	sb.WriteString("{\\rtf1\\ansi\\deff0{\\fonttbl{\\f0 Arial;}{\\f1 SimSun;}}\\uc1\n")
	sb.WriteString("\\fs32\\b ")
	sb.WriteString(rtfEscapeText(reportTitle))
	sb.WriteString("\\b0\\par\n")
	sb.WriteString("\\fs20 ")
	sb.WriteString(rtfEscapeText(fmt.Sprintf("生成时间: %s", time.Now().Format("2006-01-02 15:04:05"))))
	sb.WriteString("\\par\n")
	sb.WriteString(rtfEscapeText(fmt.Sprintf("任务版本: %d", version)))
	sb.WriteString("\\par\n")
	sb.WriteString(rtfEscapeText(fmt.Sprintf("任务状态: %s", task.Status)))
	sb.WriteString("\\par\n")
	if !task.CreatedAt.IsZero() {
		sb.WriteString(rtfEscapeText(fmt.Sprintf("开始时间: %s", task.CreatedAt.Format("2006-01-02 15:04:05"))))
		sb.WriteString("\\par\n")
	}
	if task.CompletedAt != nil {
		sb.WriteString(rtfEscapeText(fmt.Sprintf("完成时间: %s", task.CompletedAt.Format("2006-01-02 15:04:05"))))
		sb.WriteString("\\par\n")
	}
	sb.WriteString("\\par\n")
}

func setSummarySheet(f *excelize.File, sheetName string, taskID string, task database.VulnScanTask, taskName string, version int) {
	f.SetCellValue(sheetName, "A1", "任务ID")
	f.SetCellValue(sheetName, "B1", taskID)
	f.SetCellValue(sheetName, "A2", "任务名称")
	if strings.TrimSpace(taskName) == "" {
		taskName = task.TaskName
	}
	f.SetCellValue(sheetName, "B2", taskName)
	f.SetCellValue(sheetName, "A3", "版本")
	f.SetCellValue(sheetName, "B3", version)
	f.SetCellValue(sheetName, "A4", "类型")
	f.SetCellValue(sheetName, "B4", task.ScanType)
	f.SetCellValue(sheetName, "A5", "状态")
	f.SetCellValue(sheetName, "B5", task.Status)
	f.SetCellValue(sheetName, "A6", "生成时间")
	f.SetCellValue(sheetName, "B6", time.Now().Format("2006-01-02 15:04:05"))
	if !task.CreatedAt.IsZero() {
		f.SetCellValue(sheetName, "A7", "开始时间")
		f.SetCellValue(sheetName, "B7", task.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if task.CompletedAt != nil {
		f.SetCellValue(sheetName, "A8", "完成时间")
		f.SetCellValue(sheetName, "B8", task.CompletedAt.Format("2006-01-02 15:04:05"))
	}
}

func htmlEscape(s string) string {
	return html.EscapeString(s)
}

func disposalStatusLabel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "in_progress":
		return "跟进中"
	case "fixed":
		return "已修复"
	case "false_positive":
		return "误报"
	case "ignored":
		return "已忽略"
	default:
		return "待处理"
	}
}

func truncateForReportText(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "\n...(内容过长，已截断)"
}
