package toolbox

import (
	"net/http"
	"strings"

	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
)

// ListPoCDebugTabsHandler 获取 PoC 调试工作台标签页。
func ListPoCDebugTabsHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	var tabs []database.PoCDebugTab
	if err := db.Order("sort_order ASC, created_at ASC").Find(&tabs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取 PoC 调试标签页失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tabs,
	})
}

// SavePoCDebugTabsHandler 批量保存 PoC 调试工作台标签页。
func SavePoCDebugTabsHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	var tabs []database.PoCDebugTab
	if err := c.ShouldBindJSON(&tabs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Exec("DELETE FROM poc_debug_tab").Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "清空 PoC 调试标签页失败: " + err.Error(),
		})
		return
	}

	for i := range tabs {
		tabs[i].SortOrder = i
		sanitizePoCDebugTabForPersistence(&tabs[i])
	}

	if len(tabs) > 0 {
		if err := tx.Create(&tabs).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "保存 PoC 调试标签页失败: " + err.Error(),
			})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "提交事务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tabs,
	})
}

func sanitizePoCDebugTabForPersistence(tab *database.PoCDebugTab) {
	if tab == nil {
		return
	}

	tab.ID = sanitizePoCDebugTextForPersistence(tab.ID)
	tab.Name = sanitizePoCDebugTextForPersistence(tab.Name)
	tab.TemplateID = sanitizePoCDebugTextForPersistence(tab.TemplateID)
	tab.YAML = sanitizePoCDebugTextForPersistence(tab.YAML)
	tab.Target = sanitizePoCDebugTextForPersistence(tab.Target)
	tab.Source = normalizePoCDebugSource(sanitizePoCDebugTextForPersistence(tab.Source))
	tab.SourceName = sanitizePoCDebugTextForPersistence(tab.SourceName)
	tab.ActiveView = normalizePoCDebugActiveView(sanitizePoCDebugTextForPersistence(tab.ActiveView))
	tab.LastStatus = normalizePoCDebugLastStatus(sanitizePoCDebugTextForPersistence(tab.LastStatus))
}

func sanitizePoCDebugTextForPersistence(value string) string {
	if value == "" {
		return value
	}
	return strings.ReplaceAll(value, "\x00", `\x00`)
}

func normalizePoCDebugSource(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "repeater", "poc", "manual":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "manual"
	}
}

func normalizePoCDebugActiveView(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return "debug"
	default:
		return "template"
	}
}

func normalizePoCDebugLastStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "matched", "not_matched", "failed":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}
