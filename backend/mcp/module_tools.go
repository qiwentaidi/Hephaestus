package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"hephaestus-backend/handlers/asset"
	"hephaestus-backend/handlers/toolbox"
	"hephaestus-backend/pkg/company"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/scantask"
	spaceenginepkg "hephaestus-backend/pkg/space/engine"
	"hephaestus-backend/pkg/subdomain"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	mcpTaskDefaultPageSize = 20
	mcpTaskMaxPageSize     = 100
)

type mcpSpaceNativeSearchArgs struct {
	Engine   string `json:"engine"`
	Query    string `json:"query"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

type mcpWebScanCreateTaskArgs struct {
	ScanType                     string                 `json:"scanType"`
	TaskName                     string                 `json:"taskName"`
	Targets                      []string               `json:"targets"`
	ScanMode                     string                 `json:"scanMode"`
	AgentID                      string                 `json:"agentId"`
	Thread                       int                    `json:"thread"`
	Threads                      int                    `json:"threads"`
	Workers                      int                    `json:"workers"`
	NucleiThreads                int                    `json:"nucleiThreads"`
	Timeout                      float64                `json:"timeout"`
	Headers                      []string               `json:"headers"`
	Template                     []string               `json:"template"`
	TemplateFiles                []string               `json:"templateFiles"`
	Tags                         []string               `json:"tags"`
	NoPoc                        *bool                  `json:"noPoc"`
	CallNuclei                   *bool                  `json:"callNuclei"`
	Screenshot                   *bool                  `json:"screenshot"`
	DeepScan                     *bool                  `json:"deepScan"`
	RootPath                     *bool                  `json:"rootPath"`
	SkipNucleiWithoutTags        *bool                  `json:"skipNucleiWithoutTags"`
	SkipNucleiWithWAF            *bool                  `json:"skipNucleiWithWAF"`
	DisableMaxHostErr            *bool                  `json:"disableMaxHostErr"`
	Proxy                        string                 `json:"proxy"`
	NetworkCard                  string                 `json:"networkCard"`
	CustomHeaders                string                 `json:"customHeaders"`
	WebscanOption                int                    `json:"webscanOption"`
	OOBEnabled                   *bool                  `json:"oobEnabled"`
	InteractshURL                string                 `json:"interactshUrl"`
	InteractshToken              string                 `json:"interactshToken"`
	PortGroupID                  string                 `json:"portGroupId"`
	PortRange                    string                 `json:"portRange"`
	AliveDetection               *bool                  `json:"aliveDetection"`
	SubnetDiscoveryEnabled       *bool                  `json:"subnetDiscoveryEnabled"`
	SubnetDiscoveryMode          string                 `json:"subnetDiscoveryMode"`
	SubnetDiscoveryHostSuffixes  string                 `json:"subnetDiscoveryHostSuffixes"`
	SubnetDiscoveryRandomSamples *int                   `json:"subnetDiscoveryRandomSamples"`
	TCPHeuristicEnabled          *bool                  `json:"tcpHeuristicEnabled"`
	TCPHeuristicPorts            string                 `json:"tcpHeuristicPorts"`
	TCPHeuristicTimeoutMs        int                    `json:"tcpHeuristicTimeoutMs"`
	TCPDiscoveryMode             string                 `json:"tcpDiscoveryMode"`
	SynRate                      int                    `json:"synRate"`
	SynDiscoveryAttempts         int                    `json:"synDiscoveryAttempts"`
	Method                       string                 `json:"method"`
	Paths                        []string               `json:"paths"`
	DictPaths                    []string               `json:"dictPaths"`
	DictNames                    []string               `json:"dictNames"`
	Exts                         []string               `json:"exts"`
	StatusCodeExclude            []int                  `json:"statusCodeExclude"`
	BodyExclude                  string                 `json:"bodyExclude"`
	BodyLengthExcludeTimes       int                    `json:"bodyLengthExcludeTimes"`
	Redirect                     *bool                  `json:"redirect"`
	Interval                     int                    `json:"interval"`
	CustomHeader                 string                 `json:"customHeader"`
	Backupscan                   *bool                  `json:"backupscan"`
	Recursive                    int                    `json:"recursive"`
	Username                     string                 `json:"username"`
	Password                     string                 `json:"password"`
	DefaultDict                  *bool                  `json:"defaultDict"`
	TargetTimeout                int                    `json:"targetTimeout"`
	SSHMaxTimeoutErrors          int                    `json:"sshMaxTimeoutErrors"`
	Config                       map[string]interface{} `json:"config"`
}

type mcpTaskIDArgs struct {
	TaskID  string `json:"taskId"`
	Version int    `json:"version"`
}

type mcpWebScanVulnerabilityArgs struct {
	TaskID         string `json:"taskId"`
	Version        int    `json:"version"`
	Page           int    `json:"page"`
	PageSize       int    `json:"pageSize"`
	Search         string `json:"search"`
	Severity       string `json:"severity"`
	DisposalStatus string `json:"disposalStatus"`
}

type mcpWebScanResultsArgs struct {
	TaskID         string `json:"taskId"`
	Version        int    `json:"version"`
	ResultType     string `json:"resultType"`
	Page           int    `json:"page"`
	PageSize       int    `json:"pageSize"`
	Limit          int    `json:"limit"`
	Offset         int    `json:"offset"`
	Search         string `json:"search"`
	AssetType      string `json:"assetType"`
	Severity       string `json:"severity"`
	Level          string `json:"level"`
	DisposalStatus string `json:"disposalStatus"`
	Status         int    `json:"status"`
	Method         string `json:"method"`
	IncludeContent bool   `json:"includeContent"`
	IncludeTree    bool   `json:"includeTree"`
}

type mcpSubdomainCreateTaskArgs struct {
	TaskName      string   `json:"taskName"`
	Domains       []string `json:"domains"`
	ScanMode      string   `json:"scanMode"`
	AgentID       string   `json:"agentId"`
	Subs          []string `json:"subs"`
	DnsServers    []string `json:"dnsServers"`
	Thread        int      `json:"thread"`
	Timeout       int      `json:"timeout"`
	Mode          int      `json:"mode"`
	AppendEngines []string `json:"appendEngines"`
}

type mcpSubdomainResultsArgs struct {
	TaskID string `json:"taskId"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
	Search string `json:"search"`
	Source string `json:"source"`
}

