package asset

import (
	"context"
	"net/http"
	"strings"

	"hephaestus-backend/pkg/company"
	"hephaestus-backend/pkg/company/riskbird"
	"hephaestus-backend/pkg/logger"

	"github.com/gin-gonic/gin"
)

// CompanyInfoRequest 企业信息收集请求
type CompanyInfoRequest struct {
	TaskName        string   `json:"taskName"`                   // 任务名称（可选）
	Company         string   `json:"company" binding:"required"` // 公司名称，多个换行分隔
	Invest          int      `json:"invest"`                     // 投资比例阈值
	DataSource      []string `json:"dataSource"`                 // 数据源：riskbird, miit, zerozone
	RiskbirdCookie  string   `json:"riskbirdCookie"`             // 风鸟Cookie
	ZeroZoneKey     string   `json:"zeroZoneKey"`                // 0.zone API Key
	ZeroZoneEnable  bool     `json:"zeroZoneEnable"`             // 是否启用 0.zone 数据源
	SubcompanyLevel int      `json:"subcompanyLevel"`            // 子公司层级
	MiitApi         string   `json:"miitApi"`                    // 工信部API地址
	MiitEnable      bool     `json:"miitEnable"`                 // 是否启用工信部数据源
}

// 使用pkg/company包中的类型定义

// CompanyInfoHandler 企业信息收集处理
func CompanyInfoHandler(c *gin.Context) {
	var req CompanyInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
		return
	}

	// 验证数据源配置
	if len(req.DataSource) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请至少选择一个数据源"})
		return
	}

	// 验证风鸟配置
	if contains(req.DataSource, "riskbird") {
		if req.RiskbirdCookie == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "风鸟Cookie不能为空"})
			return
		}
	}

	zeroZoneEnabled := req.ZeroZoneEnable || contains(req.DataSource, "zerozone")

	// 验证 0.zone 配置
	if zeroZoneEnabled {
		if strings.TrimSpace(req.ZeroZoneKey) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "0.zone API Key不能为空"})
			return
		}
	}

	miitEnabled := req.MiitEnable || contains(req.DataSource, "miit")

	// 验证工信部配置
	if miitEnabled {
		if req.MiitApi == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "工信部API地址不能为空"})
			return
		}
	}

	// 解析公司名称列表（换行分隔）
	companies := parseCompanyNames(req.Company)
	if len(companies) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入至少一个公司名称"})
		return
	}

	// 构建数据源配置
	ds := &company.DataSource{}
	if contains(req.DataSource, "riskbird") {
		ds.Riskbird.Enable = true
		ds.Riskbird.Cookie = req.RiskbirdCookie
	}
	if zeroZoneEnabled {
		ds.ZeroZone.Enable = true
		ds.ZeroZone.APIKey = req.ZeroZoneKey
	}
	if miitEnabled {
		ds.Miit.Enable = true
		ds.Miit.API = req.MiitApi
	}

	// 收集企业信息
	results := make([]company.CompanyInfo, 0)

	for _, companyName := range companies {
		logger.Info("开始收集企业信息: %s", companyName)

		info, err := company.FetchCompanyInfo(
			companyName,
			req.Invest,
			ds,
			req.SubcompanyLevel,
			req.MiitApi,
		)

		if err != nil {
			logger.Error("收集企业信息失败 %s: %s", companyName, err.Error())
			// 即使失败也返回基本信息
			info = &company.CompanyInfo{
				CompanyName: companyName,
			}
		}

		results = append(results, *info)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    results,
		"message": "企业信息收集完成",
	})
}

// ValidateMiitApiHandler 验证工信部API
func ValidateMiitApiHandler(c *gin.Context) {
	var req struct {
		Api string `json:"api" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	// 实际测试API连接
	apiAddress := strings.TrimRight(req.Api, "/")

	_, err := company.FetchWebInfo(apiAddress, "baidu.com")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "API连接失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "API连接正常",
	})
}

// ValidateRiskbirdHandler 验证风鸟Cookie
func ValidateRiskbirdHandler(c *gin.Context) {
	var req struct {
		Cookie string `json:"cookie" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	client := riskbird.NewClient(context.Background(), req.Cookie)
	if client.CheckLogin() {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "风鸟Cookie有效",
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "风鸟Cookie无效或已过期",
		})
	}
}

// 辅助函数
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func parseCompanyNames(input string) []string {
	// 按换行符分割，过滤空行
	lines := strings.Split(input, "\n")
	companies := make([]string, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			companies = append(companies, line)
		}
	}
	return companies
}
