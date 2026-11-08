package asset

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type unifiedAssetObservationRequest struct {
	ScopeKey      string         `json:"scopeKey"`
	Type          string         `json:"type"`
	Value         string         `json:"value" binding:"required"`
	DisplayValue  string         `json:"displayValue"`
	Source        string         `json:"source" binding:"required"`
	SourceID      string         `json:"sourceId"`
	SourceVersion int            `json:"sourceVersion"`
	TaskID        string         `json:"taskId"`
	TaskVersion   int            `json:"taskVersion"`
	ImportBatchID string         `json:"importBatchId"`
	Origin        string         `json:"origin"`
	GroupKey      string         `json:"groupKey"`
	Confidence    string         `json:"confidence"`
	ObservedAt    string         `json:"observedAt"`
	ExpiresAt     string         `json:"expiresAt"`
	EvidenceRef   string         `json:"evidenceRef"`
	Attributes    map[string]any `json:"attributes"`
}

type unifiedAssetBatchRequest struct {
	Observations []unifiedAssetObservationRequest `json:"observations" binding:"required,min=1,max=500"`
}

type unifiedAssetLegacySyncRequest struct {
	ScopeKey string `json:"scopeKey"`
	TaskID   string `json:"taskId"`
	Since    string `json:"since"`
}

type unifiedAssetLifecycleRequest struct {
	StaleAfterHours  int `json:"staleAfterHours"`
	RetireAfterHours int `json:"retireAfterHours"`
}

func parseAssetTime(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func convertAssetObservation(input unifiedAssetObservationRequest) (assetstore.Observation, error) {
	observedAt, err := parseAssetTime(input.ObservedAt)
	if err != nil {
		return assetstore.Observation{}, err
	}
	expiresAt, err := parseAssetTime(input.ExpiresAt)
	if err != nil {
		return assetstore.Observation{}, err
	}
	observation := assetstore.Observation{ScopeKey: input.ScopeKey, Type: input.Type, Value: input.Value, DisplayValue: input.DisplayValue, Source: input.Source, SourceID: input.SourceID, SourceVersion: input.SourceVersion, TaskID: input.TaskID, TaskVersion: input.TaskVersion, ImportBatchID: input.ImportBatchID, Origin: input.Origin, GroupKey: input.GroupKey, Confidence: input.Confidence, EvidenceRef: input.EvidenceRef, Attributes: input.Attributes, ExpiresAt: expiresAt}
	if _, err := assetstore.Normalize(observation.ScopeKey, observation.Type, observation.Value); err != nil {
		return assetstore.Observation{}, err
	}
	if observedAt != nil {
		observation.ObservedAt = *observedAt
	}
	return observation, nil
}

func assetStoreFromRequest(c *gin.Context) (assetstore.Store, error) {
	db, err := database.GetDB()
	if err != nil {
		return assetstore.Store{}, err
	}
	return assetstore.Store{DB: db.WithContext(c.Request.Context())}, nil
}

func CreateUnifiedAssetObservationsHandler(c *gin.Context) {
	var request unifiedAssetBatchRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	store, err := assetStoreFromRequest(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	inputs := make([]assetstore.Observation, 0, len(request.Observations))
	for _, input := range request.Observations {
		observation, err := convertAssetObservation(input)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}
		inputs = append(inputs, observation)
	}
	assets, observations, err := store.UpsertMany(c.Request.Context(), inputs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"assets": assets, "observations": observations}})
}

