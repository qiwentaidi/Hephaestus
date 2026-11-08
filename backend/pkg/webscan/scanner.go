package webscan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"hephaestus-backend/pkg/agentasset"
	configpkg "hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/findings"
	"hephaestus-backend/pkg/taskstate"
	"hephaestus-backend/pkg/types"

	fingerssdk "github.com/qiwentaidi/fingers/lib"

	"hephaestus-backend/pkg/logger"

	arrayutil "github.com/qiwentaidi/utils/array"
	"gorm.io/gorm"
)

func ingestVulnerabilityFinding(db *gorm.DB, taskID string, version int, vuln types.VulnerabilityInfo) {
	if db == nil {
		return
	}
	if _, err := findings.Ingest(db, findings.Input{
		Source:      "webscan",
		SourceID:    vuln.ID + "|" + vuln.URL,
		TaskID:      taskID,
		TaskVersion: version,
		TemplateID:  vuln.ID,
		Title:       vuln.Name,
		Target:      vuln.URL,
		Severity:    vuln.Severity,
		Status:      "open",
		Protocol:    vuln.Type,
		Evidence:    vuln.Extract,
		ObservedAt:  time.Now().UTC(),
		Identity:    vuln.ID,
	}); err != nil {
		logger.Error("写入漏洞归因失败: %v", err)
	}
}

const (
	nucleiTargetBatchSize      = 3
	defaultNucleiThreads       = 2
	defaultNucleiTimeout       = 15
	defaultNucleiRetries       = 3
	defaultNucleiMaxHostErrors = 30
	// FIX(BUG 10): 指纹阶段 checkpoint 落库最小间隔，降低 SQLite 单写锁下的热点写
	fingerprintCheckpointInterval = 2 * time.Second
)

func extractFingerprintViews(matches []fingerssdk.FingerprintMatch) ([]string, []string, []types.VulnFingerprint) {
	normalSet := make(map[string]struct{})
	highRiskSet := make(map[string]struct{})
	vulnByName := make(map[string]types.VulnFingerprint)

	for _, match := range matches {
		name := strings.TrimSpace(match.Name)
		if name == "" {
			continue
		}

		normalSet[name] = struct{}{}
		if match.HighRisk {
			highRiskSet[name] = struct{}{}
		}
		if match.Vuln {
			existing, ok := vulnByName[name]
			// FIX(BUG 6): 同名漏洞指纹多次命中时做字段级合并，
			// 后续命中不得用空值覆盖已捕获的 Description / MatchedRule
			if !ok {
				vulnByName[name] = types.VulnFingerprint{
					Name:        name,
					Description: match.Description,
					MatchedRule: match.MatchedRule,
				}
				continue
			}
			if strings.TrimSpace(existing.MatchedRule) == "" && strings.TrimSpace(match.MatchedRule) != "" {
				existing.MatchedRule = match.MatchedRule
			}
			if strings.TrimSpace(existing.Description) == "" && strings.TrimSpace(match.Description) != "" {
				existing.Description = match.Description
			}
			vulnByName[name] = existing
		}
	}

	normal := make([]string, 0, len(normalSet))
	for name := range normalSet {
		normal = append(normal, name)
	}

	highRisk := make([]string, 0, len(highRiskSet))
	for name := range highRiskSet {
		highRisk = append(highRisk, name)
	}

	vuln := make([]types.VulnFingerprint, 0, len(vulnByName))
	for _, item := range vulnByName {
		vuln = append(vuln, item)
	}

	return normal, highRisk, vuln
}

