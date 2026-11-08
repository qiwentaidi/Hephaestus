package asset

import (
	"net"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/findings"

	"github.com/gin-gonic/gin"
)

// ListAssetTopologyEntriesHandler returns only the selectable topology entry
// assets. Endpoint and technology assets remain derived children in the graph.
func ListAssetTopologyEntriesHandler(c *gin.Context) {
	items, err := collectAssetCenterAggregates()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": "聚合资产数据失败: " + err.Error()})
		return
	}

	assetType := strings.TrimSpace(c.DefaultQuery("type", "all"))
	if assetType != "all" && assetType != "root" && assetType != "domain" && assetType != "ip" {
		c.JSON(400, gin.H{"success": false, "error": "type 必须为 all、root、domain 或 ip"})
		return
	}
	keyword := strings.ToLower(strings.TrimSpace(c.Query("keyword")))
	filtered := make([]AssetCenterItem, 0)
	for _, item := range buildAssetTopologyEntries(items) {
		if assetType != "all" && item.Type != assetType {
			continue
		}
		if !assetCenterMatchesKeyword(item, keyword) {
			continue
		}
		filtered = append(filtered, item)
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	start := (page - 1) * pageSize
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}

	c.JSON(200, gin.H{"success": true, "data": gin.H{
		"items": filtered[start:end], "total": len(filtered), "page": page, "pageSize": pageSize,
	}})
}

// GetAssetMapOverviewHandler returns province-level IP asset distribution data
// for the asset management map. It uses the same task-independent asset
// projection as the topology view.
func GetAssetMapOverviewHandler(c *gin.Context) {
	items, err := collectAssetCenterAggregates()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": "聚合资产地图数据失败: " + err.Error()})
		return
	}
	since := assetMapWindowSince(c.Query("window"), time.Now())
	var riskIndex *findingRiskIndex
	data := buildAssetMapOverviewWithRisk(items, since, nil)
	if db, dbErr := database.GetDB(); dbErr == nil {
		riskIndex = loadFindingRiskIndexCached(db)
		items = appendSyntheticRiskIPs(items, riskIndex)
		data = buildAssetMapOverviewWithRisk(items, since, riskIndex)
		if summary, summaryErr := findings.SummarizeRisk(db); summaryErr == nil {
			data.RiskAssets = mapRiskAssetSummary(summary)
		}
	}
	c.JSON(200, gin.H{"success": true, "data": data})
}

// ListAssetFindingsHandler returns the normalised findings attributed to one
// asset. Direct findings are observed on the asset; inherited findings reach it
// through a resolved domain or certificate.
func ListAssetFindingsHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": "读取漏洞数据失败: " + err.Error()})
		return
	}
	views, err := findings.FindingsForAsset(db, c.Query("assetType"), c.Query("value"))
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": "查询资产漏洞失败: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": views})
}

// ListAssetMapAssetsHandler returns the paginated IP assets behind one map
// selection (province, city, scope or risk).
func ListAssetMapAssetsHandler(c *gin.Context) {
	items, err := collectAssetCenterAggregates()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": "聚合资产地图数据失败: " + err.Error()})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	query := assetMapAssetQuery{
		Scope:    c.DefaultQuery("scope", "located"),
		Province: strings.TrimSpace(c.Query("province")),
		City:     strings.TrimSpace(c.Query("city")),
		Country:  strings.TrimSpace(c.Query("country")),
		Risk:     strings.TrimSpace(c.Query("risk")),
		Keyword:  strings.TrimSpace(c.Query("keyword")),
		Since:    assetMapWindowSince(c.Query("window"), time.Now()),
		Page:     page,
		PageSize: pageSize,
	}
	var riskIndex *findingRiskIndex
	if db, dbErr := database.GetDB(); dbErr == nil {
		riskIndex = loadFindingRiskIndexCached(db)
	}
	items = appendSyntheticRiskIPs(items, riskIndex)
	c.JSON(200, gin.H{"success": true, "data": buildAssetMapAssetsWithRisk(items, query, riskIndex)})
}

// GetAssetTopologyHandler returns a read-only local topology. Only domain and
// root-domain, domain and IP assets are valid entry points; endpoints are
// always derived children.
func GetAssetTopologyHandler(c *gin.Context) {
	rootType := strings.TrimSpace(c.Query("type"))
	rootValue := strings.TrimSpace(c.Query("value"))
	if rootType != "root" && rootType != "domain" && rootType != "ip" {
		c.JSON(400, gin.H{"success": false, "error": "type 必须为 root、domain 或 ip"})
		return
	}
	if rootType == "root" {
		rootValue = assetTopologyRootDomain(rootValue)
	} else if rootType == "domain" {
		rootValue = normalizeAssetHost(rootValue)
	} else if parsed := net.ParseIP(rootValue); parsed != nil {
		rootValue = parsed.String()
	}
	if rootValue == "" {
		c.JSON(400, gin.H{"success": false, "error": "缺少资产值"})
		return
	}

	items, err := collectAssetCenterAggregates()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": "聚合资产数据失败: " + err.Error()})
		return
	}
	topology, ok := buildAssetTopology(items, rootType, rootValue)
	if !ok {
		c.JSON(404, gin.H{"success": false, "error": "未找到对应资产"})
		return
	}

	c.JSON(200, gin.H{"success": true, "data": topology})
}