type mcpCompanyCreateTaskArgs struct {
	TaskName        string   `json:"taskName"`
	Company         string   `json:"company"`
	Companies       []string `json:"companies"`
	Invest          int      `json:"invest"`
	DataSource      []string `json:"dataSource"`
	RiskbirdCookie  string   `json:"riskbirdCookie"`
	ZeroZoneKey     string   `json:"zeroZoneKey"`
	ZeroZoneEnable  bool     `json:"zeroZoneEnable"`
	SubcompanyLevel int      `json:"subcompanyLevel"`
	MiitApi         string   `json:"miitApi"`
	MiitEnable      bool     `json:"miitEnable"`
}

type mcpJWTBruteforceArgs struct {
	Token             string `json:"token"`
	Dictionary        string `json:"dictionary"`
	Source            string `json:"source"`
	TryBase64Variants bool   `json:"tryBase64Variants"`
}

func mcpCallJWTListDictionaries() (mcpToolCallResult, error) {
	result, err := toolbox.ListJWTDictionariesTool()
	if err != nil {
		return mcpToolCallResult{}, err
	}
	return newMCPToolResult(gin.H{"source": "jwt", "dictionaries": result}), nil
}

func mcpCallJWTBruteforce(ctx context.Context, args mcpJWTBruteforceArgs) (mcpToolCallResult, error) {
	result, err := toolbox.RunJWTBruteforceTool(ctx, args.Token, args.Dictionary, args.Source, args.TryBase64Variants)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	return newMCPToolResult(result), nil
}

func mcpCallSpaceEngineNativeSearch(args mcpSpaceNativeSearchArgs) (mcpToolCallResult, error) {
	engineName := strings.ToLower(strings.TrimSpace(args.Engine))
	if len(normalizeSpaceEngineNames([]string{engineName})) != 1 {
		return mcpToolCallResult{}, fmt.Errorf("engine 必须是 fofa、hunter、quake、censys、zoomeye 或 shodan")
	}

	query := strings.TrimSpace(args.Query)
	if query == "" {
		return mcpToolCallResult{}, fmt.Errorf("query 不能为空")
	}

	page, pageSize := normalizeMCPPagination(args.Page, args.PageSize, mcpSearchDefaultPageSize, mcpSearchMaxPageSize)
	option := buildSpaceOptionForEngines([]string{engineName}, 1)
	aggregator, err := createAggregatorFromRequest(&option)
	if err != nil {
		return mcpToolCallResult{}, err
	}

	searchEngine := aggregator.GetEngine(engineName)
	if searchEngine == nil || !searchEngine.IsConfigured() {
		return mcpToolCallResult{}, fmt.Errorf("%s 未配置或不可用", engineName)
	}

	resp := searchEngine.Search(&spaceenginepkg.SearchRequest{
		Query:    query,
		Page:     page,
		PageSize: pageSize,
	})
	if resp == nil {
		return mcpToolCallResult{}, fmt.Errorf("%s 查询失败: 空响应", engineName)
	}
	if resp.Error != nil {
		message := strings.TrimSpace(resp.Message)
		if message == "" {
			message = resp.Error.Error()
		}
		return mcpToolCallResult{}, fmt.Errorf("%s 查询失败: %s", engineName, message)
	}

	payload := gin.H{
		"engine":   engineName,
		"query":    query,
		"page":     page,
		"pageSize": pageSize,
		"total":    resp.Total,
		"message":  resp.Message,
		"results":  convertToLegacyResults(resp.Results),
	}
	return newMCPToolResult(payload), nil
}

func mcpCallWebScanCreateTask(args mcpWebScanCreateTaskArgs) (mcpToolCallResult, error) {
	targets := normalizeNonEmptyStrings(args.Targets)
	if len(targets) == 0 {
		return mcpToolCallResult{}, fmt.Errorf("targets 不能为空")
	}

	taskConfig := copyStringInterfaceMap(args.Config)
	mergeMCPWebScanConfig(taskConfig, args)

	taskID, scanType, scanMode, agentID, err := scantask.CreateExternalScanTask(args.ScanType, args.TaskName, args.ScanMode, args.AgentID, targets, taskConfig)
	if err != nil {
		return mcpToolCallResult{}, err
	}

	return newMCPToolResult(gin.H{
		"taskId":   taskID,
		"scanType": scanType,
		"scanMode": scanMode,
		"agentId":  agentID,
		"status":   "created",
	}), nil
}

func mcpCallWebScanGetTask(args mcpTaskIDArgs) (mcpToolCallResult, error) {
	_, payload, err := loadWebScanTaskPayload(args.TaskID, args.Version)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	return newMCPToolResult(payload), nil
}

func mcpCallWebScanListVulnerabilities(args mcpWebScanVulnerabilityArgs) (mcpToolCallResult, error) {
	taskID := strings.TrimSpace(args.TaskID)
	if taskID == "" {
		return mcpToolCallResult{}, fmt.Errorf("taskId 不能为空")
	}

	db, err := database.GetDB()
	if err != nil {
		return mcpToolCallResult{}, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	task, err := loadLatestWebScanTask(db, taskID, args.Version)
	if err != nil {
		return mcpToolCallResult{}, err
	}

	page, pageSize := normalizeMCPPagination(args.Page, args.PageSize, mcpTaskDefaultPageSize, mcpTaskMaxPageSize)
	query := db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ?", task.ID, task.Version)
	if search := strings.TrimSpace(args.Search); search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("vuln_name LIKE ? OR vuln_url LIKE ? OR description LIKE ?", searchPattern, searchPattern, searchPattern)
	}
	if severity := strings.TrimSpace(args.Severity); severity != "" {
		query = query.Where("LOWER(severity) = LOWER(?)", severity)
	}
	if disposalStatus := strings.TrimSpace(args.DisposalStatus); disposalStatus != "" {
		query = query.Where("disposal_status = ?", disposalStatus)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("查询漏洞总数失败: %w", err)
	}

	var vulnerabilities []database.VulnerabilityInfo
	if err := query.Order("template_id ASC").
		Order("vuln_url ASC").
		Order("protocol ASC").
		Order("severity DESC").
		Order("vuln_name ASC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&vulnerabilities).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("查询漏洞结果失败: %w", err)
	}

	return newMCPToolResult(gin.H{
		"taskId":          task.ID,
		"version":         task.Version,
		"vulnerabilities": vulnerabilities,
		"total":           total,
		"page":            page,
		"pageSize":        pageSize,
	}), nil
}

