package scan

import (
	"errors"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/findings"
	"hephaestus-backend/pkg/logger"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func getVulnScanTaskOrAbort(c *gin.Context, db *gorm.DB, taskID string) (*database.VulnScanTask, bool) {
	var task database.VulnScanTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "未找到对应的 JS 扫描任务",
			})
			return nil, false
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询任务失败: " + err.Error(),
		})
		return nil, false
	}
	return &task, true
}

// GetJSScanVulnerabilitiesHandler 获取JS扫描漏洞列表
func GetJSScanVulnerabilitiesHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	taskID := c.Param("id")
	versionParam := c.Query("version")
	level := strings.TrimSpace(c.Query("level"))
	vulnType := strings.TrimSpace(c.Query("type"))
	method := strings.TrimSpace(c.Query("method"))
	keyword := strings.TrimSpace(c.Query("keyword"))
	responseType := strings.TrimSpace(c.Query("responseType"))
	disposalStatus := strings.TrimSpace(c.Query("disposalStatus"))
	filterExprRaw := strings.TrimSpace(c.Query("filterExpr"))
	responseLengthRule := strings.TrimSpace(c.Query("responseLength"))
	excludeResponseLengthRule := strings.TrimSpace(c.Query("excludeResponseLength"))
	pageQuery := c.Query("page")
	pageSizeQuery := c.Query("pageSize")
	includePayload, includePayloadSet := parseQueryBool(c.Query("includePayload"))
	aiVerifiedValue, hasAIVerifiedFilter := parseQueryBool(c.Query("aiVerified"))
	usePagination := pageQuery != "" || pageSizeQuery != ""
	page := parseQueryInt(pageQuery, 1)
	pageSize := parseQueryInt(pageSizeQuery, 20)
	if pageSize > 200 {
		pageSize = 200
	}

	exprFilters := newVulnerabilityFilterExpression()
	if filterExprRaw != "" {
		parsedExpr, parseErr := parseVulnerabilityFilterExpression(filterExprRaw)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "筛选表达式错误: " + parseErr.Error(),
			})
			return
		}
		exprFilters = parsedExpr
	}

	if _, ok := getVulnScanTaskOrAbort(c, db, taskID); !ok {
		return
	}

	finalLevel := normalizeVulnLevel(level)
	if len(exprFilters.OrGroups) == 0 && (finalLevel == "" || finalLevel == "all") && exprFilters.Level != "" {
		finalLevel = exprFilters.Level
	}
	finalType := vulnType
	if len(exprFilters.OrGroups) == 0 && (finalType == "" || finalType == "all") && exprFilters.Type != "" {
		finalType = exprFilters.Type
	}
	finalMethod := method
	if len(exprFilters.OrGroups) == 0 && (finalMethod == "" || finalMethod == "all") && exprFilters.Method != "" {
		finalMethod = exprFilters.Method
	}
	finalResponseType := responseType
	if len(exprFilters.OrGroups) == 0 && finalResponseType == "" && exprFilters.ResponseType != "" {
		finalResponseType = exprFilters.ResponseType
	}
	finalDisposalStatus := disposalStatus
	if len(exprFilters.OrGroups) == 0 && finalDisposalStatus == "" && exprFilters.DisposalStatus != "" {
		finalDisposalStatus = exprFilters.DisposalStatus
	}
	finalKeyword := keyword
	if len(exprFilters.OrGroups) == 0 && finalKeyword == "" {
		finalKeyword = exprFilters.Keyword
	}
	finalHasAIVerified := hasAIVerifiedFilter
	finalAIVerifiedValue := aiVerifiedValue
	if len(exprFilters.OrGroups) == 0 && !finalHasAIVerified && exprFilters.AIVerified != nil {
		finalHasAIVerified = true
		finalAIVerifiedValue = *exprFilters.AIVerified
	}

	includeLengthConditions := exprFilters.ResponseLength
	if len(exprFilters.OrGroups) > 0 {
		includeLengthConditions = nil
	}
	if responseLengthRule != "" {
		conditions, err := parseResponseLengthConditions(responseLengthRule)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "响应长度筛选格式错误: " + err.Error(),
			})
			return
		}
		includeLengthConditions = conditions
	}

	excludeLengthConditions := exprFilters.ExcludeLength
	if len(exprFilters.OrGroups) > 0 {
		excludeLengthConditions = nil
	}
	if excludeResponseLengthRule != "" {
		conditions, err := parseResponseLengthConditions(excludeResponseLengthRule)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "响应长度排除格式错误: " + err.Error(),
			})
			return
		}
		excludeLengthConditions = conditions
	}

	query := db.Model(&database.VulnScanJSVulnerability{})
	if versionParam != "" {
		var task database.VulnScanTask
		if err := db.Where("id = ? AND version = ?", taskID, versionParam).First(&task).Error; err == nil &&
			task.ScanType == "combination" && strings.TrimSpace(task.ParentCombinationID) != "" {
			version, parseErr := strconv.Atoi(versionParam)
			if parseErr == nil && version > 0 {
				resultTaskIDs, idsErr := combinationResultTaskIDs(db, task, version)
				if idsErr != nil {
					c.JSON(http.StatusInternalServerError, gin.H{
						"success": false,
						"error":   "查询子任务失败: " + idsErr.Error(),
					})
					return
				}
				query = query.Where("task_id IN ? AND version = ?", resultTaskIDs, version)
			} else {
				query = query.Where("task_id = ? AND version = ?", taskID, versionParam)
			}
		} else {
			query = query.Where("task_id = ? AND version = ?", taskID, versionParam)
		}
	} else {
		query = query.Where("task_id = ?", taskID)
	}
	if finalLevel != "" && finalLevel != "all" {
		query = query.Where("level = ?", finalLevel)
	}
	if finalType != "" && finalType != "all" {
		query = query.Where("type = ?", finalType)
	}
	if finalMethod != "" && finalMethod != "all" {
		query = query.Where("method = ?", finalMethod)
	}
	if finalResponseType != "" {
		like := "%" + strings.ToLower(finalResponseType) + "%"
		query = query.Where("LOWER(response_type) LIKE ?", like)
	}
	if normalizedDisposalStatus := normalizeJSVulnDisposalStatus(finalDisposalStatus); normalizedDisposalStatus != "" {
		query = query.Where("COALESCE(disposal_status, 'open') = ?", normalizedDisposalStatus)
	}
	if finalKeyword != "" {
		like := "%" + finalKeyword + "%"
		query = query.Where(
			"title LIKE ? OR type LIKE ? OR url LIKE ? OR description LIKE ? OR method LIKE ? OR response_type LIKE ?",
			like, like, like, like, like, like,
		)
	}
	if finalHasAIVerified {
		query = query.Where("ai_verified = ?", finalAIVerifiedValue)
	}
	if len(includeLengthConditions) > 0 {
		query = applyResponseLengthInclude(query, includeLengthConditions)
	}
	if len(excludeLengthConditions) > 0 {
		query = applyResponseLengthExclude(query, excludeLengthConditions)
	}
	if len(exprFilters.OrGroups) > 0 {
		query = applyVulnerabilityOrGroups(query, exprFilters.OrGroups)
	}

	sortField := strings.TrimSpace(c.Query("sortField"))
	sortColumn := "created_at"
	if sortField == "responseLength" {
		sortColumn = "response_length"
	}
	sortOrder := normalizeSortOrder(c.Query("sortOrder"))
	orderClause := sortColumn + " " + sortOrder

	listQuery := query
	if !includePayloadSet || !includePayload {
		listQuery = listQuery.Select(
			"id",
			"task_id",
			"version",
			"title",
			"level",
			"type",
			"url",
			"method",
			"response_type",
			"trace_id",
			"has_protocol_trace",
			"decryption_status",
			"response_length",
			"confidence",
			"static_contexts",
			"ai_verified",
			"disposal_status",
			"created_at",
		)
	}

	var total int64
	if usePagination {
		if err := query.Count(&total).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "统计漏洞列表失败: " + err.Error(),
			})
			return
		}
	}

	var vulns []database.VulnScanJSVulnerability
	if usePagination {
		offset := (page - 1) * pageSize
		if err := listQuery.Order(orderClause).Order("id DESC").Offset(offset).Limit(pageSize).Find(&vulns).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "获取漏洞列表失败: " + err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"vulnerabilities": vulns,
				"total":           total,
				"page":            page,
				"pageSize":        pageSize,
			},
		})
		return
	}

	if err := listQuery.Order(orderClause).Order("id DESC").Find(&vulns).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取漏洞列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    vulns,
	})
}

