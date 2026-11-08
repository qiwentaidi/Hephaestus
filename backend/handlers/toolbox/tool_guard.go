package toolbox

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/toolguard"

	"github.com/gin-gonic/gin"
)

const toolGuardRequestMaxBytes = 1 << 20

func GetToolGuardConfigHandler(c *gin.Context) {
	guardConfig, err := config.GetToolGuardConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": guardConfig})
}

func UpdateToolGuardConfigHandler(c *gin.Context) {
	var request struct {
		Enabled *bool             `json:"enabled"`
		Rules   *[]toolguard.Rule `json:"rules"`
	}
	if err := decodeToolGuardJSON(c, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的调用拦截配置: " + err.Error()})
		return
	}
	if request.Rules == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "必须明确提供 rules；清空规则请提供空数组"})
		return
	}
	guardConfig := toolguard.EnsureRuleIDs(toolguard.Config{Enabled: true, Rules: *request.Rules, PresetVersion: toolguard.CurrentPresetVersion})
	if _, err := toolguard.Compile(guardConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := config.UpdateToolGuardConfig(guardConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := toolguard.UpdateGlobal(guardConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "应用调用拦截配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": guardConfig, "message": "调用拦截配置已保存并立即生效"})
}

func TestToolGuardConfigHandler(c *gin.Context) {
	var request struct {
		Config    *toolguard.Config `json:"config"`
		ToolName  string            `json:"toolName"`
		Arguments any               `json:"arguments"`
	}
	if err := decodeToolGuardJSON(c, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的试匹配参数: " + err.Error()})
		return
	}
	if request.Config == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "请提供待测试的 config"})
		return
	}
	testConfig := *request.Config
	testConfig.Enabled = true
	policy, err := toolguard.Compile(toolguard.EnsureRuleIDs(testConfig))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	match := policy.CheckToolCall(request.ToolName, request.Arguments)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"blocked": match != nil,
			"match":   match,
		},
	})
}

func decodeToolGuardJSON(c *gin.Context, destination any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, toolGuardRequestMaxBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("请求必须只包含一个 JSON 对象")
	}
	return nil
}
