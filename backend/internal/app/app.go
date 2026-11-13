package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"hephaestus-backend/handlers/ai"
	"hephaestus-backend/handlers/scan"
	"hephaestus-backend/internal/mcptooling"
	mcpserver "hephaestus-backend/mcp"
	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/control"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/taskqueue"
	"hephaestus-backend/pkg/toolguard"
	"hephaestus-backend/pkg/webscan"
	"hephaestus-backend/routes"
	"hephaestus-backend/webassets"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type App struct {
	isShuttingDown int32
}

func New() *App {
	return &App{}
}

func (a *App) Bootstrap() error {
	if _, err := database.GetDB(); err != nil {
		return err
	}
	if err := ai.PrepareUnifiedLocalAgent(context.Background(), scan.LoadScanRuntimeConfig().MaxConcurrentTasks); err != nil {
		return fmt.Errorf("prepare unified local Agent: %w", err)
	}

	go func() {
		db, err := database.GetDB()
		if err != nil {
			logger.Warning("获取数据库连接失败，无法回填子域名资产观察记录: %v", err)
			return
		}
		count, err := assetstore.BackfillMissingSubdomainObservations(context.Background(), db)
		if err != nil {
			logger.Warning("回填子域名资产观察记录失败: %v", err)
			return
		}
		if count > 0 {
			logger.Info("已回填子域名资产观察记录: %d 个任务", count)
		}
		summary, err := assetstore.ReclassifyMisclassifiedIPAssets(context.Background(), db)
		if err != nil {
			logger.Warning("修正 IPv6 资产类型失败: %v", err)
			return
		}
		if summary.Reclassified > 0 || summary.Archived > 0 || summary.Failed > 0 {
			logger.Info("IPv6 资产类型修正完成: 重分类 %d，归档 %d，失败 %d", summary.Reclassified, summary.Archived, summary.Failed)
		}
	}()
	assetstore.StartLifecycleScheduler()
	go func() {
		db, err := database.GetDB()
		if err != nil {
			logger.Warning("获取数据库连接失败，无法回填 favicon Icon Hash: %v", err)
			return
		}
		if err := webscan.BackfillFaviconIconHashes(db); err != nil {
			logger.Warning("回填历史 favicon Icon Hash 失败: %v", err)
		}
	}()

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
	guardConfig, err := config.GetToolGuardConfig()
	if err != nil {
		return fmt.Errorf("load tool guard config: %w", err)
	}
	if err := toolguard.UpdateGlobal(guardConfig); err != nil {
		return fmt.Errorf("initialize tool guard: %w", err)
	}

	mcptooling.Install()

	if err := ai.InitAIBatchQueues(); err != nil {
		return fmt.Errorf("initialize AI batch queues: %w", err)
	}

	scanRuntimeConfig := scan.LoadScanRuntimeConfig()
	scan.ApplyScanRuntimeConfig(scanRuntimeConfig)

	taskqueue.InitGlobalQueue(scanRuntimeConfig.MaxConcurrentTasks)
	taskqueue.InitCompanyInfoQueue()
	taskqueue.InitSpaceExportQueue()

	scan.StartCombinationScheduler(scanRuntimeConfig.MaxConcurrentTasks)
	scan.StartScheduledScanDispatcher()
	if err := webscan.RegisterCodeTemplateSignerForNuclei(); err != nil {
		logger.Warning("Nuclei code 模板签名验证器初始化失败: %v", err)
	}
	// 模板索引在后台预热：窗口不必等待 5795 个模板文件被逐个读取解析。
	webscan.StartPOCTemplateCacheWarmup()
	if err := ai.StartUnifiedLocalAgent(context.Background(), scanRuntimeConfig.MaxConcurrentTasks); err != nil {
		return fmt.Errorf("start unified local Agent: %w", err)
	}
	scan.RecoverInterruptedLocalTasks()
	if count, err := ai.RecoverAIPentestSoloRuns(context.Background()); err != nil {
		logger.Warning("AI 独立渗透对话恢复不完整: %v", err)
	} else if count > 0 {
		logger.Info("已恢复 %d 个 AI 独立渗透对话", count)
	}

	return nil
}

