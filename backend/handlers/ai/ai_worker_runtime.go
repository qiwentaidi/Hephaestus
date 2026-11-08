package ai

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"hephaestus-backend/handlers/scan"
	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/lib/pentest/taskengine"
	"hephaestus-backend/pkg/agentruntime"
	aipkg "hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/airun"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/scanexec"
	"hephaestus-backend/pkg/scantask"
	"hephaestus-backend/pkg/toolnetwork"
	"hephaestus-backend/pkg/version"
)

const aiWorkerLeaseTTL = 30 * time.Second

var localAgentPrepared sync.Once
var localAgentPrepareErr error
var localAgentSession *agentruntime.NodeSession
var localAgentWorkers config.AgentWorkerConfig
var localAgentOption agentruntime.NodeUpsertOption

var localAgentStart sync.Once
var localAgentStartErr error
var localWorkerStart sync.Once
var localAgentCancel context.CancelFunc
var localAgentWG sync.WaitGroup
var localAgentDone chan struct{}
var localWorkerCancel context.CancelFunc
var localWorkerDone chan struct{}

// PrepareUnifiedLocalAgent acquires ownership before queue recovery or any
// dispatchers run. A duplicate server must fail without changing task states.
func PrepareUnifiedLocalAgent(ctx context.Context, scanCapacity int) error {
	store, err := aipkg.StoreFromAppDB(ctx)
	if err != nil {
		return err
	}
	if !store.Distributed() {
		// SQLite switches from API-owned goroutines to the same durable turn and
		// event lifecycle as the PostgreSQL worker. Import legacy running rows so
		// an upgrade does not strand an in-flight conversation.
		pentest.EnableLocalWorker()
		if stopped, stopErr := store.AcknowledgeInterruptedLocalWorkerStops(ctx, ""); stopErr != nil {
			log.Printf("[ai-worker] recover local stops: %v", stopErr)
		} else if stopped > 0 {
			log.Printf("[ai-worker] acknowledged %d interrupted local stop(s)", stopped)
		}
		if imported, err := store.ImportLegacyWorkerTurns(ctx); err != nil {
			log.Printf("[ai-worker] import legacy local turns: %v", err)
		} else if imported > 0 {
			log.Printf("[ai-worker] imported %d legacy local turn(s)", imported)
		}
		return nil
	}
	localAgentPrepared.Do(func() {
		host, _ := os.Hostname()
		cfg, err := config.LoadConfig()
		if err != nil {
			localAgentPrepareErr = err
			return
		}
		workers, err := config.ResolveAgentWorkers(cfg.Agent, true, scanCapacity)
		if err != nil {
			localAgentPrepareErr = err
			return
		}
		db, err := database.GetDB()
		if err != nil {
			localAgentPrepareErr = err
			return
		}
		hostID, err := agentruntime.NodeHostIdentity(ctx)
		if err != nil {
			localAgentPrepareErr = err
			return
		}
		session, err := agentruntime.AcquireNodeSession(ctx, db, hostID, cfg.Agent.ID, host, agentruntime.DetectLocalIPv4(), true)
		if err != nil {
			localAgentPrepareErr = err
			return
		}
		id := session.ID
		// Acquiring the host-scoped advisory lock proves that any previous
		// embedded worker using this identity has exited. Complete only its
		// durable cancellation acknowledgements; normal interrupted turns stay
		// eligible for lease-based recovery by the worker pool.
		pentestStore, err := aipkg.StoreFromAppDB(ctx)
		recoveredStops := int64(0)
		if err == nil {
			recoveredStops, err = pentestStore.AcknowledgeInterruptedLocalWorkerStops(ctx, id)
		}
		if err != nil {
			session.Close()
			localAgentPrepareErr = fmt.Errorf("recover interrupted local worker stops: %w", err)
			return
		}
		if recoveredStops > 0 {
			log.Printf("[ai-worker] acknowledged %d interrupted local stop(s)", recoveredStops)
		}
		recoveredScanStops, scanErr := agentruntime.AcknowledgeInterruptedLocalScanStops(id)
		if scanErr != nil {
			session.Close()
			localAgentPrepareErr = fmt.Errorf("recover interrupted local scan stops: %w", scanErr)
			return
		}
		if recoveredScanStops > 0 {
			log.Printf("[scan-worker] acknowledged %d interrupted local stop(s)", recoveredScanStops)
		}
		if err = config.PersistAgentID(id); err != nil {
			session.Close()
			localAgentPrepareErr = err
			return
		}
		localAgentSession, localAgentWorkers = session, workers
		localAgentOption = agentruntime.NodeUpsertOption{HostIdentity: hostID, DisplayName: host, AgentVersion: version.Version, LocalWorker: true, AIWorker: workers.AIEnabled, ScanWorker: &workers.ScanEnabled, AIConcurrency: workers.AIConcurrency, ScanConcurrency: workers.ScanConcurrency, LastSeenHost: host}
	})
	return localAgentPrepareErr
}

