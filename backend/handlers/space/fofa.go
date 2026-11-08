package space

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/qiwentaidi/clients"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/space-engine/fofa"
	"hephaestus-backend/pkg/types"
)

// SearchHandler FOFA 搜索处理
func SearchHandler(c *gin.Context) {
	var req types.SearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
		return
	}

	// 优先从数据库读取认证信息，如果没有则从请求头读取（向后兼容）
	auth := &types.FofaAuth{}

	// 尝试从数据库读取配置
	spaceConfig, err := config.GetSpaceEngineConfig()
	if err == nil && spaceConfig.FOFA.Email != "" && spaceConfig.FOFA.Key != "" {
		auth.Email = spaceConfig.FOFA.Email
		auth.Key = spaceConfig.FOFA.Key
		auth.Address = spaceConfig.FOFA.API
	} else {
		// 降级方案：从请求头读取（向后兼容）
		auth.Email = c.GetHeader("X-FOFA-Email")
		auth.Key = c.GetHeader("X-FOFA-Key")
		auth.Address = c.GetHeader("X-FOFA-Address")
	}

	if auth.Email == "" || auth.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先在设置中配置 FOFA API 认证信息（Email 和 Key）"})
		return
	}

	if auth.Address == "" {
		auth.Address = "https://fofa.info"
	}

	// 设置默认值
	if req.PageSize <= 0 {
		req.PageSize = 100
	}
	if req.PageNum <= 0 {
		req.PageNum = 1
	}

	client := fofa.NewFofaClient(auth)
	result := client.FofaApiSearch(
		req.Query,
		strconv.Itoa(req.PageSize),
		strconv.Itoa(req.PageNum),
		req.Fraud,
		req.Cert,
	)

	c.JSON(http.StatusOK, result)
}

// UserInfoHandler 获取用户信息
func UserInfoHandler(c *gin.Context) {
	// 优先从数据库读取认证信息，如果没有则从请求头读取（向后兼容）
	auth := &types.FofaAuth{}

	// 尝试从数据库读取配置
	spaceConfig, err := config.GetSpaceEngineConfig()
	if err == nil && spaceConfig.FOFA.Key != "" {
		auth.Email = spaceConfig.FOFA.Email
		auth.Key = spaceConfig.FOFA.Key
		auth.Address = spaceConfig.FOFA.API
	} else {
		// 降级方案：从请求头读取（向后兼容）
		auth.Key = c.GetHeader("X-FOFA-Key")
		auth.Address = "https://fofa.info"
	}

	if auth.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先在设置中配置 FOFA API Key"})
		return
	}

	if auth.Address == "" {
		auth.Address = "https://fofa.info"
	}

	client := fofa.NewFofaClient(auth)
	userInfo := client.FofaApiSearchByUserInfo()

	c.JSON(http.StatusOK, userInfo)
}

// IconHashHandler 图标hash计算处理
func IconHashHandler(c *gin.Context) {
	var req types.IconHashRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
		return
	}

	var imageData []byte
	var err error

	if req.ImageData != "" {
		// 处理base64编码的图片数据
		// 格式: data:image/png;base64,iVBORw0KGgo...
		parts := strings.Split(req.ImageData, ",")
		if len(parts) != 2 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的base64图片数据格式"})
			return
		}
		imageData, err = base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "base64解码失败: " + err.Error()})
			return
		}
	} else if req.URL != "" {
		// 从URL获取图片
		resp, err := clients.SimpleGet(req.URL, clients.NewRestyClient(nil, true))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取图标失败: " + err.Error()})
			return
		}
		imageData = resp.Body()
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "必须提供URL或imageData"})
		return
	}

	hash := fofa.Mmh3Hash32(imageData)
	c.JSON(http.StatusOK, gin.H{"hash": hash})
}

// StatsHandler 统计聚合处理
func StatsHandler(c *gin.Context) {
	var req types.StatsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
		return
	}

	// 优先从数据库读取认证信息，如果没有则从请求头读取（向后兼容）
	auth := &types.FofaAuth{}

	// 尝试从数据库读取配置
	spaceConfig, err := config.GetSpaceEngineConfig()
	if err == nil && spaceConfig.FOFA.Email != "" && spaceConfig.FOFA.Key != "" {
		auth.Email = spaceConfig.FOFA.Email
		auth.Key = spaceConfig.FOFA.Key
		auth.Address = spaceConfig.FOFA.API
	} else {
		// 降级方案：从请求头读取（向后兼容）
		auth.Email = c.GetHeader("X-FOFA-Email")
		auth.Key = c.GetHeader("X-FOFA-Key")
		auth.Address = c.GetHeader("X-FOFA-Address")
	}

	if auth.Email == "" || auth.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先在设置中配置 FOFA API 认证信息（Email 和 Key）"})
		return
	}

	if auth.Address == "" {
		auth.Address = "https://fofa.info"
	}

	client := fofa.NewFofaClient(auth)
	result := client.FofaApiStats(req.Query, req.Fields)

	c.JSON(http.StatusOK, result)
}
