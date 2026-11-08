package toolbox

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/miniprogram"

	"github.com/gin-gonic/gin"
)

type miniProgramDecompileSettings struct {
	AppletPath string                  `json:"appletPath"`
	RuleGroups []miniprogram.RuleGroup `json:"ruleGroups"`
	Rules      []string                `json:"rules,omitempty"`
}

type miniProgramDecompilePayload struct {
	AppletPath string                  `json:"appletPath"`
	AppID      string                  `json:"appId"`
	Version    string                  `json:"version"`
	RuleGroups []miniprogram.RuleGroup `json:"ruleGroups"`
	Rules      []string                `json:"rules,omitempty"`
}

type miniProgramClearPayload struct {
	AppID   string `json:"appId"`
	Version string `json:"version"`
}

func miniProgramOutputRoot() string {
	outputRoot := filepath.Join(config.GetDataDir(), "mini-program-decompile")
	_ = os.MkdirAll(outputRoot, 0755)
	return outputRoot
}

func loadMiniProgramDecompileSettings() (miniProgramDecompileSettings, error) {
	settings := miniProgramDecompileSettings{
		AppletPath: miniprogram.DefaultAppletPath(),
		RuleGroups: miniprogram.CloneRuleGroups(miniprogram.DefaultRuleGroups),
	}

	raw, err := database.GetSetting(database.SettingKeyMiniProgramDecompile)
	if err != nil || raw == nil {
		return settings, err
	}

	payload, err := json.Marshal(raw)
	if err != nil {
		return settings, err
	}
	var stored miniProgramDecompileSettings
	if err := json.Unmarshal(payload, &stored); err != nil {
		return settings, err
	}
	settings.AppletPath = stored.AppletPath
	settings.RuleGroups = stored.RuleGroups
	settings.Rules = stored.Rules
	settings.AppletPath = miniprogram.ResolveAppletPath(settings.AppletPath)
	if settings.AppletPath == "" {
		settings.AppletPath = miniprogram.DefaultAppletPath()
	}
	if len(settings.RuleGroups) == 0 {
		settings.RuleGroups = miniprogram.RuleGroupsFromLegacyRules(settings.Rules)
	}
	settings.RuleGroups = miniprogram.NormalizeRuleGroups(settings.RuleGroups)
	settings.Rules = miniprogram.FlattenEnabledRulePatterns(settings.RuleGroups)
	return settings, nil
}

func saveMiniProgramDecompileSettings(settings miniProgramDecompileSettings) error {
	settings.AppletPath = miniprogram.ResolveAppletPath(settings.AppletPath)
	if len(settings.RuleGroups) == 0 {
		settings.RuleGroups = miniprogram.RuleGroupsFromLegacyRules(settings.Rules)
	}
	settings.RuleGroups = miniprogram.NormalizeRuleGroups(settings.RuleGroups)
	settings.Rules = nil
	return database.SetSetting(database.SettingKeyMiniProgramDecompile, settings)
}

func validateMiniProgramRuleGroups(groups []miniprogram.RuleGroup) error {
	for _, group := range miniprogram.NormalizeRuleGroups(groups) {
		for _, rule := range group.Rules {
			if _, err := regexp.Compile(rule.Pattern); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildMiniProgramService(settings miniProgramDecompileSettings) *miniprogram.Service {
	return miniprogram.NewServiceWithRuleGroups(settings.AppletPath, miniProgramOutputRoot(), settings.RuleGroups)
}

func resolveMiniProgramSettingsFromPayload(payload miniProgramDecompilePayload) (miniProgramDecompileSettings, error) {
	settings, err := loadMiniProgramDecompileSettings()
	if err != nil {
		return settings, err
	}
	if strings.TrimSpace(payload.AppletPath) != "" {
		settings.AppletPath = strings.TrimSpace(payload.AppletPath)
	}
	if len(payload.RuleGroups) > 0 {
		settings.RuleGroups = miniprogram.NormalizeRuleGroups(payload.RuleGroups)
		settings.Rules = miniprogram.FlattenEnabledRulePatterns(settings.RuleGroups)
	} else if len(payload.Rules) > 0 {
		settings.RuleGroups = miniprogram.RuleGroupsFromLegacyRules(payload.Rules)
		settings.Rules = miniprogram.FlattenEnabledRulePatterns(settings.RuleGroups)
	}
	return settings, nil
}

func GetMiniProgramDecompileSettingsHandler(c *gin.Context) {
	settings, err := loadMiniProgramDecompileSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "读取小程序反编译配置失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"appletPath":        settings.AppletPath,
			"ruleGroups":        settings.RuleGroups,
			"defaultRuleGroups": miniprogram.CloneRuleGroups(miniprogram.DefaultRuleGroups),
			"rules":             miniprogram.FlattenEnabledRulePatterns(settings.RuleGroups),
			"outputRoot":        miniProgramOutputRoot(),
		},
	})
}

func UpdateMiniProgramDecompileSettingsHandler(c *gin.Context) {
	var payload miniProgramDecompileSettings
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}
	if len(payload.RuleGroups) == 0 && len(payload.Rules) > 0 {
		payload.RuleGroups = miniprogram.RuleGroupsFromLegacyRules(payload.Rules)
	}
	if err := validateMiniProgramRuleGroups(payload.RuleGroups); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "正则表达式无效: " + err.Error(),
		})
		return
	}
	if err := saveMiniProgramDecompileSettings(payload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存小程序反编译配置失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func ListMiniProgramDecompileProgramsHandler(c *gin.Context) {
	settings, err := loadMiniProgramDecompileSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "读取小程序反编译配置失败: " + err.Error(),
		})
		return
	}
	if rawAppletPath := strings.TrimSpace(c.Query("appletPath")); rawAppletPath != "" {
		settings.AppletPath = rawAppletPath
	}

	items, err := buildMiniProgramService(settings).ScanPrograms()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":      items,
			"outputRoot": miniProgramOutputRoot(),
		},
	})
}

func DecompileMiniProgramHandler(c *gin.Context) {
	var payload miniProgramDecompilePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}
	settings, err := resolveMiniProgramSettingsFromPayload(payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "读取小程序反编译配置失败: " + err.Error(),
		})
		return
	}

	result, err := buildMiniProgramService(settings).Decompile(payload.AppID, payload.Version)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func GetMiniProgramMatchesHandler(c *gin.Context) {
	settings, err := loadMiniProgramDecompileSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "读取小程序反编译配置失败: " + err.Error(),
		})
		return
	}
	appID := c.Query("appId")
	version := c.Query("version")
	service := buildMiniProgramService(settings)
	matches, err := service.ReadGroupedMatches(appID, version)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    matches,
	})
}

func ClearMiniProgramDecompileHandler(c *gin.Context) {
	var payload miniProgramClearPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}
	settings, err := loadMiniProgramDecompileSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "读取小程序反编译配置失败: " + err.Error(),
		})
		return
	}
	if err := buildMiniProgramService(settings).Clear(payload.AppID, payload.Version); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
