package scan

import (
	"hephaestus-backend/pkg/webscan"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
)

// GetTemplateListHandler 获取模板列表
func GetTemplateListHandler(c *gin.Context) {
	if err := webscan.RefreshPOCTemplateCache(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "刷新 PoC 模板仓库失败: " + err.Error(),
		})
		return
	}

	// 从 WorkFlowDB 获取所有模板 ID
	templateIDs := webscan.GetTemplateList()

	// 按 ID 排序
	sort.Strings(templateIDs)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    templateIDs,
	})
}

func GetPOCTemplateFoldersHandler(c *gin.Context) {
	folders, err := webscan.GetAdditionalPOCTemplateFolders()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取追加模板目录失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"folders": folders,
		},
	})
}

func UpdatePOCTemplateFoldersHandler(c *gin.Context) {
	var req pocTemplateFoldersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数无效: " + err.Error(),
		})
		return
	}

	folders, err := webscan.SetAdditionalPOCTemplateFolders(req.Folders)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "保存追加模板目录失败: " + err.Error(),
		})
		return
	}

	if err := webscan.RefreshPOCTemplateCache(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "刷新 PoC 模板仓库失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"folders": folders,
		},
	})
}

type pocListItem struct {
	FileName              string   `json:"fileName"`
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Author                string   `json:"author"`
	Severity              string   `json:"severity"`
	Description           string   `json:"description"`
	Tags                  []string `json:"tags"`
	RelatedFingerprint    string   `json:"relatedFingerprint"`
	DetectionMethod       string   `json:"detectionMethod"`
	DAST                  bool     `json:"dast"`
	PrincipleDetection    bool     `json:"principleDetection"`
	VulnerabilityCategory string   `json:"vulnerabilityCategory"`
	VulnLabels            []string `json:"vulnLabels"`
	CreatedTime           string   `json:"createdTime"`
	UpdatedTime           string   `json:"updatedTime"`
	ModifiedTime          string   `json:"modifiedTime"`
	SearchContent         string   `json:"-"`
}

type vulnLabelBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type savePoCRequest struct {
	Content string `json:"content"`
}

type pocTemplateFoldersRequest struct {
	Folders []string `json:"folders"`
}

func parseTemplateTags(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '|' || r == '\n' || r == '\t'
	})
	tags := make([]string, 0, len(parts))
	for _, item := range parts {
		tag := strings.TrimSpace(item)
		if tag == "" {
			continue
		}
		tags = append(tags, tag)
	}
	return tags
}

func parsePositiveIntOrDefault(raw string, defaultValue int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultValue
	}
	return value
}

func parseMetadataBool(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	raw, exists := metadata[key]
	if !exists {
		return false
	}

	switch value := raw.(type) {
	case bool:
		return value
	case string:
		normalized := strings.TrimSpace(strings.ToLower(value))
		return normalized == "true" || normalized == "1" || normalized == "yes"
	case int:
		return value != 0
	case int64:
		return value != 0
	case float64:
		return value != 0
	default:
		return false
	}
}

func parseMetadataStringList(metadata map[string]any, key string) []string {
	if metadata == nil {
		return nil
	}
	raw, exists := metadata[key]
	if !exists || raw == nil {
		return nil
	}

	switch value := raw.(type) {
	case string:
		return parseTemplateTags(value)
	case []string:
		return value
	case []any:
		items := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				text = strings.TrimSpace(text)
				if text != "" {
					items = append(items, text)
				}
			}
		}
		return items
	default:
		return nil
	}
}

func resolveRelatedFingerprint(tags []string, metadata map[string]any) string {
	if metadata != nil {
		if productRaw, exists := metadata["product"]; exists {
			if product, ok := productRaw.(string); ok && strings.TrimSpace(product) != "" {
				if vendorRaw, hasVendor := metadata["vendor"]; hasVendor {
					if vendor, ok := vendorRaw.(string); ok && strings.TrimSpace(vendor) != "" {
						return strings.TrimSpace(vendor) + "-" + strings.TrimSpace(product)
					}
				}
				return strings.TrimSpace(product)
			}
		}
	}

	if len(tags) > 0 {
		return tags[0]
	}
	return ""
}

func normalizeCWE(cwe string) string {
	cwe = strings.TrimSpace(cwe)
	if cwe == "" {
		return ""
	}
	upper := strings.ToUpper(cwe)
	if strings.HasPrefix(upper, "CWE-") {
		return upper
	}
	return "CWE-" + upper
}

type pocSearchClause struct {
	Field string
	Value string
}

var pocSearchFieldPattern = regexp.MustCompile(`^([a-zA-Z0-9_.-]+)\s*:\s*(.+)$`)

