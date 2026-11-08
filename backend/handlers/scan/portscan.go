package scan

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

func resolvePortScanTaskVersion(c *gin.Context, db *gorm.DB) (database.VulnScanTask, int, bool) {
	taskID := c.Param("id")
	versionStr := strings.TrimSpace(c.Query("version"))

	var task database.VulnScanTask
	if versionStr != "" {
		version, err := strconv.Atoi(versionStr)
		if err != nil || version <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "version 参数无效",
			})
			return task, 0, false
		}
		if err := db.Where("id = ? AND version = ?", taskID, version).First(&task).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "任务不存在",
			})
			return task, 0, false
		}
		return task, version, true
	}

	if err := db.Where("id = ?", taskID).Order("version DESC").First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "任务不存在",
		})
		return task, 0, false
	}
	return task, task.Version, true
}

// GetPortScanTaskResultsHandler 获取端口扫描结果
func GetPortScanTaskResultsHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	// 获取分页参数
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "20")

	page, _ := strconv.Atoi(pageStr)
	if page <= 0 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(pageSizeStr)
	if pageSize <= 0 || pageSize > 1000 {
		pageSize = 20
	}

	// 计算偏移量
	offset := (page - 1) * pageSize
	search := strings.TrimSpace(c.Query("search"))

	_, version, ok := resolvePortScanTaskVersion(c, db)
	if !ok {
		return
	}

	baseQuery := db.Model(&database.PortScanResult{}).Where("task_id = ? AND version = ?", taskID, version)
	if search != "" {
		like := "%" + search + "%"
		searchQuery := db.Where(
			"host LIKE ? OR transport LIKE ? OR protocol LIKE ? OR product LIKE ? OR service_version LIKE ? OR raw_response LIKE ?",
			like, like, like, like, like, like,
		)
		searchQuery = searchQuery.Or(
			"id IN (?)",
			db.Model(&database.PortScanResultAsset{}).
				Select("result_id").
				Where("task_id = ? AND version = ? AND (subdomain LIKE ? OR domain LIKE ? OR asset_type LIKE ? OR asset_name LIKE ?)",
					taskID, version, like, like, like, like),
		)
		if port, err := strconv.Atoi(search); err == nil {
			searchQuery = searchQuery.Or("port = ?", port)
		}
		baseQuery = baseQuery.Where(searchQuery)
	}

	// 查询总数
	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询结果总数失败: " + err.Error(),
		})
		return
	}

	type portStatItem struct {
		Label string `json:"label"`
		Count int64  `json:"count"`
	}

	type numericPortStatItem struct {
		Port  int   `json:"port"`
		Count int64 `json:"count"`
	}

	protocolTop5 := make([]portStatItem, 0)
	if err := baseQuery.Session(&gorm.Session{}).
		Select("COALESCE(NULLIF(protocol, ''), 'unknown') AS label, COUNT(*) AS count").
		Group("COALESCE(NULLIF(protocol, ''), 'unknown')").
		Order("count DESC").
		Limit(5).
		Scan(&protocolTop5).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询协议统计失败: " + err.Error(),
		})
		return
	}

	portTop5 := make([]numericPortStatItem, 0)
	if err := baseQuery.Session(&gorm.Session{}).
		Select("port, COUNT(*) AS count").
		Group("port").
		Order("count DESC").
		Limit(5).
		Scan(&portTop5).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询端口统计失败: " + err.Error(),
		})
		return
	}

	hostOpenPortTop5 := make([]portStatItem, 0)
	if err := baseQuery.Session(&gorm.Session{}).
		Select("host AS label, COUNT(*) AS count").
		Group("host").
		Order("count DESC").
		Limit(5).
		Scan(&hostOpenPortTop5).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询主机开放端口统计失败: " + err.Error(),
		})
		return
	}

	// 查询端口结果（分页）
	var portResults []database.PortScanResult
	if err := baseQuery.
		Offset(offset).
		Limit(pageSize).
		Order("created_at DESC").
		Find(&portResults).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询端口结果失败: " + err.Error(),
		})
		return
	}

	resultIDs := make([]uint, 0, len(portResults))
	for _, result := range portResults {
		resultIDs = append(resultIDs, result.ID)
	}
	assetsByResultID := make(map[uint][]database.PortScanResultAsset)
	if len(resultIDs) > 0 {
		var assets []database.PortScanResultAsset
		if err := db.Where("task_id = ? AND version = ? AND result_id IN ?", taskID, version, resultIDs).
			Order("subdomain ASC").
			Find(&assets).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "查询端口资产关系失败: " + err.Error(),
			})
			return
		}
		for _, asset := range assets {
			assetsByResultID[asset.ResultID] = append(assetsByResultID[asset.ResultID], asset)
		}
	}

	// 转换为前端需要的格式
	ports := make([]map[string]interface{}, 0, len(portResults))
	for _, result := range portResults {
		assetItems := make([]map[string]interface{}, 0, len(assetsByResultID[result.ID]))
		subdomains := make([]string, 0, len(assetsByResultID[result.ID]))
		for _, asset := range assetsByResultID[result.ID] {
			assetItems = append(assetItems, map[string]interface{}{
				"domain":       asset.Domain,
				"subdomain":    asset.Subdomain,
				"assetType":    asset.AssetType,
				"assetName":    asset.AssetName,
				"sourceModule": asset.SourceModule,
			})
			if strings.TrimSpace(asset.Subdomain) != "" {
				subdomains = append(subdomains, asset.Subdomain)
			}
		}
		portData := map[string]interface{}{
			"host":            result.Host,
			"port":            result.Port,
			"transport":       result.Transport,
			"protocol":        result.Protocol,
			"discoveryStatus": result.DiscoveryStatus,
			"product":         result.Product,
			"version":         result.ServiceVersion,
			"rawResponse":     result.RawResponse,
			"banner":          result.RawResponse,
			"assets":          assetItems,
			"subdomains":      subdomains,
			"createdAt":       result.CreatedAt.Format(time.RFC3339),
		}
		ports = append(ports, portData)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"ports": ports,
			"pagination": gin.H{
				"page":     page,
				"pageSize": pageSize,
				"total":    total,
			},
			"stats": gin.H{
				"total":            total,
				"protocolTop5":     protocolTop5,
				"portTop5":         portTop5,
				"hostOpenPortTop5": hostOpenPortTop5,
			},
		},
	})
}

