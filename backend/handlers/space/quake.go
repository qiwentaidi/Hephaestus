package space

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/space-engine/quake"
	"hephaestus-backend/pkg/types"

	"github.com/qiwentaidi/clients"
)

// QuakeSearchHandler Quake 搜索处理
func QuakeSearchHandler(c *gin.Context) {
	var req types.QuakeRequestOptions
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
		return
	}

	// 优先从数据库读取认证信息，如果没有则从请求头读取（向后兼容）
	auth := &types.QuakeAuth{}

	spaceConfig, err := config.GetSpaceEngineConfig()
	if err == nil && spaceConfig.Quake.Key != "" {
		auth.Token = spaceConfig.Quake.Key
	} else {
		// 降级方案：从请求头读取（向后兼容）
		auth.Token = c.GetHeader("X-Quake-Token")
	}

	if auth.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先在设置中配置 Quake API Token"})
		return
	}

	// 设置默认值
	if req.PageNum <= 0 {
		req.PageNum = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}
	client := quake.NewQuakeClient(auth)
	result := client.QuakeApiSearch(&req)

	c.JSON(http.StatusOK, result)
}

// QuakeUserInfoHandler 获取用户信息
func QuakeUserInfoHandler(c *gin.Context) {
	// 优先从数据库读取认证信息
	auth := &types.QuakeAuth{}

	spaceConfig, err := config.GetSpaceEngineConfig()
	if err == nil && spaceConfig.Quake.Key != "" {
		auth.Token = spaceConfig.Quake.Key
	} else {
		// 降级方案：从请求头读取（向后兼容）
		auth.Token = c.GetHeader("X-Quake-Token")
	}

	if auth.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先在设置中配置 Quake API Token"})
		return
	}

	client := quake.NewQuakeClient(auth)
	userInfo := client.QuakeApiGetUserInfo(c.Request.Context())

	c.JSON(http.StatusOK, userInfo)
}

// QuakeTipsHandler Quake 提示处理
func QuakeTipsHandler(c *gin.Context) {
	keyword := c.Query("keyword")
	if keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "关键词不能为空"})
		return
	}

	// 优先从数据库读取认证信息
	auth := &types.QuakeAuth{}

	spaceConfig, err := config.GetSpaceEngineConfig()
	if err == nil && spaceConfig.Quake.Key != "" {
		auth.Token = spaceConfig.Quake.Key
	} else {
		// 降级方案：从请求头读取（向后兼容）
		auth.Token = c.GetHeader("X-Quake-Token")
	}

	if auth.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先在设置中配置 Quake API Token"})
		return
	}

	client := quake.NewQuakeClient(auth)
	result := client.QuakeApiGetTips(keyword)

	c.JSON(http.StatusOK, result)
}

// QuakeIconHashHandler Quake 图标hash计算处理（MD5）
func QuakeIconHashHandler(c *gin.Context) {
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

	// 计算 MD5
	hash := md5.Sum(imageData)
	hashStr := hex.EncodeToString(hash[:])

	c.JSON(http.StatusOK, gin.H{"hash": hashStr})
}