// ExecuteLocalScanWithContext 执行本地扫描（带 context 取消支持）
func ExecuteLocalScanWithContext(ctx context.Context, taskID string, targets []string, config map[string]interface{}) error {
	// 检查 context 是否已被取消
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// ⚠️ 记录扫描开始时的内存统计信息
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	logger.Info("扫描任务开始 - 初始内存: Alloc=%vMB, Sys=%vMB, NumGC=%v",
		m.Alloc/1024/1024, m.Sys/1024/1024, m.NumGC)

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	// 获取任务的当前版本号（获取最新版本）
	var task database.VulnScanTask
	if err := db.Where("id = ?", taskID).Order("version DESC").First(&task).Error; err != nil {
		return fmt.Errorf("获取任务信息失败: %w", err)
	}
	currentVersion := task.Version
	taskName := task.TaskName // 缓存任务名称，避免每次发现漏洞都查询数据库
	isCombinationTask := parseCombinationTaskFlag(config)
	isAgentItemTask := parseAgentItemTaskFlag(config)
	checkpoint := taskstate.ScanCheckpoint(task, "webscan", isCombinationTask)
	stage := checkpoint.Stage

	// 解析配置为 WebscanOptions
	options := parseConfigToWebscanOptions(config, targets)
	if options.Screenshot {
		warnIfLinuxSnapChromeScreenshot()
	}

	// FIX(BUG 4): scanCtx 提前到引擎创建之前，引擎改用 scanCtx 构造。
	// 此前引擎持有外层 ctx，暂停时 scanCancel() 只能停止 Scan 循环，
	// 引擎内部（截图/资产存储等）以构造时 ctx 启动的 goroutine 在暂停期间仍继续运行。
	scanCtx, scanCancel := context.WithCancel(ctx)
	defer scanCancel()

	var engine *fingerssdk.FingerEngine
	// 阶段事件追踪：拦截 SDK 日志识别"存活探测→目录探测→漏洞指纹匹配"阶段切换。
	// SDK 前置阶段不触发指纹回调，此前期间任务进度完全静止（卡在初始值），
	// 现在由阶段事件驱动进度下限推进，并把事件时间线写入 checkpoint 供前端悬停展示。
	stageTracker := newStageTracker()
	if len(options.Target) > 0 {
		sdkOptions := []fingerssdk.FingersSDKOptions{
			fingerssdk.WithTargets(options.Target...),
			BuiltinFingerprintOption(),
			fingerssdk.WithThread(options.Thread),
			fingerssdk.WithCustomHeaders(options.CustomHeaders),
			fingerssdk.WithHeaders(options.Headers...),
			fingerssdk.WithProxy(options.Proxy),
			fingerssdk.WithDeepScan(options.DeepScan),
			fingerssdk.WithRootPath(options.RootPath),
			fingerssdk.WithActiveTimeoutLimit(options.ActiveTimeoutLimit),
			fingerssdk.WithScreenshot(options.Screenshot),
			fingerssdk.WithAssetTagProbe(true),
			fingerssdk.WithAssetStorage(fingerssdk.AssetStorageConfig{
				Mode: fingerssdk.StorageModeLocal,
				Local: &fingerssdk.LocalStorageConfig{
					BaseDir: configpkg.GetDataDir(),
				},
			}),
			fingerssdk.WithLogOutput(&stageLogWriter{
				tracker: stageTracker,
				sink:    logger.NewSourceWriter("FINGERS", logger.INFO),
			}),
			fingerssdk.WithDefaultOutput(true),
		}

		engine, err = fingerssdk.NewFingersEngineCtx(scanCtx, sdkOptions...)
		if err != nil {
			return fmt.Errorf("初始化扫描引擎失败: %w", err)
		}
	}

	var pauseRequested atomic.Bool
	var fingerprintProcessed atomic.Int64
	fingerprintTotal := len(options.Target) // FIX(BUG 7): 指纹阶段实时进度基数
	var fingerprintMu sync.Mutex            // FIX(BUG 3): 串行化并发回调的结果落库，避免 First+Create 竞态产生重复行
	var lastFingerprintCheckpointAt time.Time
	saveCheckpoint := taskstate.SaveCheckpoint
	checkpointAndPause := taskstate.CheckpointAndPause
	if isCombinationTask {
		saveCheckpoint, checkpointAndPause = taskstate.CombinationCheckpointHandlers("webscan")
	}

	// 指纹扫描结果回调
	fingerprintCallback := func(result fingerssdk.Result) {
		// 检查 context 是否被取消
		select {
		case <-ctx.Done():
			return
		default:
		}

		// FIX(BUG 3): SDK 可能并发回调，结果落库全程持锁串行化，
		// 避免 saveFingerprintResult 的 First+Create 模式在并发下产生重复行
		fingerprintMu.Lock()
		defer fingerprintMu.Unlock()

		normalFingerprints, highRiskFingerprints, vulnFingerprints := extractFingerprintViews(result.Fingerprints)
		infoResult := types.InfoResult{
			URL:                  result.URL,
			Scheme:               result.Scheme,
			Host:                 result.Host,
			Port:                 result.Port,
			StatusCode:           result.StatusCode,
			Length:               result.Length,
			Title:                result.Title,
			Fingerprints:         normalFingerprints,
			HighRiskFingerprints: highRiskFingerprints,
			VulnFingerprints:     vulnFingerprints,
			Detect:               result.Detect,
			Screenshot:           result.Screenshot,
			Favicon:              result.Favicon,
			AssetTags: types.Tag{
				ProductName: result.AssetTags.ProductName,
				AssetType:   result.AssetTags.AssetType,
				Source:      result.AssetTags.Source,
			},
		}

		// 保存指纹结果到数据库
		saveFingerprintResult(db, taskID, currentVersion, infoResult)
		processed := int(fingerprintProcessed.Add(1))

		// FIX(BUG 2): 漏洞指纹入库前移到暂停/取消检查之前。
		// 此前若 checkpointAndPause 命中暂停则直接 return，导致该目标的漏洞指纹
		// 永久丢失（FingerprintInfo 表不存 VulnFingerprints，唯一存储位置是漏洞表）。
		if len(infoResult.VulnFingerprints) > 0 {
			for _, vulnFinger := range infoResult.VulnFingerprints {
				vuln := types.VulnerabilityInfo{
					ID:          vulnFinger.Name,        // 使用指纹名称作为 ID
					Name:        vulnFinger.Name,        // 漏洞名称
					Description: vulnFinger.Description, // 漏洞描述
					URL:         infoResult.URL,         // 目标 URL
					Type:        "FINGERPRINT",          // 类型标记为指纹识别
					Severity:    "LOW",                  // FIX(BUG 5): 默认低危，待人工确认（原注释误写为"中危"）
					TaskId:      taskID,
				}
				// 保存到漏洞表（复用现有函数）
				database.SaveVulnerabilityResult(db, taskID, currentVersion, taskName, vuln)
				ingestVulnerabilityFinding(db, taskID, currentVersion, vuln)
			}
		}

		if !isAgentItemTask {
			// FIX(BUG 7): 进度按已处理目标数在 10%~49% 区间推进（阶段完成时统一写 50），不再固定显示 10%
			progress := 10
			if fingerprintTotal > 0 {
				progress = 10 + int(float64(processed)/float64(fingerprintTotal)*40)
				if progress > 49 {
					progress = 49
				}
				if progress < 10 {
					progress = 10
				}
			}

			// FIX(BUG 10): checkpoint 落库按时间节流（每 2 秒最多一次），
			// 暂停/取消状态检查仍然每次回调执行，保证控制响应性不受节流影响
			if time.Since(lastFingerprintCheckpointAt) < fingerprintCheckpointInterval {
				if err := taskstate.CheckTaskControlState(scanCtx, db, taskID, currentVersion); err != nil && errors.Is(err, taskstate.ErrTaskPaused) {
					pauseRequested.Store(true)
					scanCancel()
				}
				return
			}
			lastFingerprintCheckpointAt = time.Now()

			if err := checkpointAndPause(scanCtx, db, taskID, currentVersion, "webscan:fingerprint", progress, map[string]interface{}{
				"processed":   processed,
				"stageEvents": stageTracker.Events(),
			}); err != nil {
				if errors.Is(err, taskstate.ErrTaskPaused) {
					pauseRequested.Store(true)
					scanCancel()
				}
				return
			}
		}
	}

	fingerprintDone := stage == "webscan:fingerprint_done" || stage == "webscan:nuclei" || stage == "webscan:nuclei_done" || stage == "completed"
	if len(options.Target) == 0 && !fingerprintDone {
		if !isAgentItemTask {
			if err := saveCheckpoint(db, taskID, currentVersion, "webscan:fingerprint_done", 50, map[string]interface{}{
				"processed": 0,
			}); err != nil {
				return err
			}
		}
		fingerprintDone = true
		if !isCombinationTask {
			db.Model(&database.VulnScanTask{}).Where("id = ? AND version = ?", taskID, currentVersion).Update("progress", 50)
		}
	}
	if !fingerprintDone {
		// 检查 context
		select {
		case <-scanCtx.Done():
			if pauseRequested.Load() {
				return taskstate.ErrTaskPaused
			}
			return scanCtx.Err()
		default:
		}

		// FIX(BUG 8): 恢复场景优化——若非深扫且所有初始目标均已存在指纹结果
		// （崩溃/暂停发生在 fingerprint_done 写入前），跳过重复指纹扫描。
		// 部分完成时 SDK 不支持断点续扫，由 saveFingerprintResult 的幂等 upsert 兜底全量重扫。
		if stage == "webscan:fingerprint" && engine != nil && !options.DeepScan &&
			allTargetsHaveFingerprint(db, taskID, currentVersion, options.Target) {
			logger.Info("恢复模式：所有目标均已存在指纹结果，跳过指纹扫描")
		} else if engine != nil {
			logger.Info("开始指纹扫描...")
			// 阶段事件刷新协程：SDK 前置阶段（存活探测/目录探测/漏洞指纹匹配）
			// 不触发指纹回调，由该协程按事件变更把进度下限与阶段时间线写入 checkpoint，
			// 保证进度条在前置阶段也能推进。指纹回调接管进度（≥10%）后自动让位。
			stageFlushStop := make(chan struct{})
			stageFlushDone := make(chan struct{})
			go func() {
				defer close(stageFlushDone)
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				lastSerial := uint64(0)
				for {
					select {
					case <-stageFlushStop:
						return
					case <-scanCtx.Done():
						return
					case <-ticker.C:
						serial, floor := stageTracker.Status()
						if serial == lastSerial || floor >= 10 {
							// 无新事件，或指纹回调已接管进度（≥10%），不再回写
							continue
						}
						lastSerial = serial
						if !isAgentItemTask {
							if isCombinationTask {
								if err := saveCheckpoint(db, taskID, currentVersion, "webscan:probe", floor, map[string]interface{}{"stageEvents": stageTracker.Events()}); err != nil {
									logger.Warning("更新组合任务指纹阶段失败: task=%s version=%d err=%v", taskID, currentVersion, err)
								}
								continue
							}
							// 条件更新（progress < floor）防止与指纹回调竞态时把已推进的进度回退；
							// 组合任务不更新 progress 字段（由组合调度器统一管理）
							updates := map[string]interface{}{
								"current_stage":   "webscan:probe",
								"checkpoint_json": taskstate.MarshalCheckpoint("webscan:probe", floor, map[string]interface{}{"stageEvents": stageTracker.Events()}),
							}
							if !isCombinationTask {
								updates["progress"] = floor
							}
							if err := db.Model(&database.VulnScanTask{}).
								Where("id = ? AND version = ? AND progress < ?", taskID, currentVersion, floor).
								Updates(updates).Error; err != nil {
								logger.Warning("更新扫描前置阶段进度失败: task=%s version=%d err=%v", taskID, currentVersion, err)
							}
						}
					}
				}
			}()
			if err := engine.Scan(scanCtx, fingerprintCallback); err != nil && !errors.Is(err, context.Canceled) {
				close(stageFlushStop)
				<-stageFlushDone
				return err
			}
			close(stageFlushStop)
			<-stageFlushDone
		}

		// FIX(BUG 1): Scan 返回后必须复查 context。
		// 用户取消时 SDK 返回 context.Canceled，此前 `err != context.Canceled` 分支
		// 会带着已取消的任务继续执行：写 fingerprint_done、把进度置为 100%、
		// stage 标记为 completed，最终把 cancelled 任务误报为"扫描完成"并发送通知。
		select {
		case <-scanCtx.Done():
			if pauseRequested.Load() {
				return taskstate.ErrTaskPaused
			}
			return scanCtx.Err()
		default:
		}

		if pauseRequested.Load() {
			return taskstate.ErrTaskPaused
		}
		if !isAgentItemTask {
			if err := saveCheckpoint(db, taskID, currentVersion, "webscan:fingerprint_done", 50, map[string]interface{}{
				"processed":   int(fingerprintProcessed.Load()),
				"stageEvents": stageTracker.Events(),
			}); err != nil {
				return err
			}
		}
		if !isCombinationTask {
			db.Model(&database.VulnScanTask{}).Where("id = ? AND version = ?", taskID, currentVersion).Update("progress", 50)
		}
	}

	// ⚠️ 指纹扫描完成后，强制 GC 清理内存
	runtime.GC()
	// 强制释放操作系统内存
	debug.FreeOSMemory()
	logger.Debug("指纹扫描完成，已执行 GC 清理")

	// FIX(BUG 1 补充): GC 与阶段切换窗口内同样可能命中取消/暂停，
	// 进入下一阶段（或直接写 completed）前统一复查，防止终态被污染
	select {
	case <-scanCtx.Done():
		if pauseRequested.Load() {
			return taskstate.ErrTaskPaused
		}
		return scanCtx.Err()
	default:
	}

	// 检查 context
	// 如果开启Nuclei扫描，执行漏洞扫描
	if options.CallNuclei && !options.NoPoc {
		if stage == "webscan:nuclei_done" || stage == "completed" {
			if !isCombinationTask {
				db.Model(&database.VulnScanTask{}).Where("id = ? AND version = ?", taskID, currentVersion).Update("progress", 100)
			}
			return nil
		}

		logger.Info("开始漏洞扫描...")
		// 记录漏洞扫描阶段事件（悬停时间线展示：主机存活探测 → 指纹扫描 → 漏洞扫描）
		stageTracker.Record(StageNuclei, "漏洞扫描 · PoC 检测", 60)

		// 准备模板目录，确保全局追加目录和任务级追加目录都进入模板仓库。
		allTemplateFolders := GetPOCTemplateFolders()
		if options.AppendTemplateFolder != "" {
			allTemplateFolders = append(allTemplateFolders, options.AppendTemplateFolder)
		}
		if err := GetTagsList(allTemplateFolders); err != nil {
			return fmt.Errorf("刷新 PoC 模板仓库失败: %w", err)
		}

		// 提取所有目标和标签
		fpm, wafTargets := loadFingerprintScanContext(db, taskID, currentVersion, options.Target)
		if len(fpm) == 0 && engine != nil {
			fpm = engine.URLWithFingerprintMap()
		}
		nucleiOpts := types.NucleiOptions{
			Proxy:               options.Proxy,
			Headers:             parseHeaders(options.CustomHeaders),
			RequireTemplateTags: options.SkipNucleiWithoutTags,
			DisableMaxHostErr:   options.DisableMaxHostErr,
			Threads:             options.NucleiThreads,
			Interactsh: types.InteractshOptions{
				OOBEnabled:      options.OOBEnabled,
				InteractshURL:   options.InteractshURL,
				InteractshToken: options.InteractshToken,
			},
		}
		var wafSkippedTargets []string
		nucleiOpts.Jobs, wafSkippedTargets = buildNucleiJobs(options, fpm, wafTargets)
		for _, target := range wafSkippedTargets {
			logger.Info("[nuclei] %s detected WAF, scan skipped", target)
		}

		if len(nucleiOpts.Jobs) == 0 {
			logger.Warning("没有可扫描的目标，跳过漏洞扫描")
			stageTracker.Record(StageComplete, "漏洞扫描完成", 100)
			if !isAgentItemTask {
				if err := saveCheckpoint(db, taskID, currentVersion, "webscan:nuclei_done", 100, map[string]interface{}{
					"stageEvents": stageTracker.Events(),
				}); err != nil {
					return err
				}
			}
			// 直接设置为 100%
			if !isCombinationTask {
				db.Model(&database.VulnScanTask{}).Where("id = ? AND version = ?", taskID, currentVersion).Update("progress", 100)
			}
		} else {
			// 漏洞扫描结果回调
			var nucleiPause atomic.Bool
			totalNucleiJobs := len(nucleiOpts.Jobs)
			batchSize := nucleiTargetBatchSize
			nextBatchIndex := 0
			completedJobs := 0
			if stage == "webscan:nuclei" {
				if raw, ok := checkpoint.Meta["batchSize"]; ok {
					if parsed := parseCheckpointInt(raw); parsed > 0 {
						batchSize = parsed
					}
				}
				if raw, ok := checkpoint.Meta["nextBatchIndex"]; ok {
					if parsed := parseCheckpointInt(raw); parsed >= 0 {
						nextBatchIndex = parsed
					}
				}
				if raw, ok := checkpoint.Meta["completedJobs"]; ok {
					if parsed := parseCheckpointInt(raw); parsed >= 0 {
						completedJobs = parsed
					}
				}
				if raw, ok := checkpoint.Meta["totalJobs"]; ok {
					if parsed := parseCheckpointInt(raw); parsed > 0 && parsed != totalNucleiJobs {
						logger.Warning("网站扫描漏洞阶段恢复数据与当前目标数不一致，改为从头开始恢复: checkpoint=%d current=%d", parsed, totalNucleiJobs)
						nextBatchIndex = 0
						completedJobs = 0
					}
				}
			}
			if batchSize <= 0 {
				batchSize = nucleiTargetBatchSize
			}
			totalBatches := (totalNucleiJobs + batchSize - 1) / batchSize
			if totalBatches == 0 {
				totalBatches = 1
			}
			if nextBatchIndex < 0 {
				nextBatchIndex = 0
			}
			if nextBatchIndex > totalBatches {
				nextBatchIndex = totalBatches
			}
			if completedJobs < 0 {
				completedJobs = 0
			}
			if completedJobs > totalNucleiJobs {
				completedJobs = totalNucleiJobs
			}
			var nucleiProgressMu sync.Mutex
			var completedJobsCounter atomic.Int32
			completedJobsCounter.Store(int32(completedJobs))

			saveNucleiProgress := func(progress int, nextBatchIndex int, checkpointCompletedJobs int) error {
				if !isCombinationTask {
					if err := db.Model(&database.VulnScanTask{}).
						Where("id = ? AND version = ?", taskID, currentVersion).
						Update("progress", progress).Error; err != nil {
						return err
					}
				}
				if isAgentItemTask {
					return nil
				}
				return saveCheckpoint(db, taskID, currentVersion, "webscan:nuclei", progress, map[string]interface{}{
					"batchSize":      batchSize,
					"nextBatchIndex": nextBatchIndex,
					"completedJobs":  checkpointCompletedJobs,
					"totalJobs":      totalNucleiJobs,
					"stageEvents":    stageTracker.Events(),
				})
			}

			vulnCallback := func(vuln types.VulnerabilityInfo) {
				// 检查 context 是否被取消
				select {
				case <-ctx.Done():
					return
				default:
				}

				// 设置任务ID
				vuln.TaskId = taskID
				// 保存漏洞结果到数据库（传递 taskName 避免重复查询）
				database.SaveVulnerabilityResult(db, taskID, currentVersion, taskName, vuln)
				ingestVulnerabilityFinding(db, taskID, currentVersion, vuln)
				if shouldPauseWebscanNuclei(db, taskID, currentVersion) {
					nucleiPause.Store(true)
					scanCancel()
				}
			}

			// 漏洞扫描进度范围：50% - 100%
			if err := saveNucleiProgress(calculateNucleiProgress(completedJobs, totalNucleiJobs), nextBatchIndex, completedJobs); err != nil {
				return err
			}

			for batchIndex := nextBatchIndex; batchIndex < totalBatches; batchIndex++ {
				start := batchIndex * batchSize
				end := start + batchSize
				if end > totalNucleiJobs {
					end = totalNucleiJobs
				}
				if start >= end {
					continue
				}

				currentBatchJobs := append([]types.NucleiScanJob(nil), nucleiOpts.Jobs[start:end]...)
				batchBaseCompletedJobs := completedJobs
				if err := saveNucleiProgress(calculateNucleiProgress(batchBaseCompletedJobs, totalNucleiJobs), batchIndex, batchBaseCompletedJobs); err != nil {
					return err
				}

				nucleiJobDone := func(url string) {
					completed := int(completedJobsCounter.Add(1))
					progress := calculateNucleiProgress(completed, totalNucleiJobs)

					nucleiProgressMu.Lock()
					if err := saveNucleiProgress(progress, batchIndex, batchBaseCompletedJobs); err != nil {
						logger.Warning("更新网站扫描漏洞阶段进度失败: task=%s version=%d target=%s err=%v", taskID, currentVersion, url, err)
					}
					nucleiProgressMu.Unlock()

					if shouldPauseWebscanNuclei(db, taskID, currentVersion) {
						nucleiPause.Store(true)
						scanCancel()
					}
				}

				batchOpts := nucleiOpts
				batchOpts.Jobs = currentBatchJobs
				batchOpts.LogProgressOffset = batchBaseCompletedJobs
				batchOpts.LogProgressTotal = totalNucleiJobs
				if err := NewThreadSafeNucleiEngine(scanCtx, batchOpts, vulnCallback, nucleiJobDone); err != nil {
					// 风格统一: 使用 errors.Is 匹配，兼容 SDK 用 %w 包装后的错误链
					if errors.Is(err, taskstate.ErrTaskPaused) {
						return err
					}
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						if nucleiPause.Load() || pauseRequested.Load() {
							return taskstate.ErrTaskPaused
						}
						return err
					}
					return err
				}
				if nucleiPause.Load() || pauseRequested.Load() {
					return taskstate.ErrTaskPaused
				}

				completedJobs = int(completedJobsCounter.Load())
				batchProgress := calculateNucleiProgress(completedJobs, totalNucleiJobs)
				if err := saveNucleiProgress(batchProgress, batchIndex+1, completedJobs); err != nil {
					return err
				}
			}
			if !isAgentItemTask {
				stageTracker.Record(StageComplete, "漏洞扫描完成", 100)
				if err := saveCheckpoint(db, taskID, currentVersion, "webscan:nuclei_done", 100, map[string]interface{}{
					"stageEvents": stageTracker.Events(),
				}); err != nil {
					return err
				}
			}
			logger.Info("漏洞扫描完成")
			// 漏洞扫描完成后会在 handler 中设置为 100%

			// ⚠️ 漏洞扫描完成后，强制 GC 清理内存
			runtime.GC()
			debug.FreeOSMemory()
			logger.Debug("漏洞扫描完成，已执行 GC 清理")
		}
	} else {
		// 没有启用漏洞扫描，直接设置为 100%
		if !isAgentItemTask {
			stageTracker.Record(StageComplete, "扫描完成", 100)
			if err := saveCheckpoint(db, taskID, currentVersion, "completed", 100, map[string]interface{}{
				"stageEvents": stageTracker.Events(),
			}); err != nil {
				return err
			}
		}
		if !isCombinationTask {
			db.Model(&database.VulnScanTask{}).Where("id = ? AND version = ?", taskID, currentVersion).Update("progress", 100)
		}
	}

	// ⚠️ 整个扫描任务完成后，执行最终的资源清理
	runtime.GC()
	debug.FreeOSMemory()

	// 记录内存统计信息（用于监控内存泄漏）
	runtime.ReadMemStats(&m)
	logger.Info("扫描任务完成 - 内存统计: Alloc=%vMB, TotalAlloc=%vMB, Sys=%vMB, NumGC=%v",
		m.Alloc/1024/1024, m.TotalAlloc/1024/1024, m.Sys/1024/1024, m.NumGC)

	return nil
}

