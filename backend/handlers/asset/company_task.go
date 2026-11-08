package asset

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"hephaestus-backend/pkg/company"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/report"
	"hephaestus-backend/pkg/taskqueue"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"hephaestus-backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type companyReportTaskItem struct {
	ID string `json:"id"`
}

type mergeCompanyInfoReportRequest struct {
	Tasks    []companyReportTaskItem `json:"tasks"`
	Format   string                  `json:"format"`
	Formats  []string                `json:"formats"`
	FileName string                  `json:"fileName"`
}

type companyGeneratedReportFile struct {
	Name        string
	ContentType string
	Bytes       []byte
}

type companyInfoTaskRequestError struct {
	status  int
	message string
}

func (e companyInfoTaskRequestError) Error() string {
	return e.message
}

func newCompanyInfoTaskRequestError(status int, message string) error {
	return companyInfoTaskRequestError{status: status, message: message}
}

func companyInfoTaskErrorStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if taskErr, ok := err.(companyInfoTaskRequestError); ok {
		return taskErr.status
	}
	return http.StatusInternalServerError
}

func normalizeCompanyReportFormats(format string, formats []string) ([]string, error) {
	seen := make(map[string]struct{})
	result := make([]string, 0, max(1, len(formats)))

	appendFormat := func(raw string) error {
		norm := strings.ToLower(strings.TrimSpace(raw))
		if norm == "" {
			return nil
		}
		switch norm {
		case "doc", "docx", "rtf":
			norm = "word"
		case "excel":
			norm = "xlsx"
		}
		if norm != "word" && norm != "html" && norm != "xlsx" {
			return fmt.Errorf("不支持的导出格式: %s", raw)
		}
		if _, ok := seen[norm]; ok {
			return nil
		}
		seen[norm] = struct{}{}
		result = append(result, norm)
		return nil
	}

	if err := appendFormat(format); err != nil {
		return nil, err
	}
	for _, item := range formats {
		if err := appendFormat(item); err != nil {
			return nil, err
		}
	}
	if len(result) == 0 {
		result = append(result, "xlsx")
	}
	return result, nil
}

func packageCompanyReportsAsZip(fileName string, files []companyGeneratedReportFile) (companyGeneratedReportFile, error) {
	base := strings.TrimSpace(strings.TrimSuffix(fileName, filepath.Ext(fileName)))
	if base == "" {
		base = "company_reports"
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, file := range files {
		w, err := zw.Create(file.Name)
		if err != nil {
			_ = zw.Close()
			return companyGeneratedReportFile{}, err
		}
		if _, err := w.Write(file.Bytes); err != nil {
			_ = zw.Close()
			return companyGeneratedReportFile{}, err
		}
	}
	if err := zw.Close(); err != nil {
		return companyGeneratedReportFile{}, err
	}

	return companyGeneratedReportFile{
		Name:        base + ".zip",
		ContentType: "application/zip",
		Bytes:       buf.Bytes(),
	}, nil
}

func sanitizeCompanyReportFilenameBase(input string) string {
	base := strings.TrimSpace(input)
	if base == "" {
		return ""
	}
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}

	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		"\n", "_",
		"\r", "_",
		"\t", "_",
	)
	base = replacer.Replace(base)
	base = strings.Trim(base, " .")

	const maxLen = 120
	r := []rune(base)
	if len(r) > maxLen {
		base = string(r[:maxLen])
	}
	return base
}