// StartUnifiedLocalAgent preserves desktop/single-server operation. Both roles
// consume durable work with separate pools, rather than API-owned goroutines.
func StartUnifiedLocalAgent(ctx context.Context, scanCapacity int) error {
	if err := PrepareUnifiedLocalAgent(ctx, scanCapacity); err != nil {
		return err
	}
	store, err := aipkg.StoreFromAppDB(ctx)
	if err != nil {
		return err
	}
	if err := store.RetireLegacyOrchestration(ctx); err != nil {
		return err
	}
	if !store.Distributed() {
		startLocalSQLiteWorker(ctx, store)
		return nil
	}
	localAgentStart.Do(func() {
		session, workers, opt := localAgentSession, localAgentWorkers, localAgentOption
		id, host := session.ID, opt.LastSeenHost
		db, err := database.GetDB()
		if err != nil {
			session.Close()
			localAgentStartErr = err
			return
		}
		agentruntime.PopulateNodeMetrics(&opt)
		if err := agentruntime.UpsertAgentNode(id, scanexec.GetSupportedScanTypes(), opt); err != nil {
			session.Close()
			localAgentStartErr = fmt.Errorf("register local worker: %w", err)
			return
		}
		if err = agentruntime.RetireLegacyLocalNodes(db, id, host, opt.LastSeenIP); err != nil {
			session.Close()
			localAgentStartErr = err
			return
		}
		ctx, localAgentCancel = context.WithCancel(ctx)
		localAgentWG.Add(1)
		go func() {
			defer localAgentWG.Done()
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			// A transient database slowdown must not silently kill the embedded
			// worker: without it every queued AI turn waits forever while the
			// HTTP API keeps serving. Require several consecutive failures
			// before treating the instance lock as lost.
			consecutiveCheckFailures := 0
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					checkCtx, cancelCheck := context.WithTimeout(ctx, 5*time.Second)
					err := session.Check(checkCtx)
					cancelCheck()
					if err != nil {
						consecutiveCheckFailures++
						logger.Warning("[agent] 本地 Agent 实例锁检查失败（%d/3）: %v", consecutiveCheckFailures, err)
						if consecutiveCheckFailures < 3 {
							continue
						}
						logger.Error("[agent] 本地 Agent 实例锁已丢失，AI Worker 停止领取任务；请重启程序: %v", err)
						localAgentCancel()
						return
					}
					consecutiveCheckFailures = 0
					agentruntime.PopulateNodeMetrics(&opt)
					if err := agentruntime.HeartbeatAgent(id, scanexec.GetSupportedScanTypes(), aiWorkerLeaseTTL, opt); err != nil {
						logger.Warning("[agent] 本地 Agent 心跳失败: %v", err)
					}
				}
			}
		}()
		aiCapacity := workers.AIConcurrency
		if !workers.AIEnabled {
			aiCapacity = 0
		}
		aiWG := StartAIWorkers(ctx, id, aiCapacity, true, "", session)
		localAgentWG.Add(1)
		go func() { defer localAgentWG.Done(); aiWG.Wait() }()
		if !workers.ScanEnabled {
			workers.ScanConcurrency = 0
		}
		for i := 0; i < workers.ScanConcurrency; i++ {
			localAgentWG.Add(1)
			go func() {
				defer localAgentWG.Done()
				for ctx.Err() == nil {
					task, err := agentruntime.ClaimNextAgentTask(id, scanexec.GetSupportedScanTypes(), aiWorkerLeaseTTL)
					if err != nil {
						log.Printf("[scan-worker] claim: %v", err)
					}
					if task != nil {
						if err = agentruntime.ExecuteAgentTaskWithContext(ctx, task); err != nil {
							log.Printf("[scan-worker] run: %v", err)
						}
					} else {
						select {
						case <-ctx.Done():
							return
						case <-time.After(time.Second):
						}
					}
				}
			}()
		}
		localAgentWG.Add(1)
		go func() { defer localAgentWG.Done(); reconcileWorkerStops(ctx, store) }()
		localAgentDone = make(chan struct{})
		go func() {
			localAgentWG.Wait()
			checkCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if session.Check(checkCtx) == nil {
				_ = agentruntime.SetAgentNodeOffline(id)
			}
			cancel()
			session.Close()
			close(localAgentDone)
		}()
	})
	return localAgentStartErr
}