// ExecuteLocalScan 执行本地扫描（整合指纹扫描和漏洞扫描）- 保留向后兼容
func ExecuteLocalScan(taskID string, targets []string, config map[string]interface{}) error {
	return ExecuteLocalScanWithContext(context.Background(), taskID, targets, config)
}

func parseCombinationTaskFlag(config map[string]interface{}) bool {
	if parseAgentItemTaskFlag(config) {
		return true
	}
	raw, ok := config["isCombination"]
	if !ok {
		return false
	}
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	default:
		return false
	}
}

func parseAgentItemTaskFlag(config map[string]interface{}) bool {
	raw, ok := config["agentItemMode"]
	if !ok {
		return false
	}
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	default:
		return false
	}
}

// parseConfigToWebscanOptions 将配置map转换为WebscanOptions
func parseConfigToWebscanOptions(config map[string]interface{}, targets []string) types.WebscanOptions {
	options := types.WebscanOptions{
		Target:                make([]string, 0, len(targets)),
		TcpTarget:             make(map[string][]string),
		Thread:                30, // 网站扫描默认30
		NucleiThreads:         defaultNucleiThreads,
		Timeout:               10, // 保留Timeout字段用于其他用途，但不在前端配置
		ActiveTimeoutLimit:    5,
		SkipNucleiWithoutTags: true,
		SkipNucleiWithWAF:     true,
		DisableMaxHostErr:     true,
		CallNuclei:            true,
		DeepScan:              false,
		RootPath:              false,
	}

	// 解析配置字段
	if thread, ok := config["threads"].(float64); ok {
		options.Thread = int(thread)
	}
	if nucleiThreads, ok := config["nucleiThreads"].(float64); ok {
		options.NucleiThreads = int(nucleiThreads)
	}
	if portScan, ok := config["portScan"].(bool); ok {
		// 端口扫描相关配置
		_ = portScan
	}
	if fingerprint, ok := config["fingerprint"].(bool); ok {
		// 指纹扫描相关配置
		_ = fingerprint
	}
	// vulnerability 和 deepScan 的处理已移到 webscanOption 解析之后，作为后备方案
	if rootPath, ok := config["rootPath"].(bool); ok {
		options.RootPath = rootPath
	}
	if proxy, ok := config["proxy"].(string); ok {
		options.Proxy = proxy
	}
	if skipNucleiWithoutTags, ok := config["skipNucleiWithoutTags"].(bool); ok {
		options.SkipNucleiWithoutTags = skipNucleiWithoutTags
	}
	if skipNucleiWithWAF, ok := config["skipNucleiWithWAF"].(bool); ok {
		options.SkipNucleiWithWAF = skipNucleiWithWAF
	}
	if customHeaders, ok := config["customHeaders"].(string); ok {
		options.CustomHeaders = customHeaders
	}
	if templateFiles, ok := config["templateFiles"].([]interface{}); ok {
		for _, tf := range templateFiles {
			if str, ok := tf.(string); ok {
				options.TemplateFiles = append(options.TemplateFiles, str)
			}
		}
	}
	if appendTemplateFolder, ok := config["appendTemplateFolder"].(string); ok {
		options.AppendTemplateFolder = strings.TrimSpace(appendTemplateFolder)
	}
	if tags, ok := config["tags"].([]interface{}); ok {
		for _, tag := range tags {
			if str, ok := tag.(string); ok {
				options.Tags = append(options.Tags, str)
			}
		}
	}
	// 解析自定义标签（指定指纹）
	if customTags, ok := config["customTags"].([]interface{}); ok {
		for _, tag := range customTags {
			if str, ok := tag.(string); ok {
				options.Tags = append(options.Tags, str)
			}
		}
	}
	// 解析自定义模板（指定漏洞）
	if customTemplate, ok := config["customTemplate"].([]interface{}); ok {
		for _, tmpl := range customTemplate {
			if str, ok := tmpl.(string); ok {
				options.TemplateFiles = append(options.TemplateFiles, str)
			}
		}
	}
	// 解析网站扫描模式（优先使用 webscanOption，因为它会覆盖 vulnerability 和 deepScan）
	var webscanOptionValue int
	if val, ok := config["webscanOption"].(int); ok {
		webscanOptionValue = val
	} else if val, ok := config["webscanOption"].(float64); ok {
		webscanOptionValue = int(val)
	}
	options.WebscanOption = webscanOptionValue
	if webscanOptionValue > 0 {
		switch webscanOptionValue {
		case 1: // 指纹扫描
			options.DeepScan = false
			options.CallNuclei = false
		case 2: // 全指纹扫描
			options.DeepScan = true
			options.CallNuclei = false
		case 3: // 指纹漏洞扫描
			options.DeepScan = true
			options.CallNuclei = true
		case 4: // 专项扫描
			options.DeepScan = false
			options.CallNuclei = true
		}
	} else {
		// 如果没有 webscanOption，则使用旧的 vulnerability 和 deepScan 配置
		if vulnerability, ok := config["vulnerability"].(bool); ok {
			options.CallNuclei = vulnerability
			options.NoPoc = !vulnerability
		}
		if deepScan, ok := config["deepScan"].(bool); ok {
			options.DeepScan = deepScan
		}
	}
	// 解析根路径扫描
	if rootPathScan, ok := config["rootPathScan"].(bool); ok {
		options.RootPath = rootPathScan
	}
	// 解析跳过无标签Nuclei
	if skipNucleiWithoutTags, ok := config["skipNucleiWithoutTags"].(bool); ok {
		options.SkipNucleiWithoutTags = skipNucleiWithoutTags
	}
	if skipNucleiWithWAF, ok := config["skipNucleiWithWAF"].(bool); ok {
		options.SkipNucleiWithWAF = skipNucleiWithWAF
	}
	// 解析网站截图
	if screenshot, ok := config["screenshot"].(bool); ok {
		options.Screenshot = screenshot
	}
	// 解析主动指纹最多失败次数
	if activeTimeoutLimit, ok := config["activeTimeoutLimit"].(float64); ok {
		options.ActiveTimeoutLimit = int(activeTimeoutLimit)
	}
	if disableMaxHostErr, ok := config["disableMaxHostErr"].(bool); ok {
		options.DisableMaxHostErr = disableMaxHostErr
	}

	// 解析 OOB / Interactsh 配置
	if oobEnabled, ok := config["oobEnabled"].(bool); ok {
		options.OOBEnabled = oobEnabled
	}
	if interactshURL, ok := config["interactshURL"].(string); ok {
		options.InteractshURL = interactshURL
	}
	if interactshURL, ok := config["interactshUrl"].(string); ok {
		options.InteractshURL = interactshURL
	}
	if interactshToken, ok := config["interactshToken"].(string); ok {
		options.InteractshToken = interactshToken
	}

	options.InteractshURL = strings.TrimSpace(options.InteractshURL)
	options.InteractshToken = strings.TrimSpace(options.InteractshToken)

	for _, target := range targets {
		trimmed := strings.TrimSpace(target)
		if trimmed == "" {
			continue
		}

		if isNetworkOrHostOnlyTarget(trimmed) {
			continue
		}

		parsed, err := url.Parse(trimmed)
		if err != nil || parsed.Scheme == "" {
			options.Target = append(options.Target, trimmed)
			continue
		}

		scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
		if scheme == "http" || scheme == "https" {
			options.Target = append(options.Target, trimmed)
			continue
		}

		options.TcpTarget[scheme] = append(options.TcpTarget[scheme], trimmed)
	}

	return options
}