func mcpCallWebScanListResults(args mcpWebScanResultsArgs) (mcpToolCallResult, error) {
	taskID := strings.TrimSpace(args.TaskID)
	if taskID == "" {
		return mcpToolCallResult{}, fmt.Errorf("taskId 不能为空")
	}

	db, err := database.GetDB()
	if err != nil {
		return mcpToolCallResult{}, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	task, err := loadLatestWebScanTask(db, taskID, args.Version)
	if err != nil {
		return mcpToolCallResult{}, err
	}

	resultType := normalizeMCPScanResultType(args.ResultType, task.ScanType)
	switch resultType {
	case "subdomains":
		return mcpListSubdomainResults(task, args)
	case "ports":
		return mcpListPortScanResults(db, task, args)
	case "dir_results":
		return mcpListDirScanResults(db, task, args)
	case "js_assets":
		return mcpListJSScanAssets(db, task, args)
	case "js_resources":
		return mcpListJSScanResources(db, task, args)
	case "js_api_resources":
		return mcpListJSScanAPIResources(db, task, args)
	case "js_site_tree":
		return mcpListJSScanSiteTrees(db, task, args)
	case "js_vulnerabilities":
		return mcpListJSScanVulnerabilities(db, task, args)
	case "vulnerabilities":
		return mcpListWebScanVulnerabilities(db, task, args)
	default:
		return mcpToolCallResult{}, fmt.Errorf("不支持的 resultType: %s", args.ResultType)
	}
}

func mcpListSubdomainResults(task database.VulnScanTask, args mcpWebScanResultsArgs) (mcpToolCallResult, error) {
	if task.ScanType != "subdomain" {
		return mcpToolCallResult{}, fmt.Errorf("任务不是子域名收集任务: %s", task.ID)
	}

	limit := args.Limit
	if limit <= 0 {
		limit = effectiveMCPPageSize(args)
	}
	if limit <= 0 {
		limit = mcpTaskDefaultPageSize
	}
	if limit > mcpTaskMaxPageSize {
		limit = mcpTaskMaxPageSize
	}
	offset := args.Offset
	if offset < 0 {
		offset = 0
	}

	results, total, err := subdomain.GetScanResults(task.ID, limit, offset, strings.TrimSpace(args.Search), "")
	if err != nil {
		return mcpToolCallResult{}, err
	}

	return newMCPToolResult(gin.H{
		"taskId":     task.ID,
		"scanType":   task.ScanType,
		"version":    task.Version,
		"resultType": "subdomains",
		"results":    results,
		"total":      total,
		"limit":      limit,
		"offset":     offset,
	}), nil
}

func mcpListWebScanVulnerabilities(db *gorm.DB, task database.VulnScanTask, args mcpWebScanResultsArgs) (mcpToolCallResult, error) {
	page, pageSize := normalizeMCPPagination(args.Page, effectiveMCPPageSize(args), mcpTaskDefaultPageSize, mcpTaskMaxPageSize)
	query := db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ?", task.ID, task.Version)
	if search := strings.TrimSpace(args.Search); search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("vuln_name LIKE ? OR vuln_url LIKE ? OR description LIKE ? OR protocol LIKE ?", searchPattern, searchPattern, searchPattern, searchPattern)
	}
	if severity := strings.TrimSpace(firstNonEmpty(args.Severity, args.Level)); severity != "" {
		query = query.Where("LOWER(severity) = LOWER(?)", severity)
	}
	if disposalStatus := strings.TrimSpace(args.DisposalStatus); disposalStatus != "" {
		query = query.Where("disposal_status = ?", disposalStatus)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("查询漏洞总数失败: %w", err)
	}

	var vulnerabilities []database.VulnerabilityInfo
	if err := query.Order("template_id ASC").
		Order("vuln_url ASC").
		Order("protocol ASC").
		Order("severity DESC").
		Order("vuln_name ASC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&vulnerabilities).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("查询漏洞结果失败: %w", err)
	}

	return newMCPToolResult(gin.H{
		"taskId":          task.ID,
		"scanType":        task.ScanType,
		"version":         task.Version,
		"resultType":      "vulnerabilities",
		"vulnerabilities": vulnerabilities,
		"total":           total,
		"page":            page,
		"pageSize":        pageSize,
	}), nil
}

func mcpListJSScanVulnerabilities(db *gorm.DB, task database.VulnScanTask, args mcpWebScanResultsArgs) (mcpToolCallResult, error) {
	page, pageSize := normalizeMCPPagination(args.Page, effectiveMCPPageSize(args), mcpTaskDefaultPageSize, mcpTaskMaxPageSize)
	query := db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ?", task.ID, task.Version)
	if search := strings.TrimSpace(args.Search); search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("title LIKE ? OR type LIKE ? OR url LIKE ? OR description LIKE ? OR method LIKE ? OR response_type LIKE ?", searchPattern, searchPattern, searchPattern, searchPattern, searchPattern, searchPattern)
	}
	if level := strings.TrimSpace(firstNonEmpty(args.Level, args.Severity)); level != "" {
		query = query.Where("LOWER(level) = LOWER(?)", level)
	}
	if method := strings.TrimSpace(args.Method); method != "" {
		query = query.Where("method = ?", method)
	}
	if disposalStatus := strings.TrimSpace(args.DisposalStatus); disposalStatus != "" {
		query = query.Where("COALESCE(disposal_status, 'open') = ?", disposalStatus)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("统计 JS 漏洞失败: %w", err)
	}

	var vulnerabilities []database.VulnScanJSVulnerability
	if err := query.Order("created_at DESC").Order("id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&vulnerabilities).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("查询 JS 漏洞失败: %w", err)
	}

	return newMCPToolResult(gin.H{
		"taskId":          task.ID,
		"scanType":        task.ScanType,
		"version":         task.Version,
		"resultType":      "js_vulnerabilities",
		"vulnerabilities": vulnerabilities,
		"total":           total,
		"page":            page,
		"pageSize":        pageSize,
	}), nil
}

func mcpListJSScanAssets(db *gorm.DB, task database.VulnScanTask, args mcpWebScanResultsArgs) (mcpToolCallResult, error) {
	if task.ScanType != "jsscan" {
		return mcpToolCallResult{}, fmt.Errorf("任务不是 JS/API 采集任务: %s", task.ID)
	}
	page, pageSize := normalizeMCPPagination(args.Page, effectiveMCPPageSize(args), mcpTaskDefaultPageSize, mcpTaskMaxPageSize)
	query := db.Model(&database.VulnScanJSAsset{}).Where("task_id = ? AND version = ?", task.ID, task.Version)
	if search := strings.TrimSpace(args.Search); search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("type LIKE ? OR value LIKE ? OR source LIKE ?", searchPattern, searchPattern, searchPattern)
	}
	if assetType := normalizeMCPJSAssetType(args.AssetType); assetType != "" && assetType != "all" {
		query = query.Where("type IN ?", expandMCPJSAssetTypes(assetType))
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("统计 JS/API 资产失败: %w", err)
	}

	var assets []database.VulnScanJSAsset
	if err := query.Order("created_at DESC").Order("id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&assets).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("查询 JS/API 资产失败: %w", err)
	}

	return newMCPToolResult(gin.H{
		"taskId":     task.ID,
		"scanType":   task.ScanType,
		"version":    task.Version,
		"resultType": "js_assets",
		"assets":     assets,
		"total":      total,
		"page":       page,
		"pageSize":   pageSize,
	}), nil
}

