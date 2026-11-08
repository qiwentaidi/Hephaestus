package portscan

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	appconfig "hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/findings"
	"hephaestus-backend/pkg/taskstate"
	"hephaestus-backend/pkg/types"

	"hephaestus-backend/lib/crack"
	"hephaestus-backend/pkg/logger"

	iputil "github.com/qiwentaidi/utils/ip"
	portutil "github.com/qiwentaidi/utils/port"
	"gorm.io/gorm"
)

func clearPendingPortResults(db *gorm.DB, taskID string, version int) error {
	return db.Where("task_id = ? AND version = ? AND discovery_status = ?", taskID, version, "pending").Delete(&database.PortScanResult{}).Error
}

func normalizePortScanHost(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}

	if parsed, err := url.Parse(target); err == nil && parsed.Host != "" {
		return strings.TrimSpace(parsed.Hostname())
	}

	if host, _, err := net.SplitHostPort(target); err == nil {
		return strings.Trim(host, "[]")
	}

	return strings.Trim(target, "[]")
}

func expandPortScanHosts(targets []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(targets))

	for _, target := range targets {
		host := normalizePortScanHost(target)
		if host == "" {
			continue
		}

		expanded := iputil.ParseIPs([]string{host})
		if len(expanded) == 0 {
			if _, ok := seen[host]; ok {
				continue
			}
			seen[host] = struct{}{}
			out = append(out, host)
			continue
		}

		for _, item := range expanded {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}

	return out
}