func isNetworkOrHostOnlyTarget(target string) bool {
	target = strings.TrimSpace(target)
	if target == "" || strings.Contains(target, "://") {
		return false
	}
	if _, _, err := net.ParseCIDR(target); err == nil {
		return true
	}
	if host, _, err := net.SplitHostPort(target); err == nil {
		return net.ParseIP(strings.Trim(host, "[]")) != nil
	}
	return net.ParseIP(strings.Trim(target, "[]")) != nil
}

func buildNucleiJobs(options types.WebscanOptions, fingerprintTagMap map[string][]string, wafTargets map[string]types.Tag) ([]types.NucleiScanJob, []string) {
	jobs := make([]types.NucleiScanJob, 0, len(fingerprintTagMap))
	skippedTargets := make([]string, 0)

	customTags := normalizeNucleiTags(options.Tags)
	useCustomTags := options.WebscanOption == 4 && len(customTags) > 0

	webTargets := make([]string, 0, len(fingerprintTagMap)+len(options.Target))
	if useCustomTags {
		webTargets = append(webTargets, options.Target...)
		if len(webTargets) == 0 {
			for target := range fingerprintTagMap {
				webTargets = append(webTargets, target)
			}
		}
	} else {
		for target := range fingerprintTagMap {
			webTargets = append(webTargets, target)
		}
	}
	webTargets = arrayutil.RemoveDuplicates(webTargets)
	sort.Strings(webTargets)

	for _, target := range webTargets {
		if shouldSkipNucleiForWAFTarget(options) && isWAFAssetTag(wafTargets[target]) {
			skippedTargets = append(skippedTargets, target)
			continue
		}
		tags := fingerprintTagMap[target]
		if useCustomTags {
			tags = customTags
		}
		jobs = append(jobs, types.NucleiScanJob{
			URL:      target,
			Template: options.TemplateFiles,
			Tags:     arrayutil.RemoveDuplicates(tags),
		})
	}

	protocols := make([]string, 0, len(options.TcpTarget))
	for protocol := range options.TcpTarget {
		protocols = append(protocols, protocol)
	}
	sort.Strings(protocols)

	for _, protocol := range protocols {
		targets := append([]string(nil), options.TcpTarget[protocol]...)
		sort.Strings(targets)
		for _, target := range targets {
			jobs = append(jobs, types.NucleiScanJob{
				URL:      target,
				Template: options.TemplateFiles,
				Tags:     []string{protocol},
			})
		}
	}

	return jobs, skippedTargets
}

