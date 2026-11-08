package asset

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// maxLedgerExportRows keeps a single export bounded so a large ledger cannot
// exhaust request memory while the complete entity set is built.
const maxLedgerExportRows = 50000

var ledgerCSVHeadersZH = []string{
	"类型", "资产值", "名称", "根域名", "主机", "关联 IP", "端口", "传输", "协议", "服务",
	"标题", "状态码", "指纹", "高危指纹", "子域名数", "IP 数", "端口数", "网站数",
	"来源", "状态", "验证状态", "是否归档", "首次发现", "最近发现",
}

var ledgerCSVHeadersEN = []string{
	"Category", "Value", "Name", "Root Domain", "Host", "IPs", "Port", "Transport", "Protocol", "Service",
	"Title", "Status Code", "Fingerprints", "High Risk Fingerprints", "Subdomains", "IP Count", "Port Count", "Website Count",
	"Sources", "Status", "Verification", "Archived", "First Seen", "Last Seen",
}

// ledgerEntitySelection identifies a ledger entity picked in the UI. AssetID is
// the standalone unified asset behind the row; aggregated rows such as a root
// domain built only from subdomains have no standalone record.
type ledgerEntitySelection struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	AssetID  string `json:"assetId"`
	Value    string `json:"value"`
}

func ledgerCSVHeaders(lang string) []string {
	if strings.EqualFold(strings.TrimSpace(lang), "en") {
		return ledgerCSVHeadersEN
	}
	return ledgerCSVHeadersZH
}

func ledgerExportTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func ledgerExportLabel(value string) string {
	return strings.TrimSpace(value)
}

func ledgerExportCount(value int) string {
	if value <= 0 {
		return ""
	}
	return strconv.Itoa(value)
}

// buildLedgerEntityCSV renders the ledger rows as an Excel-friendly UTF-8 CSV.
// Headers follow the UI language; values keep their raw keys so the file stays
// machine-readable.
func buildLedgerEntityCSV(entities []LedgerEntity, lang string) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(&buffer)
	if err := writer.Write(ledgerCSVHeaders(lang)); err != nil {
		return nil, err
	}
	for _, entity := range entities {
		record := []string{
			entity.Category,
			ledgerExportLabel(entity.Value),
			ledgerExportLabel(entity.Name),
			ledgerExportLabel(entity.RootDomain),
			ledgerExportLabel(entity.Host),
			strings.Join(entity.IPs, ";"),
			ledgerExportCount(entity.Port),
			ledgerExportLabel(entity.Transport),
			ledgerExportLabel(entity.Protocol),
			ledgerExportLabel(entity.Service),
			ledgerExportLabel(entity.Title),
			ledgerExportCount(entity.HTTPStatus),
			strings.Join(entity.Fingerprints, ";"),
			strings.Join(entity.HighRiskFingerprints, ";"),
			strconv.Itoa(entity.SubdomainCount),
			strconv.Itoa(entity.IPCount),
			strconv.Itoa(entity.PortCount),
			strconv.Itoa(entity.WebsiteCount),
			strings.Join(entity.Sources, ";"),
			ledgerExportLabel(entity.Status),
			ledgerExportLabel(entity.Verification),
			strconv.FormatBool(entity.Archived),
			ledgerExportTime(entity.FirstSeenAt),
			ledgerExportTime(entity.LastSeenAt),
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func filterLedgerEntities(entities []LedgerEntity, filter assetstore.LedgerFilter) []LedgerEntity {
	filtered := make([]LedgerEntity, 0, len(entities))
	for _, entity := range entities {
		if ledgerEntityMatches(entity, filter) {
			filtered = append(filtered, entity)
		}
	}
	return filtered
}

// selectLedgerEntities resolves the rows selected in the UI against a freshly
// built entity set, keeping the client order and dropping unknown rows.
func selectLedgerEntities(set ledgerEntitySet, items []ledgerEntitySelection) []LedgerEntity {
	index := make(map[string]LedgerEntity, len(set.roots)+len(set.ips)+len(set.subdomains)+len(set.ports)+len(set.websites))
	for _, category := range []LedgerEntityCategory{LedgerEntityRoot, LedgerEntityIP, LedgerEntitySubdomain, LedgerEntityPort, LedgerEntityWebsite} {
		for _, entity := range set.category(category) {
			index[entity.Category+"\x00"+entity.ID] = entity
		}
	}
	selected := make([]LedgerEntity, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(item.Category)) + "\x00" + id
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		entity, ok := index[key]
		if !ok {
			continue
		}
		selected = append(selected, entity)
	}
	return selected
}

func writeLedgerEntityCSV(c *gin.Context, fileLabel string, entities []LedgerEntity, lang string) {
	if len(entities) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "没有可导出的资产"})
		return
	}
	if len(entities) > maxLedgerExportRows {
		entities = entities[:maxLedgerExportRows]
	}
	content, err := buildLedgerEntityCSV(entities, lang)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "生成导出文件失败: " + err.Error()})
		return
	}
	filename := fmt.Sprintf("asset-ledger-%s-%s.csv", fileLabel, time.Now().Format("20060102-150405"))
	quoted := strings.ReplaceAll(filename, `"`, "_")
	escaped := url.PathEscape(filename)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, quoted, escaped))
	c.Header("X-Total-Count", strconv.Itoa(len(entities)))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", content)
}