func SyncLegacyUnifiedAssetsHandler(c *gin.Context) {
	var request unifiedAssetLegacySyncRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}
	}
	var since *time.Time
	if strings.TrimSpace(request.Since) != "" {
		parsed, err := parseAssetTime(request.Since)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}
		since = parsed
	}
	store, err := assetStoreFromRequest(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	summary, err := store.SyncLegacy(c.Request.Context(), assetstore.LegacySyncOptions{ScopeKey: request.ScopeKey, TaskID: strings.TrimSpace(request.TaskID), Since: since})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

func ListUnifiedAssetsHandler(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	store, err := assetStoreFromRequest(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	assets, total, err := store.List(c.Request.Context(), assetstore.ListFilter{ScopeKey: c.Query("scopeKey"), Type: c.Query("type"), Keyword: c.Query("keyword"), Status: c.Query("status"), Limit: pageSize, Offset: (page - 1) * pageSize})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": assets, "total": total, "page": page, "pageSize": pageSize}})
}

func GetUnifiedAssetHandler(c *gin.Context) {
	store, err := assetStoreFromRequest(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	assetRow, observations, err := store.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "asset not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	sources := make([]string, 0)
	sourceSeen := map[string]struct{}{}
	taskIDs := make([]string, 0)
	taskVersions := map[string]int{}
	taskSeen := map[string]struct{}{}
	for _, observation := range observations {
		source := strings.TrimSpace(observation.Source)
		if source != "" {
			if _, exists := sourceSeen[source]; !exists {
				sourceSeen[source] = struct{}{}
				sources = append(sources, source)
			}
		}
		if taskID := assetstore.ObservationTaskID(observation); taskID != "" {
			if _, exists := taskSeen[taskID]; !exists {
				taskSeen[taskID] = struct{}{}
				taskIDs = append(taskIDs, taskID)
				taskVersions[taskID] = assetstore.ObservationTaskVersion(observation)
			}
		}
	}
	sort.Strings(sources)
	if strings.TrimSpace(assetRow.Verification) == "" {
		assetRow.Verification = "declared"
		for _, observation := range observations {
			origin := assetstore.ObservationOrigin(observation)
			if origin != "manual" && origin != "import" {
				assetRow.Verification = "observed"
				break
			}
		}
	}
	sourceTasks := make([]gin.H, 0, len(taskIDs))
	if len(taskIDs) > 0 {
		db, dbErr := database.GetDB()
		if dbErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": dbErr.Error()})
			return
		}
		byID := map[string]database.VulnScanTask{}
		const taskChunkSize = 500
		for start := 0; start < len(taskIDs); start += taskChunkSize {
			end := start + taskChunkSize
			if end > len(taskIDs) {
				end = len(taskIDs)
			}
			var tasks []database.VulnScanTask
			if err := db.WithContext(c.Request.Context()).Select("id", "version", "task_name", "scan_type", "status").Where("id IN ?", taskIDs[start:end]).Order("version DESC").Find(&tasks).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
				return
			}
			for _, task := range tasks {
				if _, exists := byID[task.ID]; !exists {
					byID[task.ID] = task
				}
			}
		}
		for _, taskID := range taskIDs {
			if task, exists := byID[taskID]; exists {
				sourceTasks = append(sourceTasks, gin.H{"id": task.ID, "version": task.Version, "name": firstNonEmpty(strings.TrimSpace(task.TaskName), task.ID), "scanType": task.ScanType, "status": task.Status})
			} else {
				sourceTasks = append(sourceTasks, gin.H{"id": taskID, "version": taskVersions[taskID], "name": taskID, "scanType": "", "status": "missing", "missing": true})
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"asset": assetRow, "observations": observations, "sourceTasks": sourceTasks, "sources": sources,
	}})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func ListUnifiedAssetRelationsHandler(c *gin.Context) {
	store, err := assetStoreFromRequest(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	relations, err := store.ListRelations(c.Request.Context(), c.Param("id"), c.DefaultQuery("active", "true") != "false")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": relations, "total": len(relations)}})
}

func ReconcileUnifiedAssetLifecycleHandler(c *gin.Context) {
	var request unifiedAssetLifecycleRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}
	}
	store, err := assetStoreFromRequest(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	changed, ran, err := assetstore.RunLifecycleReconcileExclusive(
		c.Request.Context(),
		store.DB,
		time.Duration(request.StaleAfterHours)*time.Hour,
		time.Duration(request.RetireAfterHours)*time.Hour,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"changed": changed, "ran": ran}})
}
