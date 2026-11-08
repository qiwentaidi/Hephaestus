// Package platform 提供 handlers 各业务域共用的 HTTP 基础设施。
//
// 这里只放与业务无关、且被多个模块域真实复用的最小集合。新增成员前先确认：
// 它不依赖任何业务域，并且至少有一个真实调用方，避免变成杂物间。
package platform

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Username 返回中间件注入的当前用户名。
//
// JWT 中间件与 MCP 鉴权中间件都通过 c.Set("username", ...) 注入用户名；
// 未经过鉴权的请求返回空字符串，调用方应据此返回 401。
func Username(c *gin.Context) string {
	return c.GetString("username")
}

// WriteSuccess 写入 {"success": true}。
func WriteSuccess(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// WriteData 写入 {"success": true, "data": data}。
func WriteData(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// WriteMessage 写入 {"success": true, "message": message}。
func WriteMessage(c *gin.Context, message string) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": message})
}

// WriteError 写入 {"success": false, "error": message} 并设置给定状态码。
func WriteError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"success": false, "error": message})
}

// WriteUnauthorized 写入 401 {"success": false, "error": "未登录"}。
func WriteUnauthorized(c *gin.Context) {
	WriteError(c, http.StatusUnauthorized, "未登录")
}
