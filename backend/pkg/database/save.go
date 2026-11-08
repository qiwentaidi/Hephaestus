package database

import (
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/notification"
	"hephaestus-backend/pkg/types"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	"gorm.io/gorm"
)

func buildVulnerabilityIdentityQuery(db *gorm.DB, vulnerability *VulnerabilityInfo) *gorm.DB {
	return db.Where(
		"task_id = ? AND version = ? AND template_id = ? AND vuln_url = ? AND vuln_name = ? AND protocol = ?",
		vulnerability.TaskID,
		vulnerability.Version,
		vulnerability.TemplateID,
		vulnerability.VulnURL,
		vulnerability.VulnName,
		vulnerability.Protocol,
	)
}

// convertToUTF8 将字符串转换为 UTF-8 编码
// 如果字符串不是有效的 UTF-8，尝试从 GBK/GB2312 转换
func convertToUTF8(s string) string {
	// 先移除 NULL 字节（0x00），PostgreSQL 不接受文本字段中的 NULL 字节
	s = strings.ReplaceAll(s, "\x00", "")

	// 如果已经是有效的 UTF-8，直接返回
	if utf8.ValidString(s) {
		return s
	}

	// 尝试从 GBK 转换
	decoder := simplifiedchinese.GBK.NewDecoder()
	result, _, err := transform.String(decoder, s)
	if err == nil && utf8.ValidString(result) {
		// 再次确保没有 NULL 字节
		return strings.ReplaceAll(result, "\x00", "")
	}

	// 尝试从 GB18030 转换（GB18030 是 GBK 的超集）
	decoder18030 := simplifiedchinese.GB18030.NewDecoder()
	result, _, err = transform.String(decoder18030, s)
	if err == nil && utf8.ValidString(result) {
		// 再次确保没有 NULL 字节
		return strings.ReplaceAll(result, "\x00", "")
	}

	// 如果转换失败，清理无效的 UTF-8 字符
	result = strings.ToValidUTF8(s, "")
	// 最后再次移除 NULL 字节
	return strings.ReplaceAll(result, "\x00", "")
}

// NormalizeText returns text that is safe to persist into UTF-8 database columns.
func NormalizeText(s string) string {
	return convertToUTF8(s)
}

// SaveVulnerabilityResult 保存漏洞结果到数据库
func SaveVulnerabilityResult(db *gorm.DB, taskID string, version int, taskName string, vuln types.VulnerabilityInfo) {
	vulnerability := VulnerabilityInfo{
		TaskID:           taskID,
		Version:          version,
		TemplateID:       vuln.ID,
		VulnName:         convertToUTF8(vuln.Name),
		Protocol:         vuln.Type,
		Severity:         vuln.Severity,
		DisposalStatus:   "open",
		VulnURL:          vuln.URL,
		Extract:          convertToUTF8(vuln.Extract),
		Request:          convertToUTF8(vuln.Request),
		Response:         convertToUTF8(vuln.Response),
		Description:      convertToUTF8(vuln.Description),
		Impact:           convertToUTF8(vuln.Impact),
		Remediation:      convertToUTF8(vuln.Remediation),
		Reference:        convertToUTF8(vuln.Reference),
		ResponseTime:     vuln.ResponseTime,
		PrincipDetection: vuln.PrincipleDetection,
	}

	created := false
	var existing VulnerabilityInfo
	err := buildVulnerabilityIdentityQuery(db.Model(&VulnerabilityInfo{}), &vulnerability).First(&existing).Error
	switch err {
	case nil:
		if err := db.Model(&VulnerabilityInfo{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
			"severity":           vulnerability.Severity,
			"extract":            vulnerability.Extract,
			"request":            vulnerability.Request,
			"response":           vulnerability.Response,
			"description":        vulnerability.Description,
			"impact":             vulnerability.Impact,
			"remediation":        vulnerability.Remediation,
			"reference":          vulnerability.Reference,
			"response_time":      vulnerability.ResponseTime,
			"principl_detection": vulnerability.PrincipDetection,
		}).Error; err != nil {
			logger.Error("更新漏洞结果失败: %v", err)
		}
	case gorm.ErrRecordNotFound:
		if err := db.Create(&vulnerability).Error; err != nil {
			logger.Error("保存漏洞结果失败: %v", err)
			return
		}
		created = true
	default:
		logger.Error("查询漏洞结果失败: %v", err)
		return
	}

	if !created {
		return
	}

	// 发送钉钉通知（异步，不阻塞）
	// 优化：直接使用传入的 taskName，避免每次都查询数据库
	go func() {
		notification.NotifyVulnerabilityFound(
			taskID,
			taskName,
			vuln.Name,
			vuln.Severity,
			vuln.URL,
		)
	}()
}