func ledgerExportDatabase(c *gin.Context) (*gorm.DB, bool) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return nil, false
	}
	return db.WithContext(c.Request.Context()), true
}

// ledgerExportCategories resolves the requested export scope. "all" keeps the
// same order as the asset center tabs so one file covers the whole ledger.
func ledgerExportCategories(value string) ([]LedgerEntityCategory, string, bool) {
	scope := strings.ToLower(strings.TrimSpace(value))
	if scope == "" {
		scope = "root"
	}
	if scope == "all" || scope == "*" {
		return []LedgerEntityCategory{
			LedgerEntityRoot, LedgerEntityIP, LedgerEntitySubdomain, LedgerEntityPort, LedgerEntityWebsite,
		}, "all", true
	}
	category, ok := normalizeLedgerEntityCategory(scope)
	if !ok {
		return nil, "", false
	}
	return []LedgerEntityCategory{category}, string(category), true
}

// ExportAssetLedgerEntitiesHandler exports every row that matches the active
// filters, not just the current page. category=all exports the whole asset
// center in one file.
func ExportAssetLedgerEntitiesHandler(c *gin.Context) {
	categories, fileLabel, ok := ledgerExportCategories(c.DefaultQuery("category", "root"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "category must be root, ip, subdomain, port, website or all"})
		return
	}
	db, ok := ledgerExportDatabase(c)
	if !ok {
		return
	}
	includeArchived := c.Query("includeArchived") == "true" || c.Query("includeArchived") == "1"
	set, err := buildLedgerEntitySet(db, includeArchived)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	filter := assetstore.LedgerFilter{
		Status: c.Query("status"), Verification: c.Query("verification"), Source: c.Query("source"), Keyword: c.Query("keyword"),
	}
	entities := make([]LedgerEntity, 0)
	for _, category := range categories {
		entities = append(entities, filterLedgerEntities(set.category(category), filter)...)
	}
	writeLedgerEntityCSV(c, fileLabel, entities, c.DefaultQuery("lang", "zh"))
}

type ledgerExportSelectionRequest struct {
	Lang  string                  `json:"lang"`
	Items []ledgerEntitySelection `json:"items"`
}

// ExportSelectedAssetLedgerEntitiesHandler exports an explicit selection that
// may span categories and archived rows.
func ExportSelectedAssetLedgerEntitiesHandler(c *gin.Context) {
	var request ledgerExportSelectionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if len(request.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "请先选择要导出的资产"})
		return
	}
	db, ok := ledgerExportDatabase(c)
	if !ok {
		return
	}
	set, err := buildLedgerEntitySet(db, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	writeLedgerEntityCSV(c, "selection", selectLedgerEntities(set, request.Items), request.Lang)
}