func mcpListJSScanResources(db *gorm.DB, task database.VulnScanTask, args mcpWebScanResultsArgs) (mcpToolCallResult, error) {
	if task.ScanType != "jsscan" {
		return mcpToolCallResult{}, fmt.Errorf("任务不是 JS/API 采集任务: %s", task.ID)
	}
	page, pageSize := normalizeMCPPagination(args.Page, effectiveMCPPageSize(args), mcpTaskDefaultPageSize, mcpTaskMaxPageSize)
	query := db.Model(&database.VulnScanJSResource{}).Where("task_id = ? AND version = ?", task.ID, task.Version)
	if search := strings.TrimSpace(args.Search); search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("url LIKE ? OR content LIKE ?", searchPattern, searchPattern)
	}
	if args.Status > 0 {
		query = query.Where("response_code = ?", args.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("统计 JS 资源失败: %w", err)
	}

	var rows []database.VulnScanJSResource
	if err := query.Order("fetched_at DESC").Order("id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&rows).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("查询 JS 资源失败: %w", err)
	}

	resources := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		item := gin.H{
			"id":             row.ID,
			"taskId":         row.TaskID,
			"version":        row.Version,
			"url":            row.URL,
			"responseCode":   row.ResponseCode,
			"headers":        row.Headers,
			"size":           row.Size,
			"fetchedAt":      row.FetchedAt,
			"contentLength":  len(row.Content),
			"contentPreview": truncateMCPText(row.Content, 2000),
		}
		if args.IncludeContent {
			item["content"] = truncateMCPText(row.Content, 20000)
			item["contentTruncated"] = len(row.Content) > 20000
		}
		resources = append(resources, item)
	}

	return newMCPToolResult(gin.H{
		"taskId":     task.ID,
		"scanType":   task.ScanType,
		"version":    task.Version,
		"resultType": "js_resources",
		"resources":  resources,
		"total":      total,
		"page":       page,
		"pageSize":   pageSize,
	}), nil
}

func mcpListJSScanAPIResources(db *gorm.DB, task database.VulnScanTask, args mcpWebScanResultsArgs) (mcpToolCallResult, error) {
	if task.ScanType != "jsscan" {
		return mcpToolCallResult{}, fmt.Errorf("任务不是 JS/API 采集任务: %s", task.ID)
	}
	page, pageSize := normalizeMCPPagination(args.Page, effectiveMCPPageSize(args), mcpTaskDefaultPageSize, mcpTaskMaxPageSize)
	query := db.Model(&database.VulnScanJSAPIResource{}).Where("task_id = ? AND version = ?", task.ID, task.Version)
	if search := strings.TrimSpace(args.Search); search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("url LIKE ? OR method LIKE ? OR request_body LIKE ? OR response_body LIKE ?", searchPattern, searchPattern, searchPattern, searchPattern)
	}
	if method := strings.TrimSpace(args.Method); method != "" {
		query = query.Where("method = ?", method)
	}
	if args.Status > 0 {
		query = query.Where("response_code = ?", args.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("统计 JS/API 记录失败: %w", err)
	}

	var rows []database.VulnScanJSAPIResource
	if err := query.Order("fetched_at DESC").Order("id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&rows).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("查询 JS/API 记录失败: %w", err)
	}

	records := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		item := gin.H{
			"id":              row.ID,
			"taskId":          row.TaskID,
			"version":         row.Version,
			"url":             row.URL,
			"method":          row.Method,
			"responseCode":    row.ResponseCode,
			"requestHeaders":  row.RequestHeaders,
			"responseHeaders": row.ResponseHeaders,
			"headers":         row.Headers,
			"fetchedAt":       row.FetchedAt,
			"requestPreview":  truncateMCPText(row.RequestBody, 2000),
			"responsePreview": truncateMCPText(row.ResponseBody, 2000),
		}
		if args.IncludeContent {
			item["requestBody"] = truncateMCPText(row.RequestBody, 12000)
			item["responseBody"] = truncateMCPText(row.ResponseBody, 12000)
			item["bodyTruncated"] = len(row.RequestBody) > 12000 || len(row.ResponseBody) > 12000
		}
		records = append(records, item)
	}

	return newMCPToolResult(gin.H{
		"taskId":     task.ID,
		"scanType":   task.ScanType,
		"version":    task.Version,
		"resultType": "js_api_resources",
		"records":    records,
		"total":      total,
		"page":       page,
		"pageSize":   pageSize,
	}), nil
}

func mcpListJSScanSiteTrees(db *gorm.DB, task database.VulnScanTask, args mcpWebScanResultsArgs) (mcpToolCallResult, error) {
	if task.ScanType != "jsscan" {
		return mcpToolCallResult{}, fmt.Errorf("任务不是 JS/API 采集任务: %s", task.ID)
	}
	page, pageSize := normalizeMCPPagination(args.Page, effectiveMCPPageSize(args), mcpTaskDefaultPageSize, mcpTaskMaxPageSize)
	query := db.Model(&database.VulnScanJSSiteTree{}).Where("task_id = ? AND version = ?", task.ID, task.Version)
	if search := strings.TrimSpace(args.Search); search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("target_url LIKE ? OR tree_data LIKE ?", searchPattern, searchPattern)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("统计 JS/API 网站树失败: %w", err)
	}

	var rows []database.VulnScanJSSiteTree
	if err := query.Order("created_at DESC").Order("id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&rows).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("查询 JS/API 网站树失败: %w", err)
	}

	siteTrees := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		item := gin.H{
			"id":            row.ID,
			"taskId":        row.TaskID,
			"version":       row.Version,
			"targetUrl":     row.TargetURL,
			"createdAt":     row.CreatedAt,
			"apiRouteCount": countMCPJSAPIRoutesInTree(row.TreeData),
		}
		if args.IncludeTree {
			var treeData any
			if err := json.Unmarshal([]byte(row.TreeData), &treeData); err == nil {
				item["treeData"] = treeData
			} else {
				item["treeDataPreview"] = truncateMCPText(row.TreeData, 4000)
			}
		}
		siteTrees = append(siteTrees, item)
	}

	return newMCPToolResult(gin.H{
		"taskId":     task.ID,
		"scanType":   task.ScanType,
		"version":    task.Version,
		"resultType": "js_site_tree",
		"siteTrees":  siteTrees,
		"total":      total,
		"page":       page,
		"pageSize":   pageSize,
	}), nil
}