func StopUnifiedLocalAgent(ctx context.Context) error {
	// Stop both reconciliation loops before releasing the native runtime owner.
	if localWorkerCancel != nil {
		localWorkerCancel()
	}
	if localAgentCancel != nil {
		localAgentCancel()
	}
	if localWorkerCancel != nil {
		select {
		case <-localWorkerDone:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if localAgentCancel != nil {
		select {
		case <-localAgentDone:
		case <-ctx.Done():
			return ctx.Err()
		}
	} else if localAgentSession != nil {
		localAgentSession.Close()
	}
	return taskengine.Stop()
}

func startLocalSQLiteWorker(ctx context.Context, store *pentest.SQLStore) {
	localWorkerStart.Do(func() {
		if !store.LocalWorkerEnabled() {
			return
		}
		workerCtx, cancel := context.WithCancel(ctx)
		localWorkerCancel = cancel
		localWorkerDone = make(chan struct{})
		logger.Info("[ai-worker] 启动本机 SQLite AI Worker: node=local-worker 并发=1（含独立 Planner 槽位）")
		wg := StartAIWorkers(workerCtx, "local-worker", 1, true, "", nil)
		go reconcileWorkerStops(workerCtx, store)
		go func() {
			<-workerCtx.Done()
			wg.Wait()
			close(localWorkerDone)
		}()
	})
}

// StartAIWorkers is shared by the primary service and the unified Agent binary.
// Each slot owns exactly one single-Agent turn; scans have a different pool.
func StartAIWorkers(ctx context.Context, nodeID string, capacity int, local bool, serverURL string, session *agentruntime.NodeSession) *sync.WaitGroup {
	return StartAIWorkersDraining(ctx, nodeID, capacity, local, serverURL, session, nil)
}

// Draining stops the claim loops after their current execution and cleanup,
// without cancelling the active turn's context.
func StartAIWorkersDraining(ctx context.Context, nodeID string, capacity int, local bool, serverURL string, session *agentruntime.NodeSession, drain <-chan struct{}) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	if capacity <= 0 {
		logger.Warning("[ai-worker] AI Worker 未启动（并发为 0 或 AI 角色未启用），所有 AI 会话将保持排队")
		return wg
	}
	// ARTEX keeps the planner independent from the worker pool. Reserve one
	// slot for the planner and give the remaining capacity to ordinary workers.
	workerCapacity := capacity - 1
	if workerCapacity < 1 {
		workerCapacity = 1
	}
	startLoop := func(label string, planner bool) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				select {
				case <-drain:
					return
				default:
				}
				store, err := aipkg.StoreFromAppDB(ctx)
				var t *pentest.WorkerTurn
				if err == nil {
					if planner {
						t, err = store.ClaimPlannerTurn(ctx, nodeID, aiWorkerLeaseTTL)
					} else {
						t, err = store.ClaimNonPlannerTurn(ctx, nodeID, aiWorkerLeaseTTL)
					}
				}
				if err != nil {
					logger.Warning("[ai-worker] %s 领取任务失败: %v", label, err)
				}
				if t == nil {
					select {
					case <-ctx.Done():
						return
					case <-time.After(time.Second):
					}
					continue
				}
				executeAIWorkerTurn(ctx, store, *t, local, serverURL, session)
			}
		}()
	}
	for i := 0; i < workerCapacity; i++ {
		startLoop("worker", false)
	}
	startLoop("planner", true)
	return wg
}

