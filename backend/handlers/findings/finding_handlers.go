package findings

import (
	"net/http"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"
	findingsvc "hephaestus-backend/pkg/findings"

	"github.com/gin-gonic/gin"
)

type createFindingRequest struct {
	ScopeKey    string    `json:"scopeKey"`
	Source      string    `json:"source"`
	SourceID    string    `json:"sourceId"`
	TaskID      string    `json:"taskId"`
	TaskVersion int       `json:"taskVersion"`
	TemplateID  string    `json:"templateId"`
	Title       string    `json:"title"`
	Category    string    `json:"category"`
	TargetType  string    `json:"targetType"`
	Target      string    `json:"target"`
	Severity    string    `json:"severity"`
	Status      string    `json:"status"`
	Evidence    string    `json:"evidence"`
	Confidence  string    `json:"confidence"`
	ObservedAt  time.Time `json:"observedAt"`
	ResolvedIPs []string  `json:"resolvedIps"`
	CDNEdge     bool      `json:"cdnEdge"`
	Identity    string    `json:"identity"`
}

// CreateFindingHandler is the write path for scanners and the AI workbench.
// It resolves the primary asset, snapshots resolved IPs and updates the risk
// projection in one call.
func CreateFindingHandler(c *gin.Context) {
	var request createFindingRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "请求参数无效: " + err.Error()})
		return
	}
	if strings.TrimSpace(request.Source) == "" || strings.TrimSpace(request.Target) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "source 和 target 不能为空"})
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "数据库不可用: " + err.Error()})
		return
	}
	finding, err := findingsvc.Ingest(db, findingsvc.Input{
		ScopeKey:    request.ScopeKey,
		Source:      request.Source,
		SourceID:    request.SourceID,
		TaskID:      request.TaskID,
		TaskVersion: request.TaskVersion,
		TemplateID:  request.TemplateID,
		Title:       request.Title,
		Category:    request.Category,
		TargetType:  request.TargetType,
		Target:      request.Target,
		Severity:    request.Severity,
		Status:      request.Status,
		Evidence:    request.Evidence,
		Confidence:  request.Confidence,
		ObservedAt:  request.ObservedAt,
		ResolvedIPs: request.ResolvedIPs,
		CDNEdge:     request.CDNEdge,
		Identity:    request.Identity,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "写入漏洞失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": finding})
}
