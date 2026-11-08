package database

import (
	"strings"
	"time"
)

// DBConnectionManager 数据库连接管理器
type DBConnectionManager struct {
	Nanoid        string    `gorm:"primaryKey;column:nanoid;type:varchar(255)" json:"nanoid"`
	Scheme        string    `gorm:"column:scheme;type:varchar(50);not null" json:"scheme"`
	Host          string    `gorm:"column:host;type:varchar(255);not null" json:"host"`
	Port          int       `gorm:"column:port;not null" json:"port"`
	Username      string    `gorm:"column:username;type:varchar(255);not null" json:"username"`
	Password      string    `gorm:"column:password;type:varchar(255);not null" json:"password"`
	ServerName    string    `gorm:"column:serverName;type:varchar(255);default:''" json:"serverName"`
	Notes         string    `gorm:"column:notes;type:text;default:''" json:"notes"`
	DatabaseCount int       `gorm:"column:database_count;default:0" json:"databaseCount"`
	TableCount    int       `gorm:"column:table_count;default:0" json:"tableCount"`
	UpdatedAt     time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (DBConnectionManager) TableName() string {
	return "db_connection_manager"
}

// SpaceEngineSyntax 空间引擎语法
type SpaceEngineSyntax struct {
	ID      uint   `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Source  string `gorm:"column:source;type:varchar(50);not null;index" json:"source"` // 来源：fofa, hunter, quake 等
	Name    string `gorm:"column:name;type:varchar(255);not null" json:"name"`
	Content string `gorm:"column:content;type:text" json:"content"`
}

func (SpaceEngineSyntax) TableName() string {
	return "space_engine_syntax"
}

// SpaceEngineSearchHistory 空间引擎搜索历史
type SpaceEngineSearchHistory struct {
	ID        string    `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	Source    string    `gorm:"column:source;type:varchar(50);not null;uniqueIndex:idx_space_engine_search_history_unique" json:"source"`
	Query     string    `gorm:"column:query;type:text;not null;uniqueIndex:idx_space_engine_search_history_unique" json:"query"`
	CreatedBy string    `gorm:"column:created_by;type:varchar(255);not null;uniqueIndex:idx_space_engine_search_history_unique;index" json:"createdBy"`
	CreatedAt time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP;index" json:"updatedAt"`
}

func (SpaceEngineSearchHistory) TableName() string {
	return "space_engine_search_history"
}

// MCPSpaceEngineAuditLog 记录 Space Engine MCP Server 的真实调用审计。
type MCPSpaceEngineAuditLog struct {
	ID            string    `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	Server        string    `gorm:"column:server;type:varchar(100);not null;index" json:"server"`
	Method        string    `gorm:"column:method;type:varchar(100);not null;index" json:"method"`
	ToolName      string    `gorm:"column:tool_name;type:varchar(255);default:'';index" json:"toolName"`
	SessionID     string    `gorm:"column:session_id;type:varchar(255);default:'';index" json:"sessionId"`
	ClientName    string    `gorm:"column:client_name;type:varchar(255);default:'';index" json:"clientName"`
	ClientVersion string    `gorm:"column:client_version;type:varchar(100);default:''" json:"clientVersion"`
	UserAgent     string    `gorm:"column:user_agent;type:text" json:"userAgent"`
	SourceIP      string    `gorm:"column:source_ip;type:varchar(100);default:'';index" json:"sourceIp"`
	Status        string    `gorm:"column:status;type:varchar(20);not null;index" json:"status"`
	HTTPStatus    int       `gorm:"column:http_status;default:0" json:"httpStatus"`
	ErrorCode     int       `gorm:"column:error_code;default:0" json:"errorCode"`
	ErrorMessage  string    `gorm:"column:error_message;type:text" json:"errorMessage"`
	DurationMS    int64     `gorm:"column:duration_ms;default:0" json:"durationMs"`
	CreatedAt     time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index:idx_mcp_space_engine_audit_log_created_at,sort:desc" json:"createdAt"`
}

func (MCPSpaceEngineAuditLog) TableName() string {
	return "mcp_space_engine_audit_log"
}

// AIPlanItem 是 AI 智脑任务当前计划的轻量快照。
type AIPlanItem struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Rationale        string   `json:"rationale"`
	StepType         string   `json:"stepType"`
	RiskLevel        string   `json:"riskLevel"`
	ApprovalRequired bool     `json:"approvalRequired"`
	DependsOn        []string `json:"dependsOn,omitempty"`
}

// AIWorkbenchTask 记录一次 AI 智脑编排任务。
type AIWorkbenchTask struct {
	ID                   string       `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	Name                 string       `gorm:"column:name;type:varchar(255);not null" json:"name"`
	Target               string       `gorm:"column:target;type:text;not null" json:"target"`
	Scope                []string     `gorm:"column:scope;type:text;serializer:json" json:"scope"`
	Mode                 string       `gorm:"column:mode;type:varchar(50);not null;default:'agent';index" json:"mode"`
	AgentProfile         string       `gorm:"column:agent_profile;type:varchar(80);not null;default:'deep_agent';index" json:"agentProfile"`
	AgentModel           string       `gorm:"column:agent_model;type:varchar(255);default:'';index" json:"agentModel"`
	AgentProviderID      string       `gorm:"column:agent_provider_id;type:varchar(255);default:'';index" json:"agentProviderId"`
	Status               string       `gorm:"column:status;type:varchar(30);not null;default:'planned';index" json:"status"`
	RiskPolicy           string       `gorm:"column:risk_policy;type:varchar(50);not null;default:'manual_approval';index" json:"riskPolicy"`
	Objective            string       `gorm:"column:objective;type:text;not null" json:"objective"`
	SuccessCriteria      string       `gorm:"column:success_criteria;type:text" json:"successCriteria"`
	DeliveryFormat       []string     `gorm:"column:delivery_format;type:text;serializer:json" json:"deliveryFormat"`
	StrategyPacks        []string     `gorm:"column:strategy_packs;type:text;serializer:json" json:"strategyPacks"`
	CurrentPlan          []AIPlanItem `gorm:"column:current_plan;type:text;serializer:json" json:"currentPlan"`
	Summary              string       `gorm:"column:summary;type:text" json:"summary"`
	CreatedBy            string       `gorm:"column:created_by;type:varchar(255);default:'';index" json:"createdBy"`
	StartedAt            *time.Time   `gorm:"column:started_at" json:"startedAt"`
	CompletedAt          *time.Time   `gorm:"column:completed_at" json:"completedAt"`
	PendingApprovalCount int          `gorm:"-" json:"pendingApprovalCount,omitempty"`
	CreatedAt            time.Time    `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index:idx_ai_tasks_created_at,sort:desc" json:"createdAt"`
	UpdatedAt            time.Time    `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (AIWorkbenchTask) TableName() string {
	return "ai_tasks"
}

// AIWorkbenchTaskStep 记录 AI 计划拆解后的可执行步骤。
type AIWorkbenchTaskStep struct {
	ID               string     `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID           string     `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	ParentStepID     string     `gorm:"column:parent_step_id;type:varchar(255);default:'';index" json:"parentStepId"`
	StepType         string     `gorm:"column:step_type;type:varchar(80);not null;index" json:"stepType"`
	Title            string     `gorm:"column:title;type:varchar(255);not null" json:"title"`
	Rationale        string     `gorm:"column:rationale;type:text" json:"rationale"`
	Status           string     `gorm:"column:status;type:varchar(30);not null;default:'pending';index" json:"status"`
	RiskLevel        string     `gorm:"column:risk_level;type:varchar(30);not null;default:'low';index" json:"riskLevel"`
	ApprovalRequired bool       `gorm:"column:approval_required;default:false;index" json:"approvalRequired"`
	ApprovedBy       string     `gorm:"column:approved_by;type:varchar(255);default:''" json:"approvedBy"`
	ApprovalNote     string     `gorm:"column:approval_note;type:text" json:"approvalNote"`
	StartedAt        *time.Time `gorm:"column:started_at" json:"startedAt"`
	FinishedAt       *time.Time `gorm:"column:finished_at" json:"finishedAt"`
	CreatedAt        time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt        time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (AIWorkbenchTaskStep) TableName() string {
	return "ai_task_steps"
}

// AIWorkbenchPlanTask 记录 AI 编排器维护的任务板条目。
type AIWorkbenchPlanTask struct {
	ID            string         `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID        string         `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	StepID        string         `gorm:"column:step_id;type:varchar(255);default:'';index" json:"stepId"`
	ParentID      string         `gorm:"column:parent_id;type:varchar(255);default:'';index" json:"parentId"`
	Title         string         `gorm:"column:title;type:varchar(255);not null" json:"title"`
	Goal          string         `gorm:"column:goal;type:text" json:"goal"`
	Status        string         `gorm:"column:status;type:varchar(30);not null;default:'pending';index" json:"status"`
	Priority      int            `gorm:"column:priority;default:0;index" json:"priority"`
	DependsOn     []string       `gorm:"column:depends_on;type:text;serializer:json" json:"dependsOn,omitempty"`
	EvidenceRefs  []string       `gorm:"column:evidence_refs;type:text;serializer:json" json:"evidenceRefs,omitempty"`
	BlockedReason string         `gorm:"column:blocked_reason;type:text" json:"blockedReason"`
	Notes         string         `gorm:"column:notes;type:text" json:"notes"`
	Metadata      map[string]any `gorm:"column:metadata;type:text;serializer:json" json:"metadata"`
	StartedAt     *time.Time     `gorm:"column:started_at" json:"startedAt"`
	CompletedAt   *time.Time     `gorm:"column:completed_at" json:"completedAt"`
	CreatedAt     time.Time      `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt     time.Time      `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (AIWorkbenchPlanTask) TableName() string {
	return "ai_plan_tasks"
}

// AIWorkbenchApproval 记录 AI 工作台高风险或受控动作的审批请求与决定。
type AIWorkbenchApproval struct {
	ID            string     `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID        string     `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	StepID        string     `gorm:"column:step_id;type:varchar(255);not null;index" json:"stepId"`
	ActionTitle   string     `gorm:"column:action_title;type:varchar(255);not null" json:"actionTitle"`
	RiskLevel     string     `gorm:"column:risk_level;type:varchar(30);not null;default:'medium';index" json:"riskLevel"`
	RequestReason string     `gorm:"column:request_reason;type:text" json:"requestReason"`
	Status        string     `gorm:"column:status;type:varchar(30);not null;default:'pending';index" json:"status"`
	DecidedBy     string     `gorm:"column:decided_by;type:varchar(255);default:'';index" json:"decidedBy"`
	DecisionNote  string     `gorm:"column:decision_note;type:text" json:"decisionNote"`
	RequestedAt   time.Time  `gorm:"column:requested_at;default:CURRENT_TIMESTAMP;index" json:"requestedAt"`
	DecidedAt     *time.Time `gorm:"column:decided_at" json:"decidedAt"`
	CreatedAt     time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (AIWorkbenchApproval) TableName() string {
	return "ai_approvals"
}

// AIWorkbenchObservation 记录 AI 在任务中沉淀的观察结果。
type AIWorkbenchObservation struct {
	ID         string    `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID     string    `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	StepID     string    `gorm:"column:step_id;type:varchar(255);default:'';index" json:"stepId"`
	Source     string    `gorm:"column:source;type:varchar(100);not null;index" json:"source"`
	TargetRef  string    `gorm:"column:target_ref;type:text" json:"targetRef"`
	Content    string    `gorm:"column:content;type:text;not null" json:"content"`
	Confidence string    `gorm:"column:confidence;type:varchar(30);default:'medium';index" json:"confidence"`
	CreatedAt  time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
}

func (AIWorkbenchObservation) TableName() string {
	return "ai_observations"
}

// AIWorkbenchEvidenceItem 记录可被报告、评估器和假设引用的标准化证据项。
type AIWorkbenchEvidenceItem struct {
	ID             string         `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID         string         `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	StepID         string         `gorm:"column:step_id;type:varchar(255);default:'';index" json:"stepId"`
	SourceType     string         `gorm:"column:source_type;type:varchar(80);not null;index" json:"sourceType"`
	SourceID       string         `gorm:"column:source_id;type:varchar(255);default:'';index" json:"sourceId"`
	TargetRef      string         `gorm:"column:target_ref;type:text" json:"targetRef"`
	Title          string         `gorm:"column:title;type:varchar(255);not null" json:"title"`
	Summary        string         `gorm:"column:summary;type:text;not null" json:"summary"`
	RiskLevel      string         `gorm:"column:risk_level;type:varchar(30);not null;default:'info';index" json:"riskLevel"`
	Confidence     string         `gorm:"column:confidence;type:varchar(30);default:'medium';index" json:"confidence"`
	Status         string         `gorm:"column:status;type:varchar(30);not null;default:'observed';index" json:"status"`
	Reference      string         `gorm:"column:reference;type:text" json:"reference"`
	Metadata       map[string]any `gorm:"column:metadata;type:text;serializer:json" json:"metadata"`
	ObservedAt     time.Time      `gorm:"column:observed_at;default:CURRENT_TIMESTAMP;index" json:"observedAt"`
	CreatedAt      time.Time      `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	LastObservedAt time.Time      `gorm:"column:last_observed_at;default:CURRENT_TIMESTAMP;index" json:"lastObservedAt"`
}

