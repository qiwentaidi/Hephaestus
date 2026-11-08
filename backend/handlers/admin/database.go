package admin

import (
	"net/http"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/database/collect"
	"hephaestus-backend/pkg/types"

	"github.com/gin-gonic/gin"
)

// GetAllConnectionsHandler 获取所有数据库连接
func GetAllConnectionsHandler(c *gin.Context) {
	connections, err := collect.LoadConnections()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取数据库连接列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    connections,
	})
}

// AddConnectionHandler 添加数据库连接
func AddConnectionHandler(c *gin.Context) {
	var req types.DatabaseConnection
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 生成 nanoid
	if req.Nanoid == "" {
		req.Nanoid = database.GenerateNanoid()
	}

	if err := collect.AddConnection(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "添加数据库连接失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "添加成功",
		"data":    req,
	})
}

// UpdateConnectionHandler 更新数据库连接
func UpdateConnectionHandler(c *gin.Context) {
	var req types.DatabaseConnection
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	if req.Nanoid == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "nanoid 不能为空",
		})
		return
	}

	if err := collect.UpdateConnection(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "更新数据库连接失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "更新成功",
	})
}

// GetCollectCustomRulesHandler 获取数据库信息采集自定义规则
func GetCollectCustomRulesHandler(c *gin.Context) {
	rules, err := collect.ListCustomRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取自定义规则失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    rules,
	})
}

// SaveCollectCustomRulesHandler 保存数据库信息采集自定义规则
func SaveCollectCustomRulesHandler(c *gin.Context) {
	var req struct {
		Rules []types.CollectCustomRule `json:"rules"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	if err := collect.ReplaceCustomRules(req.Rules); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "保存自定义规则失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "保存成功",
	})
}

// RemoveConnectionHandler 删除数据库连接
func RemoveConnectionHandler(c *gin.Context) {
	var req struct {
		Nanoid string `json:"nanoid"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	if req.Nanoid == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "nanoid 不能为空",
		})
		return
	}

	if err := collect.RemoveConnection(req.Nanoid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "删除数据库连接失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "删除成功",
	})
}

// TestConnectionHandler 测试数据库连接
func TestConnectionHandler(c *gin.Context) {
	var req types.DatabaseConnection
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	if err := collect.TestConnection(req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "连接测试失败",
			"error":   err.Error(),
			"data": gin.H{
				"connected": false,
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "连接测试成功",
		"data": gin.H{
			"connected": true,
		},
	})
}

// GetDatabaseInfoHandler 获取数据库信息（数据库列表和表列表）
func GetDatabaseInfoHandler(c *gin.Context) {
	nanoid := c.Query("nanoid")
	if nanoid == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "nanoid 参数不能为空",
		})
		return
	}

	conn, err := collect.GetConnection(nanoid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "未找到数据库连接: " + err.Error(),
		})
		return
	}

	info, err := collect.GetDatabaseInfo(*conn)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取数据库信息失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    info,
	})
}

// GetTableInfoHandler 获取表数据信息（列、前3行数据、总行数）
func GetTableInfoHandler(c *gin.Context) {
	var req types.TableInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	if req.Nanoid == "" || req.SchemaName == "" || req.TableName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "nanoid, schemaName, tableName 不能为空",
		})
		return
	}

	conn, err := collect.GetConnection(req.Nanoid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "未找到数据库连接: " + err.Error(),
		})
		return
	}

	data, err := collect.GetTableInfo(*conn, req.SchemaName, req.TableName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取表信息失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

// SaveCollectResultHandler 保存采集结果
func SaveCollectResultHandler(c *gin.Context) {
	var req struct {
		ConnectionNanoid string              `json:"connectionNanoid"`
		Result           types.CollectResult `json:"result"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	if req.ConnectionNanoid == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "connectionNanoid 不能为空",
		})
		return
	}
	if err := collect.ValidateCollectData(req.Result); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id, err := collect.SaveCollectResult(req.ConnectionNanoid, req.Result)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "保存采集结果失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "保存成功",
		"data": gin.H{
			"id": id,
		},
	})
}

// GetCollectResultsHandler 获取采集结果列表
func GetCollectResultsHandler(c *gin.Context) {
	connectionNanoid := c.Query("connectionNanoid")
	if connectionNanoid == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "connectionNanoid 参数不能为空",
		})
		return
	}

	results, err := collect.GetCollectResults(connectionNanoid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取采集结果失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    results,
	})
}

// RemoveCollectResultHandler 删除采集结果
func RemoveCollectResultHandler(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	if req.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "id 不能为空",
		})
		return
	}

	if err := collect.RemoveCollectResult(req.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "删除采集结果失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "删除成功",
	})
}

// UpdateConnectionStatsHandler 更新连接统计信息
func UpdateConnectionStatsHandler(c *gin.Context) {
	var req struct {
		Nanoid        string `json:"nanoid"`
		DatabaseCount int    `json:"databaseCount"`
		TableCount    int    `json:"tableCount"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	if req.Nanoid == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "nanoid 不能为空",
		})
		return
	}

	if err := collect.UpdateConnectionStats(req.Nanoid, req.DatabaseCount, req.TableCount); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "更新连接统计信息失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "更新成功",
	})
}