// GetPortScanTaskResultsAsURLsHandler 获取端口扫描结果并转换为目标格式
// mode参数:
// - "web": 返回URL格式（用于网站/目录/JS 扫描）
// - "brute": 返回协议://IP:端口格式（用于密码暴破）
func GetPortScanTaskResultsAsURLsHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	mode := c.DefaultQuery("mode", "web") // 默认为web模式

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	_, version, ok := resolvePortScanTaskVersion(c, db)
	if !ok {
		return
	}

	// 查询端口结果
	var portResults []database.PortScanResult
	if err := db.Where("task_id = ? AND version = ?", taskID, version).Find(&portResults).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询端口结果失败: " + err.Error(),
		})
		return
	}

	var targets []string
	targetSet := make(map[string]bool) // 用于去重

	// 支持暴破的协议列表
	bruteForceProtocols := map[string]bool{
		"ssh":        true,
		"rdp":        true,
		"ftp":        true,
		"telnet":     true,
		"smb":        true,
		"mysql":      true,
		"mssql":      true,
		"dameng":     true,
		"postgresql": true,
		"mongodb":    true,
		"redis":      true,
		"oracle":     true,
		"pop3":       true,
		"smtp":       true,
		"ldap":       true,
		"vnc":        true,
		"mqtt":       true,
		"rabbitmq":   true,
		"memcached":  true,
		"jdwp":       true,
		"adb":        true,
		"java-rmi":   true,
		"activemq":   true,
		"rsync":      true,
		"kafka":      true,
		"clickhouse": true,
		"zookeeper":  true,
	}

	for _, result := range portResults {
		protocol := result.Protocol
		host := result.Host
		port := result.Port

		if mode == "web" {
			// 网站扫描模式：只返回http/https协议的URL
			var scheme string
			switch protocol {
			case "http", "https":
				scheme = protocol
			case "ssl", "tls":
				scheme = "https"
			default:
				// 根据常见端口判断协议
				switch port {
				case 80:
					scheme = "http"
				case 443, 8443:
					scheme = "https"
				case 8080, 8000, 8888, 9000:
					// 这些端口可能是http或https，默认使用http
					scheme = "http"
				default:
					// 其他端口跳过
					continue
				}
			}

			// 生成URL
			var url string
			if (scheme == "http" && port == 80) || (scheme == "https" && port == 443) {
				url = fmt.Sprintf("%s://%s", scheme, host)
			} else {
				url = fmt.Sprintf("%s://%s:%d", scheme, host, port)
			}

			// 去重
			if !targetSet[url] {
				targetSet[url] = true
				targets = append(targets, url)
			}
		} else if mode == "brute" {
			// 密码暴破模式：返回支持暴破的协议
			protocolLower := strings.ToLower(protocol)

			// 检查协议是否支持暴破
			if !bruteForceProtocols[protocolLower] {
				// 如果协议不在列表中，尝试根据端口判断
				switch port {
				case 22:
					protocolLower = "ssh"
				case 3389:
					protocolLower = "rdp"
				case 21:
					protocolLower = "ftp"
				case 23:
					protocolLower = "telnet"
				case 445:
					protocolLower = "smb"
				case 3306:
					protocolLower = "mysql"
				case 110, 995:
					protocolLower = "pop3"
				case 25, 465, 587:
					protocolLower = "smtp"
				case 1433:
					protocolLower = "mssql"
				case 5236:
					protocolLower = "dameng"
				case 5432:
					protocolLower = "postgresql"
				case 27017:
					protocolLower = "mongodb"
				case 6379:
					protocolLower = "redis"
				case 1521:
					protocolLower = "oracle"
				case 389, 636:
					protocolLower = "ldap"
				case 5900, 5901:
					protocolLower = "vnc"
				case 1883:
					protocolLower = "mqtt"
				case 5672, 15672:
					protocolLower = "rabbitmq"
				case 11211:
					protocolLower = "memcached"
				case 5005:
					protocolLower = "jdwp"
				case 5555:
					protocolLower = "adb"
				case 1099:
					protocolLower = "java-rmi"
				case 61616:
					protocolLower = "activemq"
				case 873:
					protocolLower = "rsync"
				case 9092:
					protocolLower = "kafka"
				case 8123:
					protocolLower = "clickhouse"
				case 2181:
					protocolLower = "zookeeper"
				default:
					// 不支持的端口，跳过
					continue
				}
			}

			if protocolLower == "dm" {
				protocolLower = "dameng"
			}

			// 生成协议://IP:端口格式
			target := fmt.Sprintf("%s://%s:%d", protocolLower, host, port)

			// 去重
			if !targetSet[target] {
				targetSet[target] = true
				targets = append(targets, target)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"targets": targets,
			"count":   len(targets),
		},
	})
}

