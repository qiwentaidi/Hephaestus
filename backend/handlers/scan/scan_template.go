package scan

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
)

// 各任务类型的配置结构

// WebScanConfig 网站扫描配置
type WebScanConfig struct {
	WebscanOption         int      `json:"webscanOption"`         // 1: 指纹扫描, 2: 全指纹扫描, 3: 指纹漏洞扫描, 4: 专项扫描
	Threads               int      `json:"threads"`               // 并发线程数
	CustomHeaders         string   `json:"customHeaders"`         // 自定义请求头
	CustomTemplate        []string `json:"customTemplate"`        // 自定义漏洞模板
	RootPathScan          bool     `json:"rootPathScan"`          // 根路径扫描
	SkipNucleiWithoutTags bool     `json:"skipNucleiWithoutTags"` // 跳过无标签模板
	Screenshot            bool     `json:"screenshot"`            // 截图
	ActiveTimeoutLimit    int      `json:"activeTimeoutLimit"`    // 活跃超时限制
}

// PortScanConfig 端口扫描配置
type PortScanConfig struct {
	PortGroup                    string `json:"portGroup"`      // 端口组
	PortRange                    string `json:"portRange"`      // 端口范围
	Threads                      int    `json:"threads"`        // 并发线程数
	Timeout                      int    `json:"timeout"`        // 超时时间（秒）
	AliveDetection               bool   `json:"aliveDetection"` // 存活探测
	SubnetDiscoveryEnabled       bool   `json:"subnetDiscoveryEnabled"`
	SubnetDiscoveryMode          string `json:"subnetDiscoveryMode"`
	SubnetDiscoveryHostSuffixes  string `json:"subnetDiscoveryHostSuffixes"`
	SubnetDiscoveryRandomSamples *int   `json:"subnetDiscoveryRandomSamples,omitempty"`
	TCPHeuristicEnabled          bool   `json:"tcpHeuristicEnabled"`
	TCPHeuristicPorts            string `json:"tcpHeuristicPorts"`
	TCPHeuristicTimeoutMs        int    `json:"tcpHeuristicTimeoutMs"`
}

// DirScanConfig 目录扫描配置
type DirScanConfig struct {
	Method         string   `json:"method"`         // HTTP 方法
	Extensions     string   `json:"extensions"`     // 扩展名
	StatusFilter   string   `json:"statusFilter"`   // 状态码过滤
	SelectedDicts  []string `json:"selectedDicts"`  // 选中的字典
	CustomDict     string   `json:"customDict"`     // 自定义字典
	Thread         int      `json:"thread"`         // 并发线程数
	Timeout        int      `json:"timeout"`        // 超时时间
	Interval       int      `json:"interval"`       // 请求间隔
	Recursion      int      `json:"recursion"`      // 递归深度
	Times          int      `json:"times"`          // 重试次数
	Exclude        string   `json:"exclude"`        // 排除规则
	Headers        string   `json:"headers"`        // 自定义请求头
	RedirectClient bool     `json:"redirectClient"` // 跟随重定向
	BackupfileScan bool     `json:"backupfileScan"` // 备份文件扫描
}

// JsScanConfig JS扫描配置
type JsScanConfig struct {
	BlackDomain           []string          `json:"blackDomain"`           // 黑名单域名
	HighRiskRouter        []string          `json:"highRiskRouter"`        // 高风险路由
	Authentication        []string          `json:"authentication"`        // 认证标识
	Placeholder           map[string]string `json:"placeholder"`           // 占位符替换
	SqlInjectionEnabled   bool              `json:"sqlInjectionEnabled"`   // SQL注入检测
	LfiEnabled            bool              `json:"lfiEnabled"`            // LFI检测
	LfiParamKeywords      []string          `json:"lfiParamKeywords"`      // LFI参数关键词
	SsrfEnabled           bool              `json:"ssrfEnabled"`           // SSRF检测
	SsrfParamKeywords     []string          `json:"ssrfParamKeywords"`     // SSRF参数关键词
	RedirectEnabled       bool              `json:"redirectEnabled"`       // 重定向检测
	RedirectParamKeywords []string          `json:"redirectParamKeywords"` // 重定向参数关键词
	XssEnabled            bool              `json:"xssEnabled"`            // XSS检测
	UploadEnabled         bool              `json:"uploadEnabled"`         // 上传检测
	UploadParamKeywords   []string          `json:"uploadParamKeywords"`   // 上传参数关键词
	UploadTestContent     string            `json:"uploadTestContent"`     // 上传测试内容
	UploadTestFileName    string            `json:"uploadTestFileName"`    // 上传测试文件名
}

// CrackScanConfig 弱口令破解配置
type CrackScanConfig struct {
	Username    string `json:"username"`    // 用户字典
	Password    string `json:"password"`    // 密码字典
	DefaultDict bool   `json:"defaultDict"` // 使用默认字典
}

// ScanTemplateRequest 扫描模板请求
type ScanTemplateRequest struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	TaskType    string      `json:"taskType"` // webscan, portscan, dirsearch, jsscan, crack, combination, ai_dialogue
	Config      interface{} `json:"config"`
}

func isValidScanTemplateType(taskType string) bool {
	switch taskType {
	case "webscan", "portscan", "dirsearch", "jsscan", "crack", "combination", "ai_dialogue":
		return true
	default:
		return false
	}
}