type workerConfigBuilder func(context.Context, *pentest.SQLStore, pentest.WorkerTurn) (pentest.AgentLoopConfig, error)

func executeAIWorkerTurn(parent context.Context, store *pentest.SQLStore, t pentest.WorkerTurn, local bool, serverURL string, session *agentruntime.NodeSession) {
	executeAIWorkerTurnWithBuilder(parent, store, t, local, serverURL, session, func(ctx context.Context, s *pentest.SQLStore, t pentest.WorkerTurn) (pentest.AgentLoopConfig, error) {
		return buildPentestAgentConfig(ctx, s, t.ProviderID, t.Model, "", t.ConversationID)
	})
}
func executeAIWorkerTurnWithBuilder(parent context.Context, store *pentest.SQLStore, t pentest.WorkerTurn, local bool, serverURL string, session *agentruntime.NodeSession, build workerConfigBuilder) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	leaseDone := make(chan struct{})
	defer func() { cancel(); <-leaseDone }()
	go func() {
		defer close(leaseDone)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				renewCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
				err := store.RenewWorkerTurn(renewCtx, t, aiWorkerLeaseTTL)
				stop := false
				if err == nil {
					stop, err = store.WorkerStopRequested(renewCtx, t)
				}
				done()
				if err != nil || stop {
					cancel()
					return
				}
			}
		}
	}()
	guarded, err := store.WithWorkerLease(t)
	var cfg pentest.AgentLoopConfig
	if err == nil {
		cfg, err = build(ctx, guarded, t)
	}
	if err == nil {
		cfg.Conversation, err = guarded.GetConversation(ctx, t.ConversationID)
	}
	if err == nil {
		cfg.Project, err = guarded.GetProject(ctx, cfg.Conversation.ProjectID)
	}
	var agentRole pentest.AgentRole
	if err == nil {
		agentRole, err = guarded.GetAgentRole(ctx, t.ConversationID)
	}
	// A delegated Worker can be enqueued just before its plan run is cancelled.
	// The turn is already durably queued by the time the cancel lands, so the
	// claim-time check here is the last point that can prevent it from doing
	// more work for a finished orchestration.
	if err == nil && planRunIsTerminalForRole(ctx, guarded, agentRole) {
		logger.Info("[ai-plan] 计划已终止，跳过排队中的编排回合: conversation=%s turn=%s", t.ConversationID, t.ID)
		finishTurnForTerminalPlanRun(ctx, store, t)
		return
	}
	if err == nil && agentRole.Role == pentest.AgentRolePlanner {
		logger.Info("[ai-plan] Planner 回合开始: conversation=%s turn=%s generation=%d", t.ConversationID, t.ID, t.Generation)
	}
	if err == nil {
		cfg.TokenUsageTurnID = t.MessageID
		cfg.WorkerTurn = &t
		cfg.WaitForScan = &pentest.AgentScanWait{}
		cfg.AsyncScanWait = true
		switch agentRole.Role {
		case pentest.AgentRoleController:
			cfg.PromptRole = pentest.AgentRoleController
			cfg.ToolAllowlist = pentest.ControllerToolAllowlist()
		case pentest.AgentRolePlanner:
			cfg.PromptRole = pentest.AgentRolePlanner
			cfg.ToolAllowlist = pentest.PlannerToolAllowlist()
		case pentest.AgentRoleWorker:
			cfg.PromptRole = pentest.AgentRoleWorker
			cfg.ToolAllowlist = pentest.WorkerToolAllowlist()
			// The controller can inject a correction while this Worker is
			// running; consume it before the next model request.
			cfg.PullSteering = func(steerCtx context.Context) []string {
				steers, steerErr := guarded.ConsumeWorkerSteers(steerCtx, t.ConversationID)
				if steerErr != nil {
					return nil
				}
				out := make([]string, 0, len(steers))
				for _, steer := range steers {
					out = append(out, steer.Content)
				}
				return out
			}
		}
		// ARTEX task mode injects operator-registered constraints into both the
		// Planner and every Worker, above exploration heuristics.
		if agentRole.Role == pentest.AgentRolePlanner || agentRole.Role == pentest.AgentRoleWorker {
			if runID := strings.TrimSpace(agentRole.PlanRunID); runID != "" {
				if block := guarded.RenderPlanConstraintPrompt(ctx, runID); block != "" {
					cfg.SystemPrompt = strings.Join([]string{strings.TrimSpace(cfg.SystemPrompt), block}, "\n\n")
				}
			}
		}
		base := cfg.WorkDir
		if base == "" {
			base = filepath.Join(config.GetDataDir(), "ai-pentest", "sessions", t.ConversationID)
		}
		cfg.WorkDir = filepath.Join(base, "runs", fmt.Sprintf("%s-%d", t.ID, t.Generation))
		cfg.Progress = recordPentestAgentProgress(guarded, t.ConversationID)
		err = guarded.RestoreWorkerWorkspace(ctx, t.ConversationID, cfg.WorkDir)
	}
	if err == nil && t.Generation > 1 {
		_, err = guarded.MarkRunningToolExecutionsOrphaned(ctx, t.ConversationID, "节点租约失效：未保存结果的工具状态未知；核对真实状态，禁止自动重放")
	}
	if err == nil {
		cfg.CheckpointWorkspace = func() error {
			saveCtx, done := context.WithTimeout(context.Background(), 20*time.Second)
			defer done()
			return guarded.PersistWorkerWorkspace(saveCtx, cfg.WorkDir)
		}
		cfg.NativeToolRunner = workerNativeToolRunner(local, serverURL, session)
		messages, readErr := guarded.ListMessages(ctx, t.ConversationID, -1)
		err = readErr
		history := []pentest.AgentMessage{}
		for _, m := range messages {
			if m.Role != "user" && m.Role != "assistant" && m.Role != "system" {
				continue
			}
			// A planner round is deliberately stateless: only the current
			// digest message is fed to the model, while the conversation keeps
			// the full audit trail.
			if agentRole.Role == pentest.AgentRolePlanner && m.ID != t.MessageID {
				continue
			}
			history = append(history, pentest.AgentMessage{Role: m.Role, Content: m.Content, ImageURLs: m.ImageURLs})
		}
		if err == nil {
			_, err = runPentestAgentLoopAndSnapshot(ctx, cfg, history, t.ProviderID, t.Model)
		}
	}
	// Do not acknowledge stopped/queued while isolation teardown is unconfirmed.
	if errors.Is(err, toolnetwork.ErrStopUnconfirmed) {
		log.Printf("AI worker %s egress cleanup unconfirmed: %v", t.ID, err)
		return
	}
	finishCtx, done := context.WithTimeout(context.Background(), 20*time.Second)
	defer done()
	status, errorText := "completed", ""
	conversationStatus := ""
	if current, readErr := store.GetConversation(finishCtx, t.ConversationID); readErr == nil {
		conversationStatus = current.Status
	}
	if err != nil {
		status = "failed"
		errorText = err.Error()
	}
	if ctx.Err() != nil {
		stopRequested, stopErr := store.WorkerStopRequested(finishCtx, t)
		if stopErr == nil && stopRequested {
			status = "stopped"
		} else if parent.Err() != nil {
			status = "queued"
		} else if err == nil {
			status = "failed"
			errorText = ctx.Err().Error()
		}
	}
	if cfg.Store != nil && cfg.CheckpointWorkspace != nil {
		if saveErr := cfg.CheckpointWorkspace(); saveErr != nil {
			if ctx.Err() == nil {
				status = "failed"
			}
			errorText = saveErr.Error()
		}
	}
	if !cfg.AsyncScanWait && cfg.WaitForScan != nil && cfg.WaitForScan.TaskID != "" && status == "completed" {
		status = "waiting"
		t.WaitTaskID = cfg.WaitForScan.TaskID
		t.WaitTaskVersion = cfg.WaitForScan.Version
		t.WaitScanType = cfg.WaitForScan.ScanType
	}
	if status == "completed" && conversationStatus == "blocked" {
		// The loop refused to finalize (for example an empty or truncated model
		// response). Preserve that durable judgement instead of replacing it
		// with the historical default completed.
		status = "blocked"
	}
	if finishErr := store.FinishWorkerTurn(finishCtx, t, status, errorText); finishErr != nil {
		log.Printf("[ai-worker] finish %s generation %d: %v", t.ID, t.Generation, finishErr)
		return
	}
	if agentRole.Role == pentest.AgentRolePlanner {
		logger.Info("[ai-plan] Planner 回合结束: conversation=%s turn=%s status=%s error=%s", t.ConversationID, t.ID, status, errorText)
	}
	// A stopped turn still needs its public state (conversation cancelled)
	// acknowledged. Do it here as well, so a stop finishes even when the
	// reconcile loop is unavailable; that loop retries if this attempt reports
	// a scan still exiting.
	if status == "stopped" {
		if err := airun.FinalizeDistributedWorkerStop(finishCtx, store, t.ConversationID); err != nil {
			logger.Warning("[ai-worker] 停止确认尚未完成，等待协调循环重试: conversation=%s err=%v", t.ConversationID, err)
		}
	}
	if status == "completed" {
		settleWorkerConversationStatus(finishCtx, store, t)
	}
	// Configuration/workspace failure can precede the loop's normal status write.

}