// ExportPortScanTaskResultsHandler 导出端口扫描结果为Excel文件
func ExportPortScanTaskResultsHandler(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "任务ID不能为空",
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	task, version, ok := resolvePortScanTaskVersion(c, db)
	if !ok {
		return
	}

	// 查询端口结果
	var portResults []database.PortScanResult
	if err := db.Where("task_id = ? AND version = ?", taskID, version).Find(&portResults).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询端口结果失败: " + err.Error(),
		})
		return
	}

	// 创建Excel文件
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("关闭Excel文件失败: %v\n", err)
		}
	}()

	sheetName := "端口扫描结果"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "创建工作表失败: " + err.Error(),
		})
		return
	}

	// 设置表头样式
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold:   true,
			Size:   12,
			Family: "Arial",
		},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"#4472C4"},
			Pattern: 1,
		},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "创建表头样式失败: " + err.Error(),
		})
		return
	}

	// 设置表头
	headers := []string{"主机", "端口", "协议", "产品", "版本", "响应内容", "发现时间"}
	for i, header := range headers {
		cell := fmt.Sprintf("%s1", string(rune('A'+i)))
		f.SetCellValue(sheetName, cell, header)
		f.SetCellStyle(sheetName, cell, cell, headerStyle)
	}

	// 设置列宽
	f.SetColWidth(sheetName, "A", "A", 20) // 主机
	f.SetColWidth(sheetName, "B", "B", 10) // 端口
	f.SetColWidth(sheetName, "C", "C", 15) // 协议
	f.SetColWidth(sheetName, "D", "D", 24) // 产品
	f.SetColWidth(sheetName, "E", "E", 18) // 版本
	f.SetColWidth(sheetName, "F", "F", 50) // 响应内容
	f.SetColWidth(sheetName, "G", "G", 20) // 发现时间

	// 填充数据
	for i, result := range portResults {
		row := i + 2
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), result.Host)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), result.Port)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), result.Protocol)
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), result.Product)
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), result.ServiceVersion)
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), result.RawResponse)
		f.SetCellValue(sheetName, fmt.Sprintf("G%d", row), result.CreatedAt.Format("2006-01-02 15:04:05"))
	}

	// 设置活动工作表
	f.SetActiveSheet(index)
	// 删除默认的Sheet1
	f.DeleteSheet("Sheet1")

	// 生成文件名
	fileName := fmt.Sprintf("端口扫描结果_%s_%s.xlsx", task.TaskName, time.Now().Format("20060102_150405"))

	// 设置响应头
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", fileName))
	c.Header("Content-Transfer-Encoding", "binary")

	// 写入响应
	if err := f.Write(c.Writer); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "写入Excel文件失败: " + err.Error(),
		})
		return
	}
}