func CreateCompanyInfoTask(req CompanyInfoRequest) (*database.CompanyInfoTask, error) {
	// 验证数据源配置
	if len(req.DataSource) == 0 {
		return nil, newCompanyInfoTaskRequestError(http.StatusBadRequest, "请至少选择一个数据源")
	}

	// 验证风鸟配置
	if contains(req.DataSource, "riskbird") {
		if strings.TrimSpace(req.RiskbirdCookie) == "" {
			return nil, newCompanyInfoTaskRequestError(http.StatusBadRequest, "风鸟Cookie不能为空")
		}
	}

	zeroZoneEnabled := req.ZeroZoneEnable || contains(req.DataSource, "zerozone")

	// 验证 0.zone 配置
	if zeroZoneEnabled {
		if strings.TrimSpace(req.ZeroZoneKey) == "" {
			return nil, newCompanyInfoTaskRequestError(http.StatusBadRequest, "0.zone API Key不能为空")
		}
	}

	// 验证工信部配置
	miitEnabled := req.MiitEnable || contains(req.DataSource, "miit")
	if miitEnabled {
		if req.MiitApi == "" {
			return nil, newCompanyInfoTaskRequestError(http.StatusBadRequest, "工信部API地址不能为空")
		}
	}

	// 解析公司名称列表（换行分隔）
	companies := parseCompanyNames(req.Company)
	if len(companies) == 0 {
		return nil, newCompanyInfoTaskRequestError(http.StatusBadRequest, "请输入至少一个公司名称")
	}

	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	// 生成任务ID
	taskID := fmt.Sprintf("company_info_%d", time.Now().UnixNano())

	// 构建配置对象
	config := map[string]interface{}{
		"company":         req.Company,
		"invest":          req.Invest,
		"dataSource":      req.DataSource,
		"zeroZoneEnable":  zeroZoneEnabled,
		"miitEnable":      miitEnabled,
		"riskbirdCookie":  req.RiskbirdCookie,
		"zeroZoneKey":     req.ZeroZoneKey,
		"subcompanyLevel": req.SubcompanyLevel,
		"miitApi":         req.MiitApi,
		"companies":       companies,
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("序列化配置失败: %w", err)
	}

	// 提取任务名称（如果用户没有提供，则自动生成）
	taskName := strings.TrimSpace(req.TaskName)
	if taskName == "" {
		// 自动生成任务名称
		if len(companies) == 1 {
			taskName = companies[0]
		} else {
			taskName = fmt.Sprintf("%s 等%d家公司", companies[0], len(companies))
		}
	}

	// 创建任务
	task := &database.CompanyInfoTask{
		ID:           taskID,
		TaskName:     taskName,
		Status:       "pending",
		Progress:     0,
		TotalCount:   len(companies),
		CurrentCount: 0,
		Config:       string(configJSON),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := db.Create(task).Error; err != nil {
		return nil, fmt.Errorf("创建任务失败: %w", err)
	}

	// 将任务提交到企业信息收集专用队列执行（最大并发数为1）
	executeTaskFunc := func() error {
		return executeCompanyInfoTask(taskID, config)
	}

	queueTask := &taskqueue.Task{
		ID:   taskID,
		Name: taskName,
		Fn:   executeTaskFunc,
	}
	if err := taskqueue.GetCompanyInfoQueue().Submit(queueTask); err != nil {
		return nil, fmt.Errorf("提交任务到队列失败: %w", err)
	}

	return task, nil
}

// CreateCompanyInfoTaskHandler 创建企业信息查询任务
func CreateCompanyInfoTaskHandler(c *gin.Context) {
	var req CompanyInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	task, err := CreateCompanyInfoTask(req)
	if err != nil {
		c.JSON(companyInfoTaskErrorStatus(err), gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    task,
		"message": "任务创建成功",
	})
}

// executeCompanyInfoTask 执行企业信息查询任务
func executeCompanyInfoTask(taskID string, config map[string]interface{}) error {
	db, err := database.GetDB()
	if err != nil {
		logger.Error("[company-info-task] 获取数据库连接失败: %s", err.Error())
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	// 更新任务状态为运行中
	now := time.Now()
	if err := db.Model(&database.CompanyInfoTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":     "running",
		"started_at": now,
		"updated_at": now,
	}).Error; err != nil {
		logger.Error("[company-info-task] 更新任务状态失败: %s", err.Error())
		return fmt.Errorf("更新任务状态失败: %w", err)
	}

	// 解析配置
	companiesInterface, ok := config["companies"]
	if !ok {
		logger.Error("[company-info-task] 配置中缺少 companies 字段")
		return fmt.Errorf("配置中缺少 companies 字段")
	}
	companies, ok := companiesInterface.([]string)
	if !ok {
		// 尝试从 []interface{} 转换
		if companiesArray, ok := companiesInterface.([]interface{}); ok {
			companies = make([]string, 0, len(companiesArray))
			for _, c := range companiesArray {
				if str, ok := c.(string); ok {
					companies = append(companies, str)
				}
			}
		} else {
			logger.Error("[company-info-task] companies 字段类型错误")
			return fmt.Errorf("companies 字段类型错误")
		}
	}

	invest, _ := config["invest"].(float64)
	if invest == 0 {
		invest = 100 // 默认值
	}

	dataSourceInterface, _ := config["dataSource"]
	var dataSource []interface{}
	if dsSlice, ok := dataSourceInterface.([]interface{}); ok {
		dataSource = dsSlice
	} else if dsStrings, ok := dataSourceInterface.([]string); ok {
		for _, item := range dsStrings {
			dataSource = append(dataSource, item)
		}
	}

	riskbirdCookie, _ := config["riskbirdCookie"].(string)
	zeroZoneKey, _ := config["zeroZoneKey"].(string)
	subcompanyLevel, _ := config["subcompanyLevel"].(float64)
	if subcompanyLevel == 0 {
		subcompanyLevel = 1 // 默认值
	}
	miitApi, _ := config["miitApi"].(string)
	zeroZoneEnabled, zeroZoneEnabledOK := config["zeroZoneEnable"].(bool)
	if !zeroZoneEnabledOK {
		zeroZoneEnabled = containsString(dataSource, "zerozone")
	}
	miitEnabled, miitEnabledOK := config["miitEnable"].(bool)
	if !miitEnabledOK {
		miitEnabled = containsString(dataSource, "miit")
	}

	// 构建数据源配置
	ds := &company.DataSource{}
	if containsString(dataSource, "riskbird") {
		ds.Riskbird.Enable = true
		ds.Riskbird.Cookie = riskbirdCookie
	}
	if zeroZoneEnabled {
		ds.ZeroZone.Enable = true
		ds.ZeroZone.APIKey = zeroZoneKey
	}
	if miitEnabled {
		ds.Miit.Enable = true
		ds.Miit.API = miitApi
	}

	// 收集企业信息
	results := make([]company.CompanyInfo, 0)
	totalCount := len(companies)

	for i, companyName := range companies {
		logger.Info("[company-info-task] 开始收集企业信息: %s (%d/%d)", companyName, i+1, totalCount)

		info, err := company.FetchCompanyInfo(
			companyName,
			int(invest),
			ds,
			int(subcompanyLevel),
			miitApi,
		)

		if err != nil {
			logger.Error("[company-info-task] 收集企业信息失败 %s: %s", companyName, err.Error())
			// 即使失败也返回基本信息
			info = &company.CompanyInfo{
				CompanyName: companyName,
			}
		}

		results = append(results, *info)

		// 更新进度
		progress := int((float64(i+1) / float64(totalCount)) * 100)
		if err := db.Model(&database.CompanyInfoTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
			"current_count": i + 1,
			"progress":      progress,
			"updated_at":    time.Now(),
		}).Error; err != nil {
			logger.Warning("[company-info-task] 更新任务进度失败: %s", err.Error())
		}
	}

	// 保存结果
	resultJSON, err := json.Marshal(results)
	if err != nil {
		logger.Error("[company-info-task] 序列化结果失败: %s", err.Error())
		// 更新任务状态为失败
		db.Model(&database.CompanyInfoTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
			"status":     "failed",
			"error_msg":  "序列化结果失败: " + err.Error(),
			"updated_at": time.Now(),
		})
		return fmt.Errorf("序列化结果失败: %w", err)
	}

	resultID := fmt.Sprintf("company_info_result_%d", time.Now().UnixNano())
	result := &database.CompanyInfoTaskResult{
		ID:        resultID,
		TaskID:    taskID,
		Data:      string(resultJSON),
		CreatedAt: time.Now(),
	}

	if err := db.Create(result).Error; err != nil {
		logger.Error("[company-info-task] 保存结果失败: %s", err.Error())
		// 更新任务状态为失败
		db.Model(&database.CompanyInfoTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
			"status":     "failed",
			"error_msg":  "保存结果失败: " + err.Error(),
			"updated_at": time.Now(),
		})
		return fmt.Errorf("保存结果失败: %w", err)
	}

	// 更新任务状态为完成
	completedAt := time.Now()
	if err := db.Model(&database.CompanyInfoTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":       "completed",
		"progress":     100,
		"completed_at": completedAt,
		"updated_at":   completedAt,
	}).Error; err != nil {
		logger.Error("[company-info-task] 更新任务状态失败: %s", err.Error())
		return fmt.Errorf("更新任务状态失败: %w", err)
	}

	logger.Info("[company-info-task] 任务 %s 执行完成", taskID)
	return nil
}