// planRunIsTerminalForRole reports whether this turn belongs to an
// orchestration whose plan run has already completed, failed, or been
// cancelled. A turn that is already queued must be acknowledged instead of
// running another round for that finished run.
func planRunIsTerminalForRole(ctx context.Context, store *pentest.SQLStore, role pentest.AgentRole) bool {
	return strings.TrimSpace(role.PlanRunID) != ""
}

// finishTurnForTerminalPlanRun stops a turn that was queued for a plan run that
// has since become terminal, without calling the model or running tools.
func finishTurnForTerminalPlanRun(ctx context.Context, store *pentest.SQLStore, t pentest.WorkerTurn) {
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := store.RequestWorkerStop(finishCtx, t.ConversationID, false, "plan run is already terminal"); err != nil {
		logger.Warning("[ai-worker] stop terminal plan turn %s: %v", t.ID, err)
	}
	if err := store.FinishWorkerTurn(finishCtx, t, "stopped", "plan run is already terminal"); err != nil {
		logger.Warning("[ai-worker] finish terminal plan turn %s: %v", t.ID, err)
		return
	}
	if err := airun.FinalizeDistributedWorkerStop(finishCtx, store, t.ConversationID); err != nil {
		logger.Warning("[ai-worker] finalize terminal plan conversation %s: %v", t.ConversationID, err)
	}
}

