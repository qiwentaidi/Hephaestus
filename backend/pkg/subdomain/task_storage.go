package subdomain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/taskstate"
	"hephaestus-backend/pkg/types"

	"gorm.io/gorm"
)

// ScanProgressDetail 保存子域名扫描的双层进度。
type ScanProgressDetail struct {
	TotalDomains          int    `json:"totalDomains"`
	CompletedDomains      int    `json:"completedDomains"`
	CurrentDomain         string `json:"currentDomain"`
	CurrentDomainIndex    int    `json:"currentDomainIndex"`
	CurrentDomainTotal    int    `json:"currentDomainTotal"`
	CurrentDomainResolved int    `json:"currentDomainResolved"`
	CurrentDomainProgress int    `json:"currentDomainProgress"`
	OverallProgress       int    `json:"overallProgress"`
}

// PrepareScanTask resets subdomain-specific progress and results for an existing
// unified attack-surface task. Task creation and lifecycle state are owned by
// vul_scan_task and the shared scan runtime.
func PrepareScanTask(taskID string, config types.SubdomainOption) error {
	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}
	return prepareScanTaskWithDB(db, taskID, config)
}

func prepareScanTaskWithDB(db *gorm.DB, taskID string, config types.SubdomainOption) error {
	return db.Transaction(func(tx *gorm.DB) error {
		task, err := findSubdomainTask(tx, taskID)
		if err != nil {
			return fmt.Errorf("查询统一子域名任务失败: %w", err)
		}
		if err := tx.Where("task_id = ?", taskID).Delete(&database.SubdomainScanResult{}).Error; err != nil {
			return err
		}
		if task.ScanType == "combination" || task.ParentCombinationID != "" {
			return taskstate.SaveCombinationCheckpoint(tx, task.ID, task.Version, "subdomain", "subdomain:scanning", 0, map[string]interface{}{
				"totalCount": len(config.Domains), "currentCount": 0, "progressDetail": map[string]interface{}{},
			})
		}
		return tx.Model(&database.VulnScanTask{}).
			Where("id = ? AND version = ?", task.ID, task.Version).
			Updates(map[string]interface{}{
				"progress":        0,
				"total_count":     len(config.Domains),
				"current_count":   0,
				"progress_detail": "{}",
				"error_msg":       "",
				"updated_at":      time.Now(),
			}).Error
	})
}

// UpdateTaskProgressWithDetail 更新任务进度和细分进度。
func UpdateTaskProgressWithDetail(taskID string, progress int, currentCount int, totalCount int, resultCount int, detail *ScanProgressDetail) error {
	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}
	return updateTaskProgressWithDetailDB(db, taskID, progress, currentCount, totalCount, resultCount, detail)
}

func updateTaskProgressWithDetailDB(db *gorm.DB, taskID string, progress int, currentCount int, totalCount int, resultCount int, detail *ScanProgressDetail) error {
	updates := map[string]interface{}{
		"progress":      progress,
		"current_count": currentCount,
		"total_count":   totalCount,
		"updated_at":    time.Now(),
	}
	_ = resultCount // Result count is derived from subdomain_scan_result.

	if detail != nil {
		detailJSON, err := json.Marshal(detail)
		if err != nil {
			return fmt.Errorf("序列化进度详情失败: %w", err)
		}
		updates["progress_detail"] = string(detailJSON)
	}

	task, err := findSubdomainTask(db, taskID)
	if err != nil {
		return fmt.Errorf("查询统一子域名任务失败: %w", err)
	}
	if task.ScanType == "combination" || task.ParentCombinationID != "" {
		stage := "subdomain:scanning"
		if progress >= 100 {
			stage = "subdomain:completed"
		}
		meta := map[string]interface{}{"currentCount": currentCount, "totalCount": totalCount}
		if detail != nil {
			meta["progressDetail"] = detail
		}
		return taskstate.SaveCombinationCheckpoint(db, task.ID, task.Version, "subdomain", stage, progress, meta)
	}
	if err := db.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ?", task.ID, task.Version).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("更新任务进度失败: %w", err)
	}
	return nil
}

func findSubdomainTask(db *gorm.DB, taskID string) (database.VulnScanTask, error) {
	var task database.VulnScanTask
	if err := db.Where("id = ?", taskID).Order("version DESC").Take(&task).Error; err != nil {
		return task, err
	}
	if task.ScanType != "subdomain" && task.ScanType != "combination" {
		return task, gorm.ErrRecordNotFound
	}
	return task, nil
}