// ListCompanyInfoTasksHandler 获取任务列表
func ListCompanyInfoTasksHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	var tasks []database.CompanyInfoTask
	if err := db.Order("created_at DESC").Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询任务列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tasks,
	})
}

// GetCompanyInfoTaskHandler 获取任务详情
func GetCompanyInfoTaskHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	var task database.CompanyInfoTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "任务不存在",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    task,
	})
}

// GetCompanyInfoTaskResultsHandler 获取任务结果
func GetCompanyInfoTaskResultsHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	var result database.CompanyInfoTaskResult
	if err := db.Where("task_id = ?", taskID).First(&result).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "任务结果不存在",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询任务结果失败: " + err.Error(),
		})
		return
	}

	// 解析结果数据
	var companiesInfo []company.CompanyInfo
	if err := json.Unmarshal([]byte(result.Data), &companiesInfo); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "解析结果数据失败: " + err.Error(),
		})
		return
	}
	companiesInfo = company.NormalizeCompanyInfos(companiesInfo)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    companiesInfo,
	})
}

// CancelCompanyInfoTaskHandler 取消任务
func CancelCompanyInfoTaskHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	// 检查任务状态
	var task database.CompanyInfoTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "任务不存在",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询任务失败: " + err.Error(),
		})
		return
	}

	// 只有 pending 或 running 状态的任务可以取消
	if task.Status != "pending" && task.Status != "running" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "只能取消待执行或执行中的任务",
		})
		return
	}

	// 更新任务状态为已取消
	if err := db.Model(&database.CompanyInfoTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":     "cancelled",
		"updated_at": time.Now(),
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "取消任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "任务已取消",
	})
}