// settleWorkerConversationStatus replaces the loop's in-turn evaluation with a
// post-turn evaluation. The loop runs while this worker turn is still marked
// running, so its terminal decision can only observe active work; restoring
// that stale status left completed goal-mode sessions spinning forever.
func settleWorkerConversationStatus(ctx context.Context, store *pentest.SQLStore, t pentest.WorkerTurn) {
	decision, err := store.EvaluateConversationTerminalStateExcludingWorkerTurn(ctx, t.ConversationID, t.ID)
	if err != nil {
		log.Printf("[ai-worker] evaluate settled conversation %s: %v", t.ConversationID, err)
		return
	}
	goalActive := decision.GoalStatus == "active" || decision.GoalStatus == "stopping"
	if !decision.RunningWorker && decision.RunningExecutions == 0 && !goalActive {
		// The turn is settled and nothing is executing. A live exploration
		// frontier is durable context for a later message, not work this turn
		// can still finish, so do not keep the conversation active for it.
		decision.Status = "completed"
	}
	if decision.Status == "" || decision.Status == "completed" {
		return
	}
	if _, err := store.SettleConversationStatusIfIdle(ctx, t.ConversationID, decision.Status); err != nil {
		log.Printf("[ai-worker] settle conversation %s status %s: %v", t.ConversationID, decision.Status, err)
	}
}

