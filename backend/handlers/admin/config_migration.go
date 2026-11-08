package admin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"hephaestus-backend/pkg/configmigration"
)

const maxConfigMigrationBodyBytes = 8 << 20

// ExportConfigMigrationHandler exports the selected configuration modules.
func ExportConfigMigrationHandler(c *gin.Context) {
	bundle, err := configmigration.Export(configmigration.ParseModules(c.Query("modules")))
	if err != nil {
		status := http.StatusInternalServerError
		var validation *configmigration.ValidationError
		if errors.As(err, &validation) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": bundle})
}

// ImportConfigMigrationHandler imports a previously exported bundle.
func ImportConfigMigrationHandler(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxConfigMigrationBodyBytes)

	var request configmigration.ImportRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的配置文件: " + err.Error()})
		return
	}

	result, err := configmigration.Import(request)
	if err != nil {
		status := http.StatusInternalServerError
		var validation *configmigration.ValidationError
		if errors.As(err, &validation) {
			status = http.StatusBadRequest
		}
		response := gin.H{"success": false, "error": err.Error()}
		if len(result.Modules) > 0 {
			response["data"] = result
		}
		c.JSON(status, response)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