// DeleteCompanyInfoTaskHandler 删除任务
func DeleteCompanyInfoTaskHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	// 删除任务（级联删除结果）
	if err := db.Where("id = ?", taskID).Delete(&database.CompanyInfoTask{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "删除任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "任务已删除",
	})
}

// ExportCompanyInfoHandler 导出企业信息报告
func ExportCompanyInfoHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	format := strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", "")))
	var formatsParam []string
	if rawFormats := strings.TrimSpace(c.Query("formats")); rawFormats != "" {
		formatsParam = strings.Split(rawFormats, ",")
	}
	customFileName := strings.TrimSpace(c.DefaultQuery("fileName", ""))

	formats, err := normalizeCompanyReportFormats(format, formatsParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	task, err := report.LoadCompanyInfoTaskByID(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}
	taskName := task.TaskName
	if taskName == "" {
		taskName = fmt.Sprintf("企业信息_%s", taskID)
	}

	buildSingleReport := func(norm string) ([]byte, string, string, error) {
		switch norm {
		case "word":
			b, err := report.GenerateCompanyInfoWordReportFromDB(taskID, taskName)
			return b, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx", err
		case "html":
			b, err := report.GenerateCompanyInfoHTMLReportFromDB(taskID, taskName)
			return b, "text/html; charset=utf-8", ".html", err
		case "xlsx":
			b, err := report.GenerateCompanyInfoXLSXReportFromDB(taskID, taskName)
			return b, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx", err
		default:
			return nil, "", "", fmt.Errorf("不支持的导出格式: %s", norm)
		}
	}

	fileBase := sanitizeCompanyReportFilenameBase(customFileName)
	if fileBase == "" {
		fileBase = fmt.Sprintf("%s_%s", taskName, time.Now().Format("20060102"))
	}

	files := make([]companyGeneratedReportFile, 0, len(formats))
	for _, norm := range formats {
		reportBytes, contentType, ext, err := buildSingleReport(norm)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "生成报告失败: " + err.Error()})
			return
		}
		files = append(files, companyGeneratedReportFile{
			Name:        fileBase + ext,
			ContentType: contentType,
			Bytes:       reportBytes,
		})
	}

	output := files[0]
	if len(files) > 1 {
		output, err = packageCompanyReportsAsZip(fileBase, files)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "打包报告失败: " + err.Error()})
			return
		}
	}

	quoted := strings.ReplaceAll(output.Name, `"`, "_")
	escaped := url.PathEscape(output.Name)
	c.Header("Content-Type", output.ContentType)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, quoted, escaped))
	c.Data(http.StatusOK, output.ContentType, output.Bytes)
}