// SaveScanResult 保存扫描结果
func SaveScanResult(taskID string, result SubdomainResult) error {
	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}
	return saveScanResultWithDB(db, taskID, result)
}

func saveScanResultWithDB(db *gorm.DB, taskID string, result SubdomainResult) error {
	ipsJSON, err := json.Marshal(result.IPs)
	if err != nil {
		return fmt.Errorf("序列化IP列表失败: %w", err)
	}
	sources := normalizeSources(result.Sources, result.Source)
	sourcesJSON, err := json.Marshal(sources)
	if err != nil {
		return fmt.Errorf("序列化来源列表失败: %w", err)
	}

	scanResult := database.SubdomainScanResult{
		ResolutionError: result.ResolutionError,
		ID:              database.GenerateNanoid(),
		TaskID:          taskID,
		Domain:          result.Domain,
		Subdomain:       result.Subdomain,
		IPs:             string(ipsJSON),
		AssetType:       result.AssetType,
		AssetName:       result.AssetName,
		Source:          firstSource(sources),
		Sources:         string(sourcesJSON),
		IsWildcard:      result.IsWildcard,
	}

	if err := db.Create(&scanResult).Error; err != nil {
		return fmt.Errorf("保存扫描结果失败: %w", err)
	}

	return nil
}

// BatchSaveScanResults 批量保存扫描结果
func BatchSaveScanResults(taskID string, results []SubdomainResult) error {
	if len(results) == 0 {
		return nil
	}

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}
	return batchSaveScanResultsWithDB(db, taskID, results)
}

func batchSaveScanResultsWithDB(db *gorm.DB, taskID string, results []SubdomainResult) error {
	scanResults := make([]database.SubdomainScanResult, 0, len(results))
	for _, result := range results {
		ipsJSON, err := json.Marshal(result.IPs)
		if err != nil {
			continue
		}
		sources := normalizeSources(result.Sources, result.Source)
		sourcesJSON, err := json.Marshal(sources)
		if err != nil {
			continue
		}

		scanResults = append(scanResults, database.SubdomainScanResult{
			ResolutionError: result.ResolutionError,
			ID:              database.GenerateNanoid(),
			TaskID:          taskID,
			Domain:          result.Domain,
			Subdomain:       result.Subdomain,
			IPs:             string(ipsJSON),
			AssetType:       result.AssetType,
			AssetName:       result.AssetName,
			Source:          firstSource(sources),
			Sources:         string(sourcesJSON),
			IsWildcard:      result.IsWildcard,
		})
	}

	if len(scanResults) > 0 {
		// 使用批量插入
		if err := db.CreateInBatches(scanResults, 100).Error; err != nil {
			return fmt.Errorf("批量保存扫描结果失败: %w", err)
		}
	}

	return nil
}

func AddScanResultSource(taskID string, domain string, subdomains []string, source string) error {
	source = strings.TrimSpace(source)
	if taskID == "" || domain == "" || len(subdomains) == 0 || source == "" {
		return nil
	}

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var records []database.SubdomainScanResult
	if err := db.Where("task_id = ? AND domain = ? AND subdomain IN ?", taskID, domain, subdomains).
		Find(&records).Error; err != nil {
		return fmt.Errorf("查询已有扫描结果失败: %w", err)
	}

	for _, record := range records {
		sources := normalizeSources(append(parseStoredSources(record.Sources, record.Source), source), "")
		sourcesJSON, err := json.Marshal(sources)
		if err != nil {
			return fmt.Errorf("序列化来源列表失败: %w", err)
		}
		if err := db.Model(&database.SubdomainScanResult{}).
			Where("id = ?", record.ID).
			Updates(map[string]interface{}{
				"source":  firstSource(sources),
				"sources": string(sourcesJSON),
			}).Error; err != nil {
			return fmt.Errorf("更新扫描结果来源失败: %w", err)
		}
	}

	return nil
}

// GetScanResults 获取任务的扫描结果
// wildcardFilters 可选值为 all、exclude、only；省略时保持查询全部结果的行为。
func GetScanResults(taskID string, limit int, offset int, searchQuery string, sourceFilter string, wildcardFilters ...string) ([]SubdomainResult, int64, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, 0, fmt.Errorf("获取数据库连接失败: %w", err)
	}
	wildcardFilter := ""
	if len(wildcardFilters) > 0 {
		wildcardFilter = wildcardFilters[0]
	}
	return getScanResultsWithDB(db, taskID, limit, offset, searchQuery, sourceFilter, wildcardFilter)
}

