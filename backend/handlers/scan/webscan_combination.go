package scan

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hephaestus-backend/handlers/platform"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/portscan"
	"hephaestus-backend/pkg/scantask"
	"hephaestus-backend/pkg/taskstate"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type combinationTaskRequest struct {
	Name     string                    `json:"name"`
	Targets  []string                  `json:"targets"`
	Nodes    []combinationPipelineNode `json:"nodes"`
	ScanMode string                    `json:"scanMode"`
	AgentID  string                    `json:"agentId"`
	Schedule json.RawMessage           `json:"schedule"`
	Proxy    string                    `json:"proxy,omitempty"`
}

type combinationPipelineNode struct {
	ID         string                 `json:"id"`
	ModuleID   string                 `json:"moduleId"`
	StageIndex int                    `json:"stageIndex"`
	LaneIndex  int                    `json:"laneIndex,omitempty"`
	Config     map[string]interface{} `json:"config,omitempty"`
}

// normalizeCombinationProxyURL validates the proxy selected for a combination
// task. An empty value means direct access.
func normalizeCombinationProxyURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("代理地址格式错误")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "socks5":
		return raw, nil
	default:
		return "", fmt.Errorf("不支持的代理协议: %s", parsed.Scheme)
	}
}

// injectCombinationProxy applies the task-level proxy to every module that
// accepts a proxy config. A node-level proxy value always wins, including an
// explicit empty string (direct access).
func injectCombinationProxy(nodes []combinationPipelineNode, proxyURL string) {
	if proxyURL == "" {
		return
	}
	for i := range nodes {
		if nodes[i].Config == nil {
			nodes[i].Config = make(map[string]interface{})
		}
		if _, exists := nodes[i].Config["proxy"]; exists {
			continue
		}
		nodes[i].Config["proxy"] = proxyURL
	}
}

type combinationTargetInputType string

const (
	combinationTargetInputTypeNetwork combinationTargetInputType = "network"
	combinationTargetInputTypeDomain  combinationTargetInputType = "domain"
	combinationTargetInputTypeWeb     combinationTargetInputType = "web"
	combinationTargetInputTypeService combinationTargetInputType = "service"
)

var combinationHostnameLabelPattern = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

func validateCombinationNodeRules(nodes []combinationPipelineNode) error {
	if len(nodes) == 0 {
		return nil
	}

	stageNodeCount := make(map[int]int)
	minStage := nodes[0].StageIndex
	portScanStage := -1
	subdomainStage := -1

	for _, node := range nodes {
		stageNodeCount[node.StageIndex]++
		if node.StageIndex < minStage {
			minStage = node.StageIndex
		}
		if node.ModuleID == "portscan" {
			portScanStage = node.StageIndex
		}
		if node.ModuleID == "subdomain" {
			subdomainStage = node.StageIndex
		}
	}

	if portScanStage != -1 {
		if subdomainStage != -1 {
			if subdomainStage >= portScanStage {
				return fmt.Errorf("子域名收集必须先于端口扫描执行")
			}
		} else if portScanStage != minStage {
			return fmt.Errorf("端口扫描必须先于所有其他任务执行（位于第一阶段）")
		}
		if stageNodeCount[portScanStage] > 1 {
			return fmt.Errorf("端口扫描必须独占一个阶段，不能与其他任务并行")
		}
	}

	if _, err := detectCombinationTargetInputType(nodes); err != nil {
		return err
	}

	return nil
}

func combinationModuleTargetInputType(moduleID string) combinationTargetInputType {
	switch strings.ToLower(strings.TrimSpace(moduleID)) {
	case "subdomain":
		return combinationTargetInputTypeDomain
	case "portscan":
		return combinationTargetInputTypeNetwork
	case "crack":
		return combinationTargetInputTypeService
	case "webscan", "dirscan", "jsscan":
		return combinationTargetInputTypeWeb
	default:
		return ""
	}
}

