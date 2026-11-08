package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	mcpWorkbenchFactSource = "workbench_fact"
)

type mcpWorkbenchTaskUpdateArgs struct {
	TaskID          string   `json:"taskId"`
	Status          string   `json:"status"`
	Summary         string   `json:"summary"`
	Objective       string   `json:"objective"`
	SuccessCriteria string   `json:"successCriteria"`
	DeliveryFormat  []string `json:"deliveryFormat"`
}

type mcpWorkbenchStepAddArgs struct {
	TaskID           string   `json:"taskId"`
	ParentStepID     string   `json:"parentStepId"`
	StepType         string   `json:"stepType"`
	Title            string   `json:"title"`
	Rationale        string   `json:"rationale"`
	RiskLevel        string   `json:"riskLevel"`
	ApprovalRequired *bool    `json:"approvalRequired"`
	DependsOn        []string `json:"dependsOn"`
}

type mcpWorkbenchFactUpsertArgs struct {
	TaskID     string                 `json:"taskId"`
	StepID     string                 `json:"stepId"`
	FactKey    string                 `json:"factKey"`
	Source     string                 `json:"source"`
	TargetRef  string                 `json:"targetRef"`
	Summary    string                 `json:"summary"`
	Body       string                 `json:"body"`
	Confidence string                 `json:"confidence"`
	Metadata   map[string]interface{} `json:"metadata"`
}

type mcpWorkbenchFactSearchArgs struct {
	TaskID string `json:"taskId"`
	Query  string `json:"query"`
	Limit  int    `json:"limit"`
}

type mcpWorkbenchEvidenceAddArgs struct {
	TaskID     string                 `json:"taskId"`
	StepID     string                 `json:"stepId"`
	SourceType string                 `json:"sourceType"`
	SourceID   string                 `json:"sourceId"`
	TargetRef  string                 `json:"targetRef"`
	Title      string                 `json:"title"`
	Summary    string                 `json:"summary"`
	RiskLevel  string                 `json:"riskLevel"`
	Confidence string                 `json:"confidence"`
	Status     string                 `json:"status"`
	Reference  string                 `json:"reference"`
	Metadata   map[string]interface{} `json:"metadata"`
}