func getScanResultsWithDB(db *gorm.DB, taskID string, limit int, offset int, searchQuery string, sourceFilter string, wildcardFilter string) ([]SubdomainResult, int64, error) {
	// 构建基础查询条件
	baseQuery := db.Model(&database.SubdomainScanResult{}).Where("task_id = ?", taskID)

	// 添加搜索条件
	if searchQuery != "" {
		searchPattern := "%" + searchQuery + "%"
		baseQuery = baseQuery.Where("subdomain LIKE ? OR domain LIKE ?", searchPattern, searchPattern)
	}

	// 添加来源筛选
	if sourceFilter != "" && sourceFilter != "all" {
		sourceFilter = strings.TrimSpace(sourceFilter)
		if sourceFilter != "" {
			sourcePattern, _ := json.Marshal(sourceFilter)
			baseQuery = baseQuery.Where("source = ? OR sources LIKE ?", sourceFilter, "%"+string(sourcePattern)+"%")
		}
	}

	switch wildcardFilter {
	case "exclude":
		baseQuery = baseQuery.Where("is_wildcard = ?", false)
	case "only":
		baseQuery = baseQuery.Where("is_wildcard = ?", true)
	}

	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("查询结果总数失败: %w", err)
	}

	var resultRecords []database.SubdomainScanResult
	query := baseQuery.Order("created_at DESC, id DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&resultRecords).Error; err != nil {
		return nil, 0, fmt.Errorf("查询扫描结果失败: %w", err)
	}

	results := make([]SubdomainResult, len(resultRecords))
	for i, rr := range resultRecords {
		var ips []string
		if err := json.Unmarshal([]byte(rr.IPs), &ips); err != nil {
			return nil, 0, fmt.Errorf("解析IP列表失败: %w", err)
		}
		sources := parseStoredSources(rr.Sources, rr.Source)

		results[i] = SubdomainResult{
			ResolutionError: rr.ResolutionError,
			Domain:          rr.Domain,
			Subdomain:       rr.Subdomain,
			IPs:             ips,
			AssetType:       rr.AssetType,
			AssetName:       rr.AssetName,
			Source:          firstSource(sources),
			Sources:         sources,
			IsWildcard:      rr.IsWildcard,
		}
	}

	return results, total, nil
}

func parseStoredSources(storedSources string, fallbackSource string) []string {
	var sources []string
	if strings.TrimSpace(storedSources) != "" {
		if err := json.Unmarshal([]byte(storedSources), &sources); err == nil {
			return normalizeSources(sources, fallbackSource)
		}
	}
	return normalizeSources(nil, fallbackSource)
}

// GetScanResultSourceStats 统计整个任务的来源数量，不受分页或结果筛选影响。
func GetScanResultSourceStats(taskID string) (map[string]int64, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}
	return getScanResultSourceStatsWithDB(db, taskID)
}

func getScanResultSourceStatsWithDB(db *gorm.DB, taskID string) (map[string]int64, error) {
	var groups []struct {
		Source      string
		Sources     string
		ResultCount int64
	}
	if err := db.Model(&database.SubdomainScanResult{}).
		Select("source, sources, COUNT(*) AS result_count").
		Where("task_id = ?", taskID).
		Group("source, sources").
		Scan(&groups).Error; err != nil {
		return nil, fmt.Errorf("统计扫描结果来源失败: %w", err)
	}
	stats := make(map[string]int64)
	for _, group := range groups {
		for _, source := range parseStoredSources(group.Sources, group.Source) {
			stats[source] += group.ResultCount
		}
	}
	return stats, nil
}

func GetAllScanResults(taskID string, batchSize int, searchQuery string, sourceFilter string) ([]SubdomainResult, int64, error) {
	if batchSize <= 0 {
		batchSize = 500
	}

	offset := 0
	allResults := make([]SubdomainResult, 0, batchSize)
	var total int64

	for {
		results, currentTotal, err := GetScanResults(taskID, batchSize, offset, searchQuery, sourceFilter)
		if err != nil {
			return nil, 0, err
		}
		total = currentTotal

		if len(results) == 0 {
			break
		}

		allResults = append(allResults, results...)
		offset += len(results)

		if int64(offset) >= total || len(results) < batchSize {
			break
		}
	}

	return allResults, total, nil
}
