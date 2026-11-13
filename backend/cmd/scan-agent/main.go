package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"hephaestus-backend/handlers/ai"
	"hephaestus-backend/internal/mcptooling"
	"hephaestus-backend/pkg/agentasset"
	"hephaestus-backend/pkg/agentruntime"
	"hephaestus-backend/pkg/agentupdate"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/toolguard"
	"hephaestus-backend/pkg/version"
	"hephaestus-backend/pkg/webscan"
)

func changeWorkDirToExecutableDir() {
	execPath, err := os.Executable()
	if err != nil || execPath == "" {
		return
	}

	if resolvedPath, resolveErr := filepath.EvalSymlinks(execPath); resolveErr == nil {
		execPath = resolvedPath
	}

	execDir := filepath.Dir(execPath)
	if execDir == "" {
		return
	}

	if err := os.Chdir(execDir); err != nil {
		log.Printf("切换工作目录失败: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func commaSeparatedEnv(key string) []string {
	var values []string
	for _, value := range strings.Split(os.Getenv(key), ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func configString(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func parseIntEnv(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil || i <= 0 {
		return fallback
	}
	return i
}

func parseDurationEnv(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func parseDuration(raw string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(raw)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func joinServerAddress(base string, path string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	path = strings.TrimLeft(strings.TrimSpace(path), "/")
	if base == "" || path == "" {
		return ""
	}
	return base + "/" + path
}

func detectLocalIPv4() string {
	return agentruntime.DetectLocalIPv4()
}

func initConfigDBBindings() {
	config.InitSpaceEngineConfigDB(
		database.GetSetting,
		database.SetSetting,
		database.SettingKeySpaceEngines,
	)
	config.InitDingTalkConfigDB(
		database.GetSetting,
		database.SetSetting,
		database.SettingKeyDingTalk,
	)
	config.InitWeChatConfigDB(
		database.GetSetting,
		database.SetSetting,
		database.SettingKeyWeChat,
	)
	config.InitFeishuConfigDB(
		database.GetSetting,
		database.SetSetting,
		database.SettingKeyFeishu,
	)
	config.InitNotificationConfigDB(
		database.GetSetting,
		database.SetSetting,
		database.SettingKeyNotifications,
	)
	config.InitAIConfigDB(
		database.GetSetting,
		database.SetSetting,
		database.SettingKeyAI,
	)
	config.InitToolGuardConfigDB(
		database.GetSetting,
		database.SetSetting,
		database.SettingKeyToolGuard,
	)
}

func loadAgentServerAddress() string {
	cfg, err := database.LoadAgentServerSetting()
	if err != nil {
		logger.Warning("[agent] 读取 agent-config 失败: %v", err)
		return ""
	}

	return strings.TrimSpace(cfg.Server.Address)
}

func taskKey(taskID string, version int) string {
	return fmt.Sprintf("%s:%d", taskID, version)
}

func taskRunKey(taskID string, version int, itemID uint) string {
	if itemID == 0 {
		return taskKey(taskID, version)
	}
	return fmt.Sprintf("%s:%d:%d", taskID, version, itemID)
}

func runDeployCommand(args []string) error {
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "--resource-dir":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return fmt.Errorf("--resource-dir 不能为空")
			}
			_ = os.Setenv("HEPHAESTUS_RESOURCE_DIR", strings.TrimSpace(args[i+1]))
			i++
		default:
			if strings.HasPrefix(arg, "--resource-dir=") {
				value := strings.TrimSpace(strings.TrimPrefix(arg, "--resource-dir="))
				if value == "" {
					return fmt.Errorf("--resource-dir 不能为空")
				}
				_ = os.Setenv("HEPHAESTUS_RESOURCE_DIR", value)
				continue
			}
			return fmt.Errorf("不支持的 deploy 参数: %s", arg)
		}
	}

	if err := webscan.RegisterCodeTemplateSignerForNuclei(); err != nil {
		return fmt.Errorf("初始化 code 模板签名验证器失败: %w", err)
	}
	if err := webscan.SignUpdatedCodeTemplatesForNuclei(webscan.GetPoCDir()); err != nil {
		return fmt.Errorf("签名 code PoC 模板失败: %w", err)
	}
	if err := webscan.EnsurePOCTemplateCache(); err != nil {
		return fmt.Errorf("刷新 PoC 模板缓存失败: %w", err)
	}
	logger.Info("[agent] deploy prepare completed: resourceDir=%s", config.GetConfigResourceRoot())
	return nil
}

func main() {
	config.SetRuntimeMode(config.RuntimeModeAgent)

	if len(os.Args) > 1 {
		switch strings.ToLower(strings.TrimSpace(os.Args[1])) {
		case "supervise":
			if err := runSupervisor(os.Args[2:]); err != nil {
				log.Fatal(err)
			}
			return
		case "deploy", "prepare":
			changeWorkDirToExecutableDir()
			if err := runDeployCommand(os.Args[2:]); err != nil {
				log.Fatalf("Agent 一键部署准备失败: %v", err)
			}
			return
		case "combo", "scan", "webscan", "crack", "dirscan", "subdomain", "jsscan", "help", "version", "-h", "--help", "-v", "--version":
			os.Exit(runCLI(os.Args[1:]))
		}
	}

	changeWorkDirToExecutableDir()

	// Agent 运行 AI worker 时同样需要工具注册表；与主服务使用同一套装配。
	mcptooling.Install()

	cfg, cfgErr := config.LoadConfig()
	if cfgErr != nil {
		log.Fatalf("配置加载失败: %v", cfgErr)
	}

	if _, err := database.GetDB(); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	initConfigDBBindings()

	agentCfg := cfg.Agent
	agentID := configString(agentCfg.ID, "scan-agent")
	agentID, _ = config.ResolveAgentIdentity(agentID, "")
	displayName := ""
	workers, err := config.ResolveAgentWorkers(agentCfg, false, 3)
	if err != nil {
		log.Fatal(err)
	}
	concurrency := workers.ScanConcurrency
	var nodeSession *agentruntime.NodeSession
	db, err := database.GetDB()
	if err != nil {
		log.Fatal(err)
	}
	if db.Dialector.Name() == "postgres" {
		hostID, err := agentruntime.NodeHostIdentity(context.Background())
		if err != nil {
			log.Fatal(err)
		}
		hostname, _ := os.Hostname()
		nodeSession, err = agentruntime.AcquireNodeSession(context.Background(), db, hostID, agentID, hostname, detectLocalIPv4(), false)
		if err != nil {
			log.Fatal(err)
		}
		defer nodeSession.Close()
		agentID = nodeSession.ID
		if err = config.PersistAgentID(agentID); err != nil {
			log.Fatal(err)
		}
	}
	scanTypes := commaSeparatedEnv("AGENT_SCAN_TYPES")
	if guard, err := config.GetToolGuardConfig(); err != nil {
		log.Fatalf("load tool guard: %v", err)
	} else if err = toolguard.UpdateGlobal(guard); err != nil {
		log.Fatalf("initialize tool guard: %v", err)
	}
	aiEnabled, scanEnabled := workers.AIEnabled, workers.ScanEnabled
	if aiEnabled && nodeSession == nil {
		log.Fatal("AI Worker requires a registered PostgreSQL node session")
	}
	aiConcurrency := workers.AIConcurrency
	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()

	pollInterval := parseDurationEnv("AGENT_POLL_INTERVAL", 2*time.Second)
	heartbeatInterval := parseDurationEnv("AGENT_HEARTBEAT_INTERVAL", 5*time.Second)
	leaseTTL := parseDurationEnv("AGENT_LEASE_TTL", 30*time.Second)
	configCheckInterval := parseDurationEnv("AGENT_CONFIG_CHECK_INTERVAL", 5*time.Minute)
	hostName, _ := os.Hostname()
	localIP := detectLocalIPv4()
	serverAddress := loadAgentServerAddress()
	var uploadAddress atomic.Value
	uploadAddress.Store(serverAddress)

	nodeOpt := agentruntime.NodeUpsertOption{
		NetworkZones: commaSeparatedEnv("AGENT_NETWORK_ZONES"),
		AIWorker:     aiEnabled, ScanWorker: &scanEnabled, AIConcurrency: aiConcurrency, ScanConcurrency: concurrency,
		DisplayName:  displayName,
		AgentVersion: version.Version,
		LastSeenIP:   localIP,
		LastSeenHost: hostName,
	}
	supervised := os.Getenv("HEPHAESTUS_AGENT_SUPERVISED") == "1"
	if supervised {
		nodeOpt.UpdateProtocol = agentupdate.Protocol
		nodeOpt.UpdateOS = runtime.GOOS
		nodeOpt.UpdateArch = runtime.GOARCH
	}
	if nodeSession != nil {
		nodeOpt.HostIdentity = nodeSession.HostIdentity
	}

	requeueRunning := strings.EqualFold(envOrDefault("AGENT_REQUEUE_RUNNING", "true"), "true")
	if requeueRunning {
		if affected, err := agentruntime.RequeueRunningAgentTasks("Agent 重启，任务已重新排队"); err != nil {
			logger.Warning("[agent] 重置 running 任务失败: %v", err)
		} else if affected > 0 {
			logger.Info("[agent] 已重置 %d 个 running 任务为 pending", affected)
		}
	}

	populateAgentResourceMetrics(&nodeOpt)
	if err := agentruntime.UpsertAgentNode(agentID, scanTypes, nodeOpt); err != nil {
		logger.Warning("[agent] 注册失败: %v", err)
	}

	lastConfigCheck := time.Now()
	if err := syncAgentConfigResources(workerCtx, nodeSession, serverAddress); err != nil {
		logger.Warning("[agent] 配置资源同步失败: %v", err)
	}
	if serverAddress == "" {
		logger.Warning("[agent] 未配置 agent-config.server.address，请通过 Web 端「Agent 管理 -> 设置」配置服务端地址；截图上传和配置资源同步将不可用")
	}

	logger.Info(
		"[agent] 启动成功: id=%s concurrency=%d poll=%s heartbeat=%s lease=%s configCheck=%s",
		agentID, concurrency, pollInterval.String(), heartbeatInterval.String(), leaseTTL.String(), configCheckInterval.String(),
	)

	stopCh := make(chan struct{})
	drainCh := make(chan struct{})
	updateReady := make(chan struct{}, 1)
	pipeClosed := make(chan struct{})
	if supervised {
		go func() { _, _ = io.Copy(io.Discard, os.Stdin); close(pipeClosed) }()
	}
	var wg sync.WaitGroup
	var scanWG sync.WaitGroup
	aiWG := &sync.WaitGroup{}
	var drainOnce sync.Once

	var runningMu sync.Mutex
	runningCancels := make(map[string]context.CancelFunc)

	hasRunningTasks := func() bool {
		runningMu.Lock()
		defer runningMu.Unlock()
		return len(runningCancels) > 0
	}
	cancelTask := func(prefix string) bool {
		runningMu.Lock()
		defer runningMu.Unlock()
		found := false
		for key, cancel := range runningCancels {
			if key == prefix || strings.HasPrefix(key, prefix+":") {
				cancel()
				found = true
			}
		}
		return found
	}
	setCancel := func(key string, cancel context.CancelFunc) {
		runningMu.Lock()
		defer runningMu.Unlock()
		runningCancels[key] = cancel
	}
	delCancel := func(key string) {
		runningMu.Lock()
		defer runningMu.Unlock()
		delete(runningCancels, key)
	}

	instanceLost := make(chan error, 1)
	commandLoop := func() {
		defer wg.Done()
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				checkCtx, cancelCheck := context.WithTimeout(context.Background(), 2*time.Second)
				checkErr := nodeSession.Check(checkCtx)
				cancelCheck()
				if checkErr != nil {
					instanceLost <- checkErr
					return
				}
				populateAgentResourceMetrics(&nodeOpt)
				if err := agentruntime.HeartbeatAgent(agentID, scanTypes, leaseTTL, nodeOpt); err != nil {
					logger.Warning("[agent] 心跳失败: %v", err)
				}
				if supervised && agentupdate.StopRequested(db, agentID) {
					drainOnce.Do(func() {
						close(drainCh)
						go func() { scanWG.Wait(); aiWG.Wait(); updateReady <- struct{}{} }()
					})
				}
				if time.Since(lastConfigCheck) >= configCheckInterval && !hasRunningTasks() {
					serverAddress := loadAgentServerAddress()
					lastConfigCheck = time.Now()
					if err := syncAgentConfigResources(workerCtx, nodeSession, serverAddress); err != nil {
						logger.Warning("[agent] 配置资源同步失败: %v", err)
					}
					uploadAddress.Store(serverAddress)
				}

				commands, err := agentruntime.ClaimPendingAgentCommands(agentID, 50)
				if err != nil {
					logger.Warning("[agent] 拉取命令失败: %v", err)
					continue
				}
				for _, cmd := range commands {
					switch strings.ToLower(strings.TrimSpace(cmd.Command)) {
					case "cancel":
						key := taskKey(cmd.TaskID, cmd.TaskVersion)
						if cancelTask(key) {
							_ = agentruntime.FinishAgentCommand(cmd.ID, "done", "cancel 信号已发送")
						} else {
							_ = agentruntime.FinishAgentCommand(cmd.ID, "done", "任务未在运行或已结束")
						}
					default:
						_ = agentruntime.FinishAgentCommand(cmd.ID, "failed", "不支持的命令: "+cmd.Command)
					}
				}
			}
		}
	}

	worker := func(workerID int) {
		defer wg.Done()
		defer scanWG.Done()
		for {
			select {
			case <-stopCh:
				return
			case <-drainCh:
				return
			default:
			}

			task, err := agentruntime.ClaimNextAgentTask(agentID, scanTypes, leaseTTL)
			if err != nil {
				logger.Warning("[agent][w%d] claim 任务失败: %v", workerID, err)
				time.Sleep(pollInterval)
				continue
			}
			if task == nil {
				time.Sleep(pollInterval)
				continue
			}

			logger.Info("[agent][w%d] 开始执行任务: id=%s version=%d type=%s", workerID, task.ID, task.Version, task.ScanType)
			ctx, cancel := context.WithCancel(context.Background())
			key := taskRunKey(task.ID, task.Version, task.ItemID)
			setCancel(key, cancel)

			if nodeSession != nil {
				endpoint := joinServerAddress(uploadAddress.Load().(string), "api/v1/webscan/agent-assets/upload")
				ctx = agentasset.WithUpload(ctx, endpoint, func(req *http.Request) error {
					return nodeSession.AuthorizePlatformRequest(req.Context(), agentID, req)
				})
			}
			err = agentruntime.ExecuteAgentTaskWithContext(ctx, task)
			delCancel(key)
			cancel()

			if err != nil {
				if errors.Is(err, context.Canceled) {
					logger.Info("[agent][w%d] 任务已取消: id=%s version=%d", workerID, task.ID, task.Version)
				} else {
					logger.Warning("[agent][w%d] 任务执行失败: id=%s version=%d err=%v", workerID, task.ID, task.Version, err)
				}
			} else {
				logger.Info("[agent][w%d] 任务执行完成: id=%s version=%d", workerID, task.ID, task.Version)
			}
		}
	}

	if aiEnabled {
		aiWG = ai.StartAIWorkersDraining(workerCtx, agentID, aiConcurrency, false, serverAddress, nodeSession, drainCh)
	}
	if !scanEnabled {
		concurrency = 0
	}
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		scanWG.Add(1)
		go worker(i + 1)
	}
	wg.Add(1)
	go commandLoop()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
	case <-pipeClosed:
	case <-updateReady:
		logger.Info("[agent] 更新前所有 Worker 已退出，释放节点会话")
	case err := <-instanceLost:
		logger.Warning("[agent] 主机运行锁已失效，停止领取与执行任务: %v", err)
	}

	logger.Info("[agent] 收到退出信号，停止中...")
	close(stopCh)
	stopWorkers()
	runningMu.Lock()
	for _, cancel := range runningCancels {
		cancel()
	}
	runningMu.Unlock()
	wg.Wait()
	aiWG.Wait()

	checkCtx, cancelCheck := context.WithTimeout(context.Background(), 2*time.Second)
	if nodeSession.Check(checkCtx) == nil {
		if err := agentruntime.SetAgentNodeOffline(agentID); err != nil {
			logger.Warning("[agent] 设置离线状态失败: %v", err)
		}
	}
	cancelCheck()
	logger.Info("[agent] 已停止")
}