func parseTargetAliases(config map[string]interface{}) map[string][]database.PortResultAssetInput {
	raw, ok := config["targetAliases"]
	if !ok || raw == nil {
		return nil
	}

	out := make(map[string][]database.PortResultAssetInput)
	addAlias := func(ip string, alias database.PortResultAssetInput) {
		ip = strings.TrimSpace(ip)
		alias.Subdomain = strings.TrimSpace(alias.Subdomain)
		if ip == "" || alias.Subdomain == "" {
			return
		}
		alias.IP = ip
		out[ip] = append(out[ip], alias)
	}

	switch typed := raw.(type) {
	case map[string][]database.PortResultAssetInput:
		for ip, aliases := range typed {
			for _, alias := range aliases {
				addAlias(ip, alias)
			}
		}
	case map[string][]map[string]string:
		for ip, aliases := range typed {
			for _, alias := range aliases {
				addAlias(ip, database.PortResultAssetInput{
					Domain:       alias["domain"],
					Subdomain:    alias["subdomain"],
					AssetType:    alias["assetType"],
					AssetName:    alias["assetName"],
					SourceModule: alias["sourceModule"],
				})
			}
		}
	case map[string]interface{}:
		for ip, rawAliases := range typed {
			items, ok := rawAliases.([]interface{})
			if !ok {
				continue
			}
			for _, item := range items {
				fields, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				addAlias(ip, database.PortResultAssetInput{
					Domain:       stringFromMap(fields, "domain"),
					Subdomain:    stringFromMap(fields, "subdomain"),
					AssetType:    stringFromMap(fields, "assetType"),
					AssetName:    stringFromMap(fields, "assetName"),
					SourceModule: stringFromMap(fields, "sourceModule"),
				})
			}
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func stringFromMap(values map[string]interface{}, key string) string {
	if value, ok := values[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func parseTCPDiscoveryConfig(config map[string]interface{}) TCPDiscoveryConfig {
	discoveryConfig := defaultTCPDiscoveryConfig()
	if config == nil {
		return discoveryConfig
	}

	mode := strings.ToLower(strings.TrimSpace(firstStringFromConfig(config,
		"portDiscoveryMode",
		"tcpDiscoveryMode",
		"scanMode",
		"tcpScanMode",
	)))
	mode = strings.ReplaceAll(mode, "_", "-")
	switch mode {
	case "connect", "tcp", "tcp-connect", "full-connect":
		discoveryConfig.Mode = "connect"
	case "syn", "tcp-syn", "syn-scan":
		discoveryConfig.Mode = "syn"
	}

	if raw, ok := firstConfigValue(config, "enableSynScan", "enableSYNScan"); ok && !boolFromConfig(raw, true) {
		discoveryConfig.Mode = "connect"
	}
	if raw, ok := firstConfigValue(config, "synRate", "portDiscoveryRate", "discoveryRate"); ok {
		discoveryConfig.Rate = intFromConfig(raw, 0)
	}
	if raw, ok := firstConfigValue(config, "synDiscoveryAttempts", "synAttempts"); ok {
		discoveryConfig.Attempts = intFromConfig(raw, defaultSYNDiscoveryAttempts)
	}

	return discoveryConfig.normalized()
}

func firstStringFromConfig(config map[string]interface{}, keys ...string) string {
	raw, ok := firstConfigValue(config, keys...)
	if !ok {
		return ""
	}
	if value, ok := raw.(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func firstConfigValue(config map[string]interface{}, keys ...string) (interface{}, bool) {
	for _, key := range keys {
		if raw, ok := config[key]; ok {
			return raw, true
		}
	}
	return nil, false
}

func boolFromConfig(raw interface{}, fallback bool) bool {
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "yes", "y", "on", "enabled":
			return true
		case "0", "false", "no", "n", "off", "disabled":
			return false
		}
	}
	return fallback
}

func crackDictDir() string {
	return filepath.Join(appconfig.GetConfigResourceRoot(), "crack")
}

const (
	defaultCrackTargetTimeout         = 30 * time.Minute
	defaultSSHCrackTargetTimeout      = 120 * time.Second
	defaultShortCrackTargetTimeout    = 3 * time.Minute
	defaultMediumCrackTargetTimeout   = 10 * time.Minute
	defaultDatabaseCrackTargetTimeout = 15 * time.Minute
)

// ExecutePortScanForTask 执行端口扫描（独立端口扫描任务）
func ExecutePortScanForTask(taskID string, targets []string, config map[string]interface{}, version int) error {
	return executePortScanInternal(taskID, targets, config, version)
}

// ExecutePortScan 执行端口扫描（兼容VulnScanTask）
func ExecutePortScan(taskID string, targets []string, config map[string]interface{}) error {
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

	return executePortScanInternal(taskID, targets, config, currentVersion)
}

// ExecutePortScanWithContext 执行端口扫描（带 context 支持，用于漏洞扫描模块）
func ExecutePortScanWithContext(ctx context.Context, taskID string, targets []string, config map[string]interface{}) error {
	// 检查 context 是否已取消
	if ctx.Err() != nil {
		return ctx.Err()
	}

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

	return executePortScanWithContextInternal(ctx, taskID, targets, config, currentVersion, task)
}

// executePortScanWithContextInternal 内部端口扫描执行函数（带 context 支持）
func executePortScanWithContextInternal(ctx context.Context, taskID string, targets []string, config map[string]interface{}, currentVersion int, task database.VulnScanTask) error {
	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}
	if err := clearPendingPortResults(db, taskID, currentVersion); err != nil {
		return err
	}
	defer func() {
		if err := clearPendingPortResults(db, taskID, currentVersion); err != nil {
			logger.Warning("清理待识别端口结果失败: %v", err)
		}
	}()
	isCombinationTask := parseCombinationTaskFlag(config)
	isAgentItemTask := parseAgentItemTaskFlag(config)
	saveCheckpoint := taskstate.SaveCheckpoint
	checkpointAndPause := taskstate.CheckpointAndPause
	if isCombinationTask {
		saveCheckpoint, checkpointAndPause = taskstate.CombinationCheckpointHandlers("portscan")
	}
	scanCtx, scanCancel := context.WithCancel(ctx)
	defer scanCancel()
	checkpoint := taskstate.ScanCheckpoint(task, "portscan", isCombinationTask)
	stageTracker := newPortScanStageTracker(checkpoint)
	resumeProgressFloor := task.Progress
	if isCombinationTask {
		resumeProgressFloor = checkpoint.Progress
	}
	if checkpoint.Progress > resumeProgressFloor {
		resumeProgressFloor = checkpoint.Progress
	}

	// 检查 context 是否已取消
	if ctx.Err() != nil {
		return ctx.Err()
	}

	stopControlWatcher := startPortScanControlWatcher(scanCtx, scanCancel, func(pollCtx context.Context) bool {
		var control struct {
			Status         string
			PauseRequested bool
		}
		err := db.WithContext(pollCtx).Model(&database.VulnScanTask{}).
			Select("status", "pause_requested").
			Where("id = ? AND version = ?", taskID, currentVersion).
			First(&control).Error
		if err != nil {
			return err == gorm.ErrRecordNotFound
		}
		return control.Status == taskstate.StatusCancelled || control.Status == taskstate.StatusDeleting || control.Status == taskstate.StatusPaused || control.PauseRequested
	})
	defer stopControlWatcher()

	// 解析配置
	workers := 600
	if w, ok := config["threads"].(float64); ok {
		workers = int(w)
	}
	if w, ok := config["threads"].(int); ok {
		workers = w
	}

	timeout := 3
	if t, ok := config["timeout"].(float64); ok {
		timeout = int(t)
	}
	if t, ok := config["timeout"].(int); ok {
		timeout = t
	}

	proxyURL := ""
	if p, ok := config["proxy"].(string); ok {
		proxyURL = p
	}
	discoveryConfig := parseTCPDiscoveryConfig(config)
	if discoveryConfig.Mode == "syn" {
		runtime := resolveSYNDiscoveryRuntime(ctx)
		if !runtime.useHelper && !runtime.hasRawSocket {
			return synAuthorizationError(runtime.helperStatus)
		}
	}

	portRange := "1-65535"
	if pr, ok := config["portRange"].(string); ok && pr != "" {
		portRange = pr
	}

	admissionConfig, err := parseHostAdmissionConfig(config, portRange, proxyURL, workers)
	if err != nil {
		return err
	}
	expandedHosts := targets
	if admissionConfig.Subnet == nil {
		expandedHosts = expandPortScanHosts(targets)
	}
	signature := hostAdmissionSignature(expandedHosts, admissionConfig, portRange)
	admission, restored := restoreHostAdmission(checkpoint, signature)
	if !restored {
		// Old or incompatible checkpoints cannot safely apply a port cursor to
		// a freshly filtered host set. Replaying is preferable to skipping work.
		if checkpoint.Stage == "portscan:scanning" {
			checkpoint.Stage = ""
			resumeProgressFloor = 0
		}
		var subnetSnapshot *subnetAdmission
		if admissionConfig.Subnet != nil {
			subnetSnapshot = restoreSubnetAdmission(checkpoint, signature)
			if subnetSnapshot == nil {
				stageTracker.Record(portScanStageSubnet, "网段预探测")
				if err := reportStage(scanCtx, portScanStageSubnet, "网段预探测"); err != nil {
					return err
				}
				if !isAgentItemTask {
					if err := checkpointAndPause(scanCtx, db, taskID, currentVersion, "portscan:subnets", resumeProgressFloor, stageTracker.Meta(nil)); err != nil {
						return err
					}
				}
				lastSubnetUpdate := time.Now()
				var progressErr error
				subnetSnapshot, err = discoverSubnetAdmission(scanCtx, targets, admissionConfig, signature, CheckLiveWithContext, func(done, hits int) {
					if time.Since(lastSubnetUpdate) < 2*time.Second {
						return
					}
					lastSubnetUpdate = time.Now()
					stageTracker.Record(portScanStageSubnet, fmt.Sprintf("网段预探测 · 已探测 %d · 命中 %d", done, hits))
					if !isAgentItemTask {
						if err := saveCheckpoint(db, taskID, currentVersion, "portscan:subnets", resumeProgressFloor, stageTracker.Meta(nil)); err != nil {
							progressErr = err
							scanCancel()
						}
					}
				})
				if progressErr != nil {
					return progressErr
				}
				if err != nil {
					return err
				}
			}
			if !isAgentItemTask {
				if err := checkpointAndPause(scanCtx, db, taskID, currentVersion, "portscan:subnets_done", resumeProgressFloor, stageTracker.Meta(map[string]interface{}{"subnetAdmission": subnetSnapshot})); err != nil {
					return err
				}
			}
		}
		showAliveStage := admissionConfig.Ping || admissionConfig.TCP != nil
		if showAliveStage {
			stageTracker.Record(portScanStageAlive, "存活探测")
			if err := reportStage(scanCtx, portScanStageAlive, "存活探测"); err != nil {
				return err
			}
		}
		if !isAgentItemTask {
			if err := checkpointAndPause(scanCtx, db, taskID, currentVersion, "portscan:alive", resumeProgressFloor, stageTracker.Meta(map[string]interface{}{"totalHosts": len(expandedHosts), "subnetAdmission": subnetSnapshot})); err != nil {
				return err
			}
		}
		if subnetSnapshot != nil {
			admission, err = discoverSubnetHosts(scanCtx, targets, admissionConfig, subnetSnapshot, CheckLiveWithContext)
		} else {
			admission, err = discoverAdmittedHosts(scanCtx, expandedHosts, admissionConfig, signature, CheckLiveWithContext)
		}
		if err != nil {
			return err
		}
		label := fmt.Sprintf("主机准入 %d/%d · Ping %d · TCP %d · 未确认跳过 %d", len(admission.Hosts), admission.Total, admission.PingAlive, admission.TCPAlive, admission.Unconfirmed)
		if admission.Subnets != nil {
			label = fmt.Sprintf("网段命中 %d/%d · ", admission.Subnets.Confirmed, len(admission.Subnets.Results)) + label
		}
		if showAliveStage {
			stageTracker.Record(portScanStageAlive, label)
		}
		logger.Info("%s", label)
		if !isAgentItemTask {
			if err := checkpointAndPause(scanCtx, db, taskID, currentVersion, "portscan:alive_done", resumeProgressFloor, stageTracker.Meta(admission.Meta(nil))); err != nil {
				return err
			}
		}
	} else {
		logger.Info("恢复已保存的主机准入集合: %d/%d", len(admission.Hosts), admission.Total)
	}
	if scanCtx.Err() != nil {
		return scanCtx.Err()
	}
	if len(admission.Hosts) == 0 {
		if err := reportStage(scanCtx, StageNoHosts, ""); err != nil {
			return err
		}
		if !isAgentItemTask {
			stageTracker.Record(StageComplete, "未确认可扫描主机")
			if err := saveCheckpoint(db, taskID, currentVersion, "portscan:alive_done", 100, stageTracker.Meta(admission.Meta(nil))); err != nil {
				return err
			}
		}
		updateVulnScanTaskProgress(db, taskID, currentVersion, 100, isCombinationTask)
		return nil
	}

	// 解析端口范围，支持 U:53 / U:53-69 这类 UDP 端口定义
	ports, err := portutil.ParsePortDefinitions(portRange)
	if err != nil {
		return fmt.Errorf("解析端口范围失败: %w", err)
	}
	targetAliases := parseTargetAliases(config)

	// 处理 IP 组并保留域名目标。
	expandedIPs := admission.Hosts
	if admission.Subnets != nil {
		discoveryConfig.HostOrder = admission.Hosts
	}

	// 计算总任务数（用于进度计算）
	totalTasks := len(expandedIPs) * len(ports)
	if checkpoint.Stage == "portscan:scanning" {
		if rawTotal, ok := checkpoint.Meta["total"]; ok {
			switch value := rawTotal.(type) {
			case float64:
				if int(value) > 0 {
					totalTasks = int(value)
				}
			case int:
				if value > 0 {
					totalTasks = value
				}
			}
		}
	}
	var resumeCompleted int32
	if checkpoint.Stage == "portscan:scanning" {
		if rawCompleted, ok := checkpoint.Meta["completed"]; ok {
			switch value := rawCompleted.(type) {
			case float64:
				resumeCompleted = int32(value)
			case int:
				resumeCompleted = int32(value)
			case int32:
				resumeCompleted = value
			}
		}
	}

	resumeCompleted = portScanResumeCompleted(discoveryConfig, resumeCompleted)
	if discoveryConfig.normalized().Mode == "syn" {
		// This stage is replayed, so show the new pass from zero.
		resumeProgressFloor = 0
	}

	logger.Info("待扫描目标 %d 个，端口数量 %d，共 %d 个端口探测项",
		len(expandedIPs), len(ports), totalTasks)
	discoveryModeLabel := strings.ToUpper(string(discoveryConfig.normalized().Mode))
	scanLabel := "端口扫描 · " + discoveryModeLabel
	stageTracker.Record(portScanStageScan, scanLabel)
	if err := reportStage(scanCtx, portScanStageScan, scanLabel); err != nil {
		return err
	}
	if !isAgentItemTask {
		meta := portScanCheckpointMeta(discoveryConfig, resumeCompleted, totalTasks)
		meta["hosts"], meta["ports"] = len(expandedIPs), len(ports)
		if err := checkpointAndPause(scanCtx, db, taskID, currentVersion, "portscan:scanning", resumeProgressFloor, stageTracker.Meta(admission.Meta(meta))); err != nil {
			if err == taskstate.ErrTaskPaused {
				scanCancel()
			}
			return err
		}
	}
	if !isCombinationTask {
		db.Model(&database.VulnScanTask{}).
			Where("id = ? AND version = ?", taskID, currentVersion).
			Updates(map[string]interface{}{
				"total_count": totalTasks,
				"progress":    resumeProgressFloor,
				"updated_at":  time.Now(),
			})
	}

	// 生成地址列表
	addressChan := make(chan Address, 1000)
	go func() {
		defer close(addressChan)
		for _, ip := range expandedIPs {
			for _, port := range ports {
				// 检查 context 是否已取消
				select {
				case <-scanCtx.Done():
					return
				case addressChan <- Address{
					IP:       ip,
					Port:     port.Port,
					Protocol: parseScanProtocol(port.Protocol),
				}:
				}
			}
		}
	}()

	// 端口扫描结果回调 - 保存到 VulnScanTask 关联的端口表
	portResultCallback := func(result *types.PortInfo) {
		// 保存端口结果到数据库（使用 VulnScanTask 的端口表）
		record := database.SavePortResult(db, taskID, currentVersion, result)
		if record == nil {
			return
		}
		if result.DiscoveryStatus == "pending" {
			return
		}
		if aliases := targetAliases[result.Host]; len(aliases) > 0 {
			enrichedAliases := make([]database.PortResultAssetInput, 0, len(aliases))
			for _, alias := range aliases {
				alias.IP = result.Host
				alias.Port = result.Port
				alias.Transport = result.Transport
				enrichedAliases = append(enrichedAliases, alias)
			}
			database.SavePortResultAssets(db, taskID, currentVersion, record.ID, enrichedAliases)
		}
	}

	// 进度回调 - 添加节流逻辑，避免频繁写入数据库
	var lastProgress int32 = -1
	var lastUpdateTime = time.Now()
	var progressMutex sync.Mutex
	const minProgressInterval = 2 * time.Second // 最小更新间隔
	const minProgressChange = 5                 // 最小进度变化阈值

	progressCallback := func(completed int32) {
		if totalTasks <= 0 {
			return
		}

		progress := portScanDisplayPercent(discoveryConfig, completed, totalTasks, resumeProgressFloor)

		progressMutex.Lock()
		defer progressMutex.Unlock()

		now := time.Now()
		progressDiff := progress - int(lastProgress)
		timeSinceLastUpdate := now.Sub(lastUpdateTime)

		// 只有满足以下条件之一才更新数据库：
		// 1. 进度变化超过阈值
		// 2. 距离上次更新超过最小时间间隔
		// 3. 进度达到 100%（任务完成）
		shouldUpdate := progressDiff >= minProgressChange ||
			timeSinceLastUpdate >= minProgressInterval ||
			progress == 100

		if shouldUpdate && int32(progress) != lastProgress {
			if !isAgentItemTask {
				persist := func() error {
					meta := stageTracker.Meta(admission.Meta(portScanCheckpointMeta(discoveryConfig, completed, totalTasks)))
					if discoveryConfig.normalized().Mode == "syn" {
						if err := taskstate.CheckTaskControlState(scanCtx, db, taskID, currentVersion); err != nil {
							scanCancel()
							return err
						}
						return saveCheckpoint(db, taskID, currentVersion, "portscan:scanning", progress, meta)
					}
					return checkpointAndPause(scanCtx, db, taskID, currentVersion, "portscan:scanning", progress, meta)
				}
				if err := persist(); err != nil {
					if err == taskstate.ErrTaskPaused {
						scanCancel()
					}
					return
				}
			}
			updateVulnScanTaskProgress(db, taskID, currentVersion, progress, isCombinationTask)
			lastProgress = int32(progress)
			lastUpdateTime = now
		}
	}

	// 执行端口扫描
	logger.Info("开始端口扫描...")
	if err := TcpScanWithDiscovery(scanCtx, taskID, addressChan, workers, timeout, proxyURL, resumeCompleted, discoveryConfig, portResultCallback, progressCallback); err != nil {
		return err
	}
	if scanCtx.Err() == context.Canceled {
		var state struct{ Status string }
		if err := db.Model(&database.VulnScanTask{}).Select("status").Where("id = ? AND version = ?", taskID, currentVersion).First(&state).Error; err == nil && state.Status == taskstate.StatusPaused {
			return taskstate.ErrTaskPaused
		}
		return scanCtx.Err()
	}
	if !isAgentItemTask {
		stageTracker.Record(StageComplete, "扫描结束")
		_ = saveCheckpoint(db, taskID, currentVersion, "portscan:completed", 100, stageTracker.Meta(admission.Meta(nil)))
	}

	return nil
}

// updateVulnScanTaskProgress 更新 VulnScanTask 的进度
func updateVulnScanTaskProgress(db *gorm.DB, taskID string, version int, progress int, skip bool) {
	if skip {
		return
	}
	db.Model(&database.VulnScanTask{}).
		Where("id = ? AND version = ?", taskID, version).
		Updates(map[string]interface{}{
			"progress":   progress,
			"updated_at": time.Now(),
		})
}

// executePortScanInternal 内部端口扫描执行函数
func executePortScanInternal(taskID string, targets []string, config map[string]interface{}, currentVersion int) error {
	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}
	if err := clearPendingPortResults(db, taskID, currentVersion); err != nil {
		return err
	}
	defer func() {
		if err := clearPendingPortResults(db, taskID, currentVersion); err != nil {
			logger.Warning("清理待识别端口结果失败: %v", err)
		}
	}()
	isCombinationTask := parseCombinationTaskFlag(config)

	// 解析配置
	workers := 600
	if w, ok := config["threads"].(float64); ok {
		workers = int(w)
	}
	if w, ok := config["threads"].(int); ok {
		workers = w
	}

	timeout := 3
	if t, ok := config["timeout"].(float64); ok {
		timeout = int(t)
	}
	if t, ok := config["timeout"].(int); ok {
		timeout = t
	}

	proxyURL := ""
	if p, ok := config["proxy"].(string); ok {
		proxyURL = p
	}
	discoveryConfig := parseTCPDiscoveryConfig(config)
	if discoveryConfig.Mode == "syn" {
		runtime := resolveSYNDiscoveryRuntime(context.Background())
		if !runtime.useHelper && !runtime.hasRawSocket {
			return synAuthorizationError(runtime.helperStatus)
		}
	}

	portRange := "1-65535"
	if pr, ok := config["portRange"].(string); ok && pr != "" {
		portRange = pr
	}

	admissionConfig, err := parseHostAdmissionConfig(config, portRange, proxyURL, workers)
	if err != nil {
		return err
	}
	var admission *hostAdmission
	if admissionConfig.Subnet != nil {
		var snapshot *subnetAdmission
		snapshot, err = discoverSubnetAdmission(context.Background(), targets, admissionConfig, "", CheckLiveWithContext, nil)
		if err == nil {
			admission, err = discoverSubnetHosts(context.Background(), targets, admissionConfig, snapshot, CheckLiveWithContext)
		}
	} else {
		admission, err = discoverAdmittedHosts(context.Background(), expandPortScanHosts(targets), admissionConfig, "", CheckLiveWithContext)
	}
	if err != nil {
		return err
	}
	logger.Info("主机准入 %d/%d · Ping %d · TCP %d · 未确认跳过 %d", len(admission.Hosts), admission.Total, admission.PingAlive, admission.TCPAlive, admission.Unconfirmed)
	if len(admission.Hosts) == 0 {
		completedAt := time.Now()
		db.Exec("UPDATE port_scan_task SET status = ?, progress = ?, completed_at = ?, updated_at = ? WHERE id = ? AND version = ?", "completed", 100, &completedAt, time.Now(), taskID, currentVersion)
		return nil
	}

	// 解析端口范围，支持 U:53 / U:53-69 这类 UDP 端口定义
	ports, err := portutil.ParsePortDefinitions(portRange)
	if err != nil {
		return fmt.Errorf("解析端口范围失败: %w", err)
	}
	targetAliases := parseTargetAliases(config)

	// 处理 IP 组并保留域名目标。
	expandedIPs := admission.Hosts
	if admission.Subnets != nil {
		discoveryConfig.HostOrder = admission.Hosts
	}

	// 计算总任务数（用于进度计算）
	totalTasks := len(expandedIPs) * len(ports)

	logger.Info("待扫描目标 %d 个，端口数量 %d，共 %d 个端口探测项",
		len(expandedIPs), len(ports), totalTasks)

	// 生成地址列表
	addressChan := make(chan Address, 1000)
	go func() {
		defer close(addressChan)
		for _, ip := range expandedIPs {
			for _, port := range ports {
				addressChan <- Address{
					IP:       ip,
					Port:     port.Port,
					Protocol: parseScanProtocol(port.Protocol),
				}
			}
		}
	}()

	// 端口扫描结果回调
	portResultCallback := func(result *types.PortInfo) {
		// 保存端口结果到数据库
		record := database.SavePortResult(db, taskID, currentVersion, result)
		if record == nil {
			return
		}
		if result.DiscoveryStatus == "pending" {
			return
		}
		if aliases := targetAliases[result.Host]; len(aliases) > 0 {
			enrichedAliases := make([]database.PortResultAssetInput, 0, len(aliases))
			for _, alias := range aliases {
				alias.IP = result.Host
				alias.Port = result.Port
				alias.Transport = result.Transport
				enrichedAliases = append(enrichedAliases, alias)
			}
			database.SavePortResultAssets(db, taskID, currentVersion, record.ID, enrichedAliases)
		}
	}

	// 进度回调
	progressCallback := func(completed int32) {
		// 更新任务进度
		if totalTasks > 0 {
			progress := int(float64(completed) / float64(totalTasks) * 100)
			if progress > 100 {
				progress = 100
			}
			updateTaskProgress(db, taskID, currentVersion, progress, isCombinationTask)
		}
	}

	// 执行端口扫描
	logger.Info("开始端口扫描...")
	if err := TcpScanWithDiscovery(context.Background(), taskID, addressChan, workers, timeout, proxyURL, 0, discoveryConfig, portResultCallback, progressCallback); err != nil {
		return err
	}

	return nil
}

// ExecuteCrackWithContext 执行密码暴破（带 context 支持，直接暴破，不进行端口扫描）
func ExecuteCrackWithContext(ctx context.Context, taskID string, targets []string, config map[string]interface{}) error {
	// 检查 context 是否已取消
	if ctx.Err() != nil {
		logger.Info("任务 %s 已被取消（启动前）", taskID)
		return ctx.Err()
	}

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
	taskName := task.TaskName
	// Crack tasks honor only the task-level application proxy. Host OS proxy
	// settings are intentionally not consulted.
	if proxyURL, ok := config["proxy"].(string); ok {
		proxyURL = strings.TrimSpace(proxyURL)
		if proxyURL != "" {
			proxyDialer, proxyErr := crack.NewProxyDialContext(proxyURL)
			if proxyErr != nil {
				return fmt.Errorf("爆破代理配置无效: %w", proxyErr)
			}
			ctx = crack.WithProxyURL(ctx, proxyURL)
			ctx = crack.WithDialContext(ctx, proxyDialer)
			logger.Info("爆破任务 %s 已启用应用代理", taskID)
		}
	}
	isCombinationTask := parseCombinationTaskFlag(config)
	isAgentItemTask := parseAgentItemTaskFlag(config)
	saveCheckpoint := taskstate.SaveCheckpoint
	checkpointAndPause := taskstate.CheckpointAndPause
	if isCombinationTask {
		saveCheckpoint, checkpointAndPause = taskstate.CombinationCheckpointHandlers("crack")
	}
	checkpoint := taskstate.ScanCheckpoint(task, "crack", isCombinationTask)
	totalTargets := len(targets)
	completedTargets := 0
	if checkpoint.Stage == "crack:scanning" {
		if raw, ok := checkpoint.Meta["processedTargets"]; ok {
			switch value := raw.(type) {
			case float64:
				completedTargets = int(value)
			case int:
				completedTargets = value
			}
		}
	}
	if completedTargets < 0 {
		completedTargets = 0
	}
	if completedTargets > totalTargets {
		completedTargets = totalTargets
	}
	resumeProgressFloor := task.Progress
	if isCombinationTask {
		resumeProgressFloor = checkpoint.Progress
	}
	if checkpoint.Progress > resumeProgressFloor {
		resumeProgressFloor = checkpoint.Progress
	}
	if !isCombinationTask {
		db.Model(&database.VulnScanTask{}).
			Where("id = ? AND version = ?", taskID, currentVersion).
			Updates(map[string]interface{}{
				"progress":    resumeProgressFloor,
				"total_count": totalTargets,
				"updated_at":  time.Now(),
			})
	}

	// 更新任务进度
	updateProgress := func(progress int) {
		if progress < resumeProgressFloor {
			progress = resumeProgressFloor
		}
		updateTaskProgress(db, taskID, currentVersion, progress, isCombinationTask)
	}

	// 漏洞扫描结果回调
	vulnCallback := func(vuln types.VulnerabilityInfo) {
		// 设置任务ID
		vuln.TaskId = taskID
		// 保存漏洞结果到数据库
		database.SaveVulnerabilityResult(db, taskID, currentVersion, taskName, vuln)
		if _, err := findings.Ingest(db, findings.Input{
			Source:      "portscan",
			SourceID:    vuln.ID + "|" + vuln.URL,
			TaskID:      taskID,
			TaskVersion: currentVersion,
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

	// 执行暴力破解
	logger.Info("开始密码暴破，目标数量: %d", len(targets))
	for i, target := range targets {
		if i < completedTargets {
			continue
		}

		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			logger.Info("任务 %s 在爆破过程中被取消（已处理 %d/%d 个目标）", taskID, completedTargets, totalTargets)
			var current database.VulnScanTask
			if err := db.Select("status").Where("id = ? AND version = ?", taskID, currentVersion).First(&current).Error; err == nil && current.Status == taskstate.StatusPaused {
				return taskstate.ErrTaskPaused
			}
			return ctx.Err()
		default:
		}

		// 从 URL 中提取协议名
		u, err := url.Parse(target)
		if err != nil {
			logger.Warning("解析URL失败: %s, 错误: %v", target, err)
			continue
		}
		protocol := u.Scheme

		// 解析暴破字典（根据协议加载系统字典并合并用户配置）
		var credentials []string
		if crackProtocolNeedsDictionary(protocol) {
			credentials = parseCrackDicts(protocol, config)
		} else {
			logger.Info("协议 %s 无需加载字典，跳过凭据准备", protocol)
		}

		targetStart := time.Now()
		logger.Info("开始爆破目标: %s, 协议: %s, 字典数量: %d", target, protocol, len(credentials))

		// 执行暴破（传递 context）
		if err := runCrackTargetWithTimeout(ctx, taskID, target, credentials, crackTargetTimeout(protocol, config), sshMaxTimeoutErrors(config), vulnCallback); err != nil {
			logger.Warning("爆破目标超时或取消: %s, 协议: %s, 错误: %v", target, protocol, err)
		}

		logger.Info("完成爆破目标: %s, 协议: %s, 耗时: %s", target, protocol, time.Since(targetStart).Round(time.Millisecond))

		select {
		case <-ctx.Done():
			logger.Info("任务 %s 在目标执行过程中被取消（已处理 %d/%d 个目标）", taskID, completedTargets, totalTargets)
			var current database.VulnScanTask
			if err := db.Select("status").Where("id = ? AND version = ?", taskID, currentVersion).First(&current).Error; err == nil && current.Status == taskstate.StatusPaused {
				return taskstate.ErrTaskPaused
			}
			return ctx.Err()
		default:
		}

		completedTargets++
		progress := 100
		if totalTargets > 0 {
			progress = int(float64(completedTargets) / float64(totalTargets) * 100)
		}
		updateProgress(progress)
		if !isAgentItemTask {
			if err := checkpointAndPause(ctx, db, taskID, currentVersion, "crack:scanning", progress, map[string]interface{}{
				"processedTargets": completedTargets,
				"totalTargets":     totalTargets,
			}); err != nil {
				if err == taskstate.ErrTaskPaused {
					return err
				}
				return err
			}
		}
	}

	// 完成
	updateProgress(100)
	if !isAgentItemTask {
		if err := saveCheckpoint(db, taskID, currentVersion, "completed", 100, map[string]interface{}{
			"processedTargets": completedTargets,
			"totalTargets":     totalTargets,
		}); err != nil {
			return fmt.Errorf("保存密码暴破完成断点失败: %w", err)
		}
	}
	return nil
}

func runCrackTargetWithTimeout(ctx context.Context, taskID, target string, credentials []string, timeout time.Duration, sshTimeoutErrorLimit int, vulnCallback VulnerabilityCallback) error {
	ctx = withSSHMaxTimeoutErrors(ctx, sshTimeoutErrorLimit)
	if timeout <= 0 {
		RunnerWithContext(ctx, taskID, target, credentials, vulnCallback)
		return ctx.Err()
	}

	targetCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		RunnerWithContext(targetCtx, taskID, target, credentials, vulnCallback)
	}()

	select {
	case <-done:
		return targetCtx.Err()
	case <-targetCtx.Done():
		return targetCtx.Err()
	}
}

func sshMaxTimeoutErrors(config map[string]interface{}) int {
	for _, key := range []string{"sshMaxTimeoutErrors", "sshMaxAuthTimeoutErrors"} {
		if raw, ok := config[key]; ok {
			return intFromConfig(raw, 5)
		}
	}
	return 5
}

func crackTargetTimeout(protocol string, config map[string]interface{}) time.Duration {
	for _, key := range []string{"targetTimeout", "crackTargetTimeout", "timeout"} {
		if raw, ok := config[key]; ok {
			if timeout := durationFromConfigSeconds(raw); timeout > 0 {
				return timeout
			}
		}
	}
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "ssh":
		return defaultSSHCrackTargetTimeout
	case "rsync", "vnc":
		return defaultShortCrackTargetTimeout
	case "ftp", "telnet", "smb", "smtp", "rdp":
		return defaultMediumCrackTargetTimeout
	case "mssql", "mysql", "oracle", "dameng", "postgresql", "mongodb", "clickhouse":
		return defaultDatabaseCrackTargetTimeout
	default:
		return defaultCrackTargetTimeout
	}
}

func intFromConfig(raw interface{}, fallback int) int {
	switch value := raw.(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float32:
		return int(value)
	case float64:
		return int(value)
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return fallback
		}
		var parsed int
		if _, err := fmt.Sscanf(trimmed, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

func durationFromConfigSeconds(raw interface{}) time.Duration {
	switch value := raw.(type) {
	case int:
		return time.Duration(value) * time.Second
	case int32:
		return time.Duration(value) * time.Second
	case int64:
		return time.Duration(value) * time.Second
	case float32:
		return time.Duration(value) * time.Second
	case float64:
		return time.Duration(value) * time.Second
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return 0
		}
		if parsed, err := time.ParseDuration(trimmed); err == nil {
			return parsed
		}
		var seconds float64
		if _, err := fmt.Sscanf(trimmed, "%f", &seconds); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return 0
}

// ExecuteCrack 执行密码暴破（直接暴破，不进行端口扫描）
func ExecuteCrack(taskID string, targets []string, config map[string]interface{}) error {
	return ExecuteCrackWithContext(context.Background(), taskID, targets, config)
}

// updateTaskProgress 更新任务进度（VulnScanTask）
func updateTaskProgress(db *gorm.DB, taskID string, currentVersion int, progress int, skip bool) {
	if skip {
		return
	}
	// 更新任务进度（每10%更新一次，避免频繁更新数据库）
	if progress%10 == 0 || progress == 100 {
		// 更新指定版本的任务进度
		db.Model(&database.VulnScanTask{}).
			Where("id = ? AND version = ?", taskID, currentVersion).
			Updates(map[string]interface{}{
				"progress":   progress,
				"updated_at": time.Now(),
			})
	}
}

// getOpenPortsFromDB 从数据库获取开放的端口（仅最新版本）
func getOpenPortsFromDB(db *gorm.DB, taskID string) []string {
	// 先获取任务的最新版本号
	var task database.VulnScanTask
	if err := db.First(&task, "id = ?", taskID).Error; err != nil {
		return nil
	}

	var results []database.PortScanResult
	if err := db.Where("task_id = ? AND version = ?", taskID, task.Version).Find(&results).Error; err != nil {
		return nil
	}

	var openPorts []string
	for _, result := range results {
		// 构建 URL 格式：protocol://host:port
		url := fmt.Sprintf("%s://%s:%d", result.Protocol, result.Host, result.Port)
		openPorts = append(openPorts, url)
	}
	return openPorts
}

// ensureCrackDictDir 确保字典目录存在
func ensureCrackDictDir() error {
	dictDir := crackDictDir()
	if _, err := os.Stat(dictDir); err == nil {
		// 已存在，加载状态
		return nil
	}

	// 尝试创建目录
	if err := os.MkdirAll(dictDir, 0o755); err != nil {
		return fmt.Errorf("创建字典目录失败: %w", err)
	}

	return nil
}

// getCrackDictByProtocol 根据协议获取可用的字典文件路径
func getCrackDictByProtocol(protocol string) (string, error) {
	protocolKey := strings.ToLower(protocol)

	// 确保目录存在并加载状态
	if err := ensureCrackDictDir(); err != nil {
		return "", err
	}

	entries, err := appconfig.ListManagedDictFiles("crack")
	if err != nil {
		return "", err
	}

	// 查找文件名包含协议名的字典文件
	for _, entry := range entries {
		// 检查文件名是否包含协议名
		fileName := strings.ToLower(strings.TrimSuffix(entry.Name, ".txt"))
		if strings.Contains(fileName, protocolKey) || strings.Contains(protocolKey, fileName) {
			return entry.Path, nil
		}
	}

	return "", fmt.Errorf("未找到协议 %s 的字典", protocol)
}

func crackDictionaryPathsForProtocol(protocol string) []string {
	files, err := appconfig.ListManagedDictFiles("crack")
	if err != nil {
		return nil
	}
	var meta map[string]struct {
		Protocol string `json:"protocol"`
	}
	if data, err := os.ReadFile(filepath.Join(appconfig.GetCustomDictDir("crack"), "_meta.json")); err == nil {
		_ = json.Unmarshal(data, &meta)
	}
	protocolKey := strings.ToLower(strings.TrimSpace(protocol))
	bundledPaths := make([]string, 0, len(files))
	customPaths := make([]string, 0, len(files))
	for _, file := range files {
		stem := strings.ToLower(strings.TrimSuffix(file.Name, filepath.Ext(file.Name)))
		if stem == protocolKey || (file.Custom && (strings.EqualFold(strings.TrimSpace(meta[file.Name].Protocol), protocolKey) ||
			strings.HasPrefix(stem, protocolKey+"-") || strings.HasPrefix(stem, protocolKey+"_"))) {
			if file.Custom {
				customPaths = append(customPaths, file.Path)
			} else {
				bundledPaths = append(bundledPaths, file.Path)
			}
		}
	}
	if len(customPaths) > 0 {
		return customPaths
	}
	return bundledPaths
}

// parseCrackDicts 解析暴破字典（合并系统字典和用户配置）
// 返回格式：[]string，每个元素是 "user:pass" 格式
// 字典文件格式：user:pass（每行一对）
// config 参数：
//   - username: 用户自定义用户名（\n 分隔）
//   - password: 用户自定义密码（\n 分隔）
//   - defaultDict: 是否启用托管协议字典（true/false）
func parseCrackDicts(protocol string, config map[string]interface{}) []string {
	credentials := make([]string, 0)
	seen := make(map[string]struct{})
	// Keep the dictionary's first-occurrence order: bounded scans may stop
	// before reaching later entries, so map iteration can hide early credentials.
	addCredential := func(credential string) {
		if _, exists := seen[credential]; exists {
			return
		}
		seen[credential] = struct{}{}
		credentials = append(credentials, credential)
	}

	// 1. 检查是否启用系统字典（defaultDict 字段）
	useDefaultDict := false
	if defaultDict, ok := config["defaultDict"].(bool); ok {
		useDefaultDict = defaultDict
	}

	// 2. 同协议有自定义字典时只加载自定义字典，否则加载内置字典。
	if useDefaultDict {
		paths := crackDictionaryPathsForProtocol(protocol)
		for _, dictPath := range paths {
			// 直接读取系统字典文件（格式：user:pass）
			data, err := os.ReadFile(dictPath)
			if err != nil {
				logger.Warning("读取系统字典文件失败: %v", err)
			} else {
				lines := strings.Split(string(data), "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line != "" && !strings.HasPrefix(line, "#") {
						addCredential(line)
					}
				}
				logger.Debug("加载系统字典 %s: %d 条记录", dictPath, len(credentials))
			}
		}
		if len(paths) == 0 {
			logger.Warning("系统字典文件不存在: %s.txt", protocol)
		}
	}

	// 3. 解析用户自定义的用户名和密码，生成笛卡尔积
	userUsernames := []string{}
	userPasswords := []string{}

	// 3.1 解析用户名
	if u, ok := config["username"].(string); ok && u != "" {
		lines := strings.SplitSeq(u, "\n")
		for line := range lines {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				userUsernames = append(userUsernames, trimmed)
			}
		}
	}

	// 3.2 解析密码
	if p, ok := config["password"].(string); ok && p != "" {
		lines := strings.SplitSeq(p, "\n")
		for line := range lines {
			trimmed := strings.TrimSpace(line)
			userPasswords = append(userPasswords, trimmed) // 允许空密码
		}
	}

	// 3.3 生成用户自定义字典的笛卡尔积
	if len(userUsernames) > 0 || len(userPasswords) > 0 {
		// 如果只有用户名，使用空密码
		if len(userPasswords) == 0 {
			userPasswords = []string{""}
		}
		// 如果只有密码，使用空用户名
		if len(userUsernames) == 0 {
			userUsernames = []string{""}
		}

		// 生成笛卡尔积
		for _, user := range userUsernames {
			for _, pass := range userPasswords {
				credential := user + ":" + pass
				addCredential(credential)
			}
		}
	}

	// 4. 如果没有任何字典，使用硬编码的默认字典作为兜底
	if len(credentials) == 0 {
		logger.Warning("未找到任何字典配置，使用硬编码的默认字典")
		credentials = fallbackCrackCredentials(protocol)
	}

	logger.Info("协议 %s 字典加载完成: %d 条记录 (defaultDict=%v)", protocol, len(credentials), useDefaultDict)
	return credentials
}

func fallbackCrackCredentials(protocol string) []string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "clickhouse":
		return []string{
			"default:",
			"default:clickhouse",
			"default:123456",
			"default:default",
			"root:123456",
			"admin:admin",
		}
	case "smtp":
		return []string{
			"admin:admin",
			"admin:123456",
			"postmaster:postmaster",
			"postmaster:123456",
			"mail:mail",
			"test:test",
		}
	case "rabbitmq":
		return []string{
			"guest:guest",
			"admin:admin",
			"admin:123456",
			"rabbitmq:rabbitmq",
			"test:test",
		}
	case "rdp":
		return []string{
			"administrator:",
			"administrator:administrator",
			"administrator:password",
			"administrator:123456",
			"admin:admin",
			"admin:123456",
			"user:user",
			"test:test",
		}
	default:
		return []string{
			"root:123456",
			"root:root",
			"admin:admin",
			"admin:123456",
			"test:test",
			"guest:guest",
		}
	}
}

func crackProtocolNeedsDictionary(protocol string) bool {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "adb", "java-rmi", "jdwp", "memcached", "zookeeper":
		return false
	default:
		return true
	}
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
