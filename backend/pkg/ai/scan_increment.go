package ai

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/taskstate"

	"gorm.io/gorm"
)

// WorkbenchScanIncrementSummary 是长扫描运行中给 AI loop 消费的轻量状态摘要。
type WorkbenchScanIncrementSummary struct {
	ScanTaskID         string                    `json:"scanTaskId"`
	ScanType           string                    `json:"scanType"`
	Status             string                    `json:"status"`
	CurrentStage       string                    `json:"currentStage"`
	Progress           int                       `json:"progress"`
	TotalCount         int                       `json:"totalCount"`
	ErrorCount         int                       `json:"errorCount"`
	FingerprintCount   int64                     `json:"fingerprintCount"`
	VulnerabilityCount int64                     `json:"vulnerabilityCount"`
	JSAssetCount       int64                     `json:"jsAssetCount"`
	DirResultCount     int64                     `json:"dirResultCount"`
	PortCount          int64                     `json:"portCount"`
	StageEvents        []WorkbenchScanStageEvent `json:"stageEvents"`
	ErrorSamples       []string                  `json:"errorSamples"`
	CheckpointSignals  []string                  `json:"checkpointSignals"`
	Highlights         []string                  `json:"highlights"`
}

// WorkbenchScanStageEvent 是从扫描运行状态提取的阶段级原始事件。
type WorkbenchScanStageEvent struct {
	Kind     string `json:"kind"`
	Stage    string `json:"stage"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Message  string `json:"message"`
}

// WorkbenchScanIncrementCompressionEvidence 是模型压缩长扫描增量时可见的上下文。
type WorkbenchScanIncrementCompressionEvidence struct {
	TaskName        string
	Target          string
	RiskPolicy      string
	StrategyPacks   []string
	SuccessCriteria string
	Increment       WorkbenchScanIncrementSummary
}

// WorkbenchScanIncrementCompressionResult 是模型型增量摘要压缩器的结构化输出。
type WorkbenchScanIncrementCompressionResult struct {
	Summary        string   `json:"summary"`
	KeyFindings    []string `json:"keyFindings"`
	FailedBranches []string `json:"failedBranches"`
	NextSignals    []string `json:"nextSignals"`
	Confidence     string   `json:"confidence"`
}

// CollectWorkbenchScanIncrementSummary 读取单个扫描任务的局部结果，生成可增量回流给 AI 的摘要。
func CollectWorkbenchScanIncrementSummary(db *gorm.DB, scanTask database.VulnScanTask) (WorkbenchScanIncrementSummary, error) {
	summary := WorkbenchScanIncrementSummary{
		ScanTaskID:   scanTask.ID,
		ScanType:     scanTask.ScanType,
		Status:       scanTask.Status,
		CurrentStage: scanTask.CurrentStage,
		Progress:     scanTask.Progress,
		TotalCount:   scanTask.TotalCount,
		ErrorCount:   scanTask.ErrorCount,
		Highlights:   make([]string, 0, 8),
	}

	base := func(model any) *gorm.DB {
		return db.Model(model).Where("task_id = ? AND version = ?", scanTask.ID, scanTask.Version)
	}
	if err := base(&database.FingerprintInfo{}).Count(&summary.FingerprintCount).Error; err != nil {
		return summary, err
	}
	if err := base(&database.VulnerabilityInfo{}).Count(&summary.VulnerabilityCount).Error; err != nil {
		return summary, err
	}
	if err := base(&database.VulnScanJSAsset{}).Count(&summary.JSAssetCount).Error; err != nil {
		return summary, err
	}
	if err := base(&database.VulnScanDirResult{}).Count(&summary.DirResultCount).Error; err != nil {
		return summary, err
	}
	if err := base(&database.PortScanResult{}).Count(&summary.PortCount).Error; err != nil {
		return summary, err
	}

	if err := collectWorkbenchScanIncrementHighlights(db, scanTask, &summary); err != nil {
		return summary, err
	}
	if err := collectWorkbenchScanIncrementErrorSamples(db, scanTask, &summary); err != nil {
		return summary, err
	}
	summary.StageEvents = BuildWorkbenchScanStageEvents(summary)
	summary.CheckpointSignals = BuildWorkbenchCheckpointSignals(scanTask.CheckpointJSON, 8)
	return summary, nil
}

func (s WorkbenchScanIncrementSummary) Text() string {
	stage := strings.TrimSpace(s.CurrentStage)
	if stage == "" {
		stage = "-"
	}
	lines := []string{
		fmt.Sprintf(
			"关联扫描任务 %s 运行中摘要：状态 %s，进度 %d%%，阶段 %s，目标 %d 个，错误 %d 个。",
			s.ScanTaskID,
			s.Status,
			s.Progress,
			stage,
			s.TotalCount,
			s.ErrorCount,
		),
		fmt.Sprintf(
			"当前已采集指纹 %d 条、漏洞 %d 条、JS 资产 %d 条、目录结果 %d 条、端口 %d 条。",
			s.FingerprintCount,
			s.VulnerabilityCount,
			s.JSAssetCount,
			s.DirResultCount,
			s.PortCount,
		),
	}
	if len(s.Highlights) > 0 {
		lines = append(lines, strings.Join(s.Highlights, "\n"))
	}
	if len(s.StageEvents) > 0 {
		eventLines := make([]string, 0, len(s.StageEvents))
		for _, event := range s.StageEvents {
			eventLines = append(eventLines, "- "+event.Message)
		}
		lines = append(lines, "阶段事件:\n"+strings.Join(eventLines, "\n"))
	}
	if len(s.ErrorSamples) > 0 {
		lines = append(lines, "错误样本:\n- "+strings.Join(s.ErrorSamples, "\n- "))
	}
	if len(s.CheckpointSignals) > 0 {
		lines = append(lines, "执行器检查点信号:\n- "+strings.Join(s.CheckpointSignals, "\n- "))
	}
	return strings.Join(lines, "\n")
}

// BuildWorkbenchScanStageEvents 将当前扫描快照转成去上下文依赖的阶段级事件。
func BuildWorkbenchScanStageEvents(summary WorkbenchScanIncrementSummary) []WorkbenchScanStageEvent {
	stage := strings.TrimSpace(summary.CurrentStage)
	if stage == "" {
		stage = "-"
	}
	events := []WorkbenchScanStageEvent{
		{
			Kind:     "stage_checkpoint",
			Stage:    stage,
			Status:   summary.Status,
			Progress: summary.Progress,
			Message:  fmt.Sprintf("阶段检查点: %s，状态 %s，进度 %d%%。", stage, summary.Status, summary.Progress),
		},
	}
	resultMessage := fmt.Sprintf(
		"阶段结果计数: 指纹 %d，漏洞 %d，JS 资产 %d，目录 %d，端口 %d。",
		summary.FingerprintCount,
		summary.VulnerabilityCount,
		summary.JSAssetCount,
		summary.DirResultCount,
		summary.PortCount,
	)
	events = append(events, WorkbenchScanStageEvent{
		Kind:     "result_counts",
		Stage:    stage,
		Status:   summary.Status,
		Progress: summary.Progress,
		Message:  resultMessage,
	})
	if summary.ErrorCount > 0 {
		events = append(events, WorkbenchScanStageEvent{
			Kind:     "error_count",
			Stage:    stage,
			Status:   summary.Status,
			Progress: summary.Progress,
			Message:  fmt.Sprintf("阶段错误计数: %s 已记录 %d 个错误目标。", stage, summary.ErrorCount),
		})
	}
	return events
}

// BuildWorkbenchCheckpointSignals 从扫描 checkpoint meta 中提取有限、去敏的执行器原始信号。
func BuildWorkbenchCheckpointSignals(raw string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	checkpoint := taskstate.ParseCheckpoint(raw)
	signals := make([]string, 0, limit)
	if checkpoint.Stage != "" {
		signals = append(signals, fmt.Sprintf("checkpoint.stage=%s progress=%d", truncateWorkbenchText(checkpoint.Stage, 80), checkpoint.Progress))
	}
	if len(signals) >= limit || len(checkpoint.Meta) == 0 {
		return signals
	}

	keys := make([]string, 0, len(checkpoint.Meta))
	for key := range checkpoint.Meta {
		key = strings.TrimSpace(key)
		if key == "" || isSensitiveWorkbenchCheckpointKey(key) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, signal := range formatWorkbenchCheckpointMetaSignal(key, checkpoint.Meta[key]) {
			signals = append(signals, signal)
			if len(signals) >= limit {
				return signals
			}
		}
	}
	return signals
}

// BuildWorkbenchScanIncrementCompressionPrompt 构造模型压缩扫描增量的提示。
func BuildWorkbenchScanIncrementCompressionPrompt(evidence WorkbenchScanIncrementCompressionEvidence) string {
	summary := evidence.Increment
	lines := []string{
		"请作为 Hephaestus AI 智脑的长扫描增量压缩器，把确定性扫描状态压缩为可用于下一轮决策的结构化摘要。",
		"只能基于输入里的计数、阶段、错误和样本做归纳；不要编造发现，不要输出工具调用、命令或漏洞利用步骤。",
		"只输出 JSON，不要输出 Markdown、代码块或额外解释。JSON schema: {\"summary\":\"...\",\"keyFindings\":[\"...\"],\"failedBranches\":[\"...\"],\"nextSignals\":[\"...\"],\"confidence\":\"low|medium|high\"}",
		fmt.Sprintf("任务: %s", evidence.TaskName),
		fmt.Sprintf("目标: %s", evidence.Target),
		fmt.Sprintf("风险策略: %s", evidence.RiskPolicy),
		fmt.Sprintf("策略包: %s", strings.Join(evidence.StrategyPacks, ", ")),
		fmt.Sprintf("成功标准: %s", evidence.SuccessCriteria),
		fmt.Sprintf("扫描任务: %s / %s", summary.ScanTaskID, summary.ScanType),
		fmt.Sprintf("状态: %s，阶段: %s，进度: %d%%，目标: %d，错误: %d", summary.Status, summary.CurrentStage, summary.Progress, summary.TotalCount, summary.ErrorCount),
		fmt.Sprintf("计数: 指纹 %d，漏洞 %d，JS 资产 %d，目录 %d，端口 %d", summary.FingerprintCount, summary.VulnerabilityCount, summary.JSAssetCount, summary.DirResultCount, summary.PortCount),
		"阶段事件:\n" + joinOrDash(formatWorkbenchScanStageEvents(summary.StageEvents)),
		"错误样本:\n" + joinOrDash(summary.ErrorSamples),
		"执行器检查点信号:\n" + joinOrDash(summary.CheckpointSignals),
		"样本:\n" + joinOrDash(summary.Highlights),
		"字段要求：summary 一句话说明状态和价值；keyFindings 最多 5 条；failedBranches 只放错误、失败、阻塞或无结果分支；nextSignals 只放值得后续 planner 使用的关键词或方向。",
	}
	return strings.Join(lines, "\n")
}

// ParseWorkbenchScanIncrementCompressionResult 解析模型压缩结果并转成可落库文本。
func ParseWorkbenchScanIncrementCompressionResult(reply string, fallback string) (string, WorkbenchScanIncrementCompressionResult, bool) {
	payload := extractWorkbenchJSONPayload(reply)
	if payload == "" {
		return fallback, WorkbenchScanIncrementCompressionResult{}, false
	}
	var parsed WorkbenchScanIncrementCompressionResult
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return fallback, WorkbenchScanIncrementCompressionResult{}, false
	}
	parsed.Summary = strings.TrimSpace(parsed.Summary)
	parsed.KeyFindings = cleanWorkbenchScanIncrementList(parsed.KeyFindings, 5)
	parsed.FailedBranches = cleanWorkbenchScanIncrementList(parsed.FailedBranches, 5)
	parsed.NextSignals = cleanWorkbenchScanIncrementList(parsed.NextSignals, 5)
	parsed.Confidence = normalizeWorkbenchHypothesisConfidence(parsed.Confidence)
	if parsed.Summary == "" && len(parsed.KeyFindings) == 0 && len(parsed.FailedBranches) == 0 && len(parsed.NextSignals) == 0 {
		return fallback, WorkbenchScanIncrementCompressionResult{}, false
	}

	sections := []string{"关联扫描模型压缩摘要："}
	if parsed.Summary != "" {
		sections = append(sections, parsed.Summary)
	}
	if len(parsed.KeyFindings) > 0 {
		sections = append(sections, "关键发现:\n- "+strings.Join(parsed.KeyFindings, "\n- "))
	}
	if len(parsed.FailedBranches) > 0 {
		sections = append(sections, "失败或阻塞分支:\n- "+strings.Join(parsed.FailedBranches, "\n- "))
	}
	if len(parsed.NextSignals) > 0 {
		sections = append(sections, "后续信号:\n- "+strings.Join(parsed.NextSignals, "\n- "))
	}
	sections = append(sections, "原始增量摘要:\n"+fallback)
	return strings.Join(sections, "\n"), parsed, true
}

func collectWorkbenchScanIncrementHighlights(db *gorm.DB, scanTask database.VulnScanTask, summary *WorkbenchScanIncrementSummary) error {
	var fingerprints []database.FingerprintInfo
	if err := db.Where("task_id = ? AND version = ?", scanTask.ID, scanTask.Version).
		Order("created_at DESC").
		Limit(3).
		Find(&fingerprints).Error; err != nil {
		return err
	}
	for _, item := range fingerprints {
		label := strings.TrimSpace(item.Title)
		if label == "" {
			label = strings.TrimSpace(item.Detect)
		}
		if label == "" {
			label = strings.TrimSpace(item.Fingerprints)
		}
		summary.Highlights = append(summary.Highlights, fmt.Sprintf("指纹样本: %s %s", item.URL, truncateWorkbenchText(label, 100)))
	}

	var vulns []database.VulnerabilityInfo
	if err := db.Where("task_id = ? AND version = ?", scanTask.ID, scanTask.Version).
		Limit(3).
		Find(&vulns).Error; err != nil {
		return err
	}
	for _, item := range vulns {
		summary.Highlights = append(summary.Highlights, fmt.Sprintf("漏洞样本: [%s] %s %s", item.Severity, item.VulnName, item.VulnURL))
	}

	var jsAssets []database.VulnScanJSAsset
	if err := db.Where("task_id = ? AND version = ?", scanTask.ID, scanTask.Version).
		Order("created_at DESC").
		Limit(3).
		Find(&jsAssets).Error; err != nil {
		return err
	}
	for _, item := range jsAssets {
		summary.Highlights = append(summary.Highlights, fmt.Sprintf("JS/API样本: [%s] %s", item.Type, truncateWorkbenchText(item.Value, 120)))
	}

	var dirResults []database.VulnScanDirResult
	if err := db.Where("task_id = ? AND version = ?", scanTask.ID, scanTask.Version).
		Order("created_at DESC").
		Limit(3).
		Find(&dirResults).Error; err != nil {
		return err
	}
	for _, item := range dirResults {
		summary.Highlights = append(summary.Highlights, fmt.Sprintf("目录样本: [%d] %s %s", item.Status, item.URL, item.Title))
	}

	var ports []database.PortScanResult
	if err := db.Where("task_id = ? AND version = ?", scanTask.ID, scanTask.Version).
		Order("created_at DESC").
		Limit(3).
		Find(&ports).Error; err != nil {
		return err
	}
	for _, item := range ports {
		summary.Highlights = append(summary.Highlights, fmt.Sprintf("端口样本: %s:%d %s %s", item.Host, item.Port, item.Protocol, item.Product))
	}
	return nil
}

func collectWorkbenchScanIncrementErrorSamples(db *gorm.DB, scanTask database.VulnScanTask, summary *WorkbenchScanIncrementSummary) error {
	samples := make([]string, 0, 5)
	if errorMsg := strings.TrimSpace(scanTask.ErrorMsg); errorMsg != "" {
		samples = append(samples, "任务错误: "+truncateWorkbenchText(errorMsg, 180))
	}

	var items []database.VulnScanTaskItem
	if err := db.Where("task_id = ? AND version = ? AND error_msg <> ''", scanTask.ID, scanTask.Version).
		Order("updated_at DESC").
		Limit(4).
		Find(&items).Error; err != nil {
		return err
	}
	for _, item := range items {
		target := strings.TrimSpace(item.Target)
		if target == "" {
			target = "-"
		}
		samples = append(samples, fmt.Sprintf("目标错误: [%s] %s %s", item.Status, target, truncateWorkbenchText(item.ErrorMsg, 160)))
		if len(samples) >= 5 {
			break
		}
	}
	summary.ErrorSamples = samples
	return nil
}

func formatWorkbenchCheckpointMetaSignal(key string, value interface{}) []string {
	if strings.EqualFold(key, "stageEvents") {
		return formatWorkbenchCheckpointStageEventSignals(value)
	}
	if text, ok := scalarWorkbenchCheckpointValue(value); ok {
		return []string{fmt.Sprintf("checkpoint.%s=%s", key, truncateWorkbenchText(text, 160))}
	}
	switch typed := value.(type) {
	case []interface{}:
		if len(typed) == 0 {
			return nil
		}
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := scalarWorkbenchCheckpointValue(item)
			if !ok {
				continue
			}
			values = append(values, text)
			if len(values) >= 4 {
				break
			}
		}
		if len(values) > 0 {
			return []string{fmt.Sprintf("checkpoint.%s=%s", key, truncateWorkbenchText(strings.Join(values, ", "), 160))}
		}
		return []string{fmt.Sprintf("checkpoint.%s=count:%d", key, len(typed))}
	case map[string]interface{}:
		return []string{fmt.Sprintf("checkpoint.%s=object_keys:%d", key, len(typed))}
	default:
		return nil
	}
}

func formatWorkbenchCheckpointStageEventSignals(value interface{}) []string {
	events, ok := value.([]interface{})
	if !ok || len(events) == 0 {
		return nil
	}
	signals := make([]string, 0, len(events))
	start := 0
	if len(events) > 4 {
		start = len(events) - 4
	}
	for _, event := range events[start:] {
		item, ok := event.(map[string]interface{})
		if !ok {
			continue
		}
		stage, _ := scalarWorkbenchCheckpointValue(item["stage"])
		label, _ := scalarWorkbenchCheckpointValue(item["label"])
		if stage == "" && label == "" {
			continue
		}
		signals = append(signals, "checkpoint.stageEvent="+truncateWorkbenchText(strings.TrimSpace(stage+" "+label), 160))
	}
	return signals
}

func scalarWorkbenchCheckpointValue(value interface{}) (string, bool) {
	switch typed := value.(type) {
	case string:
		typed = strings.TrimSpace(typed)
		return typed, typed != ""
	case bool:
		return fmt.Sprintf("%t", typed), true
	case int:
		return fmt.Sprintf("%d", typed), true
	case int64:
		return fmt.Sprintf("%d", typed), true
	case float64:
		return fmt.Sprintf("%.0f", typed), true
	case json.Number:
		return typed.String(), true
	default:
		return "", false
	}
}

func isSensitiveWorkbenchCheckpointKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, marker := range []string{"authorization", "cookie", "password", "passwd", "secret", "token", "apikey", "api_key", "key"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func cleanWorkbenchScanIncrementList(values []string, limit int) []string {
	cleaned := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		cleaned = append(cleaned, value)
		if len(cleaned) >= limit {
			break
		}
	}
	return cleaned
}

func formatWorkbenchScanStageEvents(events []WorkbenchScanStageEvent) []string {
	lines := make([]string, 0, len(events))
	for _, event := range events {
		message := strings.TrimSpace(event.Message)
		if message == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- [%s/%s/%d%%] %s", event.Kind, event.Stage, event.Progress, message))
	}
	return lines
}
