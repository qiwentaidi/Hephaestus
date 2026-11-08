package dirsearch

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/taskstate"

	arrayutil "github.com/qiwentaidi/utils/array"
	"gorm.io/gorm"
)

var dirResultSaveMu sync.Mutex

// ExecuteDirScanWithContext 执行目录扫描（用于 VulnScanTask）
// ctx: 上下文，用于取消扫描
// taskID: 任务ID
// targets: 目标URL列表
// config: 配置参数（从前端传来的 JSON config）
func ExecuteDirScanWithContext(
	ctx context.Context,
	taskID string,
	targets []string,
	config map[string]interface{},
) error {
	logger.Info("开始执行目录扫描任务: %s, 目标数量: %d", taskID, len(targets))

	// 解析配置
	options, err := parseConfig(config)
	if err != nil {
		return fmt.Errorf("解析配置失败: %w", err)
	}

	// 设置目标
	options.URLs = targets

	// 获取数据库连接
	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}
	scanCtx, scanCancel := context.WithCancel(ctx)
	defer scanCancel()

	// 获取任务版本号
	var task database.VulnScanTask
	if err := db.Where("id = ?", taskID).Order("version DESC").First(&task).Error; err != nil {
		return fmt.Errorf("获取任务信息失败: %w", err)
	}
	currentVersion := task.Version
	isCombinationTask := parseCombinationTaskFlag(config)
	isAgentItemTask := parseAgentItemTaskFlag(config)
	checkpoint := taskstate.ScanCheckpoint(task, "dirscan", isCombinationTask)
	saveCheckpoint := taskstate.SaveCheckpoint
	checkpointAndPause := taskstate.CheckpointAndPause
	if isCombinationTask {
		saveCheckpoint, checkpointAndPause = taskstate.CombinationCheckpointHandlers("dirscan")
	}
	resumeProgressFloor := task.Progress
	if isCombinationTask {
		resumeProgressFloor = checkpoint.Progress
	}
	if checkpoint.Progress > resumeProgressFloor {
		resumeProgressFloor = checkpoint.Progress
	}
	resumeTotalPaths := 0
	if rawTotal, ok := checkpoint.Meta["totalPaths"]; ok {
		switch value := rawTotal.(type) {
		case float64:
			if int(value) > resumeTotalPaths {
				resumeTotalPaths = int(value)
			}
		case int:
			if value > resumeTotalPaths {
				resumeTotalPaths = value
			}
		}
	}
	if resumeTotalPaths == 0 && checkpoint.Progress > 0 {
		resumeTotalPaths = task.TotalCount
	}

	// 组合任务由调度器统一维护父任务进度，目录扫描子模块不应覆盖 progress/total_count。
	if !isCombinationTask {
		if err := db.Model(&database.VulnScanTask{}).
			Where("id = ? AND version = ?", taskID, currentVersion).
			Updates(map[string]interface{}{
				"status":      "running",
				"progress":    resumeProgressFloor,
				"total_count": resumeTotalPaths,
				"updated_at":  time.Now(),
			}).Error; err != nil {
			return fmt.Errorf("更新任务状态失败: %w", err)
		}
	}

	// 结果计数器
	resultCount := 0
	errorCount := 0
	totalPaths := resumeTotalPaths
	processedPaths := 0

	// 批量保存结果
	var resultsBatch []Result
	var resultsMutex sync.Mutex

	// 定时保存结果的定时器（每3秒保存一次）
	saveTimer := time.NewTicker(3 * time.Second)
	defer saveTimer.Stop()

	// 启动定时保存的 goroutine
	saveDone := make(chan struct{})
	go func() {
		defer close(saveDone)
		for {
			select {
			case <-scanCtx.Done():
				// 任务取消，退出
				return
			case <-saveTimer.C:
				// 定时保存
				resultsMutex.Lock()
				if len(resultsBatch) > 0 {
					toSave := make([]Result, len(resultsBatch))
					copy(toSave, resultsBatch)
					resultsBatch = resultsBatch[:0] // 清空批次
					resultsMutex.Unlock()

					// 保存结果
					for _, r := range toSave {
						if err := saveResult(taskID, currentVersion, r); err != nil {
							logger.Error("定时保存目录扫描结果失败: %v", err)
							errorCount++
						} else {
							resultCount++
						}
					}
					logger.Debug("[DirScan] 定时保存了 %d 条结果", len(toSave))
				} else {
					resultsMutex.Unlock()
				}
			}
		}
	}()

	// 创建回调函数
	callback := func(event string, data interface{}) {
		// 检查是否已取消
		if scanCtx.Err() != nil {
			return
		}

		switch event {
		case "dirsearchCounts":
			// 路径总数
			total := 0
			switch counts := data.(type) {
			case int:
				total = counts
			case int32:
				total = int(counts)
			case int64:
				total = int(counts)
			case float64:
				total = int(counts)
			case map[string]int:
				total = counts["total"]
			case map[string]interface{}:
				switch raw := counts["total"].(type) {
				case int:
					total = raw
				case int32:
					total = int(raw)
				case int64:
					total = int(raw)
				case float64:
					total = int(raw)
				}
			}
			if total > 0 {
				totalPaths = total

				// 仅普通任务更新 total_count，避免组合任务父进度被覆盖。
				if !isCombinationTask {
					db.Model(&database.VulnScanTask{}).
						Where("id = ? AND version = ?", taskID, currentVersion).
						Updates(map[string]interface{}{
							"total_count": totalPaths,
							"updated_at":  time.Now(),
						})
				}
			}

		case "dirsearchLoading":
			// dirsearchLoading 事件包含有效结果（Status > 1）
			if result, ok := data.(Result); ok {
				// 不在这里计数，而是在实际保存成功时计数

				if result.Status > 1 && result.URL != "" {
					// 保存结果到批次中（批量保存，每20条保存一次，或定时保存）
					resultsMutex.Lock()
					resultsBatch = append(resultsBatch, result)
					shouldSave := len(resultsBatch) >= 20
					resultsToSave := make([]Result, 0)
					if shouldSave {
						resultsToSave = append(resultsToSave, resultsBatch...)
						resultsBatch = resultsBatch[:0]
					}
					resultsMutex.Unlock()

					if shouldSave {
						go func(results []Result) {
							saved := 0
							for _, r := range results {
								if err := saveResult(taskID, currentVersion, r); err != nil {
									logger.Error("批量保存结果失败: %v", err)
								} else {
									saved++
								}
							}
							// 更新计数器
							if saved > 0 {
								// 注意：这里不能直接用 resultCount++，因为在多个 goroutine 中会有竞争
								logger.Debug("[DirScan] 批量保存了 %d 条结果", saved)
							}
						}(resultsToSave)
					}
				}
			}

		case "dirsearchProgress":
			// dirsearchProgress 事件包含所有请求的计数（包括被过滤的）
			if progressData, ok := data.(map[string]interface{}); ok {
				if completedVal, ok := progressData["completed"].(int); ok {
					processedPaths = completedVal
					if errorsVal, ok := progressData["errors"].(int); ok {
						errorCount = errorsVal
					}

					// 计算并更新进度
					progress := 0
					if totalPaths > 0 {
						progress = int((int64(processedPaths) * 100) / int64(totalPaths))
						if progress > 100 {
							progress = 100
						}
					}
					// 仅在任务真正收尾并写入 completed 时才展示 100%，
					// 避免出现“运行中但进度 100%”的假完成状态。
					if progress >= 100 && scanCtx.Err() == nil {
						progress = 99
					}
					if progress < resumeProgressFloor {
						progress = resumeProgressFloor
					}

					// 每300个请求更新一次进度到数据库（避免太频繁）
					if !isCombinationTask && processedPaths > 0 && processedPaths%300 == 0 {
						db.Model(&database.VulnScanTask{}).
							Where("id = ? AND version = ?", taskID, currentVersion).
							Updates(map[string]interface{}{
								"progress":   progress,
								"updated_at": time.Now(),
							})
					}

					if !isAgentItemTask {
						if err := checkpointAndPause(scanCtx, db, taskID, currentVersion, "dirscan:scanning", progress, map[string]interface{}{
							"processedPaths": processedPaths,
							"totalPaths":     totalPaths,
							"errorCount":     errorCount,
						}); err != nil {
							scanCancel()
							return
						}
					}
				}
			}
		}
	}

	// 创建扫描引擎
	engine := NewDirsearchEngine(scanCtx, options, callback)

	// 执行普通目录扫描，并在启用时追加备份文件扫描。
	engine.Run(scanCtx)

	// 扫描结束，停止定时保存 goroutine
	saveTimer.Stop()

	// 保存剩余的结果
	resultsMutex.Lock()
	remaining := make([]Result, len(resultsBatch))
	copy(remaining, resultsBatch)
	resultsBatch = resultsBatch[:0]
	resultsMutex.Unlock()

	savedCount := 0
	if len(remaining) > 0 {
		logger.Info("[DirScan] 保存剩余 %d 条结果", len(remaining))
		for _, r := range remaining {
			if err := saveResult(taskID, currentVersion, r); err != nil {
				logger.Error("保存剩余结果失败: %v", err)
			} else {
				savedCount++
			}
		}
	}

	// 检查是否是用户取消
	if scanCtx.Err() == context.Canceled {
		logger.Info("[DirScan] 任务被取消: %s", taskID)

		var current database.VulnScanTask
		if err := db.Select("status", "pause_requested").Where("id = ? AND version = ?", taskID, currentVersion).First(&current).Error; err == nil {
			if current.Status == taskstate.StatusPaused || current.PauseRequested {
				return taskstate.ErrTaskPaused
			}
		}

		if isCombinationTask {
			// 组合任务场景交由上层调度器处理状态，避免模块覆盖父任务状态。
			return scanCtx.Err()
		}
		return scanCtx.Err()
	}

	// 从数据库查询实际保存的结果数量
	var actualResultCount int64
	db.Model(&database.VulnScanDirResult{}).
		Where("task_id = ? AND version = ?", taskID, currentVersion).
		Count(&actualResultCount)

	// 更新任务为完成状态
	if !isAgentItemTask {
		if err := saveCheckpoint(db, taskID, currentVersion, "completed", 100, map[string]interface{}{
			"processedPaths": processedPaths,
			"totalPaths":     totalPaths,
			"errorCount":     errorCount,
		}); err != nil {
			return fmt.Errorf("保存目录扫描完成断点失败: %w", err)
		}
	}

	if !isCombinationTask {
		if err := db.Model(&database.VulnScanTask{}).
			Where("id = ? AND version = ?", taskID, currentVersion).
			Updates(map[string]interface{}{
				"status":       "completed",
				"progress":     100,
				"error_count":  errorCount,
				"completed_at": time.Now(),
			}).Error; err != nil {
			return fmt.Errorf("更新任务完成状态失败: %w", err)
		}
	}

	logger.Info("[DirScan] 任务完成: %s, 实际保存结果数: %d, 错误数: %d", taskID, actualResultCount, errorCount)
	return nil
}

