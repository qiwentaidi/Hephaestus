package scan

import (
	"net/http"
	"time"

	"hephaestus-backend/pkg/scantask"

	"github.com/gin-gonic/gin"
)

// Preview uses the same normalization and clock calculation as dispatch/save.
func PreviewWebScanScheduleHandler(c *gin.Context) {
	var raw map[string]interface{}
	if err := c.ShouldBindJSON(&raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid schedule"})
		return
	}
	schedule, err := scantask.ParseScheduleConfigFromMap(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	now := time.Now()
	// Preview intentionally accepts an exhausted one-time schedule: after it
	// fires there is simply no next run, while only saving must reject a past
	// one-time slot.
	runs := make([]time.Time, 0, 5)
	for len(runs) < 5 {
		next, err := scantask.NextRunAt(schedule, now)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}
		if next == nil {
			break
		}
		runs = append(runs, *next)
		now = next.Add(time.Nanosecond)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"runs": runs}})
}