func mcpListPortScanResults(db *gorm.DB, task database.VulnScanTask, args mcpWebScanResultsArgs) (mcpToolCallResult, error) {
	page, pageSize := normalizeMCPPagination(args.Page, effectiveMCPPageSize(args), mcpTaskDefaultPageSize, mcpTaskMaxPageSize)
	query := db.Model(&database.PortScanResult{}).Where("task_id = ? AND version = ?", task.ID, task.Version)
	if search := strings.TrimSpace(args.Search); search != "" {
		like := "%" + search + "%"
		searchQuery := db.Where("host LIKE ? OR transport LIKE ? OR protocol LIKE ? OR product LIKE ? OR service_version LIKE ? OR raw_response LIKE ?", like, like, like, like, like, like)
		if port, err := parsePositiveInt(search); err == nil {
			searchQuery = searchQuery.Or("port = ?", port)
		}
		query = query.Where(searchQuery)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("查询端口结果总数失败: %w", err)
	}

	var ports []database.PortScanResult
	if err := query.Order("created_at DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&ports).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("查询端口结果失败: %w", err)
	}

	return newMCPToolResult(gin.H{
		"taskId":     task.ID,
		"scanType":   task.ScanType,
		"version":    task.Version,
		"resultType": "ports",
		"ports":      ports,
		"total":      total,
		"page":       page,
		"pageSize":   pageSize,
	}), nil
}

func mcpListDirScanResults(db *gorm.DB, task database.VulnScanTask, args mcpWebScanResultsArgs) (mcpToolCallResult, error) {
	limit := args.Limit
	if limit <= 0 {
		if args.PageSize > 0 {
			limit = args.PageSize
		} else {
			limit = mcpTaskDefaultPageSize
		}
	}
	if limit > mcpTaskMaxPageSize {
		limit = mcpTaskMaxPageSize
	}
	offset := args.Offset
	if offset < 0 {
		offset = 0
	}
	if args.Page > 1 && args.Offset == 0 {
		offset = (args.Page - 1) * limit
	}

	query := db.Model(&database.VulnScanDirResult{}).Where("task_id = ? AND version = ?", task.ID, task.Version)
	if search := strings.TrimSpace(args.Search); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		query = query.Where("LOWER(url) LIKE ? OR LOWER(title) LIKE ? OR LOWER(location) LIKE ? OR LOWER(content_type) LIKE ?", like, like, like, like)
	}
	if args.Status > 0 {
		query = query.Where("status = ?", args.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("查询目录扫描结果总数失败: %w", err)
	}

	var results []database.VulnScanDirResult
	if err := query.Order("created_at DESC").Order("id ASC").
		Limit(limit).
		Offset(offset).
		Find(&results).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("查询目录扫描结果失败: %w", err)
	}

	return newMCPToolResult(gin.H{
		"taskId":     task.ID,
		"scanType":   task.ScanType,
		"version":    task.Version,
		"resultType": "dir_results",
		"results":    results,
		"total":      total,
		"limit":      limit,
		"offset":     offset,
	}), nil
}

func mcpCallSubdomainCreateTask(args mcpSubdomainCreateTaskArgs) (mcpToolCallResult, error) {
	domains := normalizeNonEmptyStrings(args.Domains)
	if len(domains) == 0 {
		return mcpToolCallResult{}, fmt.Errorf("domains 不能为空")
	}

	taskConfig := map[string]interface{}{
		"domains":       domains,
		"subs":          normalizeNonEmptyStrings(args.Subs),
		"dnsServers":    normalizeNonEmptyStrings(args.DnsServers),
		"appendEngines": normalizeNonEmptyStrings(args.AppendEngines),
	}
	setIfInt(taskConfig, "thread", args.Thread)
	setIfInt(taskConfig, "timeout", args.Timeout)
	setIfInt(taskConfig, "mode", args.Mode)

	taskID, scanType, scanMode, agentID, err := scantask.CreateExternalScanTask(
		"subdomain",
		args.TaskName,
		args.ScanMode,
		args.AgentID,
		domains,
		taskConfig,
	)
	if err != nil {
		return mcpToolCallResult{}, err
	}

	return newMCPToolResult(gin.H{
		"taskId":   taskID,
		"scanType": scanType,
		"scanMode": scanMode,
		"agentId":  agentID,
		"version":  1,
		"status":   "created",
	}), nil
}

func mcpCallSubdomainGetTask(args mcpTaskIDArgs) (mcpToolCallResult, error) {
	task, payload, err := loadWebScanTaskPayload(args.TaskID, args.Version)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	if task.ScanType != "subdomain" {
		return mcpToolCallResult{}, fmt.Errorf("任务不是子域名收集任务: %s", task.ID)
	}

	payload["currentCount"] = task.CurrentCount
	payload["progressDetail"] = task.ProgressDetail

	return newMCPToolResult(payload), nil
}

func mcpCallSubdomainListResults(args mcpSubdomainResultsArgs) (mcpToolCallResult, error) {
	taskID := strings.TrimSpace(args.TaskID)
	if taskID == "" {
		return mcpToolCallResult{}, fmt.Errorf("taskId 不能为空")
	}

	limit := args.Limit
	if limit <= 0 {
		limit = mcpTaskDefaultPageSize
	}
	if limit > mcpTaskMaxPageSize {
		limit = mcpTaskMaxPageSize
	}
	offset := args.Offset
	if offset < 0 {
		offset = 0
	}

	results, total, err := subdomain.GetScanResults(taskID, limit, offset, strings.TrimSpace(args.Search), strings.TrimSpace(args.Source))
	if err != nil {
		return mcpToolCallResult{}, err
	}

	return newMCPToolResult(gin.H{
		"taskId":  taskID,
		"results": results,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	}), nil
}