type mcpWorkbenchHypothesisUpdateArgs struct {
	TaskID       string   `json:"taskId"`
	HypothesisID string   `json:"hypothesisId"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	EvidenceRefs []string `json:"evidenceRefs"`
	Status       string   `json:"status"`
	Confidence   string   `json:"confidence"`
}

type mcpWorkbenchScopeCheckArgs struct {
	TaskID string `json:"taskId"`
	Target string `json:"target"`
}

func mcpCallWorkbenchTaskUpdate(args mcpWorkbenchTaskUpdateArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}

	updates := map[string]interface{}{"updated_at": time.Now()}
	if status := normalizeMCPWorkbenchTaskStatus(args.Status); status != "" {
		updates["status"] = status
		if isMCPWorkbenchTerminalStatus(status) {
			now := time.Now()
			updates["completed_at"] = &now
		}
	}
	if summary := strings.TrimSpace(args.Summary); summary != "" {
		updates["summary"] = summary
	}
	if objective := strings.TrimSpace(args.Objective); objective != "" {
		updates["objective"] = objective
	}
	if successCriteria := strings.TrimSpace(args.SuccessCriteria); successCriteria != "" {
		updates["success_criteria"] = successCriteria
	}
	if len(args.DeliveryFormat) > 0 {
		updates["delivery_format"] = normalizeNonEmptyStrings(args.DeliveryFormat)
	}

	if len(updates) == 1 {
		return mcpToolCallResult{}, fmt.Errorf("至少提供一个可更新字段")
	}

	if err := db.Model(&database.AIWorkbenchTask{}).Where("id = ?", task.ID).Updates(updates).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("更新 AI 智脑任务失败: %w", err)
	}
	if err := createMCPWorkbenchEvent(db, task.ID, "", "workbench.task.updated", "info", "AI 智脑任务已更新", "MCP 控制面已更新任务状态或摘要。", updates); err != nil {
		return mcpToolCallResult{}, err
	}
	return newMCPToolResult(gin.H{
		"taskId":  task.ID,
		"status":  updates["status"],
		"updated": true,
	}), nil
}

func mcpCallWorkbenchStepAdd(args mcpWorkbenchStepAddArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	stepType := normalizeMCPWorkbenchStepType(args.StepType)
	title := strings.TrimSpace(args.Title)
	if stepType == "" {
		return mcpToolCallResult{}, fmt.Errorf("stepType 不能为空")
	}
	if title == "" {
		return mcpToolCallResult{}, fmt.Errorf("title 不能为空")
	}
	riskLevel := normalizeMCPWorkbenchRiskLevel(args.RiskLevel)
	approvalRequired := mcpWorkbenchActionRequiresApproval(task.RiskPolicy, riskLevel)
	if args.ApprovalRequired != nil {
		approvalRequired = *args.ApprovalRequired
	}

	step := database.AIWorkbenchTaskStep{
		ID:               database.GenerateNanoid(),
		TaskID:           task.ID,
		ParentStepID:     strings.TrimSpace(args.ParentStepID),
		StepType:         stepType,
		Title:            title,
		Rationale:        strings.TrimSpace(args.Rationale),
		Status:           "pending",
		RiskLevel:        riskLevel,
		ApprovalRequired: approvalRequired,
	}
	planItem := database.AIPlanItem{
		ID:               step.ID,
		Title:            step.Title,
		Rationale:        step.Rationale,
		StepType:         step.StepType,
		RiskLevel:        step.RiskLevel,
		ApprovalRequired: step.ApprovalRequired,
		DependsOn:        normalizeNonEmptyStrings(args.DependsOn),
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&step).Error; err != nil {
			return err
		}
		priority := len(task.CurrentPlan)
		task.CurrentPlan = append(task.CurrentPlan, planItem)
		task.UpdatedAt = time.Now()
		if err := tx.Save(&task).Error; err != nil {
			return err
		}
		if err := upsertMCPWorkbenchPlanTaskForStep(tx, task.ID, step, planItem.DependsOn, priority); err != nil {
			return err
		}
		return createMCPWorkbenchEvent(tx, task.ID, step.ID, "workbench.step.added", "info", "AI 智脑后续步骤已追加", step.Title, gin.H{"step": step})
	}); err != nil {
		return mcpToolCallResult{}, fmt.Errorf("追加 AI 智脑步骤失败: %w", err)
	}

	return newMCPToolResult(gin.H{
		"taskId": task.ID,
		"step":   step,
	}), nil
}

func mcpCallWorkbenchFactUpsert(args mcpWorkbenchFactUpsertArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	factKey := strings.TrimSpace(args.FactKey)
	targetRef := strings.TrimSpace(args.TargetRef)
	if factKey == "" {
		factKey = targetRef
	}
	if factKey == "" {
		return mcpToolCallResult{}, fmt.Errorf("factKey 或 targetRef 不能为空")
	}
	content := strings.TrimSpace(args.Body)
	if content == "" {
		content = strings.TrimSpace(args.Summary)
	}
	if content == "" {
		return mcpToolCallResult{}, fmt.Errorf("summary 或 body 不能为空")
	}
	source := strings.TrimSpace(args.Source)
	if source == "" {
		source = mcpWorkbenchFactSource
	}
	confidence := normalizeMCPWorkbenchConfidence(args.Confidence)
	now := time.Now()
	var observation database.AIWorkbenchObservation

	if err := db.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("task_id = ? AND source = ? AND target_ref = ?", task.ID, source, factKey).First(&observation).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			observation = database.AIWorkbenchObservation{
				ID:         database.GenerateNanoid(),
				TaskID:     task.ID,
				StepID:     strings.TrimSpace(args.StepID),
				Source:     source,
				TargetRef:  factKey,
				Content:    content,
				Confidence: confidence,
				CreatedAt:  now,
			}
			if err := tx.Create(&observation).Error; err != nil {
				return err
			}
		} else {
			observation.StepID = strings.TrimSpace(args.StepID)
			observation.Content = content
			observation.Confidence = confidence
			if err := tx.Save(&observation).Error; err != nil {
				return err
			}
		}
		return createMCPWorkbenchEvent(tx, task.ID, observation.StepID, "workbench.fact.upserted", "info", "任务事实已写入", factKey, gin.H{
			"factKey":  factKey,
			"source":   source,
			"summary":  strings.TrimSpace(args.Summary),
			"metadata": args.Metadata,
		})
	}); err != nil {
		return mcpToolCallResult{}, fmt.Errorf("写入任务事实失败: %w", err)
	}

	return newMCPToolResult(gin.H{
		"taskId":  task.ID,
		"factKey": factKey,
		"fact":    observation,
	}), nil
}

func mcpCallWorkbenchFactSearch(args mcpWorkbenchFactSearchArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	limit := args.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	query := db.Where("task_id = ?", task.ID).Order("created_at DESC").Limit(limit)
	if text := strings.TrimSpace(args.Query); text != "" {
		like := "%" + text + "%"
		query = query.Where("source LIKE ? OR target_ref LIKE ? OR content LIKE ?", like, like, like)
	}
	var facts []database.AIWorkbenchObservation
	if err := query.Find(&facts).Error; err != nil {
		return mcpToolCallResult{}, fmt.Errorf("检索任务事实失败: %w", err)
	}
	return newMCPToolResult(gin.H{
		"taskId": task.ID,
		"total":  len(facts),
		"facts":  facts,
	}), nil
}

func mcpCallWorkbenchEvidenceAdd(args mcpWorkbenchEvidenceAddArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	title := strings.TrimSpace(args.Title)
	summary := strings.TrimSpace(args.Summary)
	if title == "" || summary == "" {
		return mcpToolCallResult{}, fmt.Errorf("title 和 summary 不能为空")
	}
	now := time.Now()
	evidence := database.AIWorkbenchEvidenceItem{
		ID:             database.GenerateNanoid(),
		TaskID:         task.ID,
		StepID:         strings.TrimSpace(args.StepID),
		SourceType:     defaultString(strings.TrimSpace(args.SourceType), "mcp"),
		SourceID:       strings.TrimSpace(args.SourceID),
		TargetRef:      strings.TrimSpace(args.TargetRef),
		Title:          title,
		Summary:        summary,
		RiskLevel:      normalizeMCPWorkbenchRiskLevel(args.RiskLevel),
		Confidence:     normalizeMCPWorkbenchConfidence(args.Confidence),
		Status:         defaultString(strings.TrimSpace(args.Status), "observed"),
		Reference:      strings.TrimSpace(args.Reference),
		Metadata:       args.Metadata,
		ObservedAt:     now,
		CreatedAt:      now,
		LastObservedAt: now,
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&evidence).Error; err != nil {
			return err
		}
		return createMCPWorkbenchEvent(tx, task.ID, evidence.StepID, "workbench.evidence.added", "info", "标准证据已写入", evidence.Title, gin.H{"evidenceId": evidence.ID})
	}); err != nil {
		return mcpToolCallResult{}, fmt.Errorf("写入标准证据失败: %w", err)
	}
	return newMCPToolResult(gin.H{
		"taskId":   task.ID,
		"evidence": evidence,
	}), nil
}

func mcpCallWorkbenchHypothesisUpdate(args mcpWorkbenchHypothesisUpdateArgs) (mcpToolCallResult, error) {
	db, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	title := strings.TrimSpace(args.Title)
	if strings.TrimSpace(args.HypothesisID) == "" && title == "" {
		return mcpToolCallResult{}, fmt.Errorf("创建假设时 title 不能为空")
	}
	now := time.Now()
	var hypothesis database.AIWorkbenchHypothesis

	if err := db.Transaction(func(tx *gorm.DB) error {
		hypothesisID := strings.TrimSpace(args.HypothesisID)
		if hypothesisID != "" {
			if err := tx.Where("id = ? AND task_id = ?", hypothesisID, task.ID).First(&hypothesis).Error; err != nil {
				return err
			}
			if title != "" {
				hypothesis.Title = title
			}
			if description := strings.TrimSpace(args.Description); description != "" {
				hypothesis.Description = description
			}
			if len(args.EvidenceRefs) > 0 {
				hypothesis.EvidenceRefs = normalizeNonEmptyStrings(args.EvidenceRefs)
			}
			if status := normalizeMCPWorkbenchHypothesisStatus(args.Status); status != "" {
				hypothesis.Status = status
			}
			hypothesis.Confidence = normalizeMCPWorkbenchConfidence(defaultString(args.Confidence, hypothesis.Confidence))
			hypothesis.UpdatedAt = now
			if err := tx.Save(&hypothesis).Error; err != nil {
				return err
			}
		} else {
			hypothesis = database.AIWorkbenchHypothesis{
				ID:           database.GenerateNanoid(),
				TaskID:       task.ID,
				Title:        title,
				Description:  strings.TrimSpace(args.Description),
				EvidenceRefs: normalizeNonEmptyStrings(args.EvidenceRefs),
				Status:       defaultString(normalizeMCPWorkbenchHypothesisStatus(args.Status), "candidate"),
				Confidence:   normalizeMCPWorkbenchConfidence(args.Confidence),
				CreatedAt:    now,
				UpdatedAt:    now,
			}
			if err := tx.Create(&hypothesis).Error; err != nil {
				return err
			}
		}
		return createMCPWorkbenchEvent(tx, task.ID, "", "workbench.hypothesis.updated", "info", "测试假设已维护", hypothesis.Title, gin.H{"hypothesisId": hypothesis.ID})
	}); err != nil {
		return mcpToolCallResult{}, fmt.Errorf("维护测试假设失败: %w", err)
	}

	return newMCPToolResult(gin.H{
		"taskId":     task.ID,
		"hypothesis": hypothesis,
	}), nil
}

func mcpCallWorkbenchScopeCheck(args mcpWorkbenchScopeCheckArgs) (mcpToolCallResult, error) {
	_, task, err := loadMCPWorkbenchTask(args.TaskID)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	target := strings.TrimSpace(args.Target)
	if target == "" {
		return mcpToolCallResult{}, fmt.Errorf("target 不能为空")
	}
	inScope, reason := isMCPWorkbenchTargetInScope(target, append([]string{task.Target}, task.Scope...))
	return newMCPToolResult(gin.H{
		"taskId":  task.ID,
		"target":  target,
		"inScope": inScope,
		"reason":  reason,
		"scope":   task.Scope,
	}), nil
}

func loadMCPWorkbenchTask(taskID string) (*gorm.DB, database.AIWorkbenchTask, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, database.AIWorkbenchTask{}, fmt.Errorf("taskId 不能为空")
	}
	db, err := database.GetDB()
	if err != nil {
		return nil, database.AIWorkbenchTask{}, fmt.Errorf("获取数据库连接失败: %w", err)
	}
	var task database.AIWorkbenchTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, database.AIWorkbenchTask{}, fmt.Errorf("AI 智脑任务不存在: %s", taskID)
		}
		return nil, database.AIWorkbenchTask{}, err
	}
	return db, task, nil
}

func createMCPWorkbenchEvent(tx *gorm.DB, taskID string, stepID string, eventType string, level string, title string, content string, payload interface{}) error {
	payloadMap := map[string]interface{}{}
	if payload != nil {
		data, err := json.Marshal(payload)
		if err == nil {
			_ = json.Unmarshal(data, &payloadMap)
		}
	}
	return tx.Create(&database.AIWorkbenchEvent{
		ID:      database.GenerateNanoid(),
		TaskID:  strings.TrimSpace(taskID),
		StepID:  strings.TrimSpace(stepID),
		Type:    strings.TrimSpace(eventType),
		Level:   defaultString(strings.TrimSpace(level), "info"),
		Title:   strings.TrimSpace(title),
		Content: strings.TrimSpace(content),
		Payload: payloadMap,
	}).Error
}

func normalizeMCPWorkbenchTaskStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return ""
	case "planned", "running", "paused", "completed", "cancelled", "failed":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func isMCPWorkbenchTerminalStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "cancelled", "failed":
		return true
	default:
		return false
	}
}

func normalizeMCPWorkbenchStepType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "scope_review", "asset_context", "tool_orchestration", "http_observation", "hypothesis", "verification", "goal_evaluation":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeMCPWorkbenchRiskLevel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return "critical"
	case "high":
		return "high"
	case "medium":
		return "medium"
	case "low":
		return "low"
	case "info":
		return "info"
	default:
		return "low"
	}
}

func normalizeMCPWorkbenchConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "medium", "low":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "medium"
	}
}

func normalizeMCPWorkbenchHypothesisStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return ""
	case "candidate", "testing", "supported", "rejected", "confirmed":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func mcpWorkbenchActionRequiresApproval(riskPolicy string, riskLevel string) bool {
	riskPolicy = strings.ToLower(strings.TrimSpace(riskPolicy))
	riskLevel = strings.ToLower(strings.TrimSpace(riskLevel))
	if riskPolicy == "read_only" {
		return riskLevel == "medium" || riskLevel == "high" || riskLevel == "critical"
	}
	if riskPolicy == "aggressive" {
		return riskLevel == "critical"
	}
	return riskLevel == "high" || riskLevel == "critical"
}

func isMCPWorkbenchTargetInScope(target string, scope []string) (bool, string) {
	normalizedTarget := normalizeMCPWorkbenchTargetRef(target)
	targetHost := hostFromMCPWorkbenchTarget(target)
	for _, item := range scope {
		normalizedScope := normalizeMCPWorkbenchTargetRef(item)
		if normalizedScope == "" {
			continue
		}
		if normalizedTarget == normalizedScope {
			return true, "目标与授权范围精确匹配。"
		}
		scopeHost := hostFromMCPWorkbenchTarget(item)
		if targetHost != "" && scopeHost != "" {
			if strings.EqualFold(targetHost, scopeHost) || strings.HasSuffix(strings.ToLower(targetHost), "."+strings.ToLower(scopeHost)) {
				return true, "目标主机属于授权域名或主机范围。"
			}
		}
		if _, _, err := net.ParseCIDR(strings.TrimSpace(item)); err == nil && targetHost != "" {
			ip := net.ParseIP(targetHost)
			_, network, _ := net.ParseCIDR(strings.TrimSpace(item))
			if ip != nil && network.Contains(ip) {
				return true, "目标 IP 属于授权 CIDR。"
			}
		}
	}
	return false, "目标不在当前 AI 智脑任务授权范围内。"
}

func normalizeMCPWorkbenchTargetRef(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.Fragment = ""
		return strings.TrimRight(strings.ToLower(parsed.String()), "/")
	}
	return strings.TrimRight(strings.ToLower(value), "/")
}

func hostFromMCPWorkbenchTarget(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Hostname() != "" {
		return strings.ToLower(parsed.Hostname())
	}
	host, _, err := net.SplitHostPort(value)
	if err == nil {
		return strings.ToLower(host)
	}
	if strings.Contains(value, "/") {
		return ""
	}
	return strings.ToLower(value)
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func upsertMCPWorkbenchPlanTaskForStep(tx *gorm.DB, taskID string, step database.AIWorkbenchTaskStep, dependsOn []string, priority int) error {
	if !mcpWorkbenchPlanTasksTableReady(tx) {
		return nil
	}
	status := mcpWorkbenchPlanTaskStatusForStep(step)
	blockedReason := ""
	if status == "blocked" {
		blockedReason = "等待人工审批"
	}
	planTask := database.AIWorkbenchPlanTask{
		ID:            database.GenerateNanoid(),
		TaskID:        taskID,
		StepID:        step.ID,
		ParentID:      step.ParentStepID,
		Title:         strings.TrimSpace(step.Title),
		Goal:          strings.TrimSpace(step.Rationale),
		Status:        status,
		Priority:      priority,
		DependsOn:     normalizeNonEmptyStrings(dependsOn),
		BlockedReason: blockedReason,
		Metadata: map[string]any{
			"stepType":         step.StepType,
			"riskLevel":        step.RiskLevel,
			"approvalRequired": step.ApprovalRequired,
		},
	}
	var existing database.AIWorkbenchPlanTask
	err := tx.Where("task_id = ? AND step_id = ?", taskID, step.ID).First(&existing).Error
	if err == nil {
		planTask.ID = existing.ID
		planTask.CreatedAt = existing.CreatedAt
		return tx.Save(&planTask).Error
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Create(&planTask).Error
}

func mcpWorkbenchPlanTasksTableReady(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	return db.Migrator().HasTable(&database.AIWorkbenchPlanTask{})
}

func mcpWorkbenchPlanTaskStatusForStep(step database.AIWorkbenchTaskStep) string {
	switch strings.ToLower(strings.TrimSpace(step.Status)) {
	case "running":
		return "in_progress"
	case "completed":
		return "completed"
	case "cancelled", "rejected":
		return "cancelled"
	case "pending":
		if step.ApprovalRequired {
			return "blocked"
		}
		return "pending"
	default:
		return "pending"
	}
}