func normalizeNucleiTags(tags []string) []string {
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		normalized = append(normalized, tag)
	}
	return arrayutil.RemoveDuplicates(normalized)
}

func buildRootTargetFromFingerprint(rawURL, scheme, host string, port int) string {
	host = strings.TrimSpace(host)
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if host != "" && (scheme == "http" || scheme == "https") {
		if port > 0 {
			return fmt.Sprintf("%s://%s:%d", scheme, host, port)
		}
		return fmt.Sprintf("%s://%s", scheme, host)
	}

	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimSpace(rawURL)
	}

	if parsed.Port() != "" {
		return fmt.Sprintf("%s://%s", strings.ToLower(parsed.Scheme), parsed.Host)
	}
	return fmt.Sprintf("%s://%s", strings.ToLower(parsed.Scheme), parsed.Hostname())
}

type userTargetMatcher struct {
	bySite map[string][]string
}

func siteKey(scheme, host string, port int) string {
	return fmt.Sprintf("%s|%s|%d", strings.ToLower(strings.TrimSpace(scheme)), strings.ToLower(strings.TrimSpace(host)), port)
}

func parseTargetSiteAndPath(raw string) (string, string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", false
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	port := 0
	if parsed.Port() != "" {
		if p, convErr := strconv.Atoi(parsed.Port()); convErr == nil {
			port = p
		}
	}
	if port == 0 {
		if scheme == "https" {
			port = 443
		} else {
			port = 80
		}
	}
	path := strings.TrimSpace(parsed.EscapedPath())
	if path == "" {
		path = "/"
	}
	return siteKey(scheme, host, port), path, true
}