func mcpCallCompanyCreateTask(args mcpCompanyCreateTaskArgs) (mcpToolCallResult, error) {
	companyNames := strings.TrimSpace(args.Company)
	if companyNames == "" && len(args.Companies) > 0 {
		companyNames = strings.Join(normalizeNonEmptyStrings(args.Companies), "\n")
	}
	if companyNames == "" {
		return mcpToolCallResult{}, fmt.Errorf("company 或 companies 不能为空")
	}

	task, err := asset.CreateCompanyInfoTask(asset.CompanyInfoRequest{
		TaskName:        args.TaskName,
		Company:         companyNames,
		Invest:          args.Invest,
		DataSource:      normalizeNonEmptyStrings(args.DataSource),
		RiskbirdCookie:  args.RiskbirdCookie,
		ZeroZoneKey:     args.ZeroZoneKey,
		ZeroZoneEnable:  args.ZeroZoneEnable,
		SubcompanyLevel: args.SubcompanyLevel,
		MiitApi:         args.MiitApi,
		MiitEnable:      args.MiitEnable,
	})
	if err != nil {
		return mcpToolCallResult{}, err
	}

	return newMCPToolResult(companyTaskPayload(task)), nil
}

func mcpCallCompanyGetTask(args mcpTaskIDArgs) (mcpToolCallResult, error) {
	taskID := strings.TrimSpace(args.TaskID)
	if taskID == "" {
		return mcpToolCallResult{}, fmt.Errorf("taskId 不能为空")
	}

	db, err := database.GetDB()
	if err != nil {
		return mcpToolCallResult{}, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var task database.CompanyInfoTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return mcpToolCallResult{}, fmt.Errorf("任务不存在: %s", taskID)
		}
		return mcpToolCallResult{}, fmt.Errorf("查询任务失败: %w", err)
	}

	return newMCPToolResult(companyTaskPayload(&task)), nil
}

func mcpCallCompanyGetResults(args mcpTaskIDArgs) (mcpToolCallResult, error) {
	taskID := strings.TrimSpace(args.TaskID)
	if taskID == "" {
		return mcpToolCallResult{}, fmt.Errorf("taskId 不能为空")
	}

	db, err := database.GetDB()
	if err != nil {
		return mcpToolCallResult{}, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var result database.CompanyInfoTaskResult
	if err := db.Where("task_id = ?", taskID).First(&result).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return mcpToolCallResult{}, fmt.Errorf("任务结果不存在: %s", taskID)
		}
		return mcpToolCallResult{}, fmt.Errorf("查询任务结果失败: %w", err)
	}

	var companies []company.CompanyInfo
	if err := json.Unmarshal([]byte(result.Data), &companies); err != nil {
		return mcpToolCallResult{}, fmt.Errorf("解析结果数据失败: %w", err)
	}
	companies = company.NormalizeCompanyInfos(companies)

	return newMCPToolResult(gin.H{
		"taskId":  taskID,
		"results": companies,
		"total":   len(companies),
	}), nil
}

func loadWebScanTaskPayload(taskID string, version int) (database.VulnScanTask, gin.H, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return database.VulnScanTask{}, nil, fmt.Errorf("taskId 不能为空")
	}

	db, err := database.GetDB()
	if err != nil {
		return database.VulnScanTask{}, nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	task, err := loadLatestWebScanTask(db, taskID, version)
	if err != nil {
		return database.VulnScanTask{}, nil, err
	}

	var targets []string
	_ = json.Unmarshal([]byte(task.Targets), &targets)
	var taskConfig map[string]interface{}
	_ = json.Unmarshal([]byte(task.Config), &taskConfig)
	if taskConfig == nil {
		taskConfig = map[string]interface{}{}
	}
	taskConfig["targets"] = targets

	var fingerprintCount int64
	var portCount int64
	var vulnerabilityCount int64
	var jsVulnerabilityCount int64
	var jsAssetCount int64
	var jsResourceCount int64
	var jsAPIResourceCount int64
	var jsSiteTreeCount int64
	var dirResultCount int64
	var criticalCount int64
	var highCount int64
	var mediumCount int64
	var lowCount int64
	var infoCount int64
	var jsCriticalCount int64
	var jsHighCount int64
	var jsMediumCount int64
	var jsLowCount int64
	var jsInfoCount int64
	var subdomainCount int64

	db.Model(&database.FingerprintInfo{}).Where("task_id = ? AND version = ?", task.ID, task.Version).Count(&fingerprintCount)
	db.Model(&database.PortScanResult{}).Where("task_id = ? AND version = ?", task.ID, task.Version).Count(&portCount)
	db.Model(&database.VulnScanDirResult{}).Where("task_id = ? AND version = ?", task.ID, task.Version).Count(&dirResultCount)
	db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ?", task.ID, task.Version).Count(&vulnerabilityCount)
	db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ? AND LOWER(severity) = ?", task.ID, task.Version, "critical").Count(&criticalCount)
	db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ? AND LOWER(severity) = ?", task.ID, task.Version, "high").Count(&highCount)
	db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ? AND LOWER(severity) = ?", task.ID, task.Version, "medium").Count(&mediumCount)
	db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ? AND LOWER(severity) = ?", task.ID, task.Version, "low").Count(&lowCount)
	db.Model(&database.VulnerabilityInfo{}).Where("task_id = ? AND version = ? AND LOWER(severity) = ?", task.ID, task.Version, "info").Count(&infoCount)
	db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ?", task.ID, task.Version).Count(&jsVulnerabilityCount)
	db.Model(&database.VulnScanJSAsset{}).Where("task_id = ? AND version = ?", task.ID, task.Version).Count(&jsAssetCount)
	db.Model(&database.VulnScanJSResource{}).Where("task_id = ? AND version = ?", task.ID, task.Version).Count(&jsResourceCount)
	db.Model(&database.VulnScanJSAPIResource{}).Where("task_id = ? AND version = ?", task.ID, task.Version).Count(&jsAPIResourceCount)
	db.Model(&database.VulnScanJSSiteTree{}).Where("task_id = ? AND version = ?", task.ID, task.Version).Count(&jsSiteTreeCount)
	db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ? AND LOWER(level) = ?", task.ID, task.Version, "critical").Count(&jsCriticalCount)
	db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ? AND LOWER(level) = ?", task.ID, task.Version, "high").Count(&jsHighCount)
	db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ? AND LOWER(level) = ?", task.ID, task.Version, "medium").Count(&jsMediumCount)
	db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ? AND LOWER(level) = ?", task.ID, task.Version, "low").Count(&jsLowCount)
	db.Model(&database.VulnScanJSVulnerability{}).Where("task_id = ? AND version = ? AND LOWER(level) = ?", task.ID, task.Version, "info").Count(&jsInfoCount)
	if task.ScanType == "subdomain" {
		db.Model(&database.SubdomainScanResult{}).Where("task_id = ?", task.ID).Count(&subdomainCount)
	}

	totalVulnCount := vulnerabilityCount + jsVulnerabilityCount

	payload := gin.H{
		"id":               task.ID,
		"taskName":         task.TaskName,
		"scanMode":         task.ScanMode,
		"scanType":         task.ScanType,
		"status":           task.Status,
		"currentStage":     task.CurrentStage,
		"progress":         task.Progress,
		"totalCount":       task.TotalCount,
		"errorCount":       task.ErrorCount,
		"version":          task.Version,
		"config":           redactMapSecrets(taskConfig),
		"errorMsg":         task.ErrorMsg,
		"createdAt":        task.CreatedAt,
		"updatedAt":        task.UpdatedAt,
		"startedAt":        task.StartedAt,
		"completedAt":      task.CompletedAt,
		"fingerprintCount": int(fingerprintCount),
		"portCount":        int(portCount),
		"subdomainCount":   int(subdomainCount),
		"dirResultCount":   int(dirResultCount),
		"vulnCount":        int(totalVulnCount),
		"webVulnCount":     int(vulnerabilityCount),
		"jsVulnCount":      int(jsVulnerabilityCount),
		"jsAssetCount":     int(jsAssetCount),
		"jsResourceCount":  int(jsResourceCount),
		"jsAPIRecordCount": int(jsAPIResourceCount),
		"jsSiteTreeCount":  int(jsSiteTreeCount),
		"resultCount":      int(fingerprintCount + portCount + dirResultCount + totalVulnCount + subdomainCount + jsAssetCount + jsResourceCount + jsAPIResourceCount + jsSiteTreeCount),
		"severity": gin.H{
			"critical": int(criticalCount + jsCriticalCount),
			"high":     int(highCount + jsHighCount),
			"medium":   int(mediumCount + jsMediumCount),
			"low":      int(lowCount + jsLowCount),
			"info":     int(infoCount + jsInfoCount),
		},
	}

	return task, payload, nil
}

