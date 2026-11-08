package space

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"hephaestus-backend/handlers/platform"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/space-engine/quake"
	"hephaestus-backend/pkg/taskqueue"
	"hephaestus-backend/pkg/types"
)

const (
	quakeExportFetchPageSize = 500
	quakeExportRetryMax      = 5
	quakeExportRetryBaseSec  = 2
	quakeExportPageInterval  = 1500 * time.Millisecond
)

type quakeExportFilters struct {
	Latest bool `json:"latest"`
}

type createQuakeExportTaskRequest struct {
	Query       string   `json:"query" binding:"required"`
	ExportCount int      `json:"exportCount" binding:"required,min=1"`
	Fields      []string `json:"fields" binding:"required"`
	Format      string   `json:"format"`
	FileName    string   `json:"fileName"`
	Latest      bool     `json:"latest"`
}

type quakeExportCost struct {
	AvailableCredit int
	MaxExportable   int
	CostPerRecord   int
}

func getQuakeExportCost(auth *types.QuakeAuth, requestedCount int) (*quakeExportCost, error) {
	client := quake.NewQuakeClient(auth)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	info := client.QuakeApiGetUserInfo(ctx)
	if info == nil {
		return nil, errors.New("无法获取用户信息")
	}
	if info.Code != nil {
		switch v := info.Code.(type) {
		case float64:
			if int(v) != 0 && int(v) != 200 {
				return nil, errors.New(info.Message)
			}
		case int:
			if v != 0 && v != 200 {
				return nil, errors.New(info.Message)
			}
		}
	}

	credit := 0
	if info.Data.Credit > 0 {
		credit = info.Data.Credit
	}
	const costPerRecord = 1
	maxByCredit := credit / costPerRecord
	if maxByCredit < 0 {
		maxByCredit = 0
	}
	maxAllowed := requestedCount
	if maxByCredit < maxAllowed {
		maxAllowed = maxByCredit
	}

	return &quakeExportCost{
		AvailableCredit: credit,
		MaxExportable:   maxAllowed,
		CostPerRecord:   costPerRecord,
	}, nil
}

var quakeExportFieldMap = map[string]func(item types.QuakeData) string{
	"URL":        func(item types.QuakeData) string { return item.URL },
	"IP":         func(item types.QuakeData) string { return item.IP },
	"Port":       func(item types.QuakeData) string { return fmt.Sprintf("%d", item.Port) },
	"Protocol":   func(item types.QuakeData) string { return item.Protocol },
	"Host":       func(item types.QuakeData) string { return item.Host },
	"Title":      func(item types.QuakeData) string { return item.Title },
	"Components": func(item types.QuakeData) string { return strings.Join(item.Components, ";") },
	"ICPName":    func(item types.QuakeData) string { return item.IcpName },
	"ICPNumber":  func(item types.QuakeData) string { return item.IcpNumber },
	"CertName":   func(item types.QuakeData) string { return item.CertName },
	"ISP":        func(item types.QuakeData) string { return item.Isp },
	"Position":   func(item types.QuakeData) string { return item.Position },
}

var quakeExportFieldLabelMap = map[string]string{
	"URL":        "URL",
	"IP":         "IP地址",
	"Port":       "端口",
	"Protocol":   "协议",
	"Host":       "Host",
	"Title":      "标题",
	"Components": "组件",
	"ICPName":    "ICP备案名称",
	"ICPNumber":  "ICP备案编号",
	"CertName":   "证书名称",
	"ISP":        "运营商",
	"Position":   "地理位置",
}

func getQuakeAuthFromConfig() (*types.QuakeAuth, error) {
	spaceConfig, err := config.GetSpaceEngineConfig()
	if err != nil {
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}
	if spaceConfig.Quake.Key == "" {
		return nil, errors.New("请先在设置中配置 Quake API Token")
	}
	return &types.QuakeAuth{Token: spaceConfig.Quake.Key}, nil
}

