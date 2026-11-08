package asset

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ledgerImportError struct {
	Row    int    `json:"row"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

type ledgerImportRow struct {
	Row      int
	Type     string
	Value    string
	Original string
}

func normalizeImportType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "domain", "host", "hostname", "域名", "子域名":
		return assetstore.TypeDomain
	case "ip", "ipv4", "ipv6", "ip地址":
		return assetstore.TypeIP
	case "url", "uri", "网址", "链接":
		return assetstore.TypeURL
	default:
		return ""
	}
}

func isImportHeader(fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	first := strings.ToLower(strings.TrimSpace(fields[0]))
	if len(fields) == 1 {
		return first == "value" || first == "资产" || first == "资产值" || first == "目标"
	}
	second := strings.ToLower(strings.TrimSpace(fields[1]))
	return (first == "type" || first == "类型" || first == "资产类型") &&
		(second == "value" || second == "值" || second == "资产" || second == "资产值")
}

func parseImportContent(content string) ([]ledgerImportRow, []ledgerImportError, int, int) {
	content = strings.TrimPrefix(content, "\ufeff")
	reader := csv.NewReader(strings.NewReader(content))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	rows := make([]ledgerImportRow, 0)
	errorsOut := make([]ledgerImportError, 0)
	seen := map[string]struct{}{}
	duplicates := 0
	line := 0
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			errorsOut = append(errorsOut, ledgerImportError{Row: line + 1, Reason: "无法解析该行: " + err.Error()})
			line++
			continue
		}
		line++
		rawLine := strings.TrimSpace(strings.Join(record, ","))
		if rawLine == "" || strings.HasPrefix(rawLine, "#") {
			continue
		}
		if len(rows) == 0 && len(errorsOut) == 0 && isImportHeader(record) {
			continue
		}
		if len(record) == 0 {
			continue
		}
		typeValue := ""
		values := make([]string, 0, len(record))
		if len(record) == 1 {
			values = append(values, record[0])
		} else if len(record) == 2 {
			if parsedType := normalizeImportType(record[0]); parsedType != "" {
				typeValue = parsedType
				values = append(values, record[1])
			} else {
				values = append(values, record[0], record[1])
			}
		} else {
			errorsOut = append(errorsOut, ledgerImportError{Row: line, Value: rawLine, Reason: "每行只支持一列值或 type,value 两列"})
			continue
		}
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			normalized, err := assetstore.Normalize("", typeValue, value)
			if err != nil {
				errorsOut = append(errorsOut, ledgerImportError{Row: line, Value: value, Reason: err.Error()})
				continue
			}
			key := normalized.Type + "\x00" + normalized.CanonicalValue
			if _, exists := seen[key]; exists {
				duplicates++
				continue
			}
			seen[key] = struct{}{}
			rows = append(rows, ledgerImportRow{Row: line, Type: normalized.Type, Value: normalized.CanonicalValue, Original: value})
		}
	}
	return rows, errorsOut, duplicates, line
}

type importRequest struct {
	Content  string `json:"content"`
	GroupKey string `json:"groupKey"`
	DryRun   bool   `json:"dryRun"`
}

func readImportRequest(c *gin.Context) (importRequest, error) {
	var request importRequest
	if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		file, err := c.FormFile("file")
		if err != nil {
			return request, fmt.Errorf("请选择要导入的 CSV 文件")
		}
		opened, err := file.Open()
		if err != nil {
			return request, err
		}
		defer opened.Close()
		data, err := io.ReadAll(io.LimitReader(opened, 4<<20))
		if err != nil {
			return request, err
		}
		request.Content = string(data)
		request.GroupKey = strings.TrimSpace(c.PostForm("groupKey"))
		request.DryRun = strings.EqualFold(c.PostForm("dryRun"), "true") || c.PostForm("dryRun") == "1"
		return request, nil
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		return request, err
	}
	request.GroupKey = strings.TrimSpace(request.GroupKey)
	return request, nil
}

func buildImportPreview(rows []ledgerImportRow, errorsOut []ledgerImportError, duplicates int) gin.H {
	byType := map[string]int{assetstore.TypeDomain: 0, assetstore.TypeIP: 0, assetstore.TypeURL: 0}
	preview := make([]gin.H, 0, 20)
	for _, row := range rows {
		byType[row.Type]++
		if len(preview) < 20 {
			preview = append(preview, gin.H{"type": row.Type, "value": row.Value, "original": row.Original})
		}
	}
	limit := len(errorsOut)
	if limit > 50 {
		limit = 50
	}
	return gin.H{
		"total":      len(rows) + len(errorsOut) + duplicates,
		"valid":      len(rows),
		"invalid":    len(errorsOut),
		"duplicates": duplicates,
		"byType":     byType,
		"errors":     errorsOut[:limit],
		"preview":    preview,
	}
}

func ImportUnifiedAssetsHandler(c *gin.Context) {
	request, err := readImportRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if strings.TrimSpace(request.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "导入内容不能为空"})
		return
	}
	rows, parseErrors, duplicates, _ := parseImportContent(request.Content)
	if request.DryRun {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": buildImportPreview(rows, parseErrors, duplicates)})
		return
	}
	if len(rows) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "没有可导入的有效资产", "data": buildImportPreview(rows, parseErrors, duplicates)})
		return
	}
	store, err := assetStoreFromRequest(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	contentSum := sha256.Sum256([]byte(request.Content))
	batchID := "import_" + hex.EncodeToString(contentSum[:])[:16]
	now := time.Now().UTC()
	observations := make([]assetstore.Observation, 0, len(rows))
	assetIDs := make([]string, 0, len(rows))
	for index, row := range rows {
		assetIDs = append(assetIDs, assetstore.AssetID(assetstore.DefaultScope, row.Type, row.Value))
		observations = append(observations, assetstore.Observation{
			ScopeKey:      assetstore.DefaultScope,
			Type:          row.Type,
			Value:         row.Value,
			DisplayValue:  row.Original,
			Source:        "import",
			Origin:        "import",
			SourceID:      fmt.Sprintf("%s:%d", batchID, index),
			ImportBatchID: batchID,
			GroupKey:      request.GroupKey,
			Confidence:    "medium",
			ObservedAt:    now,
			Attributes: map[string]any{
				"importBatchId": batchID,
				"row":           row.Row,
				"originalValue": row.Original,
				"groupKey":      request.GroupKey,
			},
		})
	}
	existing := make(map[string]struct{}, len(assetIDs))
	const chunkSize = 500
	for start := 0; start < len(assetIDs); start += chunkSize {
		end := start + chunkSize
		if end > len(assetIDs) {
			end = len(assetIDs)
		}
		var existingIDs []string
		if err := store.DB.WithContext(c.Request.Context()).Model(&database.UnifiedAsset{}).Where("id IN ?", assetIDs[start:end]).Pluck("id", &existingIDs).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
		for _, id := range existingIDs {
			existing[id] = struct{}{}
		}
	}
	created := 0
	for _, id := range assetIDs {
		if _, ok := existing[id]; !ok {
			created++
		}
	}
	batchSize := 200
	for start := 0; start < len(observations); start += batchSize {
		end := start + batchSize
		if end > len(observations) {
			end = len(observations)
		}
		if _, _, err := store.UpsertMany(c.Request.Context(), observations[start:end]); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
	}
	data := buildImportPreview(rows, parseErrors, duplicates)
	data["batchId"] = batchID
	data["created"] = created
	data["updated"] = len(assetIDs) - created
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

type createLedgerAssetRequest struct {
	Type     string   `json:"type"`
	Value    string   `json:"value" binding:"required"`
	Name     string   `json:"name"`
	Notes    string   `json:"notes"`
	Tags     []string `json:"tags"`
	GroupKey string   `json:"groupKey"`
}

func CreateLedgerAssetHandler(c *gin.Context) {
	var request createLedgerAssetRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	store, err := assetStoreFromRequest(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	input := assetstore.Observation{
		Type:         request.Type,
		Value:        request.Value,
		DisplayValue: request.Value,
		Source:       "manual",
		Origin:       "manual",
		GroupKey:     request.GroupKey,
		Confidence:   "medium",
		ObservedAt:   time.Now().UTC(),
		Attributes:   map[string]any{"groupKey": strings.TrimSpace(request.GroupKey)},
	}
	assetRow, _, err := store.Upsert(c.Request.Context(), input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	update := assetstore.AssetUpdate{}
	hasUpdate := false
	if name := strings.TrimSpace(request.Name); name != "" {
		update.Name = &name
		hasUpdate = true
	}
	if notes := strings.TrimSpace(request.Notes); notes != "" {
		update.Notes = &notes
		hasUpdate = true
	}
	if request.Tags != nil {
		update.Tags = &request.Tags
		hasUpdate = true
	}
	if groupKey := strings.TrimSpace(request.GroupKey); groupKey != "" {
		update.GroupKey = &groupKey
		hasUpdate = true
	}
	if hasUpdate {
		if assetRow, err = store.UpdateAsset(c.Request.Context(), assetRow.ID, update); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": assetRow})
}

type updateLedgerAssetRequest struct {
	Website  bool      `json:"website"`
	Name     *string   `json:"name"`
	Notes    *string   `json:"notes"`
	Tags     *[]string `json:"tags"`
	GroupKey *string   `json:"groupKey"`
	Archived *bool     `json:"archived"`
}

func UpdateLedgerAssetHandler(c *gin.Context) {
	var request updateLedgerAssetRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	store, err := assetStoreFromRequest(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	var assetRow database.UnifiedAsset
	if request.Website {
		if request.Archived == nil || request.Name != nil || request.Notes != nil || request.Tags != nil || request.GroupKey != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "网站聚合操作只支持归档或恢复"})
			return
		}
		assetRow, err = store.ArchiveWebsite(c.Request.Context(), c.Param("id"), *request.Archived)
	} else {
		assetRow, err = store.UpdateAsset(c.Request.Context(), c.Param("id"), assetstore.AssetUpdate{
			Name:     request.Name,
			Notes:    request.Notes,
			Tags:     request.Tags,
			GroupKey: request.GroupKey,
			Archived: request.Archived,
		})
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "资产不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": assetRow})
}

func ListAssetLedgerHandler(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	includeArchived := c.Query("includeArchived") == "true" || c.Query("includeArchived") == "1"
	store, err := assetStoreFromRequest(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	items, total, err := store.ListLedger(c.Request.Context(), assetstore.LedgerFilter{
		Type:            c.Query("type"),
		Status:          c.Query("status"),
		Verification:    c.Query("verification"),
		Source:          c.Query("source"),
		Keyword:         c.Query("keyword"),
		GroupKey:        c.Query("groupKey"),
		IncludeArchived: includeArchived,
		Limit:           pageSize,
		Offset:          (page - 1) * pageSize,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	summary, err := store.LedgerSummary(c.Request.Context(), includeArchived)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"items": items, "total": total, "page": page, "pageSize": pageSize, "summary": summary,
	}})
}
