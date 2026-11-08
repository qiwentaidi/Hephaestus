package space

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/handlers/platform"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/space-engine/hunter"
	"hephaestus-backend/pkg/taskqueue"
	"hephaestus-backend/pkg/types"

	"github.com/gin-gonic/gin"
)

const (
	hunterExportFetchPageSize = 100
	hunterExportRetryMax      = 5
	hunterExportRetryBaseSec  = 2
	hunterExportPageInterval  = 1500 * time.Millisecond
)

type hunterExportFilters struct {
	TimeFilter    string `json:"timeFilter"`
	IsWeb         int    `json:"isWeb"`
	Deduplication bool   `json:"deduplication"`
	StartPage     int    `json:"startPage"`
	EndPage       int    `json:"endPage"`
}

type createHunterExportTaskRequest struct {
	Query         string   `json:"query" binding:"required"`
	ExportCount   int      `json:"exportCount" binding:"required,min=1"`
	Fields        []string `json:"fields" binding:"required"`
	Format        string   `json:"format"`
	FileName      string   `json:"fileName"`
	TimeFilter    string   `json:"timeFilter"`
	IsWeb         int      `json:"isWeb"`
	Deduplication bool     `json:"deduplication"`
	StartPage     int      `json:"startPage"`
	EndPage       int      `json:"endPage"`
}

func getHunterAuthFromConfig() (*types.HunterAuth, error) {
	spaceConfig, err := config.GetSpaceEngineConfig()
	if err != nil {
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}
	if spaceConfig.Hunter.Key == "" {
		return nil, errors.New("请先在设置中配置 Hunter API Key")
	}
	auth := &types.HunterAuth{
		API: spaceConfig.Hunter.API,
		Key: spaceConfig.Hunter.Key,
	}
	if auth.API == "" {
		auth.API = "https://hunter.qianxin.com/"
	}
	return auth, nil
}

func CreateHunterExportTaskHandler(c *gin.Context) {
	var req createHunterExportTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数: " + err.Error()})
		return
	}

	if _, err := getHunterAuthFromConfig(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	if len(req.Fields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "请至少选择一个导出字段"})
		return
	}
	uniqFields := make([]string, 0, len(req.Fields))
	seen := make(map[string]struct{})
	for _, field := range req.Fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		uniqFields = append(uniqFields, field)
	}
	if len(uniqFields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "请至少选择一个有效字段"})
		return
	}

	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" {
		format = "csv"
	}
	if format != "csv" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "当前仅支持 CSV 导出"})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	username := platform.Username(c)
	if username == "" {
		username = "unknown"
	}

	fieldsJSON, _ := json.Marshal(uniqFields)
	filtersJSON, _ := json.Marshal(hunterExportFilters{
		TimeFilter:    req.TimeFilter,
		IsWeb:         req.IsWeb,
		Deduplication: req.Deduplication,
		StartPage:     req.StartPage,
		EndPage:       req.EndPage,
	})

	now := time.Now()
	taskID := fmt.Sprintf("hunter_export_%d", now.UnixNano())
	fileName := ensureFileNameExtension(sanitizeFileName(req.FileName), format)
	task := &database.SpaceExportTask{
		ID:         taskID,
		Engine:     "hunter",
		TaskName:   fileName,
		Status:     "pending",
		Progress:   0,
		TotalCount: req.ExportCount,
		Format:     format,
		Query:      req.Query,
		Filters:    string(filtersJSON),
		Fields:     string(fieldsJSON),
		FileName:   fileName,
		CreatedBy:  username,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := db.Create(task).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "创建导出任务失败: " + err.Error()})
		return
	}

	queueTask := &taskqueue.Task{
		ID:   taskID,
		Name: "Hunter 导出任务",
		Fn: func() error {
			return executeHunterExportTask(taskID)
		},
	}
	if err := taskqueue.GetSpaceExportQueue().Submit(queueTask); err != nil {
		_ = db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
			"status":     "failed",
			"error_msg":  "提交导出任务失败: " + err.Error(),
			"updated_at": time.Now(),
		}).Error
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "提交导出任务失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":         task.ID,
			"status":     task.Status,
			"totalCount": task.TotalCount,
		},
		"message": "导出任务已创建，正在排队执行",
	})
}