// ExportMergedCompanyInfoHandler 合并导出多个企业信息任务结果
func ExportMergedCompanyInfoHandler(c *gin.Context) {
	var req mergeCompanyInfoReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}
	if len(req.Tasks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请至少选择一个任务",
		})
		return
	}
	if len(req.Tasks) > 30 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "一次最多合并导出 30 个任务",
		})
		return
	}

	refs := make([]report.CompanyTaskRef, 0, len(req.Tasks))
	for _, item := range req.Tasks {
		taskID := strings.TrimSpace(item.ID)
		if taskID == "" {
			continue
		}
		refs = append(refs, report.CompanyTaskRef{ID: taskID})
	}
	if len(refs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "任务参数无效"})
		return
	}

	formats, err := normalizeCompanyReportFormats(req.Format, req.Formats)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	buildMergedReport := func(norm string) ([]byte, string, string, error) {
		switch norm {
		case "word":
			b, err := report.GenerateMergedCompanyInfoWordReportFromDB(refs, req.FileName)
			return b, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx", err
		case "html":
			b, err := report.GenerateMergedCompanyInfoHTMLReportFromDB(refs, req.FileName)
			return b, "text/html; charset=utf-8", ".html", err
		case "xlsx":
			b, err := report.GenerateMergedCompanyInfoXLSXReportFromDB(refs, req.FileName)
			return b, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx", err
		default:
			return nil, "", "", fmt.Errorf("不支持的导出格式: %s", norm)
		}
	}

	fileBase := sanitizeCompanyReportFilenameBase(req.FileName)
	if fileBase == "" {
		fileBase = fmt.Sprintf("企业信息合并报告_%d项_%s", len(refs), time.Now().Format("20060102"))
	}

	files := make([]companyGeneratedReportFile, 0, len(formats))
	for _, norm := range formats {
		reportBytes, contentType, ext, err := buildMergedReport(norm)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "不支持的导出格式") || strings.Contains(err.Error(), "未提供有效任务") || strings.Contains(err.Error(), "尚未完成") {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{
				"success": false,
				"error":   "生成合并报告失败: " + err.Error(),
			})
			return
		}
		files = append(files, companyGeneratedReportFile{
			Name:        fileBase + ext,
			ContentType: contentType,
			Bytes:       reportBytes,
		})
	}

	output := files[0]
	if len(files) > 1 {
		output, err = packageCompanyReportsAsZip(fileBase, files)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "打包合并报告失败: " + err.Error()})
			return
		}
	}

	quoted := strings.ReplaceAll(output.Name, `"`, "_")
	escaped := url.PathEscape(output.Name)
	c.Header("Content-Type", output.ContentType)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, quoted, escaped))
	c.Data(http.StatusOK, output.ContentType, output.Bytes)
}

// 辅助函数
func containsString(slice []interface{}, item string) bool {
	for _, s := range slice {
		if str, ok := s.(string); ok && str == item {
			return true
		}
	}
	return false
}