func splitPoCSearchClauses(input string, separator string) []string {
	text := strings.TrimSpace(input)
	if text == "" {
		return nil
	}

	var result []string
	var current strings.Builder
	var quote byte
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if quote != 0 {
			current.WriteByte(ch)
			if ch == quote && (i == 0 || text[i-1] != '\\') {
				quote = 0
			}
			continue
		}

		if ch == '"' || ch == '\'' {
			quote = ch
			current.WriteByte(ch)
			continue
		}

		if len(separator) > 0 && i+len(separator) <= len(text) && strings.EqualFold(text[i:i+len(separator)], separator) {
			prevOK := i == 0 || unicode.IsSpace(rune(text[i-1]))
			nextIndex := i + len(separator)
			nextOK := nextIndex >= len(text) || unicode.IsSpace(rune(text[nextIndex]))
			if prevOK && nextOK {
				part := strings.TrimSpace(current.String())
				if part != "" {
					result = append(result, part)
				}
				current.Reset()
				for nextIndex < len(text) && unicode.IsSpace(rune(text[nextIndex])) {
					nextIndex++
				}
				i = nextIndex - 1
				continue
			}
		}

		current.WriteByte(ch)
	}

	if tail := strings.TrimSpace(current.String()); tail != "" {
		result = append(result, tail)
	}
	return result
}

func stripPoCSearchWrappingQuotes(raw string) string {
	value := strings.TrimSpace(raw)
	if len(value) < 2 {
		return value
	}
	if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}

func parsePoCSearchClause(raw string) pocSearchClause {
	clause := strings.TrimSpace(raw)
	match := pocSearchFieldPattern.FindStringSubmatch(clause)
	if len(match) != 3 {
		return pocSearchClause{Value: strings.ToLower(stripPoCSearchWrappingQuotes(clause))}
	}
	return pocSearchClause{
		Field: strings.ToLower(strings.TrimSpace(match[1])),
		Value: strings.ToLower(stripPoCSearchWrappingQuotes(match[2])),
	}
}

func parsePoCSearchQuery(input string) [][]pocSearchClause {
	orClauses := splitPoCSearchClauses(input, "OR")
	if len(orClauses) == 0 {
		return nil
	}

	groups := make([][]pocSearchClause, 0, len(orClauses))
	for _, orClause := range orClauses {
		andClauses := splitPoCSearchClauses(orClause, "AND")
		group := make([]pocSearchClause, 0, len(andClauses))
		for _, andClause := range andClauses {
			parsed := parsePoCSearchClause(andClause)
			if parsed.Value == "" {
				continue
			}
			group = append(group, parsed)
		}
		if len(group) > 0 {
			groups = append(groups, group)
		}
	}
	return groups
}

func matchPoCSearchClause(item pocListItem, clause pocSearchClause) bool {
	if clause.Value == "" {
		return true
	}

	contains := func(value string) bool {
		return strings.Contains(strings.ToLower(value), clause.Value)
	}
	matchTag := func() bool {
		for _, tag := range item.Tags {
			if contains(tag) {
				return true
			}
		}
		return false
	}
	matchBool := func(value bool) bool {
		switch clause.Value {
		case "true", "1", "yes":
			return value
		case "false", "0", "no":
			return !value
		default:
			return false
		}
	}

	switch clause.Field {
	case "id":
		return contains(item.ID)
	case "name":
		return contains(item.Name)
	case "author":
		return contains(item.Author)
	case "tag", "tags":
		return matchTag()
	case "severity":
		return contains(item.Severity)
	case "method", "detectionmethod":
		return contains(item.DetectionMethod)
	case "principle-detection", "principledetection", "info.metadata.principle-detection":
		return matchBool(item.PrincipleDetection)
	case "cwe", "vulnerabilitycategory":
		return contains(item.VulnerabilityCategory)
	case "", "keyword":
		return contains(item.FileName) ||
			contains(item.ID) ||
			contains(item.Name) ||
			contains(item.Author) ||
			contains(item.Description) ||
			contains(item.RelatedFingerprint) ||
			contains(item.VulnerabilityCategory) ||
			contains(item.DetectionMethod) ||
			contains(item.SearchContent) ||
			matchTag()
	default:
		return false
	}
}

func matchPoCListItem(item pocListItem, keyword string) bool {
	groups := parsePoCSearchQuery(keyword)
	if len(groups) == 0 {
		return true
	}

	for _, group := range groups {
		matched := true
		for _, clause := range group {
			if !matchPoCSearchClause(item, clause) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func containsStringFold(items []string, target string) bool {
	target = strings.TrimSpace(strings.ToLower(target))
	if target == "" {
		return false
	}
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item)) == target {
			return true
		}
	}
	return false
}