func (AIWorkbenchEvidenceItem) TableName() string {
	return "ai_evidence_items"
}

// AIWorkbenchHypothesis 记录 AI 根据上下文生成的测试假设。
type AIWorkbenchHypothesis struct {
	ID           string    `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID       string    `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	Title        string    `gorm:"column:title;type:varchar(255);not null" json:"title"`
	Description  string    `gorm:"column:description;type:text" json:"description"`
	EvidenceRefs []string  `gorm:"column:evidence_refs;type:text;serializer:json" json:"evidenceRefs"`
	Status       string    `gorm:"column:status;type:varchar(30);not null;default:'candidate';index" json:"status"`
	Confidence   string    `gorm:"column:confidence;type:varchar(30);default:'medium';index" json:"confidence"`
	CreatedAt    time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt    time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (AIWorkbenchHypothesis) TableName() string {
	return "ai_hypotheses"
}

// AIWorkbenchToolCall 记录 AI 调度能力的审计信息。
type AIWorkbenchToolCall struct {
	ID            string    `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID        string    `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	StepID        string    `gorm:"column:step_id;type:varchar(255);default:'';index" json:"stepId"`
	AgentRunID    string    `gorm:"column:agent_run_id;type:varchar(255);default:'';index" json:"agentRunId"`
	ExecutionID   string    `gorm:"column:execution_id;type:varchar(255);default:'';index" json:"executionId"`
	ToolName      string    `gorm:"column:tool_name;type:varchar(255);not null;index" json:"toolName"`
	InputJSON     string    `gorm:"column:input_json;type:text" json:"inputJson"`
	OutputSummary string    `gorm:"column:output_summary;type:text" json:"outputSummary"`
	RawOutputRef  string    `gorm:"column:raw_output_ref;type:text" json:"rawOutputRef"`
	Status        string    `gorm:"column:status;type:varchar(30);not null;default:'pending';index" json:"status"`
	ErrorMessage  string    `gorm:"column:error_message;type:text" json:"errorMessage"`
	CreatedAt     time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt     time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (AIWorkbenchToolCall) TableName() string {
	return "ai_tool_calls"
}

// AIWorkbenchToolExecution 记录 runtime 级别的工具执行生命周期。
type AIWorkbenchToolExecution struct {
	ID            string         `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID        string         `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	StepID        string         `gorm:"column:step_id;type:varchar(255);default:'';index" json:"stepId"`
	AgentRunID    string         `gorm:"column:agent_run_id;type:varchar(255);default:'';index" json:"agentRunId"`
	ToolCallID    string         `gorm:"column:tool_call_id;type:varchar(255);default:'';index" json:"toolCallId"`
	ToolName      string         `gorm:"column:tool_name;type:varchar(255);not null;index" json:"toolName"`
	Runtime       string         `gorm:"column:runtime;type:varchar(80);not null;default:'internal';index" json:"runtime"`
	InputSummary  string         `gorm:"column:input_summary;type:text" json:"inputSummary"`
	Arguments     map[string]any `gorm:"column:arguments;type:text;serializer:json" json:"arguments"`
	Status        string         `gorm:"column:status;type:varchar(30);not null;default:'queued';index" json:"status"`
	OutputSummary string         `gorm:"column:output_summary;type:text" json:"outputSummary"`
	ResultRef     string         `gorm:"column:result_ref;type:text" json:"resultRef"`
	ErrorMessage  string         `gorm:"column:error_message;type:text" json:"errorMessage"`
	StartedAt     *time.Time     `gorm:"column:started_at" json:"startedAt"`
	CompletedAt   *time.Time     `gorm:"column:completed_at" json:"completedAt"`
	DurationMS    int64          `gorm:"column:duration_ms;default:0" json:"durationMs"`
	Metadata      map[string]any `gorm:"column:metadata;type:text;serializer:json" json:"metadata"`
	CreatedAt     time.Time      `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt     time.Time      `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (AIWorkbenchToolExecution) TableName() string {
	return "ai_tool_executions"
}

// AIWorkbenchAgentRun 记录 AI 智脑围绕对话意图自动推进的一轮 AgentRun。
type AIWorkbenchAgentRun struct {
	ID            string         `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID        string         `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	ParentRunID   string         `gorm:"column:parent_run_id;type:varchar(255);default:'';index" json:"parentRunId"`
	RunIndex      int            `gorm:"column:run_index;not null;default:1;index" json:"runIndex"`
	TriggerType   string         `gorm:"column:trigger_type;type:varchar(80);not null;default:'orchestrator';index" json:"triggerType"`
	AgentProfile  string         `gorm:"column:agent_profile;type:varchar(80);not null;default:'deep_agent';index" json:"agentProfile"`
	AgentModel    string         `gorm:"column:agent_model;type:varchar(255);default:'';index" json:"agentModel"`
	Status        string         `gorm:"column:status;type:varchar(40);not null;default:'running';index" json:"status"`
	Goal          string         `gorm:"column:goal;type:text" json:"goal"`
	Summary       string         `gorm:"column:summary;type:text" json:"summary"`
	CurrentStepID string         `gorm:"column:current_step_id;type:varchar(255);default:'';index" json:"currentStepId"`
	StartedAt     *time.Time     `gorm:"column:started_at" json:"startedAt"`
	CompletedAt   *time.Time     `gorm:"column:completed_at" json:"completedAt"`
	DurationMS    int64          `gorm:"column:duration_ms;default:0" json:"durationMs"`
	Metadata      map[string]any `gorm:"column:metadata;type:text;serializer:json" json:"metadata"`
	CreatedAt     time.Time      `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt     time.Time      `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (AIWorkbenchAgentRun) TableName() string {
	return "ai_agent_runs"
}

// AIWorkbenchFinding 是 AI 智脑统一漏洞管理视图的规范化漏洞记录。
type AIWorkbenchFinding struct {
	ID            string         `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	AIRequestID   string         `gorm:"column:ai_request_id;type:varchar(255);not null;index" json:"aiRequestId"`
	SourceTaskID  string         `gorm:"column:source_task_id;type:varchar(255);not null;index" json:"sourceTaskId"`
	SourceVersion int            `gorm:"column:source_version;default:1;index" json:"sourceVersion"`
	SourceType    string         `gorm:"column:source_type;type:varchar(80);not null;index" json:"sourceType"`
	SourceID      string         `gorm:"column:source_id;type:varchar(255);default:'';index" json:"sourceId"`
	DedupKey      string         `gorm:"column:dedup_key;type:varchar(80);not null;uniqueIndex" json:"dedupKey"`
	Target        string         `gorm:"column:target;type:text;not null" json:"target"`
	URL           string         `gorm:"column:url;type:text;not null" json:"url"`
	Name          string         `gorm:"column:name;type:varchar(255);not null" json:"name"`
	Severity      string         `gorm:"column:severity;type:varchar(50);not null;default:'info';index" json:"severity"`
	Status        string         `gorm:"column:status;type:varchar(50);not null;default:'open';index" json:"status"`
	Evidence      string         `gorm:"column:evidence;type:text" json:"evidence"`
	Impact        string         `gorm:"column:impact;type:text" json:"impact"`
	Remediation   string         `gorm:"column:remediation;type:text" json:"remediation"`
	Reference     string         `gorm:"column:reference;type:text" json:"reference"`
	Metadata      map[string]any `gorm:"column:metadata;type:text;serializer:json" json:"metadata"`
	FirstSeenAt   time.Time      `gorm:"column:first_seen_at;default:CURRENT_TIMESTAMP;index" json:"firstSeenAt"`
	LastSeenAt    time.Time      `gorm:"column:last_seen_at;default:CURRENT_TIMESTAMP;index" json:"lastSeenAt"`
	CreatedAt     time.Time      `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt     time.Time      `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (AIWorkbenchFinding) TableName() string {
	return "ai_findings"
}

// AIWorkbenchMessage 记录用户与 AI 智脑围绕任务的线程消息。
type AIWorkbenchMessage struct {
	ID              string         `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID          string         `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	ParentMessageID string         `gorm:"column:parent_message_id;type:varchar(255);default:'';index" json:"parentMessageId"`
	Role            string         `gorm:"column:role;type:varchar(30);not null;index" json:"role"`
	Content         string         `gorm:"column:content;type:text;not null" json:"content"`
	Metadata        map[string]any `gorm:"column:metadata;type:text;serializer:json" json:"metadata"`
	CreatedAt       time.Time      `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
}

func (AIWorkbenchMessage) TableName() string {
	return "ai_messages"
}

// AIWorkbenchSideChat stores a tool-free side conversation separately from
// the main workbench transcript while keeping it scoped to one conversation.
type AIWorkbenchSideChat struct {
	ConversationID string                       `gorm:"primaryKey;column:conversation_id;type:varchar(255)" json:"conversationId"`
	Messages       []AIWorkbenchSideChatMessage `gorm:"column:messages;type:text;serializer:json" json:"messages"`
	UpdatedAt      time.Time                    `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
}

type AIWorkbenchSideChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (AIWorkbenchSideChat) TableName() string {
	return "ai_workbench_side_chats"
}

// AIWorkbenchEvent 是 AI 工作台的标准化事件流。
type AIWorkbenchEvent struct {
	ID        string         `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID    string         `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	StepID    string         `gorm:"column:step_id;type:varchar(255);default:'';index" json:"stepId"`
	Type      string         `gorm:"column:type;type:varchar(80);not null;index" json:"type"`
	Level     string         `gorm:"column:level;type:varchar(30);not null;default:'info';index" json:"level"`
	Title     string         `gorm:"column:title;type:varchar(255);not null" json:"title"`
	Content   string         `gorm:"column:content;type:text" json:"content"`
	Payload   map[string]any `gorm:"column:payload;type:text;serializer:json" json:"payload"`
	CreatedAt time.Time      `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
}

func (AIWorkbenchEvent) TableName() string {
	return "ai_events"
}

// AIWorkbenchAssetBucket 聚合同一目标引用下的结构化资产状态。
type AIWorkbenchAssetBucket struct {
	ID          string         `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID      string         `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	BucketType  string         `gorm:"column:bucket_type;type:varchar(50);not null;index" json:"bucketType"`
	TargetRef   string         `gorm:"column:target_ref;type:text;not null" json:"targetRef"`
	RiskScore   int            `gorm:"column:risk_score;default:0;index" json:"riskScore"`
	Summary     string         `gorm:"column:summary;type:text" json:"summary"`
	Signals     map[string]any `gorm:"column:signals;type:text;serializer:json" json:"signals"`
	LastEventID string         `gorm:"column:last_event_id;type:varchar(255);default:''" json:"lastEventId"`
	CreatedAt   time.Time      `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (AIWorkbenchAssetBucket) TableName() string {
	return "ai_asset_buckets"
}

// AIWorkbenchGoalEvaluation 记录任务是否达到成功标准的评估结果。
type AIWorkbenchGoalEvaluation struct {
	ID           string    `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID       string    `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	Status       string    `gorm:"column:status;type:varchar(30);not null;index" json:"status"`
	Score        int       `gorm:"column:score;default:0" json:"score"`
	MissingItems []string  `gorm:"column:missing_items;type:text;serializer:json" json:"missingItems"`
	Feedback     string    `gorm:"column:feedback;type:text" json:"feedback"`
	TraceSummary string    `gorm:"column:trace_summary;type:text" json:"traceSummary"`
	Evaluator    string    `gorm:"column:evaluator;type:varchar(80);not null;default:'rule'" json:"evaluator"`
	CreatedAt    time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
}

func (AIWorkbenchGoalEvaluation) TableName() string {
	return "ai_goal_evaluations"
}

// AIWorkbenchAPIEndpoint 记录 AI 智脑在接口测试中沉淀的 API 节点。
type AIWorkbenchAPIEndpoint struct {
	ID             string         `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID         string         `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	StepID         string         `gorm:"column:step_id;type:varchar(255);default:'';index" json:"stepId"`
	Method         string         `gorm:"column:method;type:varchar(20);not null;default:'GET';index" json:"method"`
	URL            string         `gorm:"column:url;type:text;not null" json:"url"`
	Host           string         `gorm:"column:host;type:varchar(255);default:'';index" json:"host"`
	Path           string         `gorm:"column:path;type:text" json:"path"`
	OperationName  string         `gorm:"column:operation_name;type:varchar(255);default:''" json:"operationName"`
	AuthRequired   bool           `gorm:"column:auth_required;default:false;index" json:"authRequired"`
	AuthProfileID  string         `gorm:"column:auth_profile_id;type:varchar(255);default:'';index" json:"authProfileId"`
	Source         string         `gorm:"column:source;type:varchar(80);not null;default:'manual';index" json:"source"`
	Tags           []string       `gorm:"column:tags;type:text;serializer:json" json:"tags"`
	Metadata       map[string]any `gorm:"column:metadata;type:text;serializer:json" json:"metadata"`
	CreatedAt      time.Time      `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt      time.Time      `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
	LastObservedAt time.Time      `gorm:"column:last_observed_at;default:CURRENT_TIMESTAMP;index" json:"lastObservedAt"`
}

func (AIWorkbenchAPIEndpoint) TableName() string {
	return "ai_api_endpoints"
}

// AIWorkbenchAPIRelation 记录接口之间的认证、变量、业务对象和调用顺序关系。
type AIWorkbenchAPIRelation struct {
	ID               string         `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID           string         `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	SourceEndpointID string         `gorm:"column:source_endpoint_id;type:varchar(255);not null;index" json:"sourceEndpointId"`
	TargetEndpointID string         `gorm:"column:target_endpoint_id;type:varchar(255);not null;index" json:"targetEndpointId"`
	RelationType     string         `gorm:"column:relation_type;type:varchar(80);not null;index" json:"relationType"`
	VariableName     string         `gorm:"column:variable_name;type:varchar(255);default:'';index" json:"variableName"`
	ObjectType       string         `gorm:"column:object_type;type:varchar(120);default:'';index" json:"objectType"`
	EvidenceRef      string         `gorm:"column:evidence_ref;type:varchar(255);default:'';index" json:"evidenceRef"`
	Confidence       string         `gorm:"column:confidence;type:varchar(30);default:'medium';index" json:"confidence"`
	Metadata         map[string]any `gorm:"column:metadata;type:text;serializer:json" json:"metadata"`
	CreatedAt        time.Time      `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt        time.Time      `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (AIWorkbenchAPIRelation) TableName() string {
	return "ai_api_relations"
}

// AIWorkbenchAuthProfile 记录接口测试中识别出的认证方式和凭证来源。
type AIWorkbenchAuthProfile struct {
	ID              string         `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID          string         `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	Name            string         `gorm:"column:name;type:varchar(255);not null" json:"name"`
	AuthType        string         `gorm:"column:auth_type;type:varchar(80);not null;index" json:"authType"`
	Location        string         `gorm:"column:location;type:varchar(80);default:'';index" json:"location"`
	KeyName         string         `gorm:"column:key_name;type:varchar(255);default:'';index" json:"keyName"`
	TokenRef        string         `gorm:"column:token_ref;type:varchar(255);default:'';index" json:"tokenRef"`
	LoginEndpointID string         `gorm:"column:login_endpoint_id;type:varchar(255);default:'';index" json:"loginEndpointId"`
	Metadata        map[string]any `gorm:"column:metadata;type:text;serializer:json" json:"metadata"`
	CreatedAt       time.Time      `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (AIWorkbenchAuthProfile) TableName() string {
	return "ai_auth_profiles"
}

// AIWorkbenchExtractedVariable 记录从响应、脚本或证据中提取出的链路变量。
type AIWorkbenchExtractedVariable struct {
	ID         string         `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID     string         `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	StepID     string         `gorm:"column:step_id;type:varchar(255);default:'';index" json:"stepId"`
	Name       string         `gorm:"column:name;type:varchar(255);not null;index" json:"name"`
	Value      string         `gorm:"column:value;type:text" json:"value"`
	ValueType  string         `gorm:"column:value_type;type:varchar(80);default:'string';index" json:"valueType"`
	SourceType string         `gorm:"column:source_type;type:varchar(80);not null;index" json:"sourceType"`
	SourceID   string         `gorm:"column:source_id;type:varchar(255);default:'';index" json:"sourceId"`
	JSONPath   string         `gorm:"column:json_path;type:text" json:"jsonPath"`
	Regex      string         `gorm:"column:regex;type:text" json:"regex"`
	HeaderName string         `gorm:"column:header_name;type:varchar(255);default:''" json:"headerName"`
	EndpointID string         `gorm:"column:endpoint_id;type:varchar(255);default:'';index" json:"endpointId"`
	Confidence string         `gorm:"column:confidence;type:varchar(30);default:'medium';index" json:"confidence"`
	Metadata   map[string]any `gorm:"column:metadata;type:text;serializer:json" json:"metadata"`
	CreatedAt  time.Time      `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
}

func (AIWorkbenchExtractedVariable) TableName() string {
	return "ai_extracted_variables"
}

// AIWorkbenchHTTPRequestEvidence 记录受控 HTTP 接口测试的请求响应摘要。
type AIWorkbenchHTTPRequestEvidence struct {
	ID                  string            `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID              string            `gorm:"column:task_id;type:varchar(255);not null;index" json:"taskId"`
	StepID              string            `gorm:"column:step_id;type:varchar(255);default:'';index" json:"stepId"`
	ToolCallID          string            `gorm:"column:tool_call_id;type:varchar(255);default:'';index" json:"toolCallId"`
	EndpointID          string            `gorm:"column:endpoint_id;type:varchar(255);default:'';index" json:"endpointId"`
	Method              string            `gorm:"column:method;type:varchar(20);not null;index" json:"method"`
	URL                 string            `gorm:"column:url;type:text;not null" json:"url"`
	RequestHeaders      map[string]string `gorm:"column:request_headers;type:text;serializer:json" json:"requestHeaders"`
	RequestBodyPreview  string            `gorm:"column:request_body_preview;type:text" json:"requestBodyPreview"`
	StatusCode          int               `gorm:"column:status_code;default:0;index" json:"statusCode"`
	ResponseHeaders     map[string]string `gorm:"column:response_headers;type:text;serializer:json" json:"responseHeaders"`
	ResponseBodyPreview string            `gorm:"column:response_body_preview;type:text" json:"responseBodyPreview"`
	ResponseLength      int               `gorm:"column:response_length;default:0" json:"responseLength"`
	DurationMS          int64             `gorm:"column:duration_ms;default:0" json:"durationMs"`
	Variables           map[string]string `gorm:"column:variables;type:text;serializer:json" json:"variables"`
	CreatedAt           time.Time         `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
}

func (AIWorkbenchHTTPRequestEvidence) TableName() string {
	return "ai_http_request_evidence"
}

// AIWorkbenchCustomStrategyPack 记录用户自定义 AI 策略包。
type AIWorkbenchCustomStrategyPack struct {
	ID                    string    `gorm:"primaryKey;column:id;type:varchar(120)" json:"id"`
	Title                 string    `gorm:"column:title;type:varchar(255);not null" json:"title"`
	Scenario              string    `gorm:"column:scenario;type:text" json:"scenario"`
	Description           string    `gorm:"column:description;type:text;not null" json:"description"`
	Triggers              []string  `gorm:"column:triggers;type:text;serializer:json" json:"triggers"`
	PreferredCapabilities []string  `gorm:"column:preferred_capabilities;type:text;serializer:json" json:"preferredCapabilities"`
	EvidenceRequirements  []string  `gorm:"column:evidence_requirements;type:text;serializer:json" json:"evidenceRequirements"`
	SafetyRules           []string  `gorm:"column:safety_rules;type:text;serializer:json" json:"safetyRules"`
	Enabled               bool      `gorm:"column:enabled;default:true;index" json:"enabled"`
	CreatedBy             string    `gorm:"column:created_by;type:varchar(255);default:'';index" json:"createdBy"`
	CreatedAt             time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt             time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (AIWorkbenchCustomStrategyPack) TableName() string {
	return "ai_custom_strategy_packs"
}

// Setting 设置表
type Setting struct {
	Key       string    `gorm:"primaryKey;column:key;type:varchar(255)" json:"key"`
	Value     string    `gorm:"column:value;type:text;not null" json:"value"`
	CreatedAt time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (Setting) TableName() string {
	return "setting"
}

// CustomTheme 自定义界面主题。
//
// Name 保留用户输入的展示名，NameKey 为去空白、转小写后的唯一键，
// 用于在数据库层保证主题名称不重复（大小写与多余空白不敏感）。
type CustomTheme struct {
	ID         uint      `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Name       string    `gorm:"column:name;type:varchar(120);not null" json:"name"`
	NameKey    string    `gorm:"column:name_key;type:varchar(120);not null;uniqueIndex:idx_custom_theme_name_key" json:"-"`
	Definition string    `gorm:"column:definition;type:text;not null" json:"-"`
	CreatedAt  time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt  time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (CustomTheme) TableName() string {
	return "custom_theme"
}

// DirsearchPathHistory 目录扫描路径历史
type DirsearchPathHistory struct {
	Path  string `gorm:"primaryKey;column:path;type:varchar(2048)" json:"path"`
	Times int    `gorm:"column:times;default:0;index:idx_dirsearch_path_history_times,sort:desc" json:"times"`
}

func (DirsearchPathHistory) TableName() string {
	return "dirsearch_path_history"
}

// FingerprintInfo 指纹信息
type FingerprintInfo struct {
	TaskID               string    `gorm:"column:task_id;type:varchar(255);index:idx_fingerprint_info_task_version" json:"taskId"`
	Version              int       `gorm:"column:version;default:1;index:idx_fingerprint_info_task_version" json:"version"` // 扫描版本号
	URL                  string    `gorm:"column:url;type:text" json:"url"`
	Status               int       `gorm:"column:status" json:"status"`
	VerificationStatus   string    `gorm:"column:verification_status;type:varchar(50);default:'pending';index" json:"verificationStatus"`
	Length               int       `gorm:"column:length" json:"length"`
	Title                string    `gorm:"column:title;type:text" json:"title"`
	Detect               string    `gorm:"column:detect;type:text" json:"detect"`
	Fingerprints         string    `gorm:"column:fingerprints;type:text" json:"fingerprints"`                   // 普通指纹列表（JSON数组）
	HighRiskFingerprints string    `gorm:"column:high_risk_fingerprints;type:text" json:"highRiskFingerprints"` // 高危指纹列表（JSON数组）
	AssetTags            string    `gorm:"column:asset_tags;type:text" json:"assetTags"`                        // 资产标签列表（JSON数组）
	Screenshot           string    `gorm:"column:screenshot;type:text" json:"screenshot"`
	Favicon              string    `gorm:"column:favicon;type:text" json:"favicon"`                                                // favicon图片路径
	IconHash             string    `gorm:"column:icon_hash;type:varchar(32);index:idx_fingerprint_info_icon_hash" json:"iconHash"` // FOFA-compatible favicon MMH3 hash
	Host                 string    `gorm:"column:host;type:varchar(255)" json:"host"`
	Scheme               string    `gorm:"column:scheme;type:varchar(50)" json:"scheme"`
	Port                 int       `gorm:"column:port" json:"port"`
	CreatedAt            time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
}

func (FingerprintInfo) TableName() string {
	return "fingerprint_info"
}

// VulnerabilityInfo 漏洞信息
type VulnerabilityInfo struct {
	ID               string `gorm:"primaryKey;column:id;type:varchar(36);not null;uniqueIndex:idx_vulnerability_info_id" json:"id"`
	TaskID           string `gorm:"column:task_id;type:varchar(255);index:idx_vulnerability_info_task_version" json:"taskId"`
	Version          int    `gorm:"column:version;default:1;index:idx_vulnerability_info_task_version" json:"version"` // 扫描版本号
	TemplateID       string `gorm:"column:template_id;type:varchar(255)" json:"templateId"`
	VulnName         string `gorm:"column:vuln_name;type:varchar(255)" json:"vulnName"`
	Protocol         string `gorm:"column:protocol;type:varchar(50)" json:"protocol"`
	Severity         string `gorm:"column:severity;type:varchar(50)" json:"severity"`
	DisposalStatus   string `gorm:"column:disposal_status;type:varchar(50);default:'open';index" json:"disposalStatus"`
	VulnURL          string `gorm:"column:vuln_url;type:text" json:"vulnUrl"`
	Extract          string `gorm:"column:extract;type:text" json:"extract"`
	Request          string `gorm:"column:request;type:text" json:"request"`
	Response         string `gorm:"column:response;type:text" json:"response"`
	Description      string `gorm:"column:description;type:text" json:"description"`
	Impact           string `gorm:"column:impact;type:text" json:"impact"`
	Remediation      string `gorm:"column:remediation;type:text" json:"remediation"`
	Reference        string `gorm:"column:reference;type:text" json:"reference"`
	ResponseTime     string `gorm:"column:response_time;type:varchar(50)" json:"responseTime"`
	PrincipDetection bool   `gorm:"column:principl_detection;default:false" json:"principDetection"`
}

func (VulnerabilityInfo) TableName() string {
	return "vulnerability_info"
}

type VulnStaticContext struct {
	SourceURL string `json:"sourceUrl"`
	Snippet   string `json:"snippet"`
}

type VulnScanJSResource struct {
	ID           string            `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID       string            `gorm:"column:task_id;type:varchar(255);not null;index:idx_vulnscan_js_resource_task_id" json:"taskId"`
	Version      int               `gorm:"column:version;default:1;index:idx_vulnscan_js_resource_task_version" json:"version"`
	URL          string            `gorm:"column:url;type:text;not null" json:"url"`
	Content      string            `gorm:"column:content;type:text" json:"content"`
	ResponseCode int               `gorm:"column:response_code;default:0" json:"responseCode"`
	Headers      map[string]string `gorm:"column:headers;type:text;serializer:json" json:"headers"`
	Size         int               `gorm:"column:size;default:0" json:"size"`
	FetchedAt    time.Time         `gorm:"column:fetched_at;default:CURRENT_TIMESTAMP;index" json:"fetchedAt"`
	Task         VulnScanTask      `gorm:"foreignKey:TaskID,Version;references:ID,Version;constraint:OnDelete:CASCADE" json:"-"`
}

func (VulnScanJSResource) TableName() string {
	return "vuln_scan_js_resource"
}

type VulnScanJSAPIResource struct {
	ID               string            `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID           string            `gorm:"column:task_id;type:varchar(255);not null;index:idx_vulnscan_js_api_resource_task_id" json:"taskId"`
	Version          int               `gorm:"column:version;default:1;index:idx_vulnscan_js_api_resource_task_version" json:"version"`
	URL              string            `gorm:"column:url;type:text;not null" json:"url"`
	Method           string            `gorm:"column:method;type:varchar(20);default:''" json:"method"`
	TraceID          string            `gorm:"column:trace_id;type:varchar(255);default:'';index" json:"traceId"`
	HasProtocolTrace bool              `gorm:"column:has_protocol_trace;default:false;index" json:"hasProtocolTrace"`
	RequestHeaders   map[string]string `gorm:"column:request_headers;type:text;serializer:json" json:"requestHeaders"`
	RequestBody      string            `gorm:"column:request_body;type:text" json:"requestBody"`
	ResponseHeaders  map[string]string `gorm:"column:response_headers;type:text;serializer:json" json:"responseHeaders"`
	ResponseBody     string            `gorm:"column:response_body;type:text" json:"responseBody"`
	ResponseCode     int               `gorm:"column:response_code;default:0" json:"responseCode"`
	Headers          map[string]string `gorm:"column:headers;type:text;serializer:json" json:"headers"`
	FetchedAt        time.Time         `gorm:"column:fetched_at;default:CURRENT_TIMESTAMP;index" json:"fetchedAt"`
	Task             VulnScanTask      `gorm:"foreignKey:TaskID,Version;references:ID,Version;constraint:OnDelete:CASCADE" json:"-"`
}

func (VulnScanJSAPIResource) TableName() string {
	return "vuln_scan_js_api_resource"
}

// VulnScanJSOperation stores Trailblazer's normalized, safe-to-reuse API
// operation template. It retains dynamic-parameter and source evidence.
type VulnScanJSOperation struct {
	ID              string    `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID          string    `gorm:"column:task_id;type:varchar(255);not null;index:idx_vulnscan_js_operation_task_version" json:"taskId"`
	Version         int       `gorm:"column:version;not null;default:1;index:idx_vulnscan_js_operation_task_version" json:"version"`
	TargetURL       string    `gorm:"column:target_url;type:text" json:"targetUrl"`
	OperationID     string    `gorm:"column:operation_id;type:varchar(255);not null;index" json:"operationId"`
	Method          string    `gorm:"column:method;type:varchar(20);default:'';index" json:"method"`
	PathTemplate    string    `gorm:"column:path_template;type:text;not null" json:"pathTemplate"`
	Source          string    `gorm:"column:source;type:varchar(50);default:'';index" json:"source"`
	Confidence      string    `gorm:"column:confidence;type:varchar(50);default:''" json:"confidence"`
	RequiresRuntime bool      `gorm:"column:requires_runtime;default:false;index" json:"requiresRuntime"`
	OperationJSON   string    `gorm:"column:operation_json;type:text;not null" json:"operationJson"`
	CreatedAt       time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt       time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (VulnScanJSOperation) TableName() string { return "vuln_scan_js_operation" }

// VulnScanJSOpenAPIDocument is the versioned OpenAPI 3.1 snapshot generated
// while Trailblazer's in-memory APIStore is still available.
type VulnScanJSOpenAPIDocument struct {
	ID        string    `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID    string    `gorm:"column:task_id;type:varchar(255);not null;index:idx_vulnscan_js_openapi_task_version" json:"taskId"`
	Version   int       `gorm:"column:version;not null;default:1;index:idx_vulnscan_js_openapi_task_version" json:"version"`
	TargetURL string    `gorm:"column:target_url;type:text;not null" json:"targetUrl"`
	Document  string    `gorm:"column:document;type:text;not null" json:"document"`
	CreatedAt time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (VulnScanJSOpenAPIDocument) TableName() string { return "vuln_scan_js_openapi_document" }

// CompanyInfoTask 企业信息查询任务表
type CompanyInfoTask struct {
	ID           string     `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskName     string     `gorm:"column:task_name;type:varchar(255)" json:"taskName"`
	Status       string     `gorm:"column:status;type:varchar(20);not null;default:'pending';index" json:"status"` // pending, running, completed, failed, cancelled
	Progress     int        `gorm:"column:progress;default:0" json:"progress"`                                     // 0-100
	TotalCount   int        `gorm:"column:total_count;default:0" json:"totalCount"`                                // 总公司数量
	CurrentCount int        `gorm:"column:current_count;default:0" json:"currentCount"`                            // 当前已处理数量
	Config       string     `gorm:"column:config;type:text;not null" json:"config"`                                // JSON 格式存储任务配置
	ErrorMsg     string     `gorm:"column:error_msg;type:text" json:"errorMsg"`                                    // 错误信息
	StartedAt    *time.Time `gorm:"column:started_at" json:"startedAt"`                                            // 开始时间
	CompletedAt  *time.Time `gorm:"column:completed_at" json:"completedAt"`                                        // 完成时间
	CreatedAt    time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index:idx_company_info_task_created_at,sort:desc" json:"createdAt"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (CompanyInfoTask) TableName() string {
	return "company_info_task"
}

// CompanyInfoTaskResult 企业信息查询结果表
type CompanyInfoTaskResult struct {
	ID        string          `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID    string          `gorm:"column:task_id;type:varchar(255);not null;index:idx_company_info_task_result_task_id" json:"taskId"`
	Data      string          `gorm:"column:data;type:text;not null" json:"data"` // JSON 格式存储企业信息
	CreatedAt time.Time       `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	Task      CompanyInfoTask `gorm:"foreignKey:TaskID;references:ID;constraint:OnDelete:CASCADE" json:"-"`
}

func (CompanyInfoTaskResult) TableName() string {
	return "company_info_task_result"
}

// APIDocTestTask 接口文档测试任务表
type APIDocTestTask struct {
	ID            string            `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskName      string            `gorm:"column:task_name;type:varchar(255);not null" json:"taskName"`
	Status        string            `gorm:"column:status;type:varchar(20);not null;default:'pending';index" json:"status"`
	Progress      int               `gorm:"column:progress;default:0" json:"progress"`
	SourceURL     string            `gorm:"column:source_url;type:text;not null" json:"sourceUrl"`
	BaseURL       string            `gorm:"column:base_url;type:text" json:"baseUrl"`
	DocumentType  string            `gorm:"column:document_type;type:varchar(30);default:'auto'" json:"documentType"`
	Headers       map[string]string `gorm:"column:headers;type:text;serializer:json" json:"headers,omitempty"`
	MaxEndpoints  int               `gorm:"column:max_endpoints;default:120" json:"maxEndpoints"`
	EndpointCount int               `gorm:"column:endpoint_count;default:0" json:"endpointCount"`
	TestedCount   int               `gorm:"column:tested_count;default:0" json:"testedCount"`
	ErrorMsg      string            `gorm:"column:error_msg;type:text" json:"errorMsg"`
	StartedAt     *time.Time        `gorm:"column:started_at" json:"startedAt"`
	CompletedAt   *time.Time        `gorm:"column:completed_at" json:"completedAt"`
	CreatedAt     time.Time         `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index:idx_api_doc_test_task_created_at,sort:desc" json:"createdAt"`
	UpdatedAt     time.Time         `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (APIDocTestTask) TableName() string {
	return "api_doc_test_task"
}

// APIDocTestResult 接口文档测试结果表
type APIDocTestResult struct {
	ID                string              `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID            string              `gorm:"column:task_id;type:varchar(255);not null;index:idx_api_doc_test_result_task_id" json:"taskId"`
	URL               string              `gorm:"column:url;type:text;not null" json:"url"`
	Method            string              `gorm:"column:method;type:varchar(20);not null" json:"method"`
	Summary           string              `gorm:"column:summary;type:text" json:"summary"`
	StatusCode        int                 `gorm:"column:status_code;default:0;index" json:"statusCode"`
	StatusText        string              `gorm:"column:status_text;type:varchar(80)" json:"statusText"`
	ResponseLength    int                 `gorm:"column:response_length;default:0" json:"responseLength"`
	DurationMS        int64               `gorm:"column:duration_ms;default:0" json:"durationMs"`
	TestTags          []string            `gorm:"column:test_tags;type:text;serializer:json" json:"testTags,omitempty"`
	RawRequest        string              `gorm:"column:raw_request;type:text" json:"rawRequest"`
	RequestHeaders    map[string]string   `gorm:"column:request_headers;type:text;serializer:json" json:"requestHeaders,omitempty"`
	RequestBody       string              `gorm:"column:request_body;type:text" json:"requestBody"`
	ResponseHeaders   map[string][]string `gorm:"column:response_headers;type:text;serializer:json" json:"responseHeaders,omitempty"`
	ResponseBody      string              `gorm:"column:response_body;type:text" json:"responseBody"`
	ResponseTruncated bool                `gorm:"column:response_truncated;default:false" json:"responseTruncated"`
	ErrorMsg          string              `gorm:"column:error_msg;type:text" json:"errorMsg"`
	CreatedAt         time.Time           `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	Task              APIDocTestTask      `gorm:"foreignKey:TaskID;references:ID;constraint:OnDelete:CASCADE" json:"-"`
}

func (APIDocTestResult) TableName() string {
	return "api_doc_test_result"
}

// RepeaterTab 请求转发标签页
type RepeaterTab struct {
	ID             string    `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	Name           string    `gorm:"column:name;type:varchar(255);not null" json:"name"`
	Method         string    `gorm:"column:method;type:varchar(50);not null" json:"method"`
	URL            string    `gorm:"column:url;type:text" json:"url"`
	RawRequest     string    `gorm:"column:raw_request;type:text" json:"rawRequest"`
	Response       string    `gorm:"column:response;type:text" json:"response"`
	ResponseLength int       `gorm:"column:response_length;default:0" json:"responseLength"`
	ResponseTime   int       `gorm:"column:response_time;default:0" json:"responseTime"`
	HasRedirect    bool      `gorm:"column:has_redirect;default:false" json:"hasRedirect"`
	ForceHttps     bool      `gorm:"column:force_https;default:false" json:"forceHttps"`
	SortOrder      int       `gorm:"column:sort_order;default:0" json:"sortOrder"`
	CreatedAt      time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt      time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (RepeaterTab) TableName() string {
	return "repeater_tab"
}

// PoCDebugTab PoC 调试工作台标签页
type PoCDebugTab struct {
	ID         string    `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	Name       string    `gorm:"column:name;type:varchar(255);not null" json:"name"`
	TemplateID string    `gorm:"column:template_id;type:varchar(255);default:''" json:"templateId"`
	YAML       string    `gorm:"column:yaml;type:text;not null" json:"yaml"`
	Target     string    `gorm:"column:target;type:text;default:''" json:"target"`
	Source     string    `gorm:"column:source;type:varchar(50);default:'manual';index" json:"source"`
	SourceName string    `gorm:"column:source_name;type:varchar(255);default:''" json:"sourceName"`
	ActiveView string    `gorm:"column:active_view;type:varchar(20);default:'template'" json:"activeView"`
	LastStatus string    `gorm:"column:last_status;type:varchar(30);default:''" json:"lastStatus"`
	IsActive   bool      `gorm:"column:is_active;default:false" json:"isActive"`
	SortOrder  int       `gorm:"column:sort_order;default:0" json:"sortOrder"`
	CreatedAt  time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt  time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (PoCDebugTab) TableName() string {
	return "poc_debug_tab"
}

// DBCollectResult 数据库采集结果
type DBCollectResult struct {
	ID               string              `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	ConnectionNanoid string              `gorm:"column:connection_nanoid;type:varchar(255);not null;index" json:"connectionNanoid"`
	TableNameField   string              `gorm:"column:table_name;type:varchar(255);not null" json:"tableName"`
	Content          string              `gorm:"column:content;type:text;not null" json:"content"` // versioned columns/rows JSON; legacy HTML is read as text only
	RowsCount        int                 `gorm:"column:rows_count;default:0" json:"rowsCount"`
	MatchedType      string              `gorm:"column:matched_type;type:varchar(50)" json:"matchedType"`
	CreatedAt        time.Time           `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	Connection       DBConnectionManager `gorm:"foreignKey:ConnectionNanoid;references:Nanoid;constraint:OnDelete:CASCADE" json:"-"`
}

func (DBCollectResult) TableName() string {
	return "db_collect_result"
}

// DBCollectCustomRule 数据库信息采集自定义行匹配规则
type DBCollectCustomRule struct {
	ID        string    `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	Label     string    `gorm:"column:label;type:varchar(100);not null" json:"label"`
	Pattern   string    `gorm:"column:pattern;type:text;not null" json:"pattern"`
	CreatedAt time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (DBCollectCustomRule) TableName() string {
	return "db_collect_custom_rule"
}

// ScanTemplate 通用扫描模板（支持多种任务类型）
type ScanTemplate struct {
	ID          string    `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	Name        string    `gorm:"column:name;type:varchar(255);not null" json:"name"`
	Description string    `gorm:"column:description;type:text" json:"description"`
	TaskType    string    `gorm:"column:task_type;type:varchar(50);not null;default:'webscan';index" json:"taskType"` // webscan, portscan, dirsearch, jsscan, crack, combination, ai_dialogue
	Config      string    `gorm:"column:config;type:text" json:"config"`                                              // JSON 格式存储任务配置
	CreatedAt   time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt   time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (ScanTemplate) TableName() string {
	return "scan_template"
}

// KeyManagerEntry 密钥管理条目
type KeyManagerEntry struct {
	ID          string    `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	Name        string    `gorm:"column:name;type:varchar(255);not null" json:"name"`
	Type        string    `gorm:"column:type;type:varchar(50);not null;index" json:"type"`
	BaseURL     string    `gorm:"column:base_url;type:varchar(500);default:''" json:"baseUrl"`
	Credentials string    `gorm:"column:credentials;type:text;default:''" json:"credentials"`
	Headers     string    `gorm:"column:headers;type:text;default:''" json:"headers"`
	Notes       string    `gorm:"column:notes;type:text;default:''" json:"notes"`
	CreatedAt   time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt   time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (KeyManagerEntry) TableName() string {
	return "key_manager_entry"
}

// VulnScanJSVulnerability 漏洞扫描任务的JS扫描漏洞结果（链接到 VulnScanTask）
type VulnScanJSVulnerability struct {
	ID                 string              `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID             string              `gorm:"column:task_id;type:varchar(255);not null;index:idx_vulnscan_js_vuln_task_id" json:"taskId"`
	Version            int                 `gorm:"column:version;default:1;index:idx_vulnscan_js_vuln_task_version" json:"version"`
	Title              string              `gorm:"column:title;type:varchar(255);not null" json:"title"`
	Level              string              `gorm:"column:level;type:varchar(50);not null;index" json:"level"` // high, medium, low, info
	Type               string              `gorm:"column:type;type:varchar(100);not null" json:"type"`
	URL                string              `gorm:"column:url;type:text;not null" json:"url"`
	Method             string              `gorm:"column:method;type:varchar(20)" json:"method"`
	Request            string              `gorm:"column:request;type:text" json:"request"`
	Response           string              `gorm:"column:response;type:text" json:"response"`
	ResponseType       string              `gorm:"column:response_type;type:varchar(255);default:'';index" json:"responseType"`
	TraceID            string              `gorm:"column:trace_id;type:varchar(255);default:'';index" json:"traceId"`
	HasProtocolTrace   bool                `gorm:"column:has_protocol_trace;default:false;index" json:"hasProtocolTrace"`
	ResponseCiphertext string              `gorm:"column:response_ciphertext;type:text" json:"responseCiphertext"`
	DecryptionStatus   string              `gorm:"column:decryption_status;type:varchar(100);default:'';index" json:"decryptionStatus"`
	DecryptionDetail   string              `gorm:"column:decryption_detail;type:text" json:"decryptionDetail"`
	ResponseLength     int                 `gorm:"column:response_length;default:0" json:"responseLength"`
	Confidence         string              `gorm:"column:confidence;type:varchar(50);default:''" json:"confidence"`
	ConfidenceReason   string              `gorm:"column:confidence_reason;type:text" json:"confidenceReason"`
	DenyTemplateID     string              `gorm:"column:deny_template_id;type:varchar(255);default:''" json:"denyTemplateId"`
	DenyTemplateKind   string              `gorm:"column:deny_template_kind;type:varchar(100);default:''" json:"denyTemplateKind"`
	DenyTemplateLabel  string              `gorm:"column:deny_template_label;type:varchar(255);default:''" json:"denyTemplateLabel"`
	DenyTemplateCount  int                 `gorm:"column:deny_template_count;default:0" json:"denyTemplateCount"`
	StaticContexts     []VulnStaticContext `gorm:"column:static_contexts;type:text;serializer:json" json:"staticContexts"`
	Description        string              `gorm:"column:description;type:text" json:"description"`
	AIVerified         bool                `gorm:"column:ai_verified;default:false" json:"aiVerified"`
	DisposalStatus     string              `gorm:"column:disposal_status;type:varchar(50);default:open;index" json:"disposalStatus"`
	CreatedAt          time.Time           `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	Task               VulnScanTask        `gorm:"foreignKey:TaskID,Version;references:ID,Version;constraint:OnDelete:CASCADE" json:"-"`
}

func (VulnScanJSVulnerability) TableName() string {
	return "vuln_scan_js_vulnerability"
}

// VulnScanJSAsset 漏洞扫描任务的JS扫描资产结果（链接到 VulnScanTask）
type VulnScanJSAsset struct {
	ID        string       `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID    string       `gorm:"column:task_id;type:varchar(255);not null;index:idx_vulnscan_js_asset_task_id" json:"taskId"`
	Version   int          `gorm:"column:version;default:1;index:idx_vulnscan_js_asset_task_version" json:"version"`
	Type      string       `gorm:"column:type;type:varchar(50);not null;index" json:"type"` // email, idCard, phone, ipUrl, sensitive, apiRoute, apiRoot
	Value     string       `gorm:"column:value;type:text;not null" json:"value"`
	Source    string       `gorm:"column:source;type:text" json:"source"`
	CreatedAt time.Time    `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	Task      VulnScanTask `gorm:"foreignKey:TaskID,Version;references:ID,Version;constraint:OnDelete:CASCADE" json:"-"`
}

func (VulnScanJSAsset) TableName() string {
	return "vuln_scan_js_asset"
}

// VulnScanJSSiteTree 漏洞扫描任务的JS扫描网站树结构（链接到 VulnScanTask）
type VulnScanJSSiteTree struct {
	ID        string       `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID    string       `gorm:"column:task_id;type:varchar(255);not null;index:idx_vulnscan_js_sitetree_task_id" json:"taskId"`
	Version   int          `gorm:"column:version;default:1;index:idx_vulnscan_js_sitetree_task_version" json:"version"`
	TargetURL string       `gorm:"column:target_url;type:text;not null;default:''" json:"targetUrl"` // 扫描目标URL
	TreeData  string       `gorm:"column:tree_data;type:text;not null" json:"treeData"`              // JSON 格式存储树结构
	CreatedAt time.Time    `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	Task      VulnScanTask `gorm:"foreignKey:TaskID,Version;references:ID,Version;constraint:OnDelete:CASCADE" json:"-"`
}

func (VulnScanJSSiteTree) TableName() string {
	return "vuln_scan_js_sitemap"
}

// JSScanConfig JS扫描配置（可保存的配置模板）
type JSScanConfig struct {
	ID          string    `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	Name        string    `gorm:"column:name;type:varchar(255);not null" json:"name"` // 配置名称
	Description string    `gorm:"column:description;type:text" json:"description"`    // 配置描述
	IsDefault   bool      `gorm:"column:is_default;default:false" json:"isDefault"`   // 是否为默认配置
	Config      string    `gorm:"column:config;type:text;not null" json:"config"`     // JSON 格式存储完整配置
	CreatedAt   time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt   time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (JSScanConfig) TableName() string {
	return "jsscan_config"
}

// VulnScanDirResult 目录扫描结果（关联到 VulnScanTask）
type VulnScanDirResult struct {
	ID          string    `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID      string    `gorm:"column:task_id;type:varchar(255);not null;index:idx_vuln_scan_dir_result_task_id" json:"taskId"`
	Version     int       `gorm:"column:version;not null;default:1;index:idx_vuln_scan_dir_result_version" json:"version"` // 扫描版本号
	URL         string    `gorm:"column:url;type:text;not null" json:"url"`
	Status      int       `gorm:"column:status;not null;index" json:"status"`
	Title       string    `gorm:"column:title;type:text" json:"title"`
	Location    string    `gorm:"column:location;type:text" json:"location"`
	ContentType string    `gorm:"column:content_type;type:varchar(255)" json:"contentType"`
	Length      int       `gorm:"column:length;default:0" json:"length"`
	Body        string    `gorm:"column:body;type:text" json:"body"`
	Recursion   int       `gorm:"column:recursion;default:0" json:"recursion"`
	CreatedAt   time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
}

func (VulnScanDirResult) TableName() string {
	return "vuln_scan_dir_result"
}

// SubdomainScanResult 子域名扫描结果
type SubdomainScanResult struct {
	ID              string    `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	TaskID          string    `gorm:"column:task_id;type:varchar(255);not null;index:idx_subdomain_scan_result_task_id" json:"taskId"`
	Domain          string    `gorm:"column:domain;type:varchar(255);not null" json:"domain"`
	Subdomain       string    `gorm:"column:subdomain;type:varchar(255);not null;index:idx_subdomain_scan_result_subdomain" json:"subdomain"`
	IPs             string    `gorm:"column:ips;type:text;not null" json:"ips"` // JSON 格式存储
	AssetType       string    `gorm:"column:asset_type;type:varchar(50)" json:"assetType"`
	AssetName       string    `gorm:"column:asset_name;type:varchar(255)" json:"assetName"`
	Source          string    `gorm:"column:source;type:varchar(100)" json:"source"`
	Sources         string    `gorm:"column:sources;type:text" json:"sources"` // JSON 格式存储
	IsWildcard      bool      `gorm:"column:is_wildcard;default:false" json:"isWildcard"`
	CreatedAt       time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	ResolutionError string    `gorm:"column:resolution_error;type:text" json:"resolutionError,omitempty"`
}

func (SubdomainScanResult) TableName() string {
	return "subdomain_scan_result"
}

// ScanTask 扫描任务表
// 注意：每次重新扫描时，会在表中创建新记录（相同的 task_id，version 递增）
// task_id + version 作为联合主键
type VulnScanTask struct {
	ID                  string     `gorm:"primaryKey;column:id;type:varchar(100)" json:"id"`                                                                    // 任务ID（保持不变）
	Version             int        `gorm:"primaryKey;column:version;type:integer;default:1;autoIncrement:false;index:idx_vul_scan_task_version" json:"version"` // 扫描版本号，每次重新扫描时递增，与 task_id 组成联合主键
	TaskName            string     `gorm:"column:task_name;type:varchar(255);default:''" json:"taskName"`                                                       // 任务名称
	CreatedByAI         bool       `gorm:"column:created_by_ai;default:false" json:"createdByAI"`                                                               // 由 AI 对话创建
	ScanMode            string     `gorm:"column:scan_mode;type:varchar(50)" json:"scanMode"`                                                                   // local / distributed
	ScanType            string     `gorm:"column:scan_type;type:varchar(50);default:'webscan'" json:"scanType"`                                                 // webscan / crack
	AssignedAgentID     string     `gorm:"column:assigned_agent_id;type:varchar(100);index" json:"assignedAgentId"`                                             // 指定下发的 Agent（可为空）
	WorkerAgentID       string     `gorm:"column:worker_agent_id;type:varchar(100);index" json:"workerAgentId"`                                                 // 实际执行 Agent（运行时绑定）
	LeaseExpiresAt      *time.Time `gorm:"column:lease_expires_at;index" json:"leaseExpiresAt,omitempty"`                                                       // Agent 运行租约过期时间
	ParentCombinationID string     `gorm:"column:parent_combination_id;type:varchar(255)" json:"parentCombinationId"`                                           // 组合任务ID（子任务用）
	StepIndex           int        `gorm:"column:step_index;default:0" json:"stepIndex"`                                                                        // 组合任务步骤索引
	StepModule          string     `gorm:"column:step_module;type:varchar(50)" json:"stepModule"`                                                               // 组合任务模块
	Targets             string     `gorm:"column:targets;type:text" json:"targets"`                                                                             // JSON array of URLs/IPs
	Status              string     `gorm:"column:status;type:varchar(20);default:'pending';index" json:"status"`                                                // pending, running, paused, recovering, completed, failed, cancelled, deleting
	CurrentStage        string     `gorm:"column:current_stage;type:varchar(100);default:''" json:"currentStage"`
	CheckpointJSON      string     `gorm:"column:checkpoint_json;type:text" json:"checkpointJson,omitempty"`
	PauseRequested      bool       `gorm:"column:pause_requested;default:false;index" json:"pauseRequested"`
	Progress            int        `gorm:"column:progress;default:0" json:"progress"`      // 0~100
	TotalCount          int        `gorm:"column:total_count;default:0" json:"totalCount"` // 总目标数量
	CurrentCount        int        `gorm:"column:current_count;default:0" json:"currentCount"`
	ProgressDetail      string     `gorm:"column:progress_detail;type:text;default:''" json:"progressDetail,omitempty"`
	StartedAt           *time.Time `gorm:"column:started_at" json:"startedAt,omitempty"` // 开始时间
	HeartbeatAt         *time.Time `gorm:"column:heartbeat_at;index" json:"heartbeatAt,omitempty"`
	ErrorCount          int        `gorm:"column:error_count;default:0" json:"errorCount"` // 错误数量
	CompletedAt         *time.Time `gorm:"column:completed_at" json:"completedAt,omitempty"`
	Config              string     `gorm:"column:config;type:text" json:"config"` // JSON 配置
	ErrorMsg            string     `gorm:"column:error_msg;type:text" json:"errorMsg,omitempty"`
	CreatedAt           time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index:idx_vul_scan_task_version" json:"createdAt"` // 版本开始时间
	UpdatedAt           time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (VulnScanTask) TableName() string {
	return "vul_scan_task"
}

// VulnScanTaskItem is the target-level work unit used by distributed Agent scans.
type VulnScanTaskItem struct {
	WorkerGeneration int64      `gorm:"column:worker_generation;default:0" json:"workerGeneration"`
	ID               uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID           string     `gorm:"column:task_id;type:varchar(100);not null;index:idx_vul_scan_task_item_task_status,priority:1;index:idx_vul_scan_task_item_task_version,priority:1" json:"taskId"`
	Version          int        `gorm:"column:version;default:1;not null;index:idx_vul_scan_task_item_task_version,priority:2" json:"version"`
	ScanType         string     `gorm:"column:scan_type;type:varchar(50);default:'webscan';index" json:"scanType"`
	Target           string     `gorm:"column:target;type:text;not null" json:"target"`
	Status           string     `gorm:"column:status;type:varchar(20);default:'pending';index:idx_vul_scan_task_item_task_status,priority:2" json:"status"`
	WorkerAgentID    string     `gorm:"column:worker_agent_id;type:varchar(100);index" json:"workerAgentId"`
	LeaseExpiresAt   *time.Time `gorm:"column:lease_expires_at;index" json:"leaseExpiresAt,omitempty"`
	RetryCount       int        `gorm:"column:retry_count;default:0" json:"retryCount"`
	MaxRetries       int        `gorm:"column:max_retries;default:2" json:"maxRetries"`
	PortScanNoHosts  bool       `gorm:"column:port_scan_no_hosts;not null;default:false" json:"-"`
	ErrorMsg         string     `gorm:"column:error_msg;type:text" json:"errorMsg,omitempty"`
	StartedAt        *time.Time `gorm:"column:started_at" json:"startedAt,omitempty"`
	CompletedAt      *time.Time `gorm:"column:completed_at" json:"completedAt,omitempty"`
	CreatedAt        time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt        time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (VulnScanTaskItem) TableName() string {
	return "vul_scan_task_item"
}

// ScanSchedule is the authoritative schedule definition for a logical task.
// It is keyed by the logical task id, not by an execution version, so editing a
// plan no longer writes to (or copies across) vul_scan_task rows.
type ScanSchedule struct {
	ID        string     `gorm:"primaryKey;column:id;type:varchar(100)" json:"id"`
	TaskID    string     `gorm:"column:task_id;type:varchar(100);not null;uniqueIndex" json:"taskId"`
	Enabled   bool       `gorm:"column:enabled;not null" json:"enabled"`
	Spec      string     `gorm:"column:spec;type:text;not null" json:"-"`
	NextRunAt *time.Time `gorm:"column:next_run_at;index" json:"nextRunAt,omitempty"`
	Revision  int64      `gorm:"column:revision;not null;default:0" json:"-"`
	CreatedAt time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (ScanSchedule) TableName() string {
	return "scan_schedule"
}

// ScanScheduleOccurrence is the authoritative trigger ledger. The unique
// (schedule_id, scheduled_for) pair makes claiming an occurrence exactly-once
// across restarts and multiple dispatchers.
type ScanScheduleOccurrence struct {
	ID           uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	ScheduleID   string     `gorm:"column:schedule_id;type:varchar(100);not null;uniqueIndex:idx_scan_schedule_occurrence,priority:1" json:"scheduleId"`
	ScheduledFor time.Time  `gorm:"column:scheduled_for;not null;uniqueIndex:idx_scan_schedule_occurrence,priority:2" json:"scheduledFor"`
	Status       string     `gorm:"column:status;type:varchar(20);not null;index" json:"status"`
	TaskVersion  *int       `gorm:"column:task_version" json:"taskVersion,omitempty"`
	ScanType     string     `gorm:"column:scan_type;type:varchar(50);default:''" json:"scanType"`
	ScheduleMode string     `gorm:"column:schedule_mode;type:varchar(20);default:''" json:"scheduleMode"`
	Reason       string     `gorm:"column:reason;type:text" json:"reason,omitempty"`
	NextRunAt    *time.Time `gorm:"column:next_run_at" json:"nextRunAt,omitempty"`
	ClaimedAt    time.Time  `gorm:"column:claimed_at;not null" json:"claimedAt"`
	FinishedAt   *time.Time `gorm:"column:finished_at" json:"finishedAt,omitempty"`
	CreatedAt    time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
}

func (ScanScheduleOccurrence) TableName() string {
	return "scan_schedule_occurrence"
}

// PortScanResult 端口扫描结果表
type PortScanResult struct {
	ID              uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID          string    `gorm:"column:task_id;type:varchar(100);index:idx_task_id;index:idx_task_host_port;index:idx_port_scan_result_task_version" json:"taskId"` // 任务ID
	Version         int       `gorm:"column:version;default:1;index:idx_port_scan_result_task_version" json:"version"`                                                   // 扫描版本号
	Host            string    `gorm:"column:host;type:varchar(255);index:idx_host;index:idx_task_host_port" json:"host"`                                                 // 主机地址
	Port            int       `gorm:"column:port;type:int;index:idx_port;index:idx_task_host_port" json:"port"`                                                          // 端口号
	Transport       string    `gorm:"column:transport;type:varchar(10);index:idx_task_host_port" json:"transport"`                                                       // 传输层协议（tcp/udp）
	Protocol        string    `gorm:"column:protocol;type:varchar(50)" json:"protocol"`                                                                                  // 协议类型
	DiscoveryStatus string    `gorm:"column:discovery_status;type:varchar(20);default:'verified'" json:"discoveryStatus"`
	Product         string    `gorm:"column:product;type:varchar(255)" json:"product"`                // 产品名
	ServiceVersion  string    `gorm:"column:service_version;type:varchar(255)" json:"serviceVersion"` // 服务版本
	RawResponse     string    `gorm:"column:raw_response;type:text" json:"rawResponse"`               // 原始响应
	CreatedAt       time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
}

func (PortScanResult) TableName() string {
	return "port_scan_result"
}

// PortScanResultAsset preserves the domain asset context for a scanned IP port.
type PortScanResultAsset struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID       string    `gorm:"column:task_id;type:varchar(100);not null;index:idx_port_scan_result_asset_task_version,priority:1;index:idx_port_scan_result_asset_lookup,priority:1" json:"taskId"`
	Version      int       `gorm:"column:version;default:1;not null;index:idx_port_scan_result_asset_task_version,priority:2" json:"version"`
	ResultID     uint      `gorm:"column:result_id;not null;index:idx_port_scan_result_asset_result" json:"resultId"`
	IP           string    `gorm:"column:ip;type:varchar(255);not null;index:idx_port_scan_result_asset_lookup,priority:2" json:"ip"`
	Port         int       `gorm:"column:port;type:int;not null;index:idx_port_scan_result_asset_lookup,priority:3" json:"port"`
	Transport    string    `gorm:"column:transport;type:varchar(10);not null;index:idx_port_scan_result_asset_lookup,priority:4" json:"transport"`
	Domain       string    `gorm:"column:domain;type:varchar(255)" json:"domain"`
	Subdomain    string    `gorm:"column:subdomain;type:varchar(255);not null;index:idx_port_scan_result_asset_subdomain" json:"subdomain"`
	AssetType    string    `gorm:"column:asset_type;type:varchar(50)" json:"assetType"`
	AssetName    string    `gorm:"column:asset_name;type:varchar(255)" json:"assetName"`
	SourceModule string    `gorm:"column:source_module;type:varchar(50)" json:"sourceModule"`
	CreatedAt    time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
}

func (PortScanResultAsset) TableName() string {
	return "port_scan_result_asset"
}

// migrateVulnScanTaskPrimaryKey 迁移 vul_scan_task 表的主键结构
// 从单主键 (id) 迁移到联合主键 (id, version)
// User 用户表
type User struct {
	ID        uint      `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Username  string    `gorm:"column:username;type:varchar(255);not null;uniqueIndex" json:"username"`
	Password  string    `gorm:"column:password;type:varchar(255);not null" json:"-"` // 密码不返回给前端
	Email     string    `gorm:"column:email;type:varchar(255);default:''" json:"email"`
	Role      string    `gorm:"column:role;type:varchar(50);default:'admin'" json:"role"` // admin, user
	CreatedAt time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (User) TableName() string {
	return "user"
}

// isCombinationParentTask reports whether a unified task row belongs to a
// combination run. Older releases executed every combination step on the parent
// task row and left same-ID rows in the retired subdomain_scan_task table.
func isCombinationParentTask(task VulnScanTask) bool {
	return strings.EqualFold(strings.TrimSpace(task.ScanType), "combination") ||
		strings.TrimSpace(task.ParentCombinationID) != ""
}

// SpaceExportTask 空间引擎导出任务
type SpaceExportTask struct {
	ID           string     `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	Engine       string     `gorm:"column:engine;type:varchar(50);not null;index" json:"engine"` // fofa/hunter/quake
	TaskName     string     `gorm:"column:task_name;type:varchar(255)" json:"taskName"`
	Status       string     `gorm:"column:status;type:varchar(20);not null;default:'pending';index" json:"status"` // pending/running/success/failed/cancelled
	Progress     int        `gorm:"column:progress;default:0" json:"progress"`
	TotalCount   int        `gorm:"column:total_count;default:0" json:"totalCount"`
	CurrentCount int        `gorm:"column:current_count;default:0" json:"currentCount"`
	Format       string     `gorm:"column:format;type:varchar(20);default:'csv'" json:"format"`
	Query        string     `gorm:"column:query;type:text" json:"query"`
	Filters      string     `gorm:"column:filters;type:text" json:"filters"` // JSON
	Fields       string     `gorm:"column:fields;type:text" json:"fields"`   // JSON
	FileName     string     `gorm:"column:file_name;type:varchar(255)" json:"fileName"`
	FilePath     string     `gorm:"column:file_path;type:text" json:"filePath"`
	FileSize     int64      `gorm:"column:file_size;default:0" json:"fileSize"`
	ErrorMsg     string     `gorm:"column:error_msg;type:text" json:"errorMsg"`
	CreatedBy    string     `gorm:"column:created_by;type:varchar(255);index" json:"createdBy"`
	StartedAt    *time.Time `gorm:"column:started_at" json:"startedAt"`
	CompletedAt  *time.Time `gorm:"column:completed_at" json:"completedAt"`
	CreatedAt    time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (SpaceExportTask) TableName() string {
	return "space_export_task"
}

// SpaceSearchHistory 聚合搜索历史记录
type SpaceSearchHistory struct {
	ID              string    `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	SessionID       string    `gorm:"column:session_id;type:varchar(255);not null;uniqueIndex" json:"sessionId"`
	Query           string    `gorm:"column:query;type:text;not null;index" json:"query"`
	QueryType       string    `gorm:"column:query_type;type:varchar(50);default:'default'" json:"queryType"`
	GroupID         string    `gorm:"column:group_id;type:varchar(255);index" json:"groupId"`
	GroupName       string    `gorm:"column:group_name;type:varchar(255)" json:"groupName"`
	Engines         string    `gorm:"column:engines;type:text" json:"engines"`            // JSON array
	Results         string    `gorm:"column:results;type:text" json:"results"`            // JSON array
	EngineStatus    string    `gorm:"column:engine_status;type:text" json:"engineStatus"` // JSON object
	TotalCount      int       `gorm:"column:total_count;default:0" json:"totalCount"`
	HasMore         bool      `gorm:"column:has_more;default:false" json:"hasMore"`
	ReachedMaxLimit bool      `gorm:"column:reached_max_limit;default:false" json:"reachedMaxLimit"`
	MaxResults      int       `gorm:"column:max_results;default:0" json:"maxResults"`
	CreatedBy       string    `gorm:"column:created_by;type:varchar(255);index" json:"createdBy"`
	CreatedAt       time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt       time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (SpaceSearchHistory) TableName() string {
	return "space_search_history"
}

// CombinationTask 组合扫描任务
type CombinationTask struct {
	ID             string     `gorm:"primaryKey;column:id;type:varchar(255)" json:"id"`
	Name           string     `gorm:"column:name;type:varchar(255);not null" json:"name"`
	Targets        string     `gorm:"column:targets;type:text;not null" json:"targets"`    // JSON array
	Nodes          string     `gorm:"column:nodes;type:text;not null" json:"nodes"`        // JSON array
	ChildTaskIDs   string     `gorm:"column:child_task_ids;type:text" json:"childTaskIds"` // JSON array
	Context        string     `gorm:"column:context;type:text" json:"context"`             // JSON context（下游临时数据）
	Status         string     `gorm:"column:status;type:varchar(20);default:'pending';index" json:"status"`
	PauseRequested bool       `gorm:"column:pause_requested;default:false;index" json:"pauseRequested"`
	Progress       int        `gorm:"column:progress;default:0" json:"progress"`
	CurrentStep    int        `gorm:"column:current_step;default:0" json:"currentStep"`
	TotalSteps     int        `gorm:"column:total_steps;default:0" json:"totalSteps"`
	ErrorMsg       string     `gorm:"column:error_msg;type:text" json:"errorMsg"`
	CreatedBy      string     `gorm:"column:created_by;type:varchar(255);index" json:"createdBy"`
	StartedAt      *time.Time `gorm:"column:started_at" json:"startedAt"`
	CompletedAt    *time.Time `gorm:"column:completed_at" json:"completedAt"`
	CreatedAt      time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (CombinationTask) TableName() string {
	return "combination_task"
}

type ScanAgentNode struct {
	UpdateProtocol     int    `gorm:"default:0" json:"updateProtocol"`
	UpdatePending      bool   `gorm:"default:false" json:"updatePending"`
	UpdateOS           string `gorm:"type:varchar(20);default:''" json:"updateOS"`
	UpdateArch         string `gorm:"type:varchar(20);default:''" json:"updateArch"`
	PlatformTokenHash  string `gorm:"column:platform_token_hash;type:varchar(64);default:''" json:"-"`
	PlatformSessionPID int    `gorm:"column:platform_session_pid;default:0" json:"-"`
	HostIdentity       string `gorm:"column:host_identity;type:varchar(64);default:'';uniqueIndex:idx_agent_host_identity,where:host_identity <> ''" json:"-"`
	Retired            bool   `gorm:"column:retired;default:false" json:"-"`
	AIWorker           bool   `gorm:"column:ai_worker;default:false" json:"aiWorker"`
	ScanWorker         bool   `gorm:"column:scan_worker;default:true" json:"scanWorker"`
	LocalWorker        bool   `gorm:"column:local_worker;default:false" json:"localWorker"`
	AIConcurrency      int    `gorm:"column:ai_concurrency;default:1" json:"aiConcurrency"`
	ScanConcurrency    int    `gorm:"column:scan_concurrency;default:1" json:"scanConcurrency"`
	SupportedTypes     string `gorm:"column:supported_types;type:text;default:'[]'" json:"supportedTypes"`
	NetworkZones       string `gorm:"column:network_zones;type:text;default:'[]'" json:"networkZones"`

	AgentID            string    `gorm:"primaryKey;column:agent_id;type:varchar(100)" json:"agentId"`
	DisplayName        string    `gorm:"column:display_name;type:varchar(255)" json:"displayName"`
	AgentVersion       string    `gorm:"column:agent_version;type:varchar(50);default:''" json:"agentVersion"`
	Enabled            bool      `gorm:"column:enabled;default:true;index" json:"enabled"`
	Status             string    `gorm:"column:status;type:varchar(20);default:'online';index" json:"status"` // online/offline
	LastHeartbeat      time.Time `gorm:"column:last_heartbeat;index" json:"lastHeartbeat"`
	LastSeenIP         string    `gorm:"column:last_seen_ip;type:varchar(100)" json:"lastSeenIp"`
	LastSeenHost       string    `gorm:"column:last_seen_host;type:varchar(255)" json:"lastSeenHost"`
	CPUUsagePercent    float64   `gorm:"column:cpu_usage_percent;default:0" json:"cpuUsagePercent"`
	MemoryUsageBytes   uint64    `gorm:"column:memory_usage_bytes;default:0" json:"memoryUsageBytes"`
	MemoryTotalBytes   uint64    `gorm:"column:memory_total_bytes;default:0" json:"memoryTotalBytes"`
	MemoryUsagePercent float64   `gorm:"column:memory_usage_percent;default:0" json:"memoryUsagePercent"`
	DiskUsageBytes     uint64    `gorm:"column:disk_usage_bytes;default:0" json:"diskUsageBytes"`
	DiskTotalBytes     uint64    `gorm:"column:disk_total_bytes;default:0" json:"diskTotalBytes"`
	DiskUsagePercent   float64   `gorm:"column:disk_usage_percent;default:0" json:"diskUsagePercent"`
	CreatedAt          time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt          time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (ScanAgentNode) TableName() string {
	return "scan_agent_node"
}

// AgentUpdate stores the latest durable update request for each external node.
type AgentUpdate struct {
	AgentID         string    `gorm:"primaryKey;type:varchar(100)" json:"agentId"`
	RequestID       string    `gorm:"type:varchar(36)" json:"requestId"`
	TargetVersion   string    `gorm:"type:varchar(50)" json:"targetVersion"`
	PreviousVersion string    `gorm:"type:varchar(50)" json:"previousVersion"`
	Status          string    `gorm:"type:varchar(30);index" json:"status"`
	Progress        int       `json:"progress"`
	DownloadedBytes int64     `json:"downloadedBytes"`
	Error           string    `gorm:"type:text" json:"error"`
	CancelRequested bool      `gorm:"default:false" json:"cancelRequested"`
	RequestedAt     time.Time `json:"requestedAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func (AgentUpdate) TableName() string { return "agent_update" }

type ScanAgentCommand struct {
	ID           uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	AgentID      string     `gorm:"column:agent_id;type:varchar(100);index:idx_agent_cmd_agent_status,priority:1;uniqueIndex:idx_agent_cmd_unique,priority:1" json:"agentId"`
	TaskID       string     `gorm:"column:task_id;type:varchar(100);index;uniqueIndex:idx_agent_cmd_unique,priority:2" json:"taskId"`
	TaskVersion  int        `gorm:"column:task_version;index;uniqueIndex:idx_agent_cmd_unique,priority:3" json:"taskVersion"`
	Command      string     `gorm:"column:command;type:varchar(50);index;uniqueIndex:idx_agent_cmd_unique,priority:4" json:"command"` // cancel
	Status       string     `gorm:"column:status;type:varchar(20);default:'pending';index:idx_agent_cmd_agent_status,priority:2" json:"status"`
	ResultMsg    string     `gorm:"column:result_msg;type:text" json:"resultMsg"`
	DispatchedAt *time.Time `gorm:"column:dispatched_at" json:"dispatchedAt,omitempty"`
	AckedAt      *time.Time `gorm:"column:acked_at" json:"ackedAt,omitempty"`
	CreatedAt    time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (ScanAgentCommand) TableName() string {
	return "scan_agent_command"
}

// ScanAssetFile Agent/服务端上传的截图与图标文件
type ScanAssetFile struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	AssetType     string    `gorm:"column:asset_type;type:varchar(20);not null;index:idx_scan_asset_file_type_name,priority:1;unique" json:"assetType"` // screenshot / favicon
	FileName      string    `gorm:"column:file_name;type:varchar(255);not null;index:idx_scan_asset_file_type_name,priority:2;unique" json:"fileName"`
	ContentType   string    `gorm:"column:content_type;type:varchar(100)" json:"contentType"`
	Data          []byte    `gorm:"column:data" json:"-"`
	Size          int64     `gorm:"column:size;default:0" json:"size"`
	SourceTaskID  string    `gorm:"column:source_task_id;type:varchar(255);index" json:"sourceTaskId"`
	SourceVersion int       `gorm:"column:source_version;default:1;index" json:"sourceVersion"`
	CreatedAt     time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt     time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (ScanAssetFile) TableName() string {
	return "scan_asset_file"
}

// SystemStatsSample 系统性能监控短期采样
type SystemStatsSample struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CPU           float64   `gorm:"column:cpu;default:0" json:"cpu"`
	Mem           float64   `gorm:"column:mem;default:0" json:"mem"`
	MemUsedGB     float64   `gorm:"column:mem_used_gb;default:0" json:"memUsedGB"`
	MemTotalGB    float64   `gorm:"column:mem_total_gb;default:0" json:"memTotalGB"`
	Disk          float64   `gorm:"column:disk;default:0" json:"disk"`
	NetworkRxMbps float64   `gorm:"column:network_rx_mbps;default:0" json:"networkRxMbps"`
	NetworkTxMbps float64   `gorm:"column:network_tx_mbps;default:0" json:"networkTxMbps"`
	UptimeSec     uint64    `gorm:"column:uptime_sec;default:0" json:"uptimeSec"`
	DiskUsedGB    float64   `gorm:"column:disk_used_gb;default:0" json:"diskUsedGB"`
	DiskFreeGB    float64   `gorm:"column:disk_free_gb;default:0" json:"diskFreeGB"`
	DiskTotalGB   float64   `gorm:"column:disk_total_gb;default:0" json:"diskTotalGB"`
	SampledAt     time.Time `gorm:"column:sampled_at;default:CURRENT_TIMESTAMP;index:idx_system_stats_sample_sampled_at,sort:desc" json:"sampledAt"`
	CreatedAt     time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
}

func (SystemStatsSample) TableName() string {
	return "system_stats_sample"
}