func validateScanTemplateConfig(taskType string, config interface{}) string {
	if taskType != "ai_dialogue" {
		return ""
	}
	values, ok := config.(map[string]interface{})
	if !ok {
		return "AI 对话模板配置无效"
	}
	prompt, _ := values["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return "AI 对话模板规则不能为空"
	}
	if !strings.Contains(prompt, "{{.goal}}") {
		return "AI 对话模板规则必须包含 {{.goal}} 占位符"
	}
	return ""
}

// ListScanTemplatesV2Handler 获取所有扫描模板（新版）
func ListScanTemplatesV2Handler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	// 支持按任务类型过滤
	taskType := c.Query("taskType")

	var templates []database.ScanTemplate
	query := db.Order("created_at desc")
	if taskType != "" {
		query = query.Where("task_type = ?", taskType)
	}

	if err := query.Find(&templates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取模板列表失败: " + err.Error(),
		})
		return
	}

	// 解析配置 JSON
	result := make([]map[string]interface{}, 0, len(templates))
	for _, t := range templates {
		item := map[string]interface{}{
			"id":          t.ID,
			"name":        t.Name,
			"description": t.Description,
			"taskType":    t.TaskType,
			"createdAt":   t.CreatedAt,
			"updatedAt":   t.UpdatedAt,
		}

		// 解析配置
		if t.Config != "" {
			var config interface{}
			if err := json.Unmarshal([]byte(t.Config), &config); err == nil {
				item["config"] = config
			}
		}

		result = append(result, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// CreateScanTemplateV2Handler 创建扫描模板（新版）
func CreateScanTemplateV2Handler(c *gin.Context) {
	var req ScanTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "模板名称不能为空",
		})
		return
	}

	if req.TaskType == "" {
		req.TaskType = "webscan"
	}

	if !isValidScanTemplateType(req.TaskType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的任务类型",
		})
		return
	}
	if validationError := validateScanTemplateConfig(req.TaskType, req.Config); validationError != "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": validationError})
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

	// 序列化配置
	configJSON := ""
	if req.Config != nil {
		configBytes, err := json.Marshal(req.Config)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "配置格式错误: " + err.Error(),
			})
			return
		}
		configJSON = string(configBytes)
	}

	template := database.ScanTemplate{
		ID:          database.GenerateNanoid(),
		Name:        req.Name,
		Description: req.Description,
		TaskType:    req.TaskType,
		Config:      configJSON,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := db.Create(&template).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "创建模板失败: " + err.Error(),
		})
		return
	}

	// 返回创建的模板
	result := map[string]interface{}{
		"id":          template.ID,
		"name":        template.Name,
		"description": template.Description,
		"taskType":    template.TaskType,
		"createdAt":   template.CreatedAt,
		"updatedAt":   template.UpdatedAt,
	}
	if template.Config != "" {
		var config interface{}
		if err := json.Unmarshal([]byte(template.Config), &config); err == nil {
			result["config"] = config
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// UpdateScanTemplateV2Handler 更新扫描模板（新版）
func UpdateScanTemplateV2Handler(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "模板ID不能为空",
		})
		return
	}

	var req ScanTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "模板名称不能为空",
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

	// 查找模板
	var template database.ScanTemplate
	if err := db.Where("id = ?", id).First(&template).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "模板不存在",
		})
		return
	}
	taskType := req.TaskType
	if taskType == "" {
		taskType = template.TaskType
	}
	if !isValidScanTemplateType(taskType) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的任务类型"})
		return
	}
	if validationError := validateScanTemplateConfig(taskType, req.Config); validationError != "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": validationError})
		return
	}

	// 序列化配置
	configJSON := ""
	if req.Config != nil {
		configBytes, err := json.Marshal(req.Config)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "配置格式错误: " + err.Error(),
			})
			return
		}
		configJSON = string(configBytes)
	}

	// 更新字段
	updates := map[string]interface{}{
		"name":        req.Name,
		"description": req.Description,
		"config":      configJSON,
		"updated_at":  time.Now(),
	}

	// 如果提供了任务类型，也更新它
	if req.TaskType != "" {
		updates["task_type"] = req.TaskType
	}

	if err := db.Model(&template).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "更新模板失败: " + err.Error(),
		})
		return
	}

	// 重新获取更新后的模板
	db.Where("id = ?", id).First(&template)

	result := map[string]interface{}{
		"id":          template.ID,
		"name":        template.Name,
		"description": template.Description,
		"taskType":    template.TaskType,
		"createdAt":   template.CreatedAt,
		"updatedAt":   template.UpdatedAt,
	}
	if template.Config != "" {
		var config interface{}
		if err := json.Unmarshal([]byte(template.Config), &config); err == nil {
			result["config"] = config
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// DeleteScanTemplateV2Handler 删除扫描模板（新版）
func DeleteScanTemplateV2Handler(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "模板ID不能为空",
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

	var template database.ScanTemplate
	if err := db.Where("id = ?", id).First(&template).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "模板不存在",
		})
		return
	}

	if err := db.Delete(&template).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "删除模板失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "模板删除成功",
	})
}

// GetScanTemplateV2Handler 获取单个扫描模板（新版）
func GetScanTemplateV2Handler(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "模板ID不能为空",
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

	var template database.ScanTemplate
	if err := db.Where("id = ?", id).First(&template).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "模板不存在",
		})
		return
	}

	result := map[string]interface{}{
		"id":          template.ID,
		"name":        template.Name,
		"description": template.Description,
		"taskType":    template.TaskType,
		"createdAt":   template.CreatedAt,
		"updatedAt":   template.UpdatedAt,
	}
	if template.Config != "" {
		var config interface{}
		if err := json.Unmarshal([]byte(template.Config), &config); err == nil {
			result["config"] = config
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}