func parseCombinationTaskFlag(config map[string]interface{}) bool {
	if parseAgentItemTaskFlag(config) {
		return true
	}
	raw, ok := config["isCombination"]
	if !ok {
		return false
	}

	switch v := raw.(type) {
	case bool:
		return v
	case string:
		normalized := strings.ToLower(strings.TrimSpace(v))
		return normalized == "1" || normalized == "true" || normalized == "yes"
	default:
		return false
	}
}

func parseAgentItemTaskFlag(config map[string]interface{}) bool {
	raw, ok := config["agentItemMode"]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		normalized := strings.ToLower(strings.TrimSpace(v))
		return normalized == "1" || normalized == "true" || normalized == "yes"
	default:
		return false
	}
}

// parseConfig 解析配置参数
func parseConfig(config map[string]interface{}) (Options, error) {
	options := Options{
		Method:                 "GET",
		Workers:                25,
		Timeout:                8,
		BodyLengthExcludeTimes: 5,
		Redirect:               false,
		Interval:               0,
		Recursion:              0,
		Backupscan:             false,
	}

	extStrs := make([]string, 0)

	// 解析 exts（扩展名）
	if exts, ok := config["exts"].([]interface{}); ok {
		for _, ext := range exts {
			if extStr, ok := ext.(string); ok && strings.TrimSpace(extStr) != "" {
				extStrs = append(extStrs, strings.TrimSpace(extStr))
			}
		}
	}

	// 解析 method
	if method, ok := config["method"].(string); ok {
		options.Method = method
	}

	// 解析 dictPaths（字典文件路径）
	if dictPaths, ok := config["dictPaths"].([]interface{}); ok && len(dictPaths) > 0 {
		for _, path := range dictPaths {
			if pathStr, ok := path.(string); ok {
				// 从字典文件加载路径，并按扩展名展开 %EXT% 占位符
				paths := LoadDirsearchDict([]string{pathStr}, extStrs)
				if len(paths) == 0 {
					logger.Warning("加载字典文件为空或失败: %s", pathStr)
					continue
				}
				options.Paths = append(options.Paths, paths...)
			}
		}
	}

	// 解析 dictNames（托管字典名，解析到 config/dirsearch 目录）
	if dictNames, ok := config["dictNames"].([]interface{}); ok && len(dictNames) > 0 {
		names := make([]string, 0, len(dictNames))
		for _, name := range dictNames {
			if nameStr, ok := name.(string); ok && strings.TrimSpace(nameStr) != "" {
				names = append(names, nameStr)
			}
		}
		if len(names) > 0 {
			resolved, missing := ResolveDirsearchDictPaths(names)
			for _, name := range missing {
				logger.Warning("托管字典不存在或名称非法，已跳过: %s", name)
			}
			if len(resolved) > 0 {
				options.Paths = append(options.Paths, LoadDirsearchDict(resolved, extStrs)...)
			}
		}
	}

	// 解析 defaultDict（是否加载平台默认目录字典）
	defaultDictDisabled := false
	if defaultDict, ok := config["defaultDict"].(bool); ok {
		if defaultDict {
			options.Paths = append(options.Paths, LoadDefaultDirsearchDict(extStrs)...)
		} else {
			defaultDictDisabled = true
		}
	}

	// 解析 paths（自定义路径列表）
	if paths, ok := config["paths"].([]interface{}); ok && len(paths) > 0 {
		for _, path := range paths {
			if pathStr, ok := path.(string); ok {
				options.Paths = append(options.Paths, pathStr)
			}
		}
	}

	// 兜底：未提供任何有效字典/路径且未显式关闭默认字典时，加载平台默认目录字典，
	// 避免 MCP/AI 调用方不传字典导致空扫描。
	if len(options.Paths) == 0 && !defaultDictDisabled {
		defaultPaths := LoadDefaultDirsearchDict(extStrs)
		if len(defaultPaths) > 0 {
			logger.Info("未配置目录字典，回退到平台默认字典（%d 条路径）", len(defaultPaths))
			options.Paths = defaultPaths
		}
	}

	// 去重，dictPaths/dictNames/defaultDict 与 paths 之间可能存在重复条目
	if len(options.Paths) > 0 {
		options.Paths = arrayutil.RemoveDuplicates(options.Paths)
	}

	// 解析 workers（并发数）
	if workers, ok := config["workers"].(float64); ok {
		options.Workers = int(workers)
	}

	// 解析 timeout（超时时间）
	if timeout, ok := config["timeout"].(float64); ok {
		options.Timeout = int(timeout)
	}

	// 解析 bodyExclude（响应体排除）
	if bodyExclude, ok := config["bodyExclude"].(string); ok {
		options.BodyExclude = bodyExclude
	}

	// 解析 bodyLengthExcludeTimes
	if times, ok := config["bodyLengthExcludeTimes"].(float64); ok {
		options.BodyLengthExcludeTimes = int(times)
	}

	// 解析 statusCodeExclude（状态码排除）
	if statusCodes, ok := config["statusCodeExclude"].([]interface{}); ok {
		for _, code := range statusCodes {
			if codeFloat, ok := code.(float64); ok {
				options.StatusCodeExclude = append(options.StatusCodeExclude, int(codeFloat))
			}
		}
	}

	// 解析 redirect
	if redirect, ok := config["redirect"].(bool); ok {
		options.Redirect = redirect
	}

	// 解析 interval
	if interval, ok := config["interval"].(float64); ok {
		options.Interval = int(interval)
	}

	// 解析 customHeader
	if customHeader, ok := config["customHeader"].(string); ok {
		options.CustomHeader = customHeader
	}

	// 解析 backupscan
	if backupscan, ok := config["backupscan"].(bool); ok {
		options.Backupscan = backupscan
	}

	// 解析 proxy
	if proxy, ok := config["proxy"].(string); ok {
		options.Proxy = proxy
	}

	// 解析 recursive
	if recursive, ok := config["recursive"].(float64); ok {
		options.Recursion = int(recursive)
	} else if recursiveBool, ok := config["recursive"].(bool); ok {
		if recursiveBool {
			options.Recursion = 1
		}
	}

	return options, nil
}

