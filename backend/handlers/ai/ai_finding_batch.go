package ai

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"hephaestus-backend/handlers/platform"
	"hephaestus-backend/lib/pentest"
	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/findings"
	"hephaestus-backend/pkg/logger"
)

type findingDispositionRequest struct {
	FindingIDs []string `json:"findingIds"`
	Status     string   `json:"status"`
	Note       string   `json:"note"`
}

func UpdateAIPentestFindingStatusesHandler(c *gin.Context) {
	var input findingDispositionRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的批量处置请求"})
		return
	}
	if err := validateFindingDisposition(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	store, err := aipkg.StoreFromAppDB(c.Request.Context())
	if err != nil {
		findingDetailError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": applyFindingDispositions(c, store, input)})
}

func validateFindingDisposition(input *findingDispositionRequest) error {
	switch input.Status {
	case "open", "in_progress", "fixed", "false_positive", "ignored":
	default:
		return errors.New("无效的处置状态")
	}
	if len(input.FindingIDs) == 0 || len(input.FindingIDs) > 100 {
		return errors.New("每次请选择 1 至 100 条漏洞")
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, id := range input.FindingIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return errors.New("漏洞 ID 不能为空")
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	input.FindingIDs = ids
	input.Note = strings.TrimSpace(input.Note)
	if len([]rune(input.Note)) > 2000 {
		return errors.New("处置备注不能超过 2000 字")
	}
	return nil
}

func applyFindingDispositions(c *gin.Context, store *pentest.SQLStore, input findingDispositionRequest) gin.H {
	updated, failed := []gin.H{}, []gin.H{}
	for _, id := range input.FindingIDs {
		finding, err := store.DisposeFinding(c.Request.Context(), id, input.Status, input.Note, platform.Username(c))
		if err != nil {
			message := err.Error()
			if errors.Is(err, sql.ErrNoRows) {
				message = "漏洞不存在或已被删除"
			}
			failed = append(failed, gin.H{"findingId": id, "error": message})
		} else {
			updated = append(updated, pentestFindingView(finding))
			if db, dbErr := database.GetDB(); dbErr == nil {
				if statusErr := findings.UpdateStatusBySourceID(db, "ai", finding.ID, input.Status); statusErr != nil && !errors.Is(statusErr, sql.ErrNoRows) {
					logger.Warning("同步 AI 漏洞归因状态失败: %v", statusErr)
				}
			}
		}
	}
	return gin.H{"updated": updated, "canonicalFindings": updated, "failed": failed, "total": len(input.FindingIDs), "updatedCount": len(updated), "failedCount": len(failed), "sourceUpdatedCount": len(updated), "status": input.Status}
}