// GetJSScanVulnerabilityDetailHandler 获取单条 JS 扫描漏洞详情（包含请求与响应）
func GetJSScanVulnerabilityDetailHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	taskID := c.Param("id")
	vulnID := c.Param("vulnId")
	versionParam := c.Query("version")

	if _, ok := getVulnScanTaskOrAbort(c, db, taskID); !ok {
		return
	}

	query := db.Where("task_id = ? AND id = ?", taskID, vulnID)
	if versionParam != "" {
		query = query.Where("version = ?", versionParam)
	}

	var vuln database.VulnScanJSVulnerability
	if err := query.First(&vuln).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "未找到漏洞详情",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取漏洞详情失败: " + err.Error(),
		})
		return
	}

	detail := jsScanVulnerabilityDetailResponse{
		VulnScanJSVulnerability: vuln,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    detail,
	})
}

type updateJSScanVulnerabilityDisposalStatusRequest struct {
	Version        int    `json:"version"`
	DisposalStatus string `json:"disposalStatus"`
}

func UpdateJSScanVulnerabilityDisposalStatusHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	taskID := c.Param("id")
	vulnID := c.Param("vulnId")
	if taskID == "" || vulnID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "缺少漏洞定位参数",
		})
		return
	}

	if _, ok := getVulnScanTaskOrAbort(c, db, taskID); !ok {
		return
	}

	var req updateJSScanVulnerabilityDisposalStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数无效: " + err.Error(),
		})
		return
	}

	status, ok := normalizeVulnerabilityDisposalStatus(req.DisposalStatus)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "处置状态无效，仅支持: open, in_progress, fixed, false_positive, ignored",
		})
		return
	}

	query := db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND id = ?", taskID, vulnID)
	if req.Version > 0 {
		query = query.Where("version = ?", req.Version)
	}

	var vuln database.VulnScanJSVulnerability
	if err := query.First(&vuln).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "漏洞不存在",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询漏洞失败: " + err.Error(),
		})
		return
	}

	if err := query.Update("disposal_status", status).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "更新漏洞处置状态失败: " + err.Error(),
		})
		return
	}
	if key, keyErr := findings.ComputeDedupKey(findings.Input{
		Source:            "jsscan",
		SourceID:          vuln.ID,
		TaskID:            taskID,
		TaskVersion:       vuln.Version,
		Title:             vuln.Title,
		VulnerabilityType: vuln.Type,
		Target:            vuln.URL,
		Severity:          vuln.Level,
		Identity:          strings.TrimSpace(vuln.Type) + "|" + strings.TrimSpace(vuln.Title),
	}); keyErr == nil {
		if statusErr := findings.UpdateStatusByDedupKey(db, key, status); statusErr != nil && !errors.Is(statusErr, gorm.ErrRecordNotFound) {
			logger.Error("同步漏洞归因状态失败: %v", statusErr)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"disposalStatus": status,
		},
	})
}

func DeleteJSScanVulnerabilityHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	taskID := c.Param("id")
	vulnID := c.Param("vulnId")
	versionParam := c.Query("version")
	if taskID == "" || vulnID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "缺少漏洞定位参数",
		})
		return
	}

	if _, ok := getVulnScanTaskOrAbort(c, db, taskID); !ok {
		return
	}

	query := db.Where("task_id = ? AND id = ?", taskID, vulnID)
	if versionParam != "" {
		query = query.Where("version = ?", versionParam)
	}

	var vuln database.VulnScanJSVulnerability
	if err := query.First(&vuln).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "漏洞不存在",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询漏洞失败: " + err.Error(),
		})
		return
	}

	if err := db.Delete(&vuln).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "删除漏洞失败: " + err.Error(),
		})
		return
	}
	if statusErr := findings.UpdateStatusBySourceID(db, "jsscan", vuln.ID, "ignored"); statusErr != nil && !errors.Is(statusErr, gorm.ErrRecordNotFound) {
		logger.Error("同步漏洞归因状态失败: %v", statusErr)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "漏洞已删除",
	})
}