// saveResult 保存扫描结果到数据库
func saveResult(taskID string, version int, result Result) error {
	dirResultSaveMu.Lock()
	defer dirResultSaveMu.Unlock()

	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	// 清理字符串，确保是有效的 UTF-8
	sanitizeUTF8 := func(s string, maxLen int) string {
		s = strings.ToValidUTF8(s, "")

		// 移除 null 字节
		s = strings.ReplaceAll(s, "\x00", "")

		// 限制长度
		if maxLen > 0 && len(s) > maxLen {
			if len(s) > maxLen {
				s = s[:maxLen]
			}
		}
		return s
	}

	dirResult := database.VulnScanDirResult{
		ID:          database.GenerateNanoid(),
		TaskID:      taskID,
		Version:     version,
		URL:         sanitizeUTF8(result.URL, 0),
		Status:      result.Status,
		Title:       sanitizeUTF8(result.Title, 1000),
		Location:    sanitizeUTF8(result.Location, 2000),
		ContentType: sanitizeUTF8(result.ContentType, 200),
		Length:      result.Length,
		Body:        sanitizeUTF8(result.Body, 100*1024), // 限制为 100KB
		Recursion:   result.Recursion,
	}

	var existing database.VulnScanDirResult
	err = db.Where("task_id = ? AND version = ? AND url = ?", taskID, version, dirResult.URL).First(&existing).Error
	switch err {
	case nil:
		if err := db.Model(&existing).Updates(map[string]interface{}{
			"status":       dirResult.Status,
			"title":        dirResult.Title,
			"location":     dirResult.Location,
			"content_type": dirResult.ContentType,
			"length":       dirResult.Length,
			"body":         dirResult.Body,
			"recursion":    dirResult.Recursion,
		}).Error; err != nil {
			return fmt.Errorf("更新目录扫描结果失败: %w", err)
		}
	case gorm.ErrRecordNotFound:
		if err := db.Create(&dirResult).Error; err != nil {
			return fmt.Errorf("创建目录扫描结果失败: %w", err)
		}
	default:
		return fmt.Errorf("查询目录扫描结果失败: %w", err)
	}

	return nil
}

// LoadDictPaths 从字典文件加载路径列表
func LoadDictPaths(dictPath string) ([]string, error) {
	// 从文件加载字典
	file, err := os.Open(dictPath)
	if err != nil {
		return nil, fmt.Errorf("打开字典文件失败: %w", err)
	}
	defer file.Close()

	// 读取文件内容
	content := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			content = append(content, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	// 按行分割
	lines := strings.Split(string(content), "\n")
	paths := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			paths = append(paths, line)
		}
	}

	return paths, nil
}
