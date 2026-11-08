package space

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/space-engine/hunter"
	"hephaestus-backend/pkg/types"
)

// HunterSearchHandler Hunter 搜索处理
func HunterSearchHandler(c *gin.Context) {
	var req types.HunterSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
		return
	}

	// 优先从数据库读取认证信息
	auth := &types.HunterAuth{}

	spaceConfig, err := config.GetSpaceEngineConfig()
	if err == nil && spaceConfig.Hunter.Key != "" {
		auth.API = spaceConfig.Hunter.API
		auth.Key = spaceConfig.Hunter.Key
	} else {
		// 降级方案：从请求头读取（向后兼容）
		auth.API = c.GetHeader("X-Hunter-API")
		auth.Key = c.GetHeader("X-Hunter-Key")
	}

	if auth.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先在设置中配置 Hunter API Key"})
		return
	}

	if auth.API == "" {
		auth.API = "https://hunter.qianxin.com/"
	}

	// 设置默认值
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	client := hunter.NewHunterClient(auth)
	result := client.HunterApiSearch(&req)

	c.JSON(http.StatusOK, result)
}

// HunterUserInfoHandler 获取用户信息
func HunterUserInfoHandler(c *gin.Context) {
	// 优先从数据库读取认证信息
	auth := &types.HunterAuth{}

	spaceConfig, err := config.GetSpaceEngineConfig()
	if err == nil && spaceConfig.Hunter.Key != "" {
		auth.API = spaceConfig.Hunter.API
		auth.Key = spaceConfig.Hunter.Key
	} else {
		// 降级方案：从请求头读取（向后兼容）
		auth.API = c.GetHeader("X-Hunter-API")
		auth.Key = c.GetHeader("X-Hunter-Key")
	}

	if auth.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先在设置中配置 Hunter API Key"})
		return
	}

	if auth.API == "" {
		auth.API = "https://hunter.qianxin.com/"
	}

	client := hunter.NewHunterClient(auth)
	userInfo := client.HunterApiGetUserInfo()

	c.JSON(http.StatusOK, userInfo)
}

// HunterTipsHandler Hunter 提示处理
func HunterTipsHandler(c *gin.Context) {
	keyword := c.Query("keyword")
	if keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "关键词不能为空"})
		return
	}

	client := hunter.NewHunterClient(&types.HunterAuth{})
	result := client.SearchHunterTips(keyword)

	c.JSON(http.StatusOK, result)
}
