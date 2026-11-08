package space

import (
	"net/http"

	dbspace "hephaestus-backend/pkg/database/space"

	"github.com/gin-gonic/gin"
)

// ListSpaceEngineSyntaxHandler 获取空间引擎语法列表
func ListSpaceEngineSyntaxHandler(c *gin.Context) {
	source := c.Query("source") // fofa, hunter, quake 等

	syntaxes, err := dbspace.GetSpaceEngineSyntaxList(source)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取语法列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    syntaxes,
	})
}

// CreateSpaceEngineSyntaxHandler 创建空间引擎语法
func CreateSpaceEngineSyntaxHandler(c *gin.Context) {
	var req struct {
		Source  string `json:"source" binding:"required"` // fofa, hunter, quake 等
		Name    string `json:"name" binding:"required"`
		Content string `json:"content" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	syntax, err := dbspace.SaveSpaceEngineSyntax(req.Source, req.Name, req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存语法失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    syntax,
	})
}

// DeleteSpaceEngineSyntaxHandler 删除空间引擎语法
func DeleteSpaceEngineSyntaxHandler(c *gin.Context) {
	var req struct {
		ID uint `json:"id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	if err := dbspace.DeleteSpaceEngineSyntax(req.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "删除语法失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "删除成功",
	})
}

// DeleteSpaceEngineSyntaxByNameHandler 根据来源和名称删除语法
func DeleteSpaceEngineSyntaxByNameHandler(c *gin.Context) {
	var req struct {
		Source string `json:"source" binding:"required"`
		Name   string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	if err := dbspace.DeleteSpaceEngineSyntaxByName(req.Source, req.Name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "删除语法失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "删除成功",
	})
}
