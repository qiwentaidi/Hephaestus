package space

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/handlers/platform"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/space-engine/fofa"
	"hephaestus-backend/pkg/taskqueue"
	"hephaestus-backend/pkg/types"

	"github.com/gin-gonic/gin"
)

const (
	fofaExportFetchPageSize = 10000
	fofaExportRetryMax      = 5
	fofaExportRetryBaseSec  = 2
)

type fofaExportFilters struct {
	Fraud bool `json:"fraud"`
	Cert  bool `json:"cert"`
}

type createFofaExportTaskRequest struct {
	Query       string   `json:"query" binding:"required"`
	ExportCount int      `json:"exportCount" binding:"required,min=1"`
	Fields      []string `json:"fields" binding:"required"`
	Format      string   `json:"format"`
	FileName    string   `json:"fileName"`
	Fraud       bool     `json:"fraud"`
	Cert        bool     `json:"cert"`
}

var fofaExportFieldMap = map[string]func(item types.FofaResult) string{
	"IP":       func(item types.FofaResult) string { return item.IP },
	"Port":     func(item types.FofaResult) string { return item.Port },
	"Protocol": func(item types.FofaResult) string { return item.Protocol },
	"Title":    func(item types.FofaResult) string { return item.Title },
	"Host":     func(item types.FofaResult) string { return item.Host },
	"Domain":   func(item types.FofaResult) string { return item.Domain },
	"Country":  func(item types.FofaResult) string { return item.Country },
	"ICP":      func(item types.FofaResult) string { return item.ICP },
	"URL":      func(item types.FofaResult) string { return item.URL },
	"Product":  func(item types.FofaResult) string { return item.Product },
}

var fofaExportFieldLabelMap = map[string]string{
	"IP":       "IP地址",
	"Port":     "端口",
	"Protocol": "协议名",
	"Title":    "网站标题",
	"Host":     "Host",
	"Domain":   "域名",
	"Country":  "国家代码",
	"ICP":      "备案号",
	"URL":      "网站链接",
	"Product":  "组件",
}

func sanitizeFileName(name string) string {
	fileName := strings.TrimSpace(name)
	if fileName == "" {
		return fmt.Sprintf("fofa_export_%s", time.Now().Format("20060102_150405"))
	}
	re := regexp.MustCompile(`[^a-zA-Z0-9_\-\.\p{Han}]+`)
	fileName = re.ReplaceAllString(fileName, "_")
	fileName = strings.Trim(fileName, "._")
	if fileName == "" {
		fileName = fmt.Sprintf("fofa_export_%s", time.Now().Format("20060102_150405"))
	}
	return fileName
}

func ensureFileNameExtension(fileName string, format string) string {
	normalizedFormat := strings.ToLower(strings.TrimSpace(format))
	if normalizedFormat == "" {
		normalizedFormat = "csv"
	}
	ext := "." + normalizedFormat
	if strings.HasSuffix(strings.ToLower(fileName), ext) {
		return fileName
	}
	return fileName + ext
}

func writeCSVUtf8BOM(file *os.File) error {
	_, err := file.Write([]byte{0xEF, 0xBB, 0xBF})
	return err
}

func getFofaAuthFromConfig() (*types.FofaAuth, error) {
	spaceConfig, err := config.GetSpaceEngineConfig()
	if err != nil {
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}
	if spaceConfig.FOFA.Email == "" || spaceConfig.FOFA.Key == "" {
		return nil, errors.New("请先在设置中配置 FOFA API 认证信息（Email 和 Key）")
	}
	auth := &types.FofaAuth{
		Email:   spaceConfig.FOFA.Email,
		Key:     spaceConfig.FOFA.Key,
		Address: spaceConfig.FOFA.API,
	}
	if auth.Address == "" {
		auth.Address = "https://fofa.info"
	}
	return auth, nil
}

