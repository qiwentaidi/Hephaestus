package routes

import (
	"hephaestus-backend/handlers/admin"
	"hephaestus-backend/handlers/agent"
	"hephaestus-backend/handlers/ai"
	"hephaestus-backend/handlers/asset"
	"hephaestus-backend/handlers/dashboard"
	findinghandlers "hephaestus-backend/handlers/findings"
	"hephaestus-backend/handlers/health"
	mcphandlers "hephaestus-backend/handlers/mcp"
	"hephaestus-backend/handlers/scan"
	"hephaestus-backend/handlers/space"
	themehandlers "hephaestus-backend/handlers/theme"
	"hephaestus-backend/handlers/toolbox"
	mcpserver "hephaestus-backend/mcp"
	"hephaestus-backend/middleware"
	appconfig "hephaestus-backend/pkg/config"

	"github.com/gin-gonic/gin"
)

// SetupRoutes 设置路由
func SetupRoutes(r *gin.Engine) {
	// Container/orchestrator readiness probe. Keep it outside /api/v1 and auth.
	r.GET("/healthz", health.Handler)

	api := r.Group("/api/v1")
	api.GET("/agent/release", agent.AgentReleaseHandler)
	api.GET("/agent/:file", agent.AgentInstallerHandler)
	api.POST("/internal/agents/v1/tools/call", ai.WorkerPlatformToolHandler)
	{
		// 认证相关路由（不需要JWT认证）
		auth := api.Group("/auth")
		{
			auth.GET("/check", admin.CheckInitHandler) // 检查系统是否初始化
			auth.POST("/init", admin.InitAdminHandler) // 初始化管理员账户
			auth.POST("/login", admin.LoginHandler)    // 用户登录
		}

		// 需要JWT认证的用户接口
		authProtected := api.Group("/auth")
		authProtected.Use(middleware.JWTAuthMiddleware())
		{
			authProtected.GET("/user", admin.GetCurrentUserHandler)     // 获取当前用户信息
			authProtected.PUT("/user", admin.UpdateUserHandler)         // 更新用户信息
			authProtected.PUT("/password", admin.ChangePasswordHandler) // 修改密码
		}

		// 以下所有接口都需要JWT认证
		fofa := api.Group("/fofa")
		fofa.Use(middleware.JWTAuthMiddleware())
		{
			fofa.POST("/search", space.SearchHandler)
			fofa.GET("/user", space.UserInfoHandler)
			fofa.GET("/history", space.ListFofaSearchHistoryHandler)
			fofa.POST("/history", space.SaveFofaSearchHistoryHandler)
			fofa.POST("/icon-hash", space.IconHashHandler)
			fofa.POST("/stats", space.StatsHandler)
			fofa.POST("/export/tasks", space.CreateFofaExportTaskHandler)
			fofa.GET("/export/tasks", space.ListFofaExportTasksHandler)
			fofa.GET("/export/tasks/:id", space.GetFofaExportTaskHandler)
			fofa.GET("/export/tasks/:id/download", space.DownloadFofaExportTaskHandler)
		}
		hunter := api.Group("/hunter")
		hunter.Use(middleware.JWTAuthMiddleware())
		{
			hunter.POST("/search", space.HunterSearchHandler)
			hunter.GET("/tips", space.HunterTipsHandler)
			hunter.GET("/user", space.HunterUserInfoHandler)
			hunter.GET("/history", space.ListHunterSearchHistoryHandler)
			hunter.POST("/history", space.SaveHunterSearchHistoryHandler)
			hunter.POST("/export/tasks", space.CreateHunterExportTaskHandler)
		}
		quake := api.Group("/quake")
		quake.Use(middleware.JWTAuthMiddleware())
		{
			quake.POST("/search", space.QuakeSearchHandler)
			quake.GET("/tips", space.QuakeTipsHandler)
			quake.GET("/user", space.QuakeUserInfoHandler)
			quake.GET("/history", space.ListQuakeSearchHistoryHandler)
			quake.POST("/history", space.SaveQuakeSearchHistoryHandler)
			quake.POST("/icon-hash", space.QuakeIconHashHandler)
			quake.POST("/export/tasks", space.CreateQuakeExportTaskHandler)
		}
		shodan := api.Group("/shodan")
		shodan.Use(middleware.JWTAuthMiddleware())
		{
			shodan.GET("/ip", space.ShodanIpReportHandler)
		}
		config := api.Group("/config")
		config.Use(middleware.JWTAuthMiddleware())
		{
			config.GET("", admin.GetConfigHandler)
			config.PUT("", admin.UpdateConfigHandler)
			config.GET("/space-engines", admin.GetSpaceEngineConfigHandler)    // 获取所有空间引擎配置（向后兼容）
			config.PUT("/space-engines", admin.UpdateSpaceEngineConfigHandler) // 更新所有空间引擎配置（向后兼容）
			config.GET("/database", admin.GetDatabaseConfigHandler)
			config.PUT("/database", admin.UpdateDatabaseConfigHandler)
			config.GET("/dingtalk", admin.GetDingTalkConfigHandler)    // 获取钉钉配置
			config.PUT("/dingtalk", admin.UpdateDingTalkConfigHandler) // 更新钉钉配置
			config.POST("/dingtalk/test", admin.TestDingTalkNotificationHandler)
			config.GET("/wechat", admin.GetWeChatConfigHandler)    // 获取企业微信配置
			config.PUT("/wechat", admin.UpdateWeChatConfigHandler) // 更新企业微信配置
			config.POST("/wechat/test", admin.TestWeChatNotificationHandler)
			config.GET("/feishu", admin.GetFeishuConfigHandler)    // 获取飞书配置
			config.PUT("/feishu", admin.UpdateFeishuConfigHandler) // 更新飞书配置
			config.POST("/feishu/test", admin.TestFeishuNotificationHandler)
			config.GET("/notifications", admin.GetNotificationConfigHandler)
			config.PUT("/notifications", admin.UpdateNotificationConfigHandler)
			config.POST("/notifications/test", admin.TestNotificationChannelHandler)
			config.GET("/ai", admin.GetAIConfigHandler)                          // 获取AI配置
			config.PUT("/ai", admin.UpdateAIConfigHandler)                       // 更新AI配置
			config.POST("/ai/test", admin.TestAIConfigHandler)                   // 测试AI配置
			config.POST("/ai/models", admin.ListAIModelsHandler)                 // 获取渠道模型列表
			config.GET("/migration/export", admin.ExportConfigMigrationHandler)  // 导出配置迁移包
			config.POST("/migration/import", admin.ImportConfigMigrationHandler) // 导入配置迁移包
			config.GET("/tool-guard", toolbox.GetToolGuardConfigHandler)         // 获取调用拦截配置
			config.PUT("/tool-guard", toolbox.UpdateToolGuardConfigHandler)      // 更新调用拦截配置
			config.POST("/tool-guard/test", toolbox.TestToolGuardConfigHandler)  // 试匹配调用拦截规则
			config.GET("/update-server", admin.GetUpdateServerHandler)           // 获取更新服务器地址
			config.PUT("/update-server", admin.UpdateUpdateServerHandler)        // 更新服务器地址
			config.GET("/query-presets", scan.ListSavedQueryPresetsHandler)
			config.POST("/query-presets", scan.SaveSavedQueryPresetHandler)
			config.DELETE("/query-presets/:id", scan.DeleteSavedQueryPresetHandler)
			config.GET("/port-groups", scan.ListCustomPortGroupsHandler)
			config.POST("/port-groups", scan.SaveCustomPortGroupHandler)
			config.DELETE("/port-groups/:id", scan.DeleteCustomPortGroupHandler)
			config.GET("/port-groups/default", scan.GetDefaultPortGroupHandler)
			config.PUT("/port-groups/default", scan.SaveDefaultPortGroupHandler)
			// 通用扫描模板（支持多任务类型）
			config.GET("/scan-templates", scan.ListScanTemplatesV2Handler)         // 获取扫描模板列表
			config.POST("/scan-templates", scan.CreateScanTemplateV2Handler)       // 创建扫描模板
			config.GET("/scan-templates/:id", scan.GetScanTemplateV2Handler)       // 获取单个扫描模板
			config.PUT("/scan-templates/:id", scan.UpdateScanTemplateV2Handler)    // 更新扫描模板
			config.DELETE("/scan-templates/:id", scan.DeleteScanTemplateV2Handler) // 删除扫描模板
		}
		themes := api.Group("/themes")
		themes.Use(middleware.JWTAuthMiddleware())
		{
			themes.GET("", themehandlers.ListCustomThemesHandler)
			themes.POST("", themehandlers.CreateCustomThemeHandler)
			themes.POST("/generate", themehandlers.GenerateCustomThemeHandler)
			themes.POST("/import", themehandlers.ImportCustomThemesHandler)
			themes.PUT("/:id", themehandlers.UpdateCustomThemeHandler)
			themes.DELETE("/:id", themehandlers.DeleteCustomThemeHandler)
		}
		dict := api.Group("/dict")
		dict.Use(middleware.JWTAuthMiddleware())
		{
			dict.GET("/crack", toolbox.ListCrackDictHandler)                     // 列出爆破字典
			dict.GET("/crack/content", toolbox.GetCrackDictContentHandler)       // 获取指定字典内容
			dict.POST("/crack/content", toolbox.SaveCrackDictContentHandler)     // 保存字典内容
			dict.DELETE("/crack/content", toolbox.DeleteCrackDictContentHandler) // 删除字典内容
		}
		spaceEngine := api.Group("/space-engine")
		spaceEngine.Use(middleware.JWTAuthMiddleware())
		{
			spaceEngine.GET("", admin.GetSpaceEngineConfigHandler)                     // 获取所有空间引擎配置
			spaceEngine.PUT("", admin.UpdateSpaceEngineConfigHandler)                  // 更新所有空间引擎配置
			spaceEngine.GET("/:type", space.GetSpaceEngineConfigByTypeHandler)         // 获取指定类型配置
			spaceEngine.PUT("/:type", space.UpdateSpaceEngineConfigByTypeHandler)      // 更新指定类型配置（部分更新）
			spaceEngine.GET("/:type/validate", space.ValidateSpaceEngineConfigHandler) // 验证指定类型配置
		}
		mcpAudit := api.Group("/mcp")
		mcpAudit.Use(middleware.JWTAuthMiddleware())
		{
			mcpAudit.GET("/servers", mcpserver.ListMCPServersHandler)
			mcpAudit.GET("/external-servers", mcphandlers.ListExternalMCPServersHandler)
			mcpAudit.GET("/system-tools", mcphandlers.ListSystemToolsHandler)
			mcpAudit.GET("/callable-tools", mcphandlers.ListCallableAIToolsHandler)
			mcpAudit.GET("/ai-tool-call-counts", mcphandlers.AIToolCallCountsHandler)
			mcpAudit.GET("/agent-tool-visibility", mcphandlers.GetAgentToolVisibilityHandler)
			mcpAudit.PUT("/agent-tool-visibility", mcphandlers.SetAgentToolVisibilityHandler)
			mcpAudit.POST("/system-tools/bulk", mcphandlers.SetAIToolsEnabledBulkHandler)
			mcpAudit.PUT("/system-tools/:name", mcphandlers.SetAIToolEnabledHandler)
			mcpAudit.POST("/external-servers", mcphandlers.SaveExternalMCPServerHandler)
			mcpAudit.PUT("/external-servers/:id", mcphandlers.SaveExternalMCPServerHandler)
			mcpAudit.DELETE("/external-servers/:id", mcphandlers.DeleteExternalMCPServerHandler)
			mcpAudit.POST("/external-servers/:id/discover", mcphandlers.DiscoverExternalMCPServerHandler)
			mcpAudit.GET("/space-engine/audit", mcpserver.ListMCPAuditLogsHandler)
			mcpAudit.GET("/listener", mcpserver.GetMCPListenerHandler)       // 获取 MCP 监听配置
			mcpAudit.PUT("/listener", mcpserver.UpdateMCPListenerHandler(r)) // 更新 MCP 监听配置
		}
		mcp := api.Group("/mcp")
		mcp.Use(middleware.MCPAuthMiddleware())
		{
			mcp.POST("/hephaestus", mcpserver.MCPToolHephaestusHandler)
			mcp.DELETE("/hephaestus", mcpserver.MCPToolHephaestusHandler)
			mcp.POST("/space-engine", mcpserver.MCPToolSpaceEngineHandler)
			mcp.DELETE("/space-engine", mcpserver.MCPToolSpaceEngineHandler)
		}
		aiGroup := api.Group("/ai")
		aiGroup.Use(middleware.JWTAuthMiddleware())
		{
			ai.RegisterAIBatchRoutes(aiGroup)
			aiGroup.GET("/agent-prompts", ai.ListAIAgentPromptsHandler)
			aiGroup.PUT("/agent-prompts/:role", ai.SaveAIAgentPromptHandler)
			aiGroup.POST("/side-chat/stream", ai.StreamAIWorkbenchSideChatHandler)
			aiGroup.GET("/model-options", ai.ListAIPentestModelOptionsHandler)
			aiGroup.GET("/tools", ai.ListAIPentestToolsHandler)
			aiGroup.GET("/strategy-packs", ai.ListAIPentestStrategyPacksHandler)
			aiGroup.GET("/nodes", ai.ListAIPentestNodesHandler)
			aiGroup.GET("/projects", ai.ListAIPentestProjectsHandler)
			aiGroup.POST("/projects", ai.CreateAIPentestProjectHandler)
			aiGroup.GET("/projects/:id/token-usage", ai.GetAIPentestProjectTokenUsageHandler)
			aiGroup.GET("/projects/:id/storage", ai.GetAIPentestProjectStorageHandler)
			aiGroup.GET("/projects/:id/blackboard", ai.GetAIPentestProjectBlackboardHandler)
			aiGroup.GET("/projects/:id/conversation-options", ai.ListAIPentestProjectConversationOptionsHandler)
			aiGroup.PATCH("/projects/:id", ai.UpdateAIPentestProjectHandler)
			aiGroup.DELETE("/projects/:id", ai.DeleteAIPentestProjectHandler)
			aiGroup.GET("/asset-reference-tasks", ai.ListAIAssetReferenceTasksHandler)
			aiGroup.GET("/asset-reference-tasks/:id/assets", ai.ListAIReferenceAssetsHandler)
			aiGroup.GET("/sessions/bootstrap", ai.GetAIPentestSessionBootstrapHandler)
			aiGroup.GET("/sessions", ai.ListAIPentestSessionsHandler)
			aiGroup.POST("/sessions", ai.CreateAIPentestSessionHandler)
			aiGroup.GET("/plan-runs", ai.ListAIPlanRunsHandler)
			aiGroup.POST("/plan-runs", ai.CreateAIPlanRunHandler)
			aiGroup.GET("/plan-runs/:id", ai.GetAIPlanRunHandler)
			aiGroup.GET("/plan-runs/:id/items", ai.ListAIPlanItemRowsHandler)
			aiGroup.GET("/plan-runs/:id/items/:itemId", ai.GetAIPlanItemHandler)
			aiGroup.POST("/plan-runs/:id/controller", ai.EnsureAIPlanRunControllerHandler)
			aiGroup.POST("/plan-runs/:id/pause", ai.PauseAIPlanRunHandler)
			aiGroup.POST("/plan-runs/:id/cancel", ai.CancelAIPlanRunHandler)
			aiGroup.POST("/plan-runs/:id/resume", ai.ResumeAIPlanRunHandler)
			aiGroup.POST("/plan-runs/:id/reopen", ai.ReopenAIPlanRunHandler)
			aiGroup.GET("/sessions/:id", ai.GetAIPentestSessionHandler)
			aiGroup.PATCH("/sessions/:id", ai.UpdateAIPentestSessionHandler)
			aiGroup.DELETE("/sessions/:id", ai.DeleteAIPentestSessionHandler)
			aiGroup.GET("/sessions/:id/side-chat", ai.GetAIPentestSideChatHistoryHandler)
			aiGroup.PUT("/sessions/:id/side-chat", ai.SaveAIPentestSideChatHistoryHandler)
			aiGroup.DELETE("/sessions/:id/side-chat", ai.DeleteAIPentestSideChatHistoryHandler)
			aiGroup.GET("/sessions/:id/stream", ai.StreamAIPentestSessionHandler)
			aiGroup.POST("/sessions/:id/message", ai.CreateAIPentestSessionMessageHandler)
			aiGroup.POST("/sessions/:id/message/stream", ai.CreateAIPentestSessionMessageStreamHandler)
			aiGroup.POST("/sessions/:id/cancel", ai.ApplyAIPentestSessionActionHandler)
			aiGroup.POST("/sessions/:id/action", ai.ApplyAIPentestSessionActionHandler)
			aiGroup.GET("/sessions/:id/blackboard", ai.GetAIPentestSessionBlackboardHandler)
			aiGroup.GET("/sessions/:id/executions/:executionId", ai.GetAIPentestSessionExecutionEvidenceHandler)
			aiGroup.GET("/sessions/:id/report", ai.GetAIPentestReportHandler)
			aiGroup.GET("/sessions/:id/report/download", ai.DownloadAIPentestReportHandler)
			aiGroup.GET("/sessions/:id/messages/:messageId/images", ai.GetAIPentestMessageImagesHandler)
			aiGroup.GET("/sessions/:id/turns/:turnId", ai.GetAIPentestSessionTurnHandler)
			aiGroup.GET("/sessions/:id/rounds", ai.GetAIPentestEarlierRoundsHandler)
			aiGroup.GET("/sessions/:id/history", ai.GetAIPentestSessionHistoryHandler)
			aiGroup.POST("/reports/projects", ai.ExportAIPentestProjectReportsHandler)
			aiGroup.POST("/reports/findings", ai.ExportAIPentestFindingReportsHandler)
			aiGroup.GET("/findings", ai.ListAIPentestFindingsHandler)
			aiGroup.GET("/findings/management", ai.GetAIPentestFindingsManagementHandler)
			aiGroup.GET("/findings/:findingId/lineage", ai.GetAIPentestFindingLineageHandler)
			aiGroup.GET("/findings/:findingId", ai.GetAIPentestFindingDetailHandler)
			aiGroup.PUT("/findings/status", ai.UpdateAIPentestFindingStatusesHandler)
			aiGroup.PUT("/findings/:findingId/status", ai.UpdateAIPentestFindingStatusHandler)
		}
		system := api.Group("/system")
		system.Use(middleware.JWTAuthMiddleware())
		{
			// 网卡配置：获取与保存默认网卡配置
			system.GET("/network-cards", admin.GetNetworkCardsHandler)
			system.POST("/network-cards", admin.SaveNetworkConfigHandler)
			system.GET("/info", admin.GetSystemInfoHandler)   // 获取系统信息
			system.GET("/proxy", admin.GetSystemProxyHandler) // 获取系统代理配置
			system.GET("/browser-runtime", admin.GetBrowserRuntimeHandler)
			system.GET("/syn/status", admin.GetSYNCapabilityStatusHandler)
			system.POST("/syn/authorize", admin.RequestSYNAuthorizationHandler)
			system.GET("/logs/files", admin.ListSystemLogFilesHandler)
			system.GET("/logs/read", admin.ReadSystemLogHandler)
			// CPU 控制：获取与设置应用可用核心数（GOMAXPROCS）
			system.GET("/cpu", admin.GetAppCPUHandler)
			system.POST("/cpu", admin.SetAppCPUHandler)
			system.GET("/scan-runtime", scan.GetScanRuntimeConfigHandler)
			system.POST("/scan-runtime", scan.UpdateScanRuntimeConfigHandler)
			system.GET("/agent-config", agent.GetAgentConfigHandler)
			system.PUT("/agent-config", agent.UpdateAgentConfigHandler)
			// 系统更新
			system.GET("/update/check", admin.CheckUpdate)                   // 检查更新
			system.GET("/update/history", admin.GetUpdateHistory)            // 获取更新历史
			system.POST("/update/download", admin.DownloadAndUpdate)         // 下载并更新
			system.POST("/update/restart", admin.RestartService)             // 重启服务
			system.GET("/update/info", admin.GetSystemInfo)                  // 获取系统信息
			system.GET("/update/running-tasks", admin.GetUpdateRunningTasks) // 重启前的运行中任务概况
			// 配置文件更新
			system.GET("/update/config/info", admin.GetConfigUpdateInfo) // 获取配置更新信息
			system.GET("/update/config/check", admin.CheckConfigUpdate)  // 检查配置更新
		}
		// SSE 下载（认证在 handler 内部通过 query token 处理）
		systemSSE := api.Group("/system")
		{
			systemSSE.GET("/update/download-stream", admin.DownloadWithProgress)
			systemSSE.GET("/update/config/download-stream", admin.DownloadConfigWithProgress)
		}
		tools := api.Group("/tools")
		tools.Use(middleware.JWTAuthMiddleware())
		{
			tools.POST("/key-manager/request", admin.KeyManagerRequestHandler)
			tools.POST("/key-manager/dgwork/token", admin.KeyManagerDGWorkTokenHandler)
			tools.GET("/key-manager/entries", admin.ListKeyManagerEntriesHandler)
			tools.POST("/key-manager/entries", admin.CreateKeyManagerEntryHandler)
			tools.GET("/key-manager/entries/:id", admin.GetKeyManagerEntryHandler)
			tools.PUT("/key-manager/entries/:id", admin.UpdateKeyManagerEntryHandler)
			tools.DELETE("/key-manager/entries/:id", admin.DeleteKeyManagerEntryHandler)
			tools.POST("/jwt/bruteforce", toolbox.JWTBruteforceHandler)
			tools.POST("/api-doc-test/analyze", toolbox.APIDocTestAnalyzeHandler)
			tools.POST("/api-doc-test/run", toolbox.APIDocTestRunHandler)
			tools.GET("/api-doc-test/tasks", toolbox.APIDocTestListTasksHandler)
			tools.POST("/api-doc-test/tasks", toolbox.APIDocTestCreateTaskHandler)
			tools.GET("/api-doc-test/tasks/:id", toolbox.APIDocTestGetTaskHandler)
			tools.PUT("/api-doc-test/tasks/:id", toolbox.APIDocTestUpdateTaskHandler)
			tools.POST("/api-doc-test/tasks/:id/retest", toolbox.APIDocTestRetestTaskHandler)
			tools.DELETE("/api-doc-test/tasks/:id", toolbox.APIDocTestDeleteTaskHandler)
			if appconfig.GetRuntimeMode() == appconfig.RuntimeModeDesktop {
				tools.GET("/mini-program-decompile/settings", toolbox.GetMiniProgramDecompileSettingsHandler)
				tools.PUT("/mini-program-decompile/settings", toolbox.UpdateMiniProgramDecompileSettingsHandler)
				tools.GET("/mini-program-decompile/programs", toolbox.ListMiniProgramDecompileProgramsHandler)
				tools.POST("/mini-program-decompile/decompile", toolbox.DecompileMiniProgramHandler)
				tools.GET("/mini-program-decompile/matches", toolbox.GetMiniProgramMatchesHandler)
				tools.POST("/mini-program-decompile/clear", toolbox.ClearMiniProgramDecompileHandler)
			}
		}
		subdomain := api.Group("/subdomain")
		subdomain.Use(middleware.JWTAuthMiddleware())
		{
			subdomain.GET("/dns-servers", scan.GetSubdomainDNSServersHandler)         // 获取DNS服务器配置
			subdomain.PUT("/dns-servers", scan.UpdateSubdomainDNSServersHandler)      // 更新DNS服务器配置
			subdomain.GET("/tasks", scan.ListSubdomainScanTasksHandler)               // 获取攻击面评估子域名任务列表
			subdomain.GET("/task/:id/results", scan.GetSubdomainScanResultsHandler)   // 获取攻击面评估子域名结果
			subdomain.GET("/task/:id/export", scan.ExportSubdomainScanResultsHandler) // 导出攻击面评估子域名结果（CSV）
			subdomain.GET("/task/:id/copy", scan.CopySubdomainScanResultsHandler)     // 获取攻击面评估子域名复制文本
		}
		dirsearch := api.Group("/dirsearch")
		dirsearch.Use(middleware.JWTAuthMiddleware())
		{
			dirsearch.GET("/dicts", scan.GetDirsearchDictsHandler)                         // 获取字典文件列表
			dirsearch.POST("/dicts/upload", scan.UploadDirsearchDictHandler)               // 上传字典文件        // 删除任务
			dirsearch.GET("/path-history", scan.ListDirsearchPathHistoryHandler)           // 获取路径历史记录列表
			dirsearch.GET("/path-history/get", scan.GetDirsearchPathHistoryHandler)        // 根据路径获取历史记录
			dirsearch.GET("/path-history/search", scan.SearchDirsearchPathHistoryHandler)  // 搜索路径历史记录
			dirsearch.DELETE("/path-history", scan.DeleteDirsearchPathHistoryHandler)      // 删除路径历史记录
			dirsearch.DELETE("/path-history/clear", scan.ClearDirsearchPathHistoryHandler) // 清空所有路径历史记录
			dirsearch.GET("/path-history/stats", scan.GetDirsearchPathHistoryStatsHandler) // 获取路径历史统计信息
		}
		company := api.Group("/company")
		company.Use(middleware.JWTAuthMiddleware())
		{
			company.POST("/info", asset.CompanyInfoHandler)                          // 企业信息收集（同步，保留兼容）
			company.POST("/validate-miit", asset.ValidateMiitApiHandler)             // 验证工信部API
			company.POST("/validate-riskbird", asset.ValidateRiskbirdHandler)        // 验证风鸟Cookie
			company.POST("/task", asset.CreateCompanyInfoTaskHandler)                // 创建企业信息查询任务
			company.GET("/tasks", asset.ListCompanyInfoTasksHandler)                 // 获取任务列表
			company.GET("/task/:id", asset.GetCompanyInfoTaskHandler)                // 获取任务详情
			company.GET("/task/:id/results", asset.GetCompanyInfoTaskResultsHandler) // 获取任务结果
			company.GET("/task/:id/export", asset.ExportCompanyInfoHandler)          // 导出任务结果为XLSX
			company.POST("/reports/merge", asset.ExportMergedCompanyInfoHandler)     // 合并导出企业信息结果为XLSX
			company.POST("/task/:id/cancel", asset.CancelCompanyInfoTaskHandler)     // 取消任务
			company.DELETE("/task/:id", asset.DeleteCompanyInfoTaskHandler)          // 删除任务
		}
		database := api.Group("/database")
		database.Use(middleware.JWTAuthMiddleware())
		{
			database.GET("/connections", admin.GetAllConnectionsHandler)         // 获取所有连接
			database.GET("/collect/rules", admin.GetCollectCustomRulesHandler)   // 获取自定义采集规则
			database.PUT("/collect/rules", admin.SaveCollectCustomRulesHandler)  // 保存自定义采集规则
			database.POST("/connection", admin.AddConnectionHandler)             // 添加连接
			database.PUT("/connection", admin.UpdateConnectionHandler)           // 更新连接
			database.DELETE("/connection", admin.RemoveConnectionHandler)        // 删除连接
			database.POST("/test", admin.TestConnectionHandler)                  // 测试连接
			database.GET("/info", admin.GetDatabaseInfoHandler)                  // 获取数据库信息
			database.POST("/table", admin.GetTableInfoHandler)                   // 获取表信息
			database.POST("/collect/save", admin.SaveCollectResultHandler)       // 保存采集结果
			database.GET("/collect/results", admin.GetCollectResultsHandler)     // 获取采集结果列表
			database.DELETE("/collect/result", admin.RemoveCollectResultHandler) // 删除采集结果
			database.POST("/stats", admin.UpdateConnectionStatsHandler)          // 更新连接统计信息
		}
		syntax := api.Group("/syntax")
		syntax.Use(middleware.JWTAuthMiddleware())
		{
			syntax.GET("", space.ListSpaceEngineSyntaxHandler)                    // 获取语法列表
			syntax.POST("", space.CreateSpaceEngineSyntaxHandler)                 // 创建语法
			syntax.DELETE("", space.DeleteSpaceEngineSyntaxHandler)               // 删除语法（根据ID）
			syntax.DELETE("/by-name", space.DeleteSpaceEngineSyntaxByNameHandler) // 删除语法（根据来源和名称）
		}
		// 截图和favicon静态资源路由（无需鉴权，供浏览器直接加载）
		webscanAssets := api.Group("/webscan")
		{
			webscanAssets.POST("/agent-assets/upload", scan.AgentUploadImageHandler) // Agent 上传截图/图标
			webscanAssets.GET("/agent-config/version", scan.AgentConfigResourceVersionHandler)
			webscanAssets.GET("/agent-config/download", scan.AgentConfigResourceDownloadHandler)
			webscanAssets.GET("/screenshot/*path", scan.ScreenshotHandler) // 获取截图文件（支持后续扩展对象存储）
			webscanAssets.GET("/favicon/*path", scan.FaviconHandler)       // 获取favicon文件（支持后续扩展对象存储）
		}
		// webscan API 路由（需要鉴权）
		webscan := api.Group("/webscan")
		webscan.Use(middleware.JWTAuthMiddleware())
		{
			webscan.POST("/scan", scan.CreateScanTaskHandler)                     // 创建扫描任务（别名，兼容前端）
			webscan.POST("/combination/tasks", scan.CreateCombinationTaskHandler) // 创建组合任务
			webscan.GET("/tasks", scan.ListWebScanTasksHandler)                   // 获取任务列表
			webscan.GET("/task/:id", scan.GetWebScanTaskHandler)                  // 获取任务详情
			webscan.GET("/trend", scan.GetWebScanTrendHandler)                    // 获取扫描趋势
			webscan.GET("/vuln-stats", scan.GetVulnSeverityStatsHandler)          // 获取漏洞等级分布
			webscan.GET("/vulns", scan.GetRecentVulnerabilitiesHandler)           // 获取最近发现的漏洞列表
			webscan.GET("/agent-updates", agent.AgentUpdatesHandler)
			webscan.POST("/agent-updates", agent.AgentUpdatesHandler)
			webscan.POST("/agent-updates/:id/cancel", agent.AgentUpdatesHandler)
			webscan.GET("/agents/:id/python", agent.AgentPythonHandler)
			webscan.POST("/agents/:id/python", agent.AgentPythonHandler)
			webscan.GET("/agents", scan.ListScanAgentsHandler)                                               // 获取扫描 Agent 列表
			webscan.PUT("/agents/:id", scan.UpdateScanAgentHandler)                                          // 更新扫描 Agent 显示名
			webscan.PUT("/agents/:id/enabled", scan.SetScanAgentEnabledHandler)                              // 启用/禁用扫描 Agent
			webscan.DELETE("/agents/:id", scan.DeleteScanAgentHandler)                                       // 删除离线 Agent 记录
			webscan.POST("/task/:id/start", scan.StartWebScanTaskHandler)                                    // 开始任务
			webscan.POST("/task/:id/pause", scan.PauseWebScanTaskHandler)                                    // 暂停任务
			webscan.POST("/task/:id/resume", scan.ResumeWebScanTaskHandler)                                  // 恢复任务
			webscan.PUT("/task/:id/name", scan.UpdateWebScanTaskNameHandler)                                 // 更新任务名称
			webscan.PUT("/task/:id/schedule", scan.UpdateWebScanTaskScheduleHandler)                         // 更新任务计划
			webscan.POST("/schedule/preview", scan.PreviewWebScanScheduleHandler)                            // 预览未来执行时间
			webscan.GET("/task/:id/schedule/logs", scan.ListWebScanTaskScheduleLogsHandler)                  // 获取任务计划调度记录
			webscan.POST("/task/:id/cancel", scan.CancelWebScanTaskHandler)                                  // 取消任务
			webscan.DELETE("/task/:id", scan.DeleteWebScanTaskHandler)                                       // 删除任务
			webscan.GET("/task/:id/items", scan.GetWebScanTaskItemsHandler)                                  // 获取 Agent 分片明细
			webscan.GET("/task/:id/fingerprints", scan.GetWebScanTaskFingerprintsHandler)                    // 获取指纹结果
			webscan.GET("/task/:id/fingerprint-icons", scan.GetWebScanTaskFingerprintIconsHandler)           // 获取 Top favicon 图标聚合
			webscan.PUT("/fingerprint/verification-status", scan.UpdateFingerprintVerificationStatusHandler) // 更新指纹验证状态
			webscan.GET("/task/:id/ports", scan.GetWebScanTaskPortsHandler)                                  // 获取端口结果
			webscan.GET("/task/:id/vulnerabilities", scan.GetWebScanTaskVulnerabilitiesHandler)              // 获取漏洞结果
			webscan.GET("/task/:id/dir-results", scan.GetDirScanResultsHandler)                              // 获取目录扫描结果
			webscan.GET("/task/:id/version/:version/time", scan.GetWebScanTaskVersionTimeHandler)            // 获取指定版本的时间信息
			webscan.GET("/task/:id/versions/history", scan.GetWebScanTaskVersionsHistoryHandler)             // 获取所有版本的历史统计
			webscan.DELETE("/vulnerability/:id", scan.DeleteVulnerabilityHandler)                            // 删除漏洞
			webscan.PUT("/vulnerability/:id/status", scan.UpdateVulnerabilityDisposalStatusHandler)          // 更新漏洞处置状态
			webscan.PUT("/vulnerability/status", scan.UpdateVulnerabilityDisposalStatusHandler)              // 更新漏洞处置状态（兼容无ID场景）
			webscan.GET("/templates", scan.GetTemplateListHandler)                                           // 获取模板列表             // 删除扫描模板
			webscan.GET("/poc-template-folders", scan.GetPOCTemplateFoldersHandler)                          // 获取追加 PoC 模板目录
			webscan.PUT("/poc-template-folders", scan.UpdatePOCTemplateFoldersHandler)                       // 更新追加 PoC 模板目录
			webscan.GET("/pocs", scan.ListPoCsHandler)                                                       // 获取 PoC 列表
			webscan.GET("/pocs/:id", scan.GetPoCContentHandler)                                              // 获取 PoC 内容
			webscan.PUT("/pocs/:id", scan.UpdatePoCContentHandler)                                           // 保存 PoC 内容
			webscan.DELETE("/pocs/:id", scan.DeletePoCHandler)                                               // 删除 PoC
			webscan.POST("/oob/test", scan.TestWebScanOOBHandler)                                            // 测试 OOB 连通性
			webscan.POST("/reports/merge", scan.ExportMergedWebScanReportsHandler)                           // 合并导出扫描报告
			webscan.GET("/task/:id/report", scan.ExportWebScanReportHandler)                                 // 导出扫描报告（最新版本）
			webscan.GET("/task/:id/version/:version/report", scan.ExportWebScanReportHandler)                // 导出指定版本的扫描报告
		}
		portscan := api.Group("/portscan")
		portscan.Use(middleware.JWTAuthMiddleware())
		{
			portscan.GET("/task/:id/results", scan.GetPortScanTaskResultsHandler)            // 获取端口扫描结果
			portscan.GET("/task/:id/results/urls", scan.GetPortScanTaskResultsAsURLsHandler) // 获取端口扫描结果（URL格式）
			portscan.GET("/task/:id/export", scan.ExportPortScanTaskResultsHandler)          // 导出端口扫描结果（Excel格式）
		}
		findingAPI := api.Group("/findings")
		findingAPI.Use(middleware.JWTAuthMiddleware())
		{
			findingAPI.POST("", findinghandlers.CreateFindingHandler)
			findingAPI.GET("/center", findinghandlers.ListCenterHandler)
			findingAPI.GET("/center/scopes", findinghandlers.CenterScopesHandler)
			findingAPI.GET("/center/web-detail", findinghandlers.CenterWebDetailHandler)
		}

		assetCenter := api.Group("/assets")
		assetCenter.Use(middleware.JWTAuthMiddleware())
		{
			assetCenter.GET("/inventory", asset.ListAssetInventoryHandler)
			assetCenter.GET("/inventory/detail", asset.GetAssetInventoryDetailHandler)
			assetCenter.GET("/ledger", asset.ListAssetLedgerHandler)
			assetCenter.GET("/ledger/entities", asset.ListAssetLedgerEntitiesHandler)
			assetCenter.GET("/ledger/entities/export", asset.ExportAssetLedgerEntitiesHandler)
			assetCenter.POST("/ledger/entities/export", asset.ExportSelectedAssetLedgerEntitiesHandler)
			assetCenter.POST("/ledger/entities/batch-archive", asset.BatchArchiveAssetLedgerEntitiesHandler)
			assetCenter.POST("/ledger/import", asset.ImportUnifiedAssetsHandler)
			assetCenter.POST("/ledger/assets", asset.CreateLedgerAssetHandler)
			assetCenter.PATCH("/ledger/assets/:id", asset.UpdateLedgerAssetHandler)
			assetCenter.POST("/catalog/observations", asset.CreateUnifiedAssetObservationsHandler)
			assetCenter.POST("/catalog/sync-legacy", asset.SyncLegacyUnifiedAssetsHandler)
			assetCenter.GET("/catalog", asset.ListUnifiedAssetsHandler)
			assetCenter.GET("/catalog/:id", asset.GetUnifiedAssetHandler)
			assetCenter.GET("/catalog/:id/relations", asset.ListUnifiedAssetRelationsHandler)
			assetCenter.POST("/catalog/reconcile", asset.ReconcileUnifiedAssetLifecycleHandler)
			assetCenter.GET("/map/overview", asset.GetAssetMapOverviewHandler)
			assetCenter.GET("/map/assets", asset.ListAssetMapAssetsHandler)
			assetCenter.GET("/findings", asset.ListAssetFindingsHandler)
			assetCenter.GET("/topology/entries", asset.ListAssetTopologyEntriesHandler)
			assetCenter.GET("/topology/table", asset.ListAssetTopologyTableHandler)
			assetCenter.GET("/topology", asset.GetAssetTopologyHandler)
		}
		repeater := api.Group("/repeater")
		repeater.Use(middleware.JWTAuthMiddleware())
		{
			repeater.POST("/send", toolbox.SendRequestHandler) // 发送HTTP请求
			repeater.POST("/nuclei-template/polish", toolbox.PolishNucleiTemplateHandler)
			repeater.POST("/nuclei-template/debug", toolbox.DebugNucleiTemplateHandler)
			repeater.GET("/tabs", toolbox.GetRepeaterTabsHandler)         // 获取所有标签页
			repeater.POST("/tabs", toolbox.SaveAllRepeaterTabsHandler)    // 批量保存标签页
			repeater.PUT("/tab/:id", toolbox.UpdateRepeaterTabHandler)    // 更新标签页
			repeater.DELETE("/tab/:id", toolbox.DeleteRepeaterTabHandler) // 删除标签页
		}
		pocDebug := api.Group("/poc-debug")
		pocDebug.Use(middleware.JWTAuthMiddleware())
		{
			pocDebug.GET("/tabs", toolbox.ListPoCDebugTabsHandler)
			pocDebug.POST("/tabs", toolbox.SavePoCDebugTabsHandler)
		}
		spaceGroup := api.Group("/space")
		spaceGroup.Use(middleware.JWTAuthMiddleware())
		{
			spaceGroup.POST("/search", space.SpaceSearchHandler)                // 空间引擎聚合搜索（传统模式）
			spaceGroup.POST("/session/search", space.SpaceSessionSearchHandler) // 创建搜索会话并获取初始数据
			spaceGroup.POST("/session/more", space.SpaceFetchMoreHandler)       // 获取更多数据
			spaceGroup.POST("/session/page", space.SpaceGetPageHandler)         // 获取指定页数据（从缓存）
			spaceGroup.POST("/session/export", space.SpaceExportHandler)        // 导出数据
			spaceGroup.GET("/history", space.ListSpaceSearchHistoryHandler)     // 聚合搜索历史
			spaceGroup.GET("/history/:id", space.GetSpaceSearchHistoryHandler)
			spaceGroup.POST("/history/group", space.AssignSpaceSearchHistoryGroupHandler)
			spaceGroup.PUT("/history/group/:groupId", space.RenameSpaceSearchHistoryGroupHandler)
			spaceGroup.DELETE("/history/:id", space.DeleteSpaceSearchHistoryHandler)
			spaceGroup.GET("/export/tasks", space.ListSpaceExportTasksHandler) // 下载中心任务列表
			spaceGroup.GET("/export/tasks/:id", space.GetSpaceExportTaskHandler)
			spaceGroup.GET("/export/tasks/:id/download", space.DownloadSpaceExportTaskHandler)
			spaceGroup.DELETE("/export/tasks/:id", space.DeleteSpaceExportTaskHandler)
			spaceGroup.DELETE("/session", space.SpaceCloseSessionHandler) // 关闭会话
			spaceGroup.GET("/quota", space.SpaceQuotaHandler)             // 获取各引擎配额信息
		}
		jsscan := api.Group("/jsscan")
		jsscan.Use(middleware.JWTAuthMiddleware())
		{
			// 支持通过任务ID访问（兼容 VulnScanTask 创建的 JS 扫描任务）
			jsscan.GET("/vulnerabilities/:id", scan.GetJSScanVulnerabilitiesHandler)             // 漏洞列表（支持分页/筛选）
			jsscan.GET("/vulnerabilities/:id/:vulnId", scan.GetJSScanVulnerabilityDetailHandler) // 单条漏洞详情
			jsscan.PUT("/vulnerabilities/:id/:vulnId/status", scan.UpdateJSScanVulnerabilityDisposalStatusHandler)
			jsscan.DELETE("/vulnerabilities/:id/:vulnId", scan.DeleteJSScanVulnerabilityHandler)
			jsscan.GET("/assets/:id", scan.GetJSScanAssetsHandler)                   // 资产列表（支持分页/筛选）
			jsscan.GET("/interfaces/:id", scan.GetJSScanInterfacesHandler)           // OpenAPI 接口列表
			jsscan.GET("/sitetree/:id", scan.GetJSScanSiteTreeHandler)               // 网站树列表（支持分页/按需加载）
			jsscan.GET("/sitetree/:id/:treeId", scan.GetJSScanSiteTreeDetailHandler) // 单个网站树详情
			jsscan.GET("/default-config", scan.GetJSScanDefaultConfigHandler)        // 获取系统默认配置
		}
		dashboardGroup := api.Group("/dashboard")
		dashboardGroup.Use(middleware.JWTAuthMiddleware())
		{
			dashboardGroup.GET("/tasks", dashboard.GetDashboardTasksHandler)                  // 获取 Dashboard 任务统计数据
			dashboardGroup.GET("/asset-stats", dashboard.GetAssetStatsHandler)                // 获取资产统计数据（IP、端口、网站、子域名）
			dashboardGroup.GET("/vuln-disposal-stats", dashboard.GetVulnDisposalStatsHandler) // 获取漏洞处置状态统计数据
			dashboardGroup.GET("/token-usage", dashboard.GetDashboardTokenUsageHandler)       // 获取 AI Token 用量汇总与趋势
		}
	}

	// 兼容 Poc-Manager 的 API 路径（使用 /api/ 前缀，避免与 /api/v1/ 冲突）
	// 这些接口也需要 JWT 认证
	fingerprint := r.Group("/api")
	fingerprint.Use(middleware.JWTAuthMiddleware())
	{
		fingerprint.POST("/addFingerprintGroup", asset.AddFingerprintGroupHandler)
		fingerprint.POST("/deleteFingerprintGroup", asset.DeleteFingerprintGroupHandler)
		fingerprint.POST("/addFingerprint", asset.AddFingerprintHandler)
		fingerprint.POST("/deleteFingerprint", asset.DeleteFingerprintHandler)
		fingerprint.POST("/updateFingerprint", asset.UpdateFingerprintHandler)
		fingerprint.GET("/fingerprint/list", asset.GetFingerprintListHandler)
	}

	// 系统性能监控 WebSocket（认证在 handler 内部通过 query token 处理）
	system := api.Group("/system")
	{
		system.GET("/stats/history", middleware.JWTAuthMiddleware(), admin.GetSystemStatsHistoryHandler) // 历史性能采样
		system.GET("/stats/ws", admin.SystemStatsWebSocketHandler)                                       // WebSocket 实时性能数据
	}
}