func shouldRetryHunterExport(message string) bool {
	msg := strings.ToLower(message)
	keywords := []string{"rate", "limit", "too many", "frequently", "429", "速度过快", "频率"}
	for _, keyword := range keywords {
		if strings.Contains(msg, keyword) {
			return true
		}
	}
	return false
}

func hunterRowToMap(item types.HunterDataArr) map[string]string {
	componentNames := make([]string, 0, len(item.Component))
	for _, comp := range item.Component {
		if comp.Name == "" {
			continue
		}
		if comp.Version != "" {
			componentNames = append(componentNames, fmt.Sprintf("%s %s", comp.Name, comp.Version))
		} else {
			componentNames = append(componentNames, comp.Name)
		}
	}
	position := strings.Trim(strings.Join([]string{item.Country, item.Province, item.City}, "/"), "/")
	return map[string]string{
		"URL":        item.URL,
		"IP":         item.IP,
		"Port":       strconv.FormatInt(item.Port, 10),
		"Protocol":   item.Protocol,
		"Domain":     item.Domain,
		"Component":  strings.Join(componentNames, ", "),
		"Title":      item.WebTitle,
		"Status":     strconv.FormatInt(item.StatusCode, 10),
		"ICP":        item.Number,
		"ISP":        item.Isp,
		"Position":   position,
		"UpdateTime": item.UpdatedAt,
		// 额外保留常见字段
		"Country":      item.Country,
		"Province":     item.Province,
		"City":         item.City,
		"AsOrg":        item.AsOrg,
		"Company":      item.Company,
		"Banner":       item.Banner,
		"BaseProtocol": item.BaseProtocol,
		"Os":           item.Os,
	}
}

