package scan

import (
	"encoding/json"
	"errors"
	"fmt"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/jsscan"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetJSScanAssetsHandler 获取JS扫描资产列表
func GetJSScanAssetsHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	taskID := c.Param("id")
	assetType := strings.TrimSpace(c.Query("type")) // 可选过滤资产类型
	keyword := strings.TrimSpace(c.Query("keyword"))
	versionParam := c.Query("version") // 版本参数
	pageQuery := c.Query("page")
	pageSizeQuery := c.Query("pageSize")
	usePagination := pageQuery != "" || pageSizeQuery != ""
	page := parseQueryInt(pageQuery, 1)
	pageSize := parseQueryInt(pageSizeQuery, 20)
	if pageSize > 200 {
		pageSize = 200
	}

	if _, ok := getVulnScanTaskOrAbort(c, db, taskID); !ok {
		return
	}

	buildAssetsQuery := func() *gorm.DB {
		query := db.Model(&database.VulnScanJSAsset{}).Where("task_id = ?", taskID)
		if keyword != "" {
			like := "%" + keyword + "%"
			query = query.Where("value LIKE ? OR source LIKE ?", like, like)
		}
		if versionParam != "" {
			query = query.Where("version = ?", versionParam)
		}
		return query
	}

	type assetTypeCountRow struct {
		Type  string `json:"type"`
		Count int64  `json:"count"`
	}

	typeCounts := make(map[string]int64)
	var aggregatedTotal int64
	if err := buildAssetsQuery().Count(&aggregatedTotal).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "统计资产总数失败: " + err.Error(),
		})
		return
	}
	var countRows []assetTypeCountRow
	if err := buildAssetsQuery().Select("type, COUNT(*) AS count").Group("type").Scan(&countRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "统计资产类型数量失败: " + err.Error(),
		})
		return
	}
	for _, row := range countRows {
		normalizedType := normalizeJSAssetType(row.Type)
		if normalizedType == "" {
			continue
		}
		typeCounts[normalizedType] += row.Count
	}

	query := buildAssetsQuery()
	if assetType != "" && assetType != "all" {
		filterTypes := expandJSAssetFilterTypes(assetType)
		if len(filterTypes) > 0 {
			query = query.Where("type IN ?", filterTypes)
		}
	}

	var total int64
	if usePagination {
		if err := query.Count(&total).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "统计资产列表失败: " + err.Error(),
			})
			return
		}
	}

	var assets []database.VulnScanJSAsset
	query = query.Select("id", "task_id", "version", "type", "value", "source", "created_at")
	if usePagination {
		offset := (page - 1) * pageSize
		if err := query.Order("created_at DESC").Order("id DESC").Offset(offset).Limit(pageSize).Find(&assets).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "获取资产列表失败: " + err.Error(),
			})
			return
		}
		for i := range assets {
			assets[i].Type = normalizeJSAssetType(assets[i].Type)
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"assets":     assets,
				"total":      total,
				"totalAll":   aggregatedTotal,
				"typeCounts": typeCounts,
				"page":       page,
				"pageSize":   pageSize,
			},
		})
		return
	}

	if err := query.Order("created_at DESC").Order("id DESC").Find(&assets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取资产列表失败: " + err.Error(),
		})
		return
	}
	for i := range assets {
		assets[i].Type = normalizeJSAssetType(assets[i].Type)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"assets":     assets,
			"total":      len(assets),
			"totalAll":   aggregatedTotal,
			"typeCounts": typeCounts,
		},
	})
}

