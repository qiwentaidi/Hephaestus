package scan

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
)

const (
	QueryPresetSceneJSVulnerability = "jsscan_vulnerability"
	QueryPresetSceneDirScanResult   = "dirscan_result"
)

var supportedQueryPresetScenes = map[string]struct{}{
	QueryPresetSceneJSVulnerability: {},
	QueryPresetSceneDirScanResult:   {},
}

type SavedQueryPreset struct {
	ID        string `json:"id"`
	Scene     string `json:"scene"`
	Name      string `json:"name"`
	Query     string `json:"query"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type saveSavedQueryPresetRequest struct {
	ID    string `json:"id"`
	Scene string `json:"scene" binding:"required"`
	Name  string `json:"name" binding:"required"`
	Query string `json:"query" binding:"required"`
}

func ListSavedQueryPresetsHandler(c *gin.Context) {
	scene := strings.TrimSpace(c.Query("scene"))
	if scene != "" && !isSupportedQueryPresetScene(scene) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "不支持的查询场景",
		})
		return
	}

	presets, err := loadSavedQueryPresets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "读取常用查询失败: " + err.Error(),
		})
		return
	}

	if scene != "" {
		filtered := make([]SavedQueryPreset, 0, len(presets))
		for _, preset := range presets {
			if preset.Scene == scene {
				filtered = append(filtered, preset)
			}
		}
		presets = filtered
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    presets,
	})
}

func SaveSavedQueryPresetHandler(c *gin.Context) {
	var req saveSavedQueryPresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数",
		})
		return
	}

	scene := strings.TrimSpace(req.Scene)
	name := strings.TrimSpace(req.Name)
	query := strings.TrimSpace(req.Query)
	id := strings.TrimSpace(req.ID)

	if !isSupportedQueryPresetScene(scene) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "不支持的查询场景"})
		return
	}
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "查询名称不能为空"})
		return
	}
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "查询语法不能为空"})
		return
	}

	presets, err := loadSavedQueryPresets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "读取常用查询失败: " + err.Error()})
		return
	}

	now := time.Now().Format(time.RFC3339)
	saved := SavedQueryPreset{
		ID:        id,
		Scene:     scene,
		Name:      name,
		Query:     query,
		UpdatedAt: now,
	}
	if saved.ID == "" {
		saved.ID = database.GenerateNanoid()
		saved.CreatedAt = now
	}

	updated := false
	for i := range presets {
		if presets[i].ID != saved.ID {
			continue
		}
		saved.CreatedAt = presets[i].CreatedAt
		if saved.CreatedAt == "" {
			saved.CreatedAt = now
		}
		presets[i] = saved
		updated = true
		break
	}
	if !updated {
		presets = append(presets, saved)
	}

	sortSavedQueryPresets(presets)

	if err := database.SetSetting(database.SettingKeySavedQueryPresets, presets); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "保存常用查询失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    saved,
	})
}

func DeleteSavedQueryPresetHandler(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "查询 ID 不能为空"})
		return
	}

	presets, err := loadSavedQueryPresets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "读取常用查询失败: " + err.Error()})
		return
	}

	next := make([]SavedQueryPreset, 0, len(presets))
	removed := false
	for _, preset := range presets {
		if preset.ID == id {
			removed = true
			continue
		}
		next = append(next, preset)
	}
	if !removed {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "查询不存在"})
		return
	}

	if err := database.SetSetting(database.SettingKeySavedQueryPresets, next); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "删除常用查询失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func loadSavedQueryPresets() ([]SavedQueryPreset, error) {
	value, err := database.GetSetting(database.SettingKeySavedQueryPresets)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return []SavedQueryPreset{}, nil
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	var presets []SavedQueryPreset
	if err := json.Unmarshal(raw, &presets); err != nil {
		return nil, err
	}

	filtered := make([]SavedQueryPreset, 0, len(presets))
	for _, preset := range presets {
		preset.ID = strings.TrimSpace(preset.ID)
		preset.Scene = strings.TrimSpace(preset.Scene)
		preset.Name = strings.TrimSpace(preset.Name)
		preset.Query = strings.TrimSpace(preset.Query)
		if preset.ID == "" || preset.Name == "" || preset.Query == "" || !isSupportedQueryPresetScene(preset.Scene) {
			continue
		}
		filtered = append(filtered, preset)
	}

	sortSavedQueryPresets(filtered)
	return filtered, nil
}

func sortSavedQueryPresets(presets []SavedQueryPreset) {
	sort.SliceStable(presets, func(i, j int) bool {
		left, leftOk := parsePresetTime(presets[i].UpdatedAt)
		right, rightOk := parsePresetTime(presets[j].UpdatedAt)
		switch {
		case leftOk && rightOk:
			if left.Equal(right) {
				return strings.ToLower(presets[i].Name) < strings.ToLower(presets[j].Name)
			}
			return left.After(right)
		case leftOk:
			return true
		case rightOk:
			return false
		default:
			return strings.ToLower(presets[i].Name) < strings.ToLower(presets[j].Name)
		}
	})
}

func parsePresetTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func isSupportedQueryPresetScene(scene string) bool {
	_, ok := supportedQueryPresetScenes[scene]
	return ok
}