func CreateQuakeExportTaskHandler(c *gin.Context) {
	var req createQuakeExportTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数: " + err.Error()})
		return
	}

	auth, err := getQuakeAuthFromConfig()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	costInfo, err := getQuakeExportCost(auth, req.ExportCount)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if costInfo.MaxExportable <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "积分不足，无法执行导出"})
		return
	}
	req.ExportCount = costInfo.MaxExportable

	validFields := make([]string, 0, len(req.Fields))
	seen := make(map[string]struct{})
	for _, field := range req.Fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, ok := quakeExportFieldMap[field]; !ok {
			continue
		}
		if _, exists := seen[field]; exists {
			continue
		}
		seen[field] = struct{}{}
		validFields = append(validFields, field)
	}
	if len(validFields) == 0 {
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

	fieldsJSON, _ := json.Marshal(validFields)
	filtersJSON, _ := json.Marshal(quakeExportFilters{
		Latest: req.Latest,
	})

	now := time.Now()
	taskID := fmt.Sprintf("quake_export_%d", now.UnixNano())
	fileName := ensureFileNameExtension(sanitizeFileName(req.FileName), format)
	task := &database.SpaceExportTask{
		ID:         taskID,
		Engine:     "quake",
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
		Name: "Quake 导出任务",
		Fn: func() error {
			return executeQuakeExportTask(taskID)
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

func shouldRetryQuakeExport(message string) bool {
	msg := strings.ToLower(message)
	keywords := []string{"rate", "limit", "频率", "too many", "429", "请求太多"}
	for _, keyword := range keywords {
		if strings.Contains(msg, keyword) {
			return true
		}
	}
	return false
}

func executeQuakeExportTask(taskID string) error {
	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var task database.SpaceExportTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		return fmt.Errorf("读取导出任务失败: %w", err)
	}

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
		err := errors.New("导出字段为空")
		now := time.Now()
		_ = db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
			"status":       "failed",
			"error_msg":    err.Error(),
			"completed_at": now,
			"updated_at":   now,
		}).Error
		return err
	}

	var filters quakeExportFilters
	if task.Filters != "" {
		_ = json.Unmarshal([]byte(task.Filters), &filters)
	}

	auth, err := getQuakeAuthFromConfig()
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
	client := quake.NewQuakeClient(auth)

	exportDir := filepath.Join(config.GetDataDir(), "exports", "quake", task.CreatedBy)
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
		if label, ok := quakeExportFieldLabelMap[field]; ok {
			header = append(header, label)
		} else {
			header = append(header, field)
		}
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
	pageNum := 1
	targetCount := task.TotalCount
	if targetCount <= 0 {
		targetCount = 1
	}

	startedAt := time.Now()
	if err := db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
		"status":     "running",
		"started_at": startedAt,
		"updated_at": startedAt,
	}).Error; err != nil {
		return fmt.Errorf("更新导出任务状态失败: %w", err)
	}

	for exportedCount < task.TotalCount {
		pageSize := quakeExportFetchPageSize
		remain := task.TotalCount - exportedCount
		if remain < pageSize {
			pageSize = remain
		}
		if pageSize <= 0 {
			break
		}

		var result *types.QuakeResult
		var callErr error

		for attempt := 1; attempt <= quakeExportRetryMax; attempt++ {
			result = client.QuakeApiSearch(&types.QuakeRequestOptions{
				Query:    task.Query,
				PageNum:  pageNum,
				PageSize: pageSize,
				Latest:   filters.Latest,
			})
			if result != nil && (result.Code == 0 || result.Code == 200) {
				callErr = nil
				break
			}
			message := "Quake 导出请求失败"
			if result != nil && result.Message != "" {
				message = result.Message
			}
			if !shouldRetryQuakeExport(message) || attempt == quakeExportRetryMax {
				callErr = errors.New(message)
				break
			}
			sleepSec := quakeExportRetryBaseSec * attempt
			time.Sleep(time.Duration(sleepSec) * time.Second)
		}

		if callErr != nil {
			now := time.Now()
			progress := int(float64(exportedCount) / float64(targetCount) * 100)
			if progress > 100 {
				progress = 100
			}
			_ = db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
				"status":        "failed",
				"error_msg":     "导出查询失败: " + callErr.Error(),
				"current_count": exportedCount,
				"progress":      progress,
				"completed_at":  now,
				"updated_at":    now,
			}).Error
			return callErr
		}

		if result == nil || len(result.Data) == 0 {
			break
		}

		for _, item := range result.Data {
			if exportedCount >= task.TotalCount {
				break
			}
			record := make([]string, 0, len(selectedFields))
			for _, field := range selectedFields {
				if fn, ok := quakeExportFieldMap[field]; ok {
					record = append(record, fn(item))
				} else {
					record = append(record, "")
				}
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
		time.Sleep(quakeExportPageInterval)
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

	logger.Info("[quake-export] 任务执行成功: task=%s, count=%d, file=%s", taskID, exportedCount, outputPath)
	return nil
}