func normalizePathForPrefix(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}

func buildUserTargetMatcher(inputTargets []string) *userTargetMatcher {
	matcher := &userTargetMatcher{bySite: make(map[string][]string)}
	seen := make(map[string]struct{})
	for _, target := range inputTargets {
		site, _, ok := parseTargetSiteAndPath(target)
		if !ok {
			continue
		}
		normalized := strings.TrimSpace(target)
		key := site + "|" + normalized
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		matcher.bySite[site] = append(matcher.bySite[site], normalized)
	}
	for site := range matcher.bySite {
		candidates := matcher.bySite[site]
		sort.SliceStable(candidates, func(i, j int) bool {
			_, pi, _ := parseTargetSiteAndPath(candidates[i])
			_, pj, _ := parseTargetSiteAndPath(candidates[j])
			return len(normalizePathForPrefix(pi)) > len(normalizePathForPrefix(pj))
		})
		matcher.bySite[site] = candidates
	}
	return matcher
}

func (m *userTargetMatcher) selectMergeTarget(row database.FingerprintInfo) string {
	rowSite, rowPath, ok := parseTargetSiteAndPath(row.URL)
	if !ok {
		return buildRootTargetFromFingerprint(row.URL, row.Scheme, row.Host, row.Port)
	}

	candidates := m.bySite[rowSite]
	if len(candidates) == 0 {
		return buildRootTargetFromFingerprint(row.URL, row.Scheme, row.Host, row.Port)
	}

	normalizedRowPath := normalizePathForPrefix(rowPath)
	for _, candidate := range candidates {
		_, candidatePath, valid := parseTargetSiteAndPath(candidate)
		if !valid {
			continue
		}
		normalizedCandidatePath := normalizePathForPrefix(candidatePath)
		if normalizedCandidatePath == "/" {
			return candidate
		}
		if normalizedRowPath == normalizedCandidatePath || strings.HasPrefix(normalizedRowPath, normalizedCandidatePath+"/") {
			return candidate
		}
	}
	return buildRootTargetFromFingerprint(row.URL, row.Scheme, row.Host, row.Port)
}