// ListPoCsHandler 获取 PoC 列表
func ListPoCsHandler(c *gin.Context) {
	if err := webscan.RefreshPOCTemplateCache(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "刷新 PoC 模板仓库失败: " + err.Error(),
		})
		return
	}

	page := parsePositiveIntOrDefault(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveIntOrDefault(c.DefaultQuery("pageSize", "20"), 20)
	if pageSize > 200 {
		pageSize = 200
	}
	keyword := strings.TrimSpace(c.Query("keyword"))
	selectedVulnLabel := strings.TrimSpace(c.Query("vulnLabel"))

	templateMap := webscan.GetTemplateMapCopy()
	items := make([]pocListItem, 0, len(templateMap))
	for fileName, template := range templateMap {
		tags := parseTemplateTags(template.Info.Tags)
		dast := parseMetadataBool(template.Info.Metadata, "dast")
		principleDetection := parseMetadataBool(template.Info.Metadata, "principle-detection")
		detectionMethod := "exploit"
		if principleDetection {
			detectionMethod = "principle"
		}

		items = append(items, pocListItem{
			FileName:              fileName,
			ID:                    template.Id,
			Name:                  template.Info.Name,
			Author:                template.Info.Author,
			Severity:              strings.ToUpper(template.Info.Severity),
			Description:           template.Info.Description,
			Tags:                  tags,
			RelatedFingerprint:    resolveRelatedFingerprint(tags, template.Info.Metadata),
			DetectionMethod:       detectionMethod,
			DAST:                  dast,
			PrincipleDetection:    principleDetection,
			VulnerabilityCategory: normalizeCWE(template.Info.Classification.CweID),
			VulnLabels:            parseMetadataStringList(template.Info.Metadata, "vuln-labels"),
			CreatedTime:           template.CreatedTime(),
			UpdatedTime:           template.UpdatedTime(),
			ModifiedTime:          template.ModifiedTime,
			SearchContent:         template.RawContent,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].ModifiedTime == items[j].ModifiedTime {
			return items[i].FileName < items[j].FileName
		}
		return items[i].ModifiedTime > items[j].ModifiedTime
	})

	filtered := make([]pocListItem, 0, len(items))
	for _, item := range items {
		if matchPoCListItem(item, keyword) {
			filtered = append(filtered, item)
		}
	}

	labelCounter := make(map[string]int)
	for _, item := range items {
		for _, label := range item.VulnLabels {
			label = strings.TrimSpace(label)
			if label == "" {
				continue
			}
			labelCounter[label]++
		}
	}

	labelBuckets := make([]vulnLabelBucket, 0, len(labelCounter))
	for label, count := range labelCounter {
		labelBuckets = append(labelBuckets, vulnLabelBucket{
			Label: label,
			Count: count,
		})
	}
	sort.Slice(labelBuckets, func(i, j int) bool {
		if labelBuckets[i].Count == labelBuckets[j].Count {
			return labelBuckets[i].Label < labelBuckets[j].Label
		}
		return labelBuckets[i].Count > labelBuckets[j].Count
	})

	totalBeforeVulnLabel := len(items)

	if selectedVulnLabel != "" {
		groupFiltered := make([]pocListItem, 0, len(filtered))
		for _, item := range filtered {
			if containsStringFold(item.VulnLabels, selectedVulnLabel) {
				groupFiltered = append(groupFiltered, item)
			}
		}
		filtered = groupFiltered
	}

	total := len(filtered)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	pagedItems := filtered[start:end]

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":           pagedItems,
			"total":           total,
			"totalAll":        totalBeforeVulnLabel,
			"page":            page,
			"pageSize":        pageSize,
			"vulnLabelGroups": labelBuckets,
		},
	})
}

// GetPoCContentHandler 获取单个 PoC 内容
func GetPoCContentHandler(c *gin.Context) {
	if err := webscan.RefreshPOCTemplateCache(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "刷新 PoC 模板仓库失败: " + err.Error(),
		})
		return
	}

	pocID := c.Param("id")
	content, err := webscan.ReadPOCContent(pocID)
	if err != nil {
		switch {
		case os.IsNotExist(err):
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "PoC 文件不存在",
			})
		default:
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "读取 PoC 失败: " + err.Error(),
			})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"content": content,
		},
	})
}

// UpdatePoCContentHandler 保存 PoC 内容
func UpdatePoCContentHandler(c *gin.Context) {
	if err := webscan.RefreshPOCTemplateCache(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "刷新 PoC 模板仓库失败: " + err.Error(),
		})
		return
	}

	pocID := c.Param("id")

	var req savePoCRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数无效: " + err.Error(),
		})
		return
	}

	if err := webscan.SavePOCContent(pocID, req.Content); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "保存 PoC 失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id": pocID,
		},
	})
}

// DeletePoCHandler 删除 PoC 文件
func DeletePoCHandler(c *gin.Context) {
	if err := webscan.RefreshPOCTemplateCache(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "刷新 PoC 模板仓库失败: " + err.Error(),
		})
		return
	}

	pocID := c.Param("id")
	if err := webscan.DeletePOC(pocID); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "PoC 文件不存在",
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "删除 PoC 失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}