func detectCombinationTargetInputType(nodes []combinationPipelineNode) (combinationTargetInputType, error) {
	if len(nodes) == 0 {
		return "", nil
	}

	minStage := nodes[0].StageIndex
	inputTypes := make(map[combinationTargetInputType]struct{})
	for _, node := range nodes {
		if node.StageIndex < minStage {
			minStage = node.StageIndex
		}
	}
	for _, node := range nodes {
		if node.StageIndex != minStage {
			continue
		}
		if inputType := combinationModuleTargetInputType(node.ModuleID); inputType != "" {
			inputTypes[inputType] = struct{}{}
		}
	}

	switch len(inputTypes) {
	case 0:
		return "", nil
	case 1:
		for inputType := range inputTypes {
			return inputType, nil
		}
	}

	return "", fmt.Errorf("第一阶段包含不同目标类型的模块，无法共用同一批目标")
}

func validateCombinationTargetsByInputType(targets []string, inputType combinationTargetInputType) []string {
	if inputType == "" {
		return nil
	}

	invalid := make([]string, 0)
	for _, raw := range targets {
		target := strings.TrimSpace(raw)
		if target == "" {
			continue
		}

		valid := false
		switch inputType {
		case combinationTargetInputTypeDomain:
			valid = isValidCombinationDomainTarget(target)
		case combinationTargetInputTypeNetwork:
			valid = isValidCombinationNetworkTarget(target)
		case combinationTargetInputTypeWeb:
			valid = isValidCombinationWebTarget(target)
		case combinationTargetInputTypeService:
			valid = isValidCombinationServiceTarget(target)
		}

		if !valid {
			invalid = append(invalid, target)
		}
	}

	return invalid
}