func loadLatestWebScanTask(db *gorm.DB, taskID string, version int) (database.VulnScanTask, error) {
	var task database.VulnScanTask
	query := db.Where("id = ?", taskID)
	if version > 0 {
		query = query.Where("version = ?", version)
	}
	if err := query.Order("version DESC").First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return database.VulnScanTask{}, fmt.Errorf("任务不存在: %s", taskID)
		}
		return database.VulnScanTask{}, fmt.Errorf("查询任务失败: %w", err)
	}
	return task, nil
}

func mergeMCPWebScanConfig(config map[string]interface{}, args mcpWebScanCreateTaskArgs) {
	setIfInt(config, "thread", args.Thread)
	threads := args.Threads
	if threads <= 0 {
		threads = args.Thread
	}
	setIfInt(config, "threads", threads)
	setIfInt(config, "workers", args.Workers)
	setIfInt(config, "nucleiThreads", args.NucleiThreads)
	if args.Timeout > 0 {
		config["timeout"] = args.Timeout
	}
	setIfStringSlice(config, "headers", args.Headers)
	setIfStringSlice(config, "template", args.Template)
	setIfStringSlice(config, "templateFiles", args.TemplateFiles)
	setIfStringSlice(config, "tags", args.Tags)
	setIfBoolPtr(config, "noPoc", args.NoPoc)
	setIfBoolPtr(config, "callNuclei", args.CallNuclei)
	setIfBoolPtr(config, "screenshot", args.Screenshot)
	setIfBoolPtr(config, "deepScan", args.DeepScan)
	setIfBoolPtr(config, "rootPath", args.RootPath)
	setIfBoolPtr(config, "skipNucleiWithoutTags", args.SkipNucleiWithoutTags)
	setIfBoolPtr(config, "skipNucleiWithWAF", args.SkipNucleiWithWAF)
	setIfBoolPtr(config, "disableMaxHostErr", args.DisableMaxHostErr)
	setIfString(config, "proxy", args.Proxy)
	setIfString(config, "networkCard", args.NetworkCard)
	setIfString(config, "customHeaders", args.CustomHeaders)
	setIfInt(config, "webscanOption", args.WebscanOption)
	setIfBoolPtr(config, "oobEnabled", args.OOBEnabled)
	setIfString(config, "interactshUrl", args.InteractshURL)
	setIfString(config, "interactshToken", args.InteractshToken)
	setIfString(config, "portGroupId", args.PortGroupID)
	setIfString(config, "portRange", args.PortRange)
	setIfBoolPtr(config, "aliveDetection", args.AliveDetection)
	setIfBoolPtr(config, "subnetDiscoveryEnabled", args.SubnetDiscoveryEnabled)
	setIfString(config, "subnetDiscoveryMode", args.SubnetDiscoveryMode)
	setIfString(config, "subnetDiscoveryHostSuffixes", args.SubnetDiscoveryHostSuffixes)
	if args.SubnetDiscoveryRandomSamples != nil {
		config["subnetDiscoveryRandomSamples"] = *args.SubnetDiscoveryRandomSamples
	}
	setIfBoolPtr(config, "tcpHeuristicEnabled", args.TCPHeuristicEnabled)
	setIfString(config, "tcpHeuristicPorts", args.TCPHeuristicPorts)
	setIfInt(config, "tcpHeuristicTimeoutMs", args.TCPHeuristicTimeoutMs)
	setIfString(config, "tcpDiscoveryMode", args.TCPDiscoveryMode)
	setIfInt(config, "synRate", args.SynRate)
	setIfInt(config, "synDiscoveryAttempts", args.SynDiscoveryAttempts)
	setIfString(config, "method", args.Method)
	setIfStringSlice(config, "paths", args.Paths)
	setIfStringSlice(config, "dictPaths", args.DictPaths)
	setIfStringSlice(config, "dictNames", args.DictNames)
	setIfStringSlice(config, "exts", args.Exts)
	setIfIntSlice(config, "statusCodeExclude", args.StatusCodeExclude)
	setIfString(config, "bodyExclude", args.BodyExclude)
	setIfInt(config, "bodyLengthExcludeTimes", args.BodyLengthExcludeTimes)
	setIfBoolPtr(config, "redirect", args.Redirect)
	setIfInt(config, "interval", args.Interval)
	setIfString(config, "customHeader", args.CustomHeader)
	setIfBoolPtr(config, "backupscan", args.Backupscan)
	setIfInt(config, "recursive", args.Recursive)
	setIfString(config, "username", args.Username)
	setIfString(config, "password", args.Password)
	setIfBoolPtr(config, "defaultDict", args.DefaultDict)
	setIfInt(config, "targetTimeout", args.TargetTimeout)
	setIfInt(config, "sshMaxTimeoutErrors", args.SSHMaxTimeoutErrors)
}