func executeHunterExportTask(taskID string) error {
	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var task database.SpaceExportTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		return fmt.Errorf("读取导出任务失败: %w", err)
	}

	startedAt := time.Now()
	if err := db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
		"status":     "running",
		"started_at": startedAt,
		"updated_at": startedAt,
	}).Error; err != nil {
		return fmt.Errorf("更新导出任务状态失败: %w", err)
	}

	auth, err := getHunterAuthFromConfig()
	if err != nil {
		now := time.Now()
		_ = db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
			"status":       "failed",
			"error_msg":    err.Error(),
			"completed_at": now,
			"updated_at":   now,
		}).Error
		return err
	}
	client := hunter.NewHunterClient(auth)

	var selectedFields []string
	if err := json.Unmarshal([]byte(task.Fields), &selectedFields); err != nil {
		now := time.Now()
		_ = db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
			"status":       "failed",
			"error_msg":    "解析导出字段失败: " + err.Error(),
			"completed_at": now,
			"updated_at":   now,
		}).Error
		return err
	}
	if len(selectedFields) == 0 {
		now := time.Now()
		_ = db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
			"status":       "failed",
			"error_msg":    "导出字段为空",
			"completed_at": now,
			"updated_at":   now,
		}).Error
		return errors.New("导出字段为空")
	}

	var filters hunterExportFilters
	if task.Filters != "" {
		_ = json.Unmarshal([]byte(task.Filters), &filters)
	}
	if filters.TimeFilter == "" {
		filters.TimeFilter = "0"
	}
	if filters.IsWeb == 0 {
		filters.IsWeb = 3
	}
	if filters.StartPage <= 0 {
		filters.StartPage = 1
	}
	if filters.EndPage < filters.StartPage {
		filters.EndPage = filters.StartPage
	}

	exportDir := filepath.Join(config.GetDataDir(), "exports", "hunter", task.CreatedBy)
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		now := time.Now()
		_ = db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
			"status":       "failed",
			"error_msg":    "创建导出目录失败: " + err.Error(),
			"completed_at": now,
			"updated_at":   now,
		}).Error
		return err
	}

	outputPath := filepath.Join(exportDir, ensureFileNameExtension(sanitizeFileName(task.FileName), task.Format))
	file, err := os.Create(outputPath)
	if err != nil {
		now := time.Now()
		_ = db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
			"status":       "failed",
			"error_msg":    "创建导出文件失败: " + err.Error(),
			"completed_at": now,
			"updated_at":   now,
		}).Error
		return err
	}
	defer file.Close()

	if err := writeCSVUtf8BOM(file); err != nil {
		now := time.Now()
		_ = db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
			"status":       "failed",
			"error_msg":    "写入 CSV 编码标记失败: " + err.Error(),
			"completed_at": now,
			"updated_at":   now,
		}).Error
		return err
	}

	writer := csv.NewWriter(file)
	header := make([]string, 0, len(selectedFields))
	for _, field := range selectedFields {
		header = append(header, field)
	}
	if err := writer.Write(header); err != nil {
		now := time.Now()
		_ = db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
			"status":       "failed",
			"error_msg":    "写入导出头失败: " + err.Error(),
			"completed_at": now,
			"updated_at":   now,
		}).Error
		return err
	}

	exportedCount := 0
	pageNum := filters.StartPage
	targetCount := task.TotalCount

	for exportedCount < targetCount && pageNum <= filters.EndPage {
		// Hunter API page_size 仅支持固定枚举值（10/20/50/100），
		// 不能使用余数（如 92）作为最后一页大小。
		// 统一按固定大小拉取，写入时再按 targetCount 截断。
		pageSize := hunterExportFetchPageSize

		var result *types.HunterSearchResponse
		var callErr error
		for attempt := 1; attempt <= hunterExportRetryMax; attempt++ {
			result = client.HunterApiSearch(&types.HunterSearchRequest{
				Query:         task.Query,
				Page:          pageNum,
				PageSize:      pageSize,
				TimeFilter:    filters.TimeFilter,
				IsWeb:         filters.IsWeb,
				Deduplication: filters.Deduplication,
			})
			if result != nil && result.Code == 200 {
				callErr = nil
				break
			}
			message := "Hunter 导出请求失败"
			if result != nil && result.Message != "" {
				message = result.Message
			}
			if !shouldRetryHunterExport(message) || attempt == hunterExportRetryMax {
				callErr = errors.New(message)
				break
			}
			sleepSec := hunterExportRetryBaseSec * attempt
			time.Sleep(time.Duration(sleepSec) * time.Second)
		}
		if callErr != nil {
			now := time.Now()
			_ = db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
				"status":        "failed",
				"error_msg":     "导出查询失败: " + callErr.Error(),
				"current_count": exportedCount,
				"progress":      int(float64(exportedCount) / float64(targetCount) * 100),
				"completed_at":  now,
				"updated_at":    now,
			}).Error
			return callErr
		}

		if result == nil || len(result.Data.Arr) == 0 {
			break
		}

		for _, item := range result.Data.Arr {
			if exportedCount >= targetCount {
				break
			}
			rowMap := hunterRowToMap(item)
			record := make([]string, 0, len(selectedFields))
			for _, field := range selectedFields {
				record = append(record, rowMap[field])
			}
			if err := writer.Write(record); err != nil {
				now := time.Now()
				_ = db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
					"status":        "failed",
					"error_msg":     "写入导出内容失败: " + err.Error(),
					"current_count": exportedCount,
					"completed_at":  now,
					"updated_at":    now,
				}).Error
				return err
			}
			exportedCount++
		}

		writer.Flush()
		if err := writer.Error(); err != nil {
			now := time.Now()
			_ = db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
				"status":        "failed",
				"error_msg":     "刷新导出文件失败: " + err.Error(),
				"current_count": exportedCount,
				"completed_at":  now,
				"updated_at":    now,
			}).Error
			return err
		}

		progress := int(float64(exportedCount) / float64(targetCount) * 100)
		if progress > 100 {
			progress = 100
		}
		_ = db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
			"current_count": exportedCount,
			"progress":      progress,
			"updated_at":    time.Now(),
		}).Error

		pageNum++
		// 降低 API 触发频控概率，分页间隔提升到 1-2s 级别
		time.Sleep(hunterExportPageInterval)
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		now := time.Now()
		_ = db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
			"status":        "failed",
			"error_msg":     "刷新导出文件失败: " + err.Error(),
			"current_count": exportedCount,
			"completed_at":  now,
			"updated_at":    now,
		}).Error
		return err
	}

	stat, statErr := os.Stat(outputPath)
	fileSize := int64(0)
	if statErr == nil {
		fileSize = stat.Size()
	}

	completedAt := time.Now()
	if err := db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
		"status":        "success",
		"progress":      100,
		"current_count": exportedCount,
		"file_path":     outputPath,
		"file_size":     fileSize,
		"completed_at":  completedAt,
		"updated_at":    completedAt,
	}).Error; err != nil {
		return fmt.Errorf("更新导出任务状态失败: %w", err)
	}

	logger.Info("[hunter-export] 任务执行成功: task=%s, count=%d, file=%s", taskID, exportedCount, outputPath)
	return nil
}