func isValidCombinationDomainTarget(target string) bool {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" || strings.Contains(trimmed, "://") || strings.ContainsAny(trimmed, "/?#@:") {
		return false
	}
	if net.ParseIP(strings.Trim(trimmed, "[]")) != nil {
		return false
	}
	labels := strings.Split(strings.Trim(trimmed, "."), ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !combinationHostnameLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

func isValidCombinationNetworkTarget(target string) bool {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "://") {
		return false
	}
	if ip := net.ParseIP(strings.Trim(trimmed, "[]")); ip != nil {
		return true
	}
	if strings.Contains(trimmed, "/") {
		_, _, err := net.ParseCIDR(trimmed)
		return err == nil
	}
	if strings.ContainsAny(trimmed, "?#") || strings.Contains(trimmed, ":") {
		return false
	}

	labels := strings.Split(trimmed, ".")
	for _, label := range labels {
		if !combinationHostnameLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

func isValidCombinationWebTarget(target string) bool {
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err != nil || parsed == nil || parsed.Hostname() == "" {
		return false
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	return scheme == "http" || scheme == "https"
}

func isValidCombinationServiceTarget(target string) bool {
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err != nil || parsed == nil || parsed.Hostname() == "" || parsed.Port() == "" {
		return false
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	for _, protocol := range portscan.SupportedCrackProtocols() {
		if scheme == strings.ToLower(strings.TrimSpace(protocol)) {
			return true
		}
	}
	return false
}

func describeCombinationTargetInputType(inputType combinationTargetInputType) string {
	switch inputType {
	case combinationTargetInputTypeDomain:
		return "根域名"
	case combinationTargetInputTypeNetwork:
		return "IP / 域名 / IP 段"
	case combinationTargetInputTypeWeb:
		return "HTTP/HTTPS URL"
	case combinationTargetInputTypeService:
		return "协议://主机:端口"
	default:
		return "目标"
	}
}

var combinationDependencies = map[string][]string{
	// 组合任务改为“显式阶段优先”，不再强制模块前置依赖。
	// 用户可通过 stageIndex 控制串并行：
	// - 同阶段并行
	// - 下一阶段等待上一阶段全部完成
}

var moduleDisplayName = map[string]string{
	"subdomain": "子域名收集",
	"portscan":  "端口扫描",
	"webscan":   "Web 漏洞扫描",
	"dirscan":   "目录探测",
	"jsscan":    "JS 分析",
	"crack":     "密码喷洒",
}

type combinedTaskRow struct {
	ID             string         `gorm:"column:id"`
	TaskName       sql.NullString `gorm:"column:task_name"`
	CreatedByAI    bool           `gorm:"column:created_by_ai"`
	ScanType       string         `gorm:"column:scan_type"`
	Status         string         `gorm:"column:status"`
	PauseRequested bool           `gorm:"column:pause_requested"`
	Progress       int            `gorm:"column:progress"`
	TotalCount     int            `gorm:"column:total_count"`
	CurrentCount   int            `gorm:"column:current_count"`
	ErrorMsg       sql.NullString `gorm:"column:error_msg"`
	CreatedAt      time.Time      `gorm:"column:created_at"`
	UpdatedAt      time.Time      `gorm:"column:updated_at"`
	CompletedAt    *time.Time     `gorm:"column:completed_at"`
	Version        int            `gorm:"column:version"`
	CurrentStage   sql.NullString `gorm:"column:current_stage"`
	CheckpointJSON sql.NullString `gorm:"column:checkpoint_json"`
}

// extractStageEvents 从 checkpoint_json 中解析阶段事件时间线
// （meta.stageEvents，由扫描器的 stageTracker 写入），供前端悬停进度条展示。
// 解析失败或不存在时返回 nil，不影响接口主流程。
func extractStageEvents(checkpointJSON string) []map[string]interface{} {
	if checkpointJSON == "" {
		return nil
	}
	checkpoint := taskstate.ParseCheckpoint(checkpointJSON)
	raw, ok := checkpoint.Meta["stageEvents"]
	if !ok {
		return nil
	}
	list, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	events := make([]map[string]interface{}, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]interface{}); ok {
			events = append(events, m)
		}
	}
	return events
}

func normalizeTargetsList(inputs []string) []string {
	unique := make(map[string]struct{})
	result := make([]string, 0, len(inputs))
	for _, value := range inputs {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := unique[trimmed]; exists {
			continue
		}
		unique[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

// CreateCombinationTaskHandler 保存组合任务并按顺序执行
func CreateCombinationTaskHandler(c *gin.Context) {

	var req combinationTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数格式错误",
		})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = fmt.Sprintf("组合任务-%s", time.Now().Format("20060102-150405"))
	}

	targets := normalizeTargetsList(req.Targets)
	if len(targets) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请至少提供一个扫描目标",
		})
		return
	}

	if len(req.Nodes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请至少选择一个扫描模块",
		})
		return
	}

	normalizedNodes := make([]combinationPipelineNode, 0, len(req.Nodes))
	selectedModules := make(map[string]bool)

	for _, node := range req.Nodes {
		module := strings.ToLower(strings.TrimSpace(node.ModuleID))
		if module == "" {
			continue
		}
		if _, ok := scanTypeHandlers[module]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   fmt.Sprintf("模块 %s 暂不支持", module),
			})
			return
		}
		if selectedModules[module] {
			continue
		}
		stageIndex := node.StageIndex
		if stageIndex < 0 {
			stageIndex = 0
		}

		normalizedNodes = append(normalizedNodes, combinationPipelineNode{
			ID:         strings.TrimSpace(node.ID),
			ModuleID:   module,
			StageIndex: stageIndex,
			LaneIndex:  node.LaneIndex,
			Config:     node.Config,
		})
		selectedModules[module] = true
	}

	if len(normalizedNodes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "有效的模块顺序为空",
		})
		return
	}

	sort.SliceStable(normalizedNodes, func(i, j int) bool {
		return normalizedNodes[i].StageIndex < normalizedNodes[j].StageIndex
	})

	if err := validateCombinationNodeRules(normalizedNodes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	inputType, err := detectCombinationTargetInputType(normalizedNodes)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	if invalidTargets := validateCombinationTargetsByInputType(targets, inputType); len(invalidTargets) > 0 {
		samples := invalidTargets
		if len(samples) > 3 {
			samples = samples[:3]
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": fmt.Sprintf(
				"当前第一阶段要求输入类型为 %s，以下目标不匹配: %s",
				describeCombinationTargetInputType(inputType),
				strings.Join(samples, ", "),
			),
		})
		return
	}

	proxyURL, err := normalizeCombinationProxyURL(req.Proxy)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	injectCombinationProxy(normalizedNodes, proxyURL)

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	scanMode, err := scantask.NormalizeModeForDB(db, req.ScanMode)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	assignedAgentID := strings.TrimSpace(req.AgentID)
	if scanMode == "local" {
		assignedAgentID = ""
	} else if assignedAgentID != "" {
		var node database.ScanAgentNode
		if err := db.Where("agent_id = ?", assignedAgentID).First(&node).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"error":   fmt.Sprintf("指定的 agentId 不存在: %s", assignedAgentID),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "查询指定 agent 失败: " + err.Error(),
			})
			return
		}
		if !node.Enabled {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   fmt.Sprintf("指定的 agent 已禁用: %s", assignedAgentID),
			})
			return
		}
	}

	schedule := scantask.ScheduleConfig{}
	if len(req.Schedule) > 0 {
		var rawSchedule map[string]interface{}
		if err := json.Unmarshal(req.Schedule, &rawSchedule); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid schedule"})
			return
		}
		schedule, err = scantask.ParseScheduleConfigFromMap(rawSchedule)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
	}
	now := time.Now()
	if err := scantask.ValidateScheduleConfig(schedule, now); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	combinationID := database.GenerateNanoid()
	parentTaskID := database.GenerateNanoid()
	targetsJSON, _ := json.Marshal(targets)
	nodesJSON, _ := json.Marshal(normalizedNodes)
	ctxState := newCombinationContext(targets)
	ctxState.ensureStepLogs(normalizedNodes, parentTaskID)

	combination := &database.CombinationTask{
		ID:         combinationID,
		Name:       name,
		Targets:    string(targetsJSON),
		Nodes:      string(nodesJSON),
		Status:     "pending",
		TotalSteps: len(normalizedNodes),
		CreatedBy:  platform.Username(c),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Context:    ctxState.serialize(),
	}

	parentConfig := map[string]interface{}{
		"targets":           targets,
		"combinationNodes":  normalizedNodes,
		"combinationTaskId": combinationID,
		"scanMode":          scanMode,
		"agentId":           assignedAgentID,
	}
	if proxyURL != "" {
		parentConfig["proxy"] = proxyURL
	}
	parentConfigJSON, _ := json.Marshal(parentConfig)

	parentTask := &database.VulnScanTask{
		ID:                  parentTaskID,
		TaskName:            name,
		ScanMode:            scanMode,
		ScanType:            "combination",
		AssignedAgentID:     assignedAgentID,
		ParentCombinationID: combinationID,
		Targets:             string(targetsJSON),
		Status:              "pending",
		Progress:            0,
		TotalCount:          len(targets),
		Version:             1,
		Config:              string(parentConfigJSON),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(combination).Error; err != nil {
			return err
		}
		if err := tx.Create(parentTask).Error; err != nil {
			return err
		}
		if err := scantask.UpsertScanSchedule(tx, parentTaskID, schedule, now); err != nil {
			return err
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存组合任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"combinationId": combinationID,
			"parentTaskId":  parentTaskID,
			"scanMode":      scanMode,
			"agentId":       assignedAgentID,
			"taskName":      name,
		},
	})
}