// SavePortResult 保存端口扫描结果到 VulnScanTask 关联的端口表
func SavePortResult(db *gorm.DB, taskID string, version int, result *types.PortInfo) *PortScanResult {
	status := "verified"
	if result.DiscoveryStatus == "pending" {
		status = "pending"
	}
	// 使用 FirstOrCreate 避免重复入库
	var existingResult PortScanResult
	err := db.Where("task_id = ? AND version = ? AND host = ? AND port = ? AND transport = ?",
		taskID, version, result.Host, result.Port, result.Transport).
		First(&existingResult).Error
	cleanedResponse := convertToUTF8(result.RawResponse)
	switch err {
	case nil:
		if status == "pending" {
			return &existingResult
		}
		// 记录已存在，更新
		if err := db.Model(&existingResult).Updates(map[string]interface{}{
			"discovery_status": "verified",
			"transport":        result.Transport,
			"protocol":         result.Protocol,
			"product":          result.Product,
			"service_version":  result.Version,
			"raw_response":     cleanedResponse,
		}).Error; err != nil {
			logger.Error("更新端口结果失败: %v", err)
			return nil
		} else {
			existingResult.Transport = result.Transport
			existingResult.Protocol = result.Protocol
			existingResult.Product = result.Product
			existingResult.ServiceVersion = result.Version
			existingResult.RawResponse = cleanedResponse
			existingResult.DiscoveryStatus = "verified"
		}
		return &existingResult
	case gorm.ErrRecordNotFound:
		// 创建新记录
		portScanResult := PortScanResult{
			TaskID:          taskID,
			Version:         version,
			Host:            result.Host,
			Port:            result.Port,
			Transport:       result.Transport,
			Protocol:        result.Protocol,
			DiscoveryStatus: status,
			Product:         result.Product,
			ServiceVersion:  result.Version,
			RawResponse:     cleanedResponse,
		}
		if err := db.Create(&portScanResult).Error; err != nil {
			logger.Error("保存端口结果失败: %v", err)
			return nil
		}
		return &portScanResult
	default:
		logger.Error("查询端口结果失败: %v", err)
		return nil
	}
}

type PortResultAssetInput struct {
	IP           string
	Port         int
	Transport    string
	Domain       string
	Subdomain    string
	AssetType    string
	AssetName    string
	SourceModule string
}

// SavePortResultAssets saves subdomain/domain aliases for an open port result.
func SavePortResultAssets(db *gorm.DB, taskID string, version int, resultID uint, aliases []PortResultAssetInput) {
	if db == nil || resultID == 0 || len(aliases) == 0 {
		return
	}

	for _, alias := range aliases {
		if strings.TrimSpace(alias.Subdomain) == "" || strings.TrimSpace(alias.IP) == "" || alias.Port <= 0 {
			continue
		}

		var existing PortScanResultAsset
		err := db.Where(
			"task_id = ? AND version = ? AND result_id = ? AND subdomain = ?",
			taskID,
			version,
			resultID,
			alias.Subdomain,
		).First(&existing).Error

		updates := map[string]interface{}{
			"ip":            alias.IP,
			"port":          alias.Port,
			"transport":     alias.Transport,
			"domain":        alias.Domain,
			"asset_type":    alias.AssetType,
			"asset_name":    alias.AssetName,
			"source_module": alias.SourceModule,
		}

		switch err {
		case nil:
			if err := db.Model(&existing).Updates(updates).Error; err != nil {
				logger.Error("更新端口资产关系失败: %v", err)
			}
		case gorm.ErrRecordNotFound:
			record := PortScanResultAsset{
				TaskID:       taskID,
				Version:      version,
				ResultID:     resultID,
				IP:           alias.IP,
				Port:         alias.Port,
				Transport:    alias.Transport,
				Domain:       alias.Domain,
				Subdomain:    alias.Subdomain,
				AssetType:    alias.AssetType,
				AssetName:    alias.AssetName,
				SourceModule: alias.SourceModule,
			}
			if err := db.Create(&record).Error; err != nil {
				logger.Error("保存端口资产关系失败: %v", err)
			}
		default:
			logger.Error("查询端口资产关系失败: %v", err)
		}
	}
}