// parseHeaders 解析自定义请求头字符串为切片
func parseHeaders(headersStr string) []string {
	if headersStr == "" {
		return nil
	}
	// 按行分割
	lines := strings.Split(headersStr, "\n")
	var headers []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			headers = append(headers, line)
		}
	}
	return headers
}

// saveFingerprintResult 保存指纹结果到数据库
func saveFingerprintResult(db *gorm.DB, taskID string, version int, result types.InfoResult) {
	// 将 Fingerprints 切片转换为 JSON 字符串
	fingerprintsJSON, _ := json.Marshal(result.Fingerprints)
	// 将 HighRiskFingerprints 切片转换为 JSON 字符串
	highRiskFingerprintsJSON, _ := json.Marshal(result.HighRiskFingerprints)
	// 将 AssetTags 切片转换为 JSON 字符串
	assetTagsJSON, _ := json.Marshal(result.AssetTags)

	fingerprint := database.FingerprintInfo{
		TaskID:               taskID,
		Version:              version,
		URL:                  result.URL,
		Status:               result.StatusCode,
		Length:               result.Length,
		Title:                result.Title,
		Detect:               result.Detect,
		Fingerprints:         string(fingerprintsJSON),
		HighRiskFingerprints: string(highRiskFingerprintsJSON),
		AssetTags:            string(assetTagsJSON),
		Screenshot:           agentasset.NormalizeScanAssetFilename(result.Screenshot),
		Favicon:              agentasset.NormalizeScanAssetFilename(result.Favicon),
		IconHash:             FaviconIconHash(result.Favicon),
		Host:                 result.Host,
		Scheme:               result.Scheme,
		Port:                 result.Port,
	}

	var existing database.FingerprintInfo
	err := db.Where("task_id = ? AND version = ? AND url = ?", taskID, version, fingerprint.URL).First(&existing).Error
	switch err {
	case nil:
		if err := db.Model(&database.FingerprintInfo{}).
			Where("task_id = ? AND version = ? AND url = ?", taskID, version, fingerprint.URL).
			Updates(map[string]interface{}{
				"status":                 fingerprint.Status,
				"length":                 fingerprint.Length,
				"title":                  fingerprint.Title,
				"detect":                 fingerprint.Detect,
				"fingerprints":           fingerprint.Fingerprints,
				"high_risk_fingerprints": fingerprint.HighRiskFingerprints,
				"asset_tags":             fingerprint.AssetTags,
				"screenshot":             fingerprint.Screenshot,
				"favicon":                fingerprint.Favicon,
				"icon_hash":              fingerprint.IconHash,
				"host":                   fingerprint.Host,
				"scheme":                 fingerprint.Scheme,
				"port":                   fingerprint.Port,
			}).Error; err != nil {
			logger.Error("更新指纹结果失败: %v", err)
		}
	case gorm.ErrRecordNotFound:
		if err := db.Create(&fingerprint).Error; err != nil {
			logger.Error("保存指纹结果失败: %v", err)
			return
		}
	default:
		logger.Error("查询指纹结果失败: %v", err)
		return
	}
}