func markCombinationFailed(db *gorm.DB, combinationID string, err error) {
	logger.Warning("组合任务 %s 失败: %v", combinationID, err)
	now := time.Now()
	db.Model(&database.CombinationTask{}).
		Where("id = ? AND status NOT IN ?", combinationID, []string{taskstate.StatusCompleted, taskstate.StatusCancelled, taskstate.StatusPaused}).
		Updates(map[string]interface{}{
			"status":       "failed",
			"error_msg":    err.Error(),
			"completed_at": &now,
			"updated_at":   now,
		})

	db.Model(&database.VulnScanTask{}).
		Where("parent_combination_id = ? AND status NOT IN ?", combinationID, []string{taskstate.StatusCompleted, taskstate.StatusCancelled, taskstate.StatusPaused, taskstate.StatusDeleting}).
		Updates(map[string]interface{}{
			"status":       "failed",
			"error_msg":    err.Error(),
			"completed_at": &now,
			"updated_at":   now,
		})
}

func deriveTargetsFromPortScanResults(db *gorm.DB, taskID string) ([]string, []string, error) {
	var task database.VulnScanTask
	if err := db.Where("id = ?", taskID).Order("version DESC").First(&task).Error; err != nil {
		return nil, nil, fmt.Errorf("获取端口扫描任务信息失败: %w", err)
	}

	var results []database.PortScanResult
	if err := db.Where("task_id = ? AND version = ?", taskID, task.Version).Find(&results).Error; err != nil {
		return nil, nil, fmt.Errorf("读取端口扫描结果失败: %w", err)
	}

	var assets []database.PortScanResultAsset
	if err := db.Where("task_id = ? AND version = ?", taskID, task.Version).Find(&assets).Error; err != nil {
		return nil, nil, fmt.Errorf("读取端口资产关系失败: %w", err)
	}
	assetsByResultID := make(map[uint][]database.PortScanResultAsset)
	for _, asset := range assets {
		if strings.TrimSpace(asset.Subdomain) == "" {
			continue
		}
		assetsByResultID[asset.ResultID] = append(assetsByResultID[asset.ResultID], asset)
	}

	httpTargets := make(map[string]struct{})
	crackTargets := make(map[string]struct{})

	crackProtocols := make(map[string]struct{})
	for _, proto := range portscan.SupportedCrackProtocols() {
		crackProtocols[proto] = struct{}{}
	}

	for _, row := range results {
		if scheme, ok := classifyWebProtocol(row.Protocol, row.Port); ok {
			if aliases := assetsByResultID[row.ID]; len(aliases) > 0 {
				for _, alias := range aliases {
					host := strings.TrimSpace(alias.Subdomain)
					if host == "" {
						continue
					}
					url := fmt.Sprintf("%s://%s:%d", scheme, host, row.Port)
					httpTargets[url] = struct{}{}
				}
			} else {
				url := fmt.Sprintf("%s://%s:%d", scheme, row.Host, row.Port)
				httpTargets[url] = struct{}{}
			}
		}

		normalized := strings.ToLower(row.Protocol)
		if _, ok := crackProtocols[normalized]; ok {
			url := fmt.Sprintf("%s://%s:%d", normalized, row.Host, row.Port)
			crackTargets[url] = struct{}{}
			continue
		}

		switch row.Port {
		case 110, 995:
			url := fmt.Sprintf("%s://%s:%d", "pop3", row.Host, row.Port)
			crackTargets[url] = struct{}{}
		}
	}

	return setToSortedSlice(httpTargets), setToSortedSlice(crackTargets), nil
}