func companyTaskPayload(task *database.CompanyInfoTask) gin.H {
	var taskConfig map[string]interface{}
	_ = json.Unmarshal([]byte(task.Config), &taskConfig)
	return gin.H{
		"id":           task.ID,
		"taskName":     task.TaskName,
		"status":       task.Status,
		"progress":     task.Progress,
		"totalCount":   task.TotalCount,
		"currentCount": task.CurrentCount,
		"config":       redactMapSecrets(taskConfig),
		"errorMsg":     task.ErrorMsg,
		"startedAt":    task.StartedAt,
		"completedAt":  task.CompletedAt,
		"createdAt":    task.CreatedAt,
		"updatedAt":    task.UpdatedAt,
	}
}

func normalizeMCPPagination(page int, pageSize int, defaultPageSize int, maxPageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if maxPageSize > 0 && pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}

func effectiveMCPPageSize(args mcpWebScanResultsArgs) int {
	if args.PageSize > 0 {
		return args.PageSize
	}
	return args.Limit
}

func normalizeMCPScanResultType(resultType string, scanType string) string {
	normalized := strings.ToLower(strings.TrimSpace(resultType))
	switch normalized {
	case "", "auto":
		switch strings.ToLower(strings.TrimSpace(scanType)) {
		case "subdomain":
			return "subdomains"
		case "portscan":
			return "ports"
		case "dirscan", "dirsearch":
			return "dir_results"
		case "jsscan":
			return "js_assets"
		default:
			return "vulnerabilities"
		}
	case "port", "ports", "portscan":
		return "ports"
	case "dir", "dirs", "dirscan", "dirsearch", "dir_results", "directory", "directories":
		return "dir_results"
	case "js_asset", "js_assets", "asset", "assets":
		return "js_assets"
	case "js_resource", "js_resources", "script", "scripts":
		return "js_resources"
	case "js_api", "js_api_resource", "js_api_resources", "api_resource", "api_resources", "api_records":
		return "js_api_resources"
	case "js_site_tree", "site_tree", "sitetree":
		return "js_site_tree"
	case "js", "jsscan":
		return "js_assets"
	case "js_vulnerability", "js_vulnerabilities":
		return "js_vulnerabilities"
	case "subdomain", "subdomains":
		return "subdomains"
	case "vuln", "vulns", "vulnerability", "vulnerabilities", "web_vulnerabilities", "crack":
		return "vulnerabilities"
	default:
		return normalized
	}
}

func normalizeMCPJSAssetType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	switch strings.ToLower(value) {
	case "api", "api_route", "api-route", "apiroute", "api_routes", "api-routes", "apiroutes":
		return "apiRoute"
	case "api_root", "api-root", "apiroot", "api_roots", "api-roots", "apiroots":
		return "apiRoot"
	case "email", "mail":
		return "email"
	case "phone", "mobile":
		return "phone"
	case "id", "idcard", "id_card", "id-card":
		return "idCard"
	case "sensitive", "secret", "token", "key":
		return "sensitive"
	case "url", "ipurl", "ip_url", "ip-url":
		return "ipUrl"
	case "all":
		return "all"
	default:
		return value
	}
}

func expandMCPJSAssetTypes(value string) []string {
	value = normalizeMCPJSAssetType(value)
	switch value {
	case "apiRoute":
		return []string{"apiRoute", "api_route", "api-route"}
	case "apiRoot":
		return []string{"apiRoot", "api_root", "api-root"}
	case "idCard":
		return []string{"idCard", "id_card", "id-card"}
	case "ipUrl":
		return []string{"ipUrl", "ip_url", "ip-url"}
	default:
		return []string{value}
	}
}

func truncateMCPText(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "...[truncated]"
}

func countMCPJSAPIRoutesInTree(treeData string) int {
	var parsed any
	if err := json.Unmarshal([]byte(treeData), &parsed); err != nil {
		return 0
	}
	count := 0
	var walk func(any)
	walk = func(value any) {
		switch current := value.(type) {
		case map[string]any:
			for key, item := range current {
				lowerKey := strings.ToLower(strings.TrimSpace(key))
				if lowerKey == "apiroutes" || lowerKey == "api_routes" {
					switch routes := item.(type) {
					case []any:
						count += len(routes)
					case []string:
						count += len(routes)
					case string:
						if strings.TrimSpace(routes) != "" {
							count++
						}
					}
				}
				walk(item)
			}
		case []any:
			for _, item := range current {
				walk(item)
			}
		}
	}
	walk(parsed)
	return count
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func parsePositiveInt(value string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("not a positive integer: %d", parsed)
	}
	return parsed, nil
}

func normalizeNonEmptyStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func copyStringInterfaceMap(input map[string]interface{}) map[string]interface{} {
	output := make(map[string]interface{})
	for key, value := range input {
		output[key] = value
	}
	return output
}

func setIfInt(config map[string]interface{}, key string, value int) {
	if value > 0 {
		config[key] = value
	}
}

func setIfString(config map[string]interface{}, key string, value string) {
	if value = strings.TrimSpace(value); value != "" {
		config[key] = value
	}
}

func setIfStringSlice(config map[string]interface{}, key string, value []string) {
	if normalized := normalizeNonEmptyStrings(value); len(normalized) > 0 {
		config[key] = normalized
	}
}

func setIfIntSlice(config map[string]interface{}, key string, value []int) {
	if len(value) > 0 {
		config[key] = value
	}
}

func setIfBoolPtr(config map[string]interface{}, key string, value *bool) {
	if value != nil {
		config[key] = *value
	}
}

func redactMapSecrets(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return map[string]interface{}{}
	}
	output := make(map[string]interface{}, len(input))
	for key, value := range input {
		if isSensitiveConfigKey(key) {
			if strings.TrimSpace(fmt.Sprint(value)) == "" {
				output[key] = ""
			} else {
				output[key] = "***"
			}
			continue
		}
		if child, ok := value.(map[string]interface{}); ok {
			output[key] = redactMapSecrets(child)
			continue
		}
		output[key] = value
	}
	return output
}

func isSensitiveConfigKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(lower, "key") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "cookie") ||
		strings.Contains(lower, "password") ||
		strings.Contains(lower, "secret")
}