// planAuditWarned keeps the progress audit from repeating the same warning on
// every reconcile tick. Entries are dropped once the turn changes or finishes.
var planAuditWarned sync.Map

const (
	planQueuedWarnAfter  = time.Minute
	planRunningWarnAfter = 10 * time.Minute
)

// auditPlanRunProgress writes the state of a Planner turn that cannot make
// progress to the application log. A queued turn means no AI worker claimed it
// (offline node, disabled AI role, or a blocked Planner slot); a running turn
// older than ten minutes means a model call is retrying or stuck. Both used to
// be visible only as a workbench timer with no server-side explanation.
func auditPlanRunProgress(ctx context.Context, store *pentest.SQLStore) {
	runs, err := store.ListPlanRuns(ctx, 50)
	if err != nil {
		return
	}
	active := make(map[string]struct{})
	for _, run := range runs {
		if planRunTerminal(run.Status) {
			continue
		}
		turn, turnErr := store.LatestWorkerTurn(ctx, run.PlannerConversationID)
		if turnErr != nil {
			continue
		}
		var threshold time.Duration
		var reason string
		switch turn.Status {
		case "queued":
			threshold, reason = planQueuedWarnAfter, "排队超过一分钟仍未领取"
		case "running", "cancelling":
			threshold, reason = planRunningWarnAfter, "执行超过十分钟"
		default:
			continue
		}
		waited := time.Since(turn.CreatedAt)
		if waited < threshold {
			continue
		}
		key := turn.ID + ":" + turn.Status
		active[key] = struct{}{}
		if _, warned := planAuditWarned.Load(key); warned {
			continue
		}
		planAuditWarned.Store(key, struct{}{})
		logger.Warning("[ai-plan] Planner 回合%s: run=%s status=%s conversation=%s turn=%s generation=%d",
			reason, run.ID, run.Status, run.PlannerConversationID, turn.ID, turn.Generation)
	}
	planAuditWarned.Range(func(key, _ any) bool {
		if _, ok := active[key.(string)]; !ok {
			planAuditWarned.Delete(key)
		}
		return true
	})
}

func reconcileWorkerStops(ctx context.Context, store *pentest.SQLStore) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	ticks := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ticks++
			if config.GetRuntimeMode() != config.RuntimeModeAgent {
				reconcileARTEX(ctx, store)
			}
			if db, err := database.GetDB(); err == nil {
				_ = scantask.ReconcilePausedTasksAfterRestart(db, scan.ReconcilePausedCombinationTasksAfterRestart)
				_ = reconcileAIBatchPauses(ctx, store, db)
			}
			if count, err := store.RequeueCompletedScanWaits(ctx); err != nil {
				log.Printf("[ai-worker] scan wait wakeup: %v", err)
			} else if count > 0 {
				log.Printf("[ai-worker] woke %d AI scan wait(s)", count)
			}
			if _, err := store.ReconcileScanWaits(ctx); err != nil {
				log.Printf("[ai-worker] scan child reconciliation: %v", err)
			} else if waits, listErr := store.ListScanWaits(ctx, "", true); listErr == nil {
				for _, wait := range waits {
					if wait.Status == "waiting" || wait.NotifiedAt != nil {
						continue
					}
					if _, wakeErr := store.EnqueueScanWaitContinuation(ctx, wait); wakeErr != nil {
						log.Printf("[ai-worker] planner wakeup for scan %s: %v", wait.TaskID, wakeErr)
					}
				}
			}
			ids, err := store.WorkerStoppingConversations(ctx)
			if err != nil {
				log.Printf("[ai-worker] stopping query: %v", err)
				continue
			}
			for _, id := range ids {
				if err := airun.FinalizeDistributedWorkerStop(ctx, store, id); err != nil {
					log.Printf("[ai-worker] stopping %s: %v", id, err)
				}
			}
			if ticks%30 == 0 {
				auditPlanRunProgress(ctx, store)
			}
		}
	}
}