func loadFingerprintScanContext(db *gorm.DB, taskID string, version int, inputTargets []string) (map[string][]string, map[string]types.Tag) {
	var rows []database.FingerprintInfo
	if err := db.Where("task_id = ? AND version = ?", taskID, version).Find(&rows).Error; err != nil {
		logger.Warning("加载已保存指纹结果失败: %v", err)
		return map[string][]string{}, map[string]types.Tag{}
	}

	result := make(map[string][]string, len(rows))
	wafTargets := make(map[string]types.Tag, len(rows))
	targetMatcher := buildUserTargetMatcher(inputTargets)
	for _, row := range rows {
		if strings.TrimSpace(row.URL) == "" {
			continue
		}
		var tags []string
		if err := json.Unmarshal([]byte(row.Fingerprints), &tags); err != nil {
			continue
		}
		rootTarget := targetMatcher.selectMergeTarget(row)
		if strings.TrimSpace(rootTarget) == "" {
			rootTarget = strings.TrimSpace(row.URL)
		}
		if len(tags) > 0 {
			result[rootTarget] = arrayutil.RemoveDuplicates(append(result[rootTarget], tags...))
		}

		var assetTag types.Tag
		if err := json.Unmarshal([]byte(row.AssetTags), &assetTag); err == nil && isWAFAssetTag(assetTag) {
			wafTargets[rootTarget] = assetTag
		}
	}
	return result, wafTargets
}

func isWAFAssetTag(tag types.Tag) bool {
	return strings.EqualFold(strings.TrimSpace(tag.AssetType), "waf")
}

// FIX(BUG 8): scanURLIdentity 将目标/结果 URL 归一化为 host+path 标识，
// 用于恢复模式下比对"初始目标是否均已有指纹结果"。
// 支持裸域名（补默认 scheme）、忽略大小写与尾部斜杠差异。
func scanURLIdentity(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimRight(s, "/")
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return strings.ToLower(s)
	}
	return strings.ToLower(u.Host) + strings.ToLower(strings.TrimRight(u.Path, "/"))
}

// FIX(BUG 8): allTargetsHaveFingerprint 判断所有初始目标是否都已存在指纹结果。
// 仅在非深扫的恢复场景使用：深扫可能产出初始目标之外的新 URL，跳过会漏扫。
func allTargetsHaveFingerprint(db *gorm.DB, taskID string, version int, targets []string) bool {
	if len(targets) == 0 {
		return false
	}
	var rows []database.FingerprintInfo
	if err := db.Select("url").Where("task_id = ? AND version = ?", taskID, version).Find(&rows).Error; err != nil {
		return false
	}
	saved := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if id := scanURLIdentity(row.URL); id != "" {
			saved[id] = struct{}{}
		}
	}
	for _, target := range targets {
		id := scanURLIdentity(target)
		if id == "" {
			return false
		}
		if _, ok := saved[id]; !ok {
			return false
		}
	}
	return true
}

func shouldSkipNucleiForWAFTarget(options types.WebscanOptions) bool {
	return options.SkipNucleiWithWAF && options.WebscanOption != 4
}

func parseCheckpointInt(raw interface{}) int {
	switch value := raw.(type) {
	case float64:
		return int(value)
	case float32:
		return int(value)
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	default:
		return 0
	}
}

func calculateNucleiProgress(completedJobs, totalJobs int) int {
	if totalJobs <= 0 {
		return 60
	}
	if completedJobs <= 0 {
		return 60
	}
	if completedJobs >= totalJobs {
		return 99
	}
	return 60 + int(float64(completedJobs)/float64(totalJobs)*39)
}

func shouldPauseWebscanNuclei(db *gorm.DB, taskID string, version int) bool {
	var control struct {
		Status         string
		PauseRequested bool
	}
	if err := db.Model(&database.VulnScanTask{}).
		Select("status", "pause_requested").
		Where("id = ? AND version = ?", taskID, version).
		First(&control).Error; err != nil {
		return false
	}
	return control.Status == taskstate.StatusCancelled || control.Status == taskstate.StatusDeleting || control.PauseRequested
}