func CreateFofaExportTaskHandler(c *gin.Context) {
	var req createFofaExportTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数: " + err.Error()})
		return
	}

	// 先校验 FOFA 配置可用性
	if _, err := getFofaAuthFromConfig(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	validFields := make([]string, 0, len(req.Fields))
	seen := make(map[string]struct{})
	for _, field := range req.Fields {
		if _, ok := fofaExportFieldMap[field]; !ok {
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

	username := platform.Username(c)
	if username == "" {
		username = "unknown"
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}

	fieldsJSON, _ := json.Marshal(validFields)
	filtersJSON, _ := json.Marshal(fofaExportFilters{
		Fraud: req.Fraud,
		Cert:  req.Cert,
	})

	now := time.Now()
	taskID := fmt.Sprintf("fofa_export_%d", now.UnixNano())
	fileName := ensureFileNameExtension(sanitizeFileName(req.FileName), format)
	task := &database.SpaceExportTask{
		ID:         taskID,
		Engine:     "fofa",
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
		Name: "FOFA 导出任务",
		Fn: func() error {
			return executeFofaExportTask(taskID)
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

func shouldRetryFofaExport(message string) bool {
	msg := strings.ToLower(message)
	keywords := []string{"rate", "limit", "too many", "frequently", "429", "速度过快", "频率"}
	for _, keyword := range keywords {
		if strings.Contains(msg, keyword) {
			return true
		}
	}
	return false
}

func executeFofaExportTask(taskID string) error {
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

	defer func() {
		if r := recover(); r != nil {
			now := time.Now()
			_ = db.Model(&database.SpaceExportTask{}).Where("id = ?", taskID).Updates(map[string]any{
				"status":       "failed",
				"error_msg":    fmt.Sprintf("导出任务异常: %v", r),
				"completed_at": now,
				"updated_at":   now,
			}).Error
		}
	}()

	auth, err := getFofaAuthFromConfig()
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
	client := fofa.NewFofaClient(auth)

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

	var filters fofaExportFilters
	if task.Filters != "" {
		_ = json.Unmarshal([]byte(task.Filters), &filters)
	}

	exportDir := filepath.Join(config.GetDataDir(), "exports", "fofa", task.CreatedBy)
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
		label := fofaExportFieldLabelMap[field]
		if label == "" {
			label = field
		}
		header = append(header, label)
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

	for exportedCount < targetCount {
		pageSize := fofaExportFetchPageSize
		remain := targetCount - exportedCount
		if remain < pageSize {
			pageSize = remain
		}

		var result *types.FofaSearchResult
		var callErr error
		for attempt := 1; attempt <= fofaExportRetryMax; attempt++ {
			result = client.FofaApiSearch(task.Query, strconv.Itoa(pageSize), strconv.Itoa(pageNum), filters.Fraud, filters.Cert)
			if result != nil && !result.Error {
				callErr = nil
				break
			}
			message := "FOFA 导出请求失败"
			if result != nil {
				message = result.Message
			}
			if !shouldRetryFofaExport(message) || attempt == fofaExportRetryMax {
				callErr = errors.New(message)
				break
			}
			sleepSec := fofaExportRetryBaseSec * attempt
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

		if result == nil || len(result.Results) == 0 {
			break
		}

		for _, item := range result.Results {
			if exportedCount >= targetCount {
				break
			}
			record := make([]string, 0, len(selectedFields))
			for _, field := range selectedFields {
				getter := fofaExportFieldMap[field]
				if getter == nil {
					record = append(record, "")
					continue
				}
				record = append(record, getter(item))
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
		// 串行限速，降低触发 FOFA 频控的概率
		time.Sleep(200 * time.Millisecond)
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

	logger.Info("[fofa-export] 任务执行成功: task=%s, count=%d, file=%s", taskID, exportedCount, outputPath)
	return nil
}

func ListFofaExportTasksHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}
	username := platform.Username(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}

	var total int64
	query := db.Model(&database.SpaceExportTask{}).Where("engine = ? AND created_by = ?", "fofa", username)
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "查询导出任务失败: " + err.Error()})
		return
	}

	var tasks []database.SpaceExportTask
	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取导出任务失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"list":     tasks,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}

func GetFofaExportTaskHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}
	username := platform.Username(c)
	id := c.Param("id")

	var task database.SpaceExportTask
	if err := db.Where("id = ? AND engine = ? AND created_by = ?", id, "fofa", username).First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "导出任务不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    task,
	})
}

func DownloadFofaExportTaskHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "获取数据库连接失败: " + err.Error()})
		return
	}
	username := platform.Username(c)
	id := c.Param("id")

	var task database.SpaceExportTask
	if err := db.Where("id = ? AND engine = ? AND created_by = ?", id, "fofa", username).First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "导出任务不存在"})
		return
	}
	if task.Status != "success" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "导出任务尚未完成"})
		return
	}
	if task.FilePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "导出文件不存在"})
		return
	}

	if _, err := os.Stat(task.FilePath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "导出文件不存在或已被清理"})
		return
	}

	downloadName := ensureFileNameExtension(sanitizeFileName(task.FileName), task.Format)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", downloadName))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.File(task.FilePath)
}