// GetJSScanSiteTreeHandler 获取JS扫描网站树
func GetJSScanSiteTreeHandler(c *gin.Context) {
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
	keyword := strings.TrimSpace(c.Query("keyword"))
	includeTree, includeTreeSet := parseQueryBool(c.Query("includeTree"))
	sortBy := strings.TrimSpace(c.DefaultQuery("sortBy", "createdAt"))
	sortOrder := normalizeSortOrder(c.Query("sortOrder"))
	pageQuery := c.Query("page")
	pageSizeQuery := c.Query("pageSize")
	usePagination := pageQuery != "" || pageSizeQuery != ""
	page := parseQueryInt(pageQuery, 1)
	pageSize := parseQueryInt(pageSizeQuery, 20)
	if pageSize > 200 {
		pageSize = 200
	}
	if !includeTreeSet {
		includeTree = !usePagination
	}

	// 解析JSON数据，返回包含target和siteTree的结构
	type SiteTreeItem struct {
		ID        string      `json:"id"`
		TargetURL string      `json:"targetUrl"`
		TreeData  interface{} `json:"treeData"`
		CreatedAt interface{} `json:"createdAt,omitempty"`
	}
	type SiteTreeSummaryItem struct {
		ID            string      `json:"id"`
		TargetURL     string      `json:"targetUrl"`
		CreatedAt     interface{} `json:"createdAt,omitempty"`
		APIRouteCount int         `json:"apiRouteCount"`
	}

	if _, ok := getVulnScanTaskOrAbort(c, db, taskID); !ok {
		return
	}

	query := db.Model(&database.VulnScanJSSiteTree{}).Where("task_id = ?", taskID)
	if versionParam != "" {
		query = query.Where("version = ?", versionParam)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("target_url LIKE ?", like)
	}

	if !includeTree {
		var siteTrees []database.VulnScanJSSiteTree
		summaryQuery := query.Select("id", "target_url", "tree_data", "created_at")
		if sortBy != "apiRoutes" {
			summaryQuery = summaryQuery.Order("created_at " + sortOrder)
			if usePagination {
				offset := (page - 1) * pageSize
				summaryQuery = summaryQuery.Offset(offset).Limit(pageSize)
			}
		}

		if err := summaryQuery.Find(&siteTrees).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "获取网站树列表失败: " + err.Error(),
			})
			return
		}

		result := make([]SiteTreeSummaryItem, 0, len(siteTrees))
		for _, tree := range siteTrees {
			result = append(result, SiteTreeSummaryItem{
				ID:            tree.ID,
				TargetURL:     tree.TargetURL,
				CreatedAt:     tree.CreatedAt,
				APIRouteCount: extractSiteTreeRouteCount(tree.TreeData),
			})
		}

		total := int64(len(result))
		if sortBy == "apiRoutes" {
			sort.SliceStable(result, func(i, j int) bool {
				if result[i].APIRouteCount != result[j].APIRouteCount {
					if sortOrder == "ASC" {
						return result[i].APIRouteCount < result[j].APIRouteCount
					}
					return result[i].APIRouteCount > result[j].APIRouteCount
				}

				leftTime := fmt.Sprint(result[i].CreatedAt)
				rightTime := fmt.Sprint(result[j].CreatedAt)
				if leftTime != rightTime {
					if sortOrder == "ASC" {
						return leftTime < rightTime
					}
					return leftTime > rightTime
				}

				if sortOrder == "ASC" {
					return result[i].TargetURL < result[j].TargetURL
				}
				return result[i].TargetURL > result[j].TargetURL
			})

			if usePagination {
				start := (page - 1) * pageSize
				if start < 0 {
					start = 0
				}
				if start > len(result) {
					start = len(result)
				}
				end := start + pageSize
				if end > len(result) {
					end = len(result)
				}
				result = result[start:end]
			}
		} else if usePagination {
			var counted int64
			if err := query.Count(&counted).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"error":   "统计网站树失败: " + err.Error(),
				})
				return
			}
			total = counted
		}

		if usePagination {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"siteTrees": result,
					"total":     total,
					"page":      page,
					"pageSize":  pageSize,
				},
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    result,
		})
		return
	}

	query = query.Order("created_at DESC")
	var total int64
	if usePagination {
		if err := query.Count(&total).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "统计网站树失败: " + err.Error(),
			})
			return
		}
	}
	if usePagination {
		offset := (page - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	var siteTrees []database.VulnScanJSSiteTree
	if err := query.Find(&siteTrees).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取网站树失败: " + err.Error(),
		})
		return
	}

	var result []SiteTreeItem
	for _, tree := range siteTrees {
		var treeData interface{}
		if err := json.Unmarshal([]byte(tree.TreeData), &treeData); err == nil {
			result = append(result, SiteTreeItem{
				ID:        tree.ID,
				TargetURL: tree.TargetURL,
				TreeData:  treeData,
				CreatedAt: tree.CreatedAt,
			})
		}
	}

	if usePagination {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"siteTrees": result,
				"total":     total,
				"page":      page,
				"pageSize":  pageSize,
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetJSScanSiteTreeDetailHandler 获取单个目标的网站树详情
func GetJSScanSiteTreeDetailHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	taskID := c.Param("id")
	treeID := c.Param("treeId")
	versionParam := c.Query("version")

	if _, ok := getVulnScanTaskOrAbort(c, db, taskID); !ok {
		return
	}

	query := db.Where("task_id = ? AND id = ?", taskID, treeID)
	if versionParam != "" {
		query = query.Where("version = ?", versionParam)
	}

	var siteTree database.VulnScanJSSiteTree
	if err := query.First(&siteTree).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "未找到网站树详情",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取网站树详情失败: " + err.Error(),
		})
		return
	}

	var treeData interface{}
	if err := json.Unmarshal([]byte(siteTree.TreeData), &treeData); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "解析网站树数据失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":        siteTree.ID,
			"targetUrl": siteTree.TargetURL,
			"treeData":  treeData,
			"createdAt": siteTree.CreatedAt,
		},
	})
}

// GetJSScanDefaultConfigHandler 获取JS扫描默认配置
func GetJSScanDefaultConfigHandler(c *gin.Context) {
	config := jsscan.GetDefaultConfig()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config,
	})
}
