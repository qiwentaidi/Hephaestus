package space

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"hephaestus-backend/pkg/config"
)

// GetSpaceEngineConfigByTypeHandler 获取指定类型的空间引擎配置
// 支持: fofa, hunter, quake, zoomeye, github, chaos, bevigil, securitytrails
func GetSpaceEngineConfigByTypeHandler(c *gin.Context) {
	engineType := c.Param("type")

	spaceConfig, err := config.GetSpaceEngineConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "加载配置失败: " + err.Error()})
		return
	}

	var result interface{}
	switch engineType {
	case "fofa":
		result = spaceConfig.FOFA
	case "hunter":
		result = spaceConfig.Hunter
	case "quake":
		result = spaceConfig.Quake
	case "zoomeye":
		result = spaceConfig.Zoomeye
	case "shodan":
		result = spaceConfig.Shodan
	case "github":
		result = spaceConfig.Github
	case "chaos":
		result = spaceConfig.Chaos
	case "bevigil":
		result = spaceConfig.Bevigil
	case "securitytrails":
		result = spaceConfig.Securitytrails
	// case "ct-radar", "ctradar":
	// 	result = spaceConfig.CTRadar
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的空间引擎类型: " + engineType})
		return
	}

	c.JSON(http.StatusOK, result)
}

// UpdateSpaceEngineConfigByTypeHandler 更新指定类型的空间引擎配置
// 支持部分更新，只更新指定引擎的配置，其他引擎配置保持不变
func UpdateSpaceEngineConfigByTypeHandler(c *gin.Context) {
	engineType := c.Param("type")

	// 先获取当前完整配置
	spaceConfig, err := config.GetSpaceEngineConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "加载配置失败: " + err.Error()})
		return
	}

	// 根据引擎类型更新对应的配置
	switch engineType {
	case "fofa":
		var fofaConfig config.FOFAConfig
		if err := c.ShouldBindJSON(&fofaConfig); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
			return
		}
		spaceConfig.FOFA = fofaConfig

	case "hunter":
		var hunterConfig config.HunterConfig
		if err := c.ShouldBindJSON(&hunterConfig); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
			return
		}
		spaceConfig.Hunter = hunterConfig

	case "quake":
		var quakeConfig config.QuakeConfig
		if err := c.ShouldBindJSON(&quakeConfig); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
			return
		}
		spaceConfig.Quake = quakeConfig

	case "zoomeye":
		var zoomeyeConfig config.ZoomeyeConfig
		if err := c.ShouldBindJSON(&zoomeyeConfig); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
			return
		}
		spaceConfig.Zoomeye = zoomeyeConfig

	case "shodan":
		var shodanConfig config.ShodanConfig
		if err := c.ShouldBindJSON(&shodanConfig); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
			return
		}
		spaceConfig.Shodan = shodanConfig

	case "github":
		var githubConfig config.GithubConfig
		if err := c.ShouldBindJSON(&githubConfig); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
			return
		}
		spaceConfig.Github = githubConfig

	case "chaos":
		var chaosConfig config.ChaosConfig
		if err := c.ShouldBindJSON(&chaosConfig); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
			return
		}
		spaceConfig.Chaos = chaosConfig

	case "bevigil":
		var bevigilConfig config.BevigilConfig
		if err := c.ShouldBindJSON(&bevigilConfig); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
			return
		}
		spaceConfig.Bevigil = bevigilConfig

	case "securitytrails":
		var securitytrailsConfig config.SecuritytrailsConfig
		if err := c.ShouldBindJSON(&securitytrailsConfig); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
			return
		}
		spaceConfig.Securitytrails = securitytrailsConfig

	// case "ct-radar", "ctradar":
	// 	var ctRadarConfig config.CTRadarConfig
	// 	if err := c.ShouldBindJSON(&ctRadarConfig); err != nil {
	// 		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数: " + err.Error()})
	// 		return
	// 	}
	// 	spaceConfig.CTRadar = ctRadarConfig

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的空间引擎类型: " + engineType})
		return
	}

	// 保存更新后的配置
	if err := config.UpdateSpaceEngineConfig(spaceConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "配置已保存"})
}

// ValidateSpaceEngineConfigHandler 验证指定类型的空间引擎配置
// 可以用于测试 API 密钥是否有效
func ValidateSpaceEngineConfigHandler(c *gin.Context) {
	engineType := c.Param("type")

	spaceConfig, err := config.GetSpaceEngineConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "加载配置失败: " + err.Error()})
		return
	}

	var isValid bool
	var message string

	switch engineType {
	case "fofa":
		isValid = spaceConfig.FOFA.Email != "" && spaceConfig.FOFA.Key != ""
		if !isValid {
			message = "FOFA 配置不完整，需要 Email 和 Key"
		} else {
			message = "FOFA 配置已填写"
		}

	case "hunter":
		isValid = spaceConfig.Hunter.Key != ""
		if !isValid {
			message = "Hunter 配置不完整，需要 Key"
		} else {
			message = "Hunter 配置已填写"
		}

	case "quake":
		isValid = spaceConfig.Quake.Key != ""
		if !isValid {
			message = "Quake 配置不完整，需要 Key"
		} else {
			message = "Quake 配置已填写"
		}

	case "zoomeye":
		isValid = spaceConfig.Zoomeye.Key != ""
		if !isValid {
			message = "Zoomeye 配置不完整，需要 Key"
		} else {
			message = "Zoomeye 配置已填写"
		}

	case "github":
		isValid = spaceConfig.Github.Token != ""
		if !isValid {
			message = "GitHub 配置不完整，需要 Token"
		} else {
			message = "GitHub 配置已填写"
		}

	case "chaos":
		isValid = spaceConfig.Chaos.Key != ""
		if !isValid {
			message = "Chaos 配置不完整，需要 Key"
		} else {
			message = "Chaos 配置已填写"
		}

	case "bevigil":
		isValid = spaceConfig.Bevigil.Key != ""
		if !isValid {
			message = "Bevigil 配置不完整，需要 Key"
		} else {
			message = "Bevigil 配置已填写"
		}

	case "securitytrails":
		isValid = spaceConfig.Securitytrails.Key != ""
		if !isValid {
			message = "Securitytrails 配置不完整，需要 Key"
		} else {
			message = "Securitytrails 配置已填写"
		}

	// case "ct-radar", "ctradar":
	// 	isValid = true
	// 	if spaceConfig.CTRadar.Key == "" {
	// 		message = "CT-Radar 未配置 Key，将使用匿名额度"
	// 	} else {
	// 		message = "CT-Radar 配置已填写"
	// 	}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的空间引擎类型: " + engineType})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":   isValid,
		"message": message,
	})
}