func (a *App) BuildRouter(registerEmbeddedAssets bool) *gin.Engine {
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false

	// gin.Default already installs Logger and Recovery. Registering them again
	// makes every request appear twice in the access log.
	r.Use(func(c *gin.Context) {
		if atomic.LoadInt32(&a.isShuttingDown) == 1 {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "Server is shutting down",
			})
			c.Abort()
			return
		}
		c.Next()
	})

	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{"*"}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{
		"Origin",
		"Content-Type",
		"Accept",
		"Authorization",
		"Last-Event-ID",
		"X-Hephaestus-Desktop-Token",
		"MCP-Protocol-Version",
		"Mcp-Session-Id",
	}
	corsConfig.ExposeHeaders = []string{
		"MCP-Protocol-Version",
		"Mcp-Session-Id",
		"Content-Disposition",
		"X-Total-Count",
	}
	corsConfig.AllowCredentials = true
	r.Use(cors.New(corsConfig))

	routes.SetupRoutes(r)

	if registerEmbeddedAssets {
		if err := webassets.Register(r); err != nil {
			log.Printf("⚠️  前端资源挂载失败: %v", err)
		}
	}

	return r
}

func (a *App) NewHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  300 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  300 * time.Second,
	}
}

func (a *App) Shutdown(ctx context.Context) error {
	atomic.StoreInt32(&a.isShuttingDown, 1)

	batchStopCtx, batchStopCancel := context.WithTimeout(ctx, 20*time.Second)
	if err := ai.StopAIBatchQueues(batchStopCtx); err != nil {
		logger.Warning("AI 队列暂停尚未完成，将在重启时恢复暂停状态: %v", err)
	}
	batchStopCancel()
	if err := ai.StopUnifiedLocalAgent(ctx); err != nil {
		logger.Warning("本机 Agent 尚未确认退出: %v", err)
	}

	logger.Info("正在取消所有运行中的任务...")
	control.GetTaskController().CancelAllTasks()
	logger.Info("所有任务已发送取消信号")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopCancel()

	scan.StopCombinationScheduler(stopCtx)

	stopDone := make(chan struct{})
	go func() {
		taskqueue.GetGlobalQueue().Stop()
		taskqueue.GetCompanyInfoQueue().Stop()
		taskqueue.GetSpaceExportQueue().Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		logger.Info("任务队列管理器已停止")
	case <-stopCtx.Done():
		logger.Warning("任务队列停止超时，强制继续关闭")
	}

	mcpserver.ShutdownMCPListener(ctx)
	a.cleanupRunningTasks()

	return ctx.Err()
}

func FormatBootstrapError(err error) string {
	errMsg := err.Error()
	configPath := config.GetResolvedConfigPath()

	if contains(errMsg, "数据库配置不完整") || contains(errMsg, "配置文件") {
		return fmt.Sprintf("\n❌ 数据库配置错误:\n%s\n\n请填写配置文件后重新启动应用。\n", errMsg)
	}

	if contains(errMsg, "sqlite") {
		return fmt.Sprintf("\n❌ SQLite 初始化失败:\n%s\n\n请检查数据库文件路径是否可写。\n配置文件路径: %s\n", errMsg, configPath)
	}

	if contains(errMsg, "连接") || contains(errMsg, "连接失败") ||
		contains(errMsg, "connect") || contains(errMsg, "connection") ||
		contains(errMsg, "dial") || contains(errMsg, "timeout") ||
		contains(errMsg, "拒绝") || contains(errMsg, "refused") {
		return fmt.Sprintf(`
❌ 数据库连接失败:
%s

请检查以下配置项:
  1. 数据库服务是否已启动
  2. 主机地址、端口是否正确
  3. 用户名和密码是否正确
  4. 数据库名称是否正确
  5. 网络连接是否正常

配置文件路径: %s

请修改配置文件后重新启动应用。
`, errMsg, configPath)
	}

	return fmt.Sprintf(`
❌ 数据库初始化失败:
%s

请检查数据库配置并重新启动应用。
配置文件路径: %s
`, errMsg, configPath)
}

func (a *App) cleanupRunningTasks() {
	logger.Info("正在清理运行中的本地模式漏洞扫描任务...")

	db, err := database.GetDB()
	if err != nil {
		logger.Warning("获取数据库连接失败，无法清理任务: %v", err)
		return
	}

	result := db.Model(&database.VulnScanTask{}).
		Where("status IN ? AND scan_mode = ? AND pause_requested = ?", []string{"running", "recovering"}, "local", false).
		Updates(map[string]interface{}{
			"status":       "recovering",
			"error_msg":    "任务因服务退出等待恢复",
			"heartbeat_at": nil,
			"updated_at":   time.Now(),
		})

	if result.Error != nil {
		logger.Warning("清理本地模式漏洞扫描任务失败: %v", result.Error)
		return
	}

	logger.Info("已标记 %d 个本地模式漏洞扫描任务为可恢复", result.RowsAffected)
}

func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