var httpPortHints = map[int]string{
	80:    "http",
	81:    "http",
	3000:  "http",
	7001:  "http",
	8000:  "http",
	8080:  "http",
	8081:  "http",
	8888:  "http",
	443:   "https",
	8443:  "https",
	9443:  "https",
	10443: "https",
}

func classifyWebProtocol(proto string, port int) (string, bool) {
	p := strings.ToLower(proto)
	switch {
	case strings.Contains(p, "https"):
		return "https", true
	case strings.Contains(p, "ssl/http"):
		return "https", true
	case strings.Contains(p, "http"):
		return "http", true
	}

	if scheme, ok := httpPortHints[port]; ok {
		return scheme, true
	}
	return "", false
}

func setToSortedSlice(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func combinationResultTaskIDs(db *gorm.DB, task database.VulnScanTask, version int) ([]string, error) {
	ids := []string{task.ID}
	if task.ScanType != "combination" || strings.TrimSpace(task.ParentCombinationID) == "" {
		return ids, nil
	}

	var childIDs []string
	if err := db.Model(&database.VulnScanTask{}).
		Where("parent_combination_id = ? AND version = ? AND scan_type <> ?", task.ParentCombinationID, version, "combination").
		Pluck("id", &childIDs).Error; err != nil {
		return nil, err
	}

	seen := map[string]struct{}{task.ID: {}}
	for _, id := range childIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		ids = append(ids, id)
		seen[id] = struct{}{}
	}
	return ids, nil
}
