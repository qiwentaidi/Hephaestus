package portscan

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/synpriv"
	"hephaestus-backend/pkg/types"

	gonmaproot "github.com/qiwentaidi/gonmap"
	gonmap "github.com/qiwentaidi/gonmap/lib"

	"github.com/panjf2000/ants/v2"
)

const (
	maxConcurrentSYNDiscoveryGroups = 4
	defaultSYNDiscoveryAttempts     = 2
)

// PortResultCallback 端口扫描结果回调函数
type PortResultCallback func(result *types.PortInfo)

// ProgressCallback reports display work units. SYN values are NOT a resume cursor.
type ProgressCallback func(progress int32)

type TCPDiscoveryConfig struct {
	Mode     gonmap.PortDiscoveryMode
	Rate     int
	Attempts int
	// Internal, persisted through hostAdmission rather than user configuration.
	HostOrder []string
}

func defaultTCPDiscoveryConfig() TCPDiscoveryConfig {
	return TCPDiscoveryConfig{
		Mode:     gonmap.PortDiscoveryModeConnect,
		Attempts: defaultSYNDiscoveryAttempts,
	}
}

func (config TCPDiscoveryConfig) normalized() TCPDiscoveryConfig {
	if config.Mode == "" {
		config.Mode = gonmap.PortDiscoveryModeConnect
	}
	switch config.Mode {
	case gonmap.PortDiscoveryModeSYN, gonmap.PortDiscoveryModeConnect:
	default:
		config.Mode = gonmap.PortDiscoveryModeConnect
	}
	if config.Attempts <= 0 {
		config.Attempts = defaultSYNDiscoveryAttempts
	} else if config.Attempts > defaultSYNDiscoveryAttempts {
		config.Attempts = defaultSYNDiscoveryAttempts
	}
	return config
}

func (config TCPDiscoveryConfig) portDiscoveryOptions(timeout time.Duration, concurrency int) gonmap.PortDiscoveryOptions {
	config = config.normalized()
	return gonmap.PortDiscoveryOptions{
		Timeout:     timeout,
		Concurrency: concurrency,
		Mode:        config.Mode,
		Rate:        config.Rate,
	}
}

func TcpScan(ctx context.Context, taskId string, addresses <-chan Address, workers, timeout int, proxyURL string, resumeCompleted int32, resultCallback PortResultCallback, progressCallback ProgressCallback) error {
	return TcpScanWithDiscovery(ctx, taskId, addresses, workers, timeout, proxyURL, resumeCompleted, defaultTCPDiscoveryConfig(), resultCallback, progressCallback)
}

func TcpScanWithDiscovery(ctx context.Context, taskId string, addresses <-chan Address, workers, timeout int, proxyURL string, resumeCompleted int32, discoveryConfig TCPDiscoveryConfig, resultCallback PortResultCallback, progressCallback ProgressCallback) error {
	if discoveryConfig.normalized().Mode == gonmap.PortDiscoveryModeSYN {
		if ctx == nil {
			ctx = context.Background()
		}
		ctx = context.WithValue(ctx, synTaskIDKey{}, taskId)
	}

	scanner := gonmap.New(&gonmap.Options{
		Timeout:  time.Second * time.Duration(timeout),
		ProxyURL: proxyURL,
	})
	groups := collectAddressGroups(ctx, addresses)
	prioritizeAddressGroups(groups, discoveryConfig.HostOrder)
	if discoveryConfig.normalized().Mode == gonmap.PortDiscoveryModeSYN && contextError(ctx) != nil {
		return contextError(ctx)
	}
	resumeCompleted = portScanResumeCompleted(discoveryConfig, resumeCompleted)
	groups = trimAddressGroupsForResume(groups, resumeCompleted)
	err := processAddressGroups(ctx, scanner, groups, workers, timeout, resumeCompleted, discoveryConfig, resultCallback, progressCallback)
	if err == nil && discoveryConfig.normalized().Mode == gonmap.PortDiscoveryModeSYN {
		return contextError(ctx)
	}
	return err
}

type Address struct {
	IP       string
	Port     int
	Protocol gonmap.ScanProtocol
}

func parseScanProtocol(protocol string) gonmap.ScanProtocol {
	switch protocol {
	case "udp":
		return gonmap.ScanProtocolUDP
	default:
		return gonmap.ScanProtocolTCP
	}
}

func Connect(scanner *gonmap.SDK, ip string, port int, transport gonmap.ScanProtocol) *types.PortInfo {
	return ConnectWithContext(context.Background(), scanner, ip, port, transport)
}

func ConnectWithContext(ctx context.Context, scanner *gonmap.SDK, ip string, port int, transport gonmap.ScanProtocol) *types.PortInfo {
	status, response := scanner.ScanWithContext(ctx, ip, port, &gonmap.ScanOptions{
		Protocol: transport,
	})

	return buildPortInfo(ip, port, transport, status, response)
}

func buildPortInfo(ip string, port int, transport gonmap.ScanProtocol, status gonmap.Status, response *gonmap.Response) *types.PortInfo {
	// 端口关闭或未知，直接返回 nil
	if status == gonmap.Closed || status == gonmap.Unknown {
		return nil
	}

	var raw string
	var product string
	var version string
	// 默认协议设为 unknown
	protocol := "unknown"
	if response != nil {
		// 提取服务名称
		if response.FingerPrint != nil && response.FingerPrint.Service != "" {
			protocol = response.FingerPrint.Service
		}
		if response.FingerPrint != nil {
			product = response.FingerPrint.ProductName
			version = response.FingerPrint.Version
		}
		// 提取原始响应（banner）
		if response.Raw != "" {
			raw = response.Raw
		}
	}

	if transport == gonmap.ScanProtocolUDP && protocol == "unknown" {
		return nil
	}

	// 如果协议是 unknown，尝试根据端口猜测协议
	if protocol == "unknown" {
		protocol = gonmaproot.GuessProtocol(port)
		if protocol == "" {
			protocol = string(transport)
		}
	}

	return &types.PortInfo{
		Host:        ip,
		Port:        port,
		Transport:   string(transport),
		Protocol:    protocol,
		Product:     product,
		Version:     version,
		RawResponse: raw,
	}
}

type addressGroupKey struct {
	IP       string
	Protocol gonmap.ScanProtocol
}

type addressGroup struct {
	IP       string
	Protocol gonmap.ScanProtocol
	Ports    []int
}

func prioritizeAddressGroups(groups []addressGroup, hosts []string) {
	if len(hosts) == 0 {
		return
	}
	rank := make(map[string]int, len(hosts))
	for i, host := range hosts {
		rank[host] = i
	}
	sort.SliceStable(groups, func(i, j int) bool {
		a, aok := rank[groups[i].IP]
		b, bok := rank[groups[j].IP]
		if aok != bok {
			return aok
		}
		if !aok {
			return false
		}
		return a < b
	})
}

func collectAddressGroups(ctx context.Context, addresses <-chan Address) []addressGroup {
	grouped := make(map[addressGroupKey][]int)
	for add := range addresses {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		key := addressGroupKey{IP: add.IP, Protocol: add.Protocol}
		grouped[key] = append(grouped[key], add.Port)
	}

	results := make([]addressGroup, 0, len(grouped))
	for key, ports := range grouped {
		ports = append([]int(nil), ports...)
		sort.Ints(ports)
		results = append(results, addressGroup{
			IP:       key.IP,
			Protocol: key.Protocol,
			Ports:    ports,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].IP != results[j].IP {
			return results[i].IP < results[j].IP
		}
		return results[i].Protocol < results[j].Protocol
	})
	return results
}

func processAddressGroups(ctx context.Context, scanner *gonmap.SDK, groups []addressGroup, workers, timeout int, resumeCompleted int32, discoveryConfig TCPDiscoveryConfig, resultCallback PortResultCallback, progressCallback ProgressCallback) error {
	if len(groups) == 0 {
		if progressCallback != nil && resumeCompleted > 0 {
			progressCallback(resumeCompleted)
		}
		return nil
	}

	completed := resumeCompleted
	var (
		portCount     = make(map[string]int)
		openPortMutex sync.Mutex
	)

	handleResult := func(pr *types.PortInfo) {
		if pr == nil {
			return
		}
		if pr.DiscoveryStatus == "pending" {
			if resultCallback != nil {
				resultCallback(pr)
			}
			return
		}

		ip := pr.Host
		protocol := pr.Protocol

		openPortMutex.Lock()
		portCount[ip]++
		totalOpen := portCount[ip]
		openPortMutex.Unlock()

		if protocol == "tcp" && totalOpen > 30 {
			return
		}
		if resultCallback != nil {
			resultCallback(pr)
		}
	}

	if discoveryConfig.normalized().Mode == gonmap.PortDiscoveryModeSYN {
		return processSYNAddressGroups(ctx, scanner, groups, workers, timeout, discoveryConfig, &completed, handleResult, progressCallback)
	}

	processAddressGroupsConcurrently(ctx, scanner, groups, workers, timeout, &completed, discoveryConfig, handleResult, progressCallback)
	return nil
}

func processAddressGroupsConcurrently(ctx context.Context, scanner *gonmap.SDK, groups []addressGroup, workers, timeout int, completed *int32, discoveryConfig TCPDiscoveryConfig, handleResult func(*types.PortInfo), progressCallback ProgressCallback) {
	if len(groups) == 0 {
		return
	}
	started := time.Now()
	defer func() {
		if discoveryConfig.normalized().Mode == gonmap.PortDiscoveryModeConnect {
			logger.Info("端口扫描 Connect 阶段耗时: total=%s groups=%d", time.Since(started).Round(time.Millisecond), len(groups))
		}
	}()

	groupWorkers, batchWorkers := splitDiscoveryWorkers(workers, len(groups), discoveryConfig)
	var wg sync.WaitGroup
	threadPool, _ := ants.NewPoolWithFunc(groupWorkers, func(payload interface{}) {
		defer wg.Done()
		group := payload.(addressGroup)

		if ctx != nil && ctx.Err() != nil {
			return
		}

		if group.Protocol == gonmap.ScanProtocolUDP {
			for _, port := range group.Ports {
				if ctx != nil && ctx.Err() != nil {
					return
				}
				handleResult(ConnectWithContext(ctx, scanner, group.IP, port, group.Protocol))
				reportProgress(completed, 1, progressCallback)
			}
			return
		}

		for _, ports := range chunkPorts(group.Ports, batchPortChunkSize(batchWorkers)) {
			if ctx != nil && ctx.Err() != nil {
				return
			}

			batchOptions := &gonmap.BatchScanOptions{
				DiscoveryOptions: discoveryConfig.portDiscoveryOptions(time.Second*time.Duration(timeout), batchWorkers),
				FingerprintOptions: gonmap.ScanOptions{
					Timeout:  time.Second * time.Duration(timeout),
					Protocol: group.Protocol,
				},
				FingerprintWorkers:      batchWorkers,
				SuspiciousHighPortStart: 1024,
			}
			batchResult := scanHostPortsWithDiscovery(ctx, scanner, group.IP, ports, batchOptions, discoveryConfig)
			if batchResult != nil {
				for _, item := range batchResult.Results {
					handleResult(batchPortResultToPortInfo(group.IP, group.Protocol, item, batchResult.LikelyNoise))
				}
			}
			reportProgress(completed, int32(len(ports)), progressCallback)
		}
	})
	defer threadPool.Release()

	for _, group := range groups {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		wg.Add(1)
		if err := threadPool.Invoke(group); err != nil {
			wg.Done()
			continue
		}
	}
	wg.Wait()
}

type synDiscoveryRuntime struct {
	hasRawSocket bool
	useHelper    bool
	helperStatus synpriv.HelperStatus
}

type synDiscoveryTimings struct {
	rounds      []time.Duration
	roundOpen   []int
	helperCalls int
}

type synFingerprintOutcome struct {
	results               map[string]gonmap.HostBatchResult
	verified              map[string][]int
	timedOut              map[string][]int
	fingerprintDuration   time.Duration
	verificationDuration  time.Duration
	candidates            int
	fallbackVerifications int
}

type portDiscoveryBatch struct {
	Hosts []string
	Ports []int
	Total int
}

func processSYNAddressGroups(ctx context.Context, scanner *gonmap.SDK, groups []addressGroup, workers, timeout int, discoveryConfig TCPDiscoveryConfig, completed *int32, handleResult func(*types.PortInfo), progressCallback ProgressCallback) error {
	discoveryConfig = discoveryConfig.normalized()
	tcpGroups, otherGroups := splitTCPAddressGroups(groups)
	if len(tcpGroups) == 0 {
		if len(otherGroups) > 0 {
			processAddressGroupsConcurrently(ctx, scanner, otherGroups, workers, timeout, completed, TCPDiscoveryConfig{
				Mode: gonmap.PortDiscoveryModeConnect,
			}, handleResult, progressCallback)
		}
		return nil
	}

	if workers <= 0 {
		workers = 1
	}
	started := time.Now()
	runtime := resolveSYNDiscoveryRuntime(ctx)
	runtimeDuration := time.Since(started)
	var discoveryDuration, fingerprintDuration, verificationDuration time.Duration
	var candidates, verified, postFilter, fallbackVerifications, timedOut int
	discoveryTimings := synDiscoveryTimings{
		rounds:    make([]time.Duration, defaultSYNDiscoveryAttempts),
		roundOpen: make([]int, defaultSYNDiscoveryAttempts),
	}
	batches := buildPortDiscoveryBatches(tcpGroups, batchPortChunkSize(workers))
	defer func() {
		logger.Info("端口扫描 SYN 阶段耗时: total=%s runtime=%s discovery=%s synRound1=%s synRound2=%s fingerprint=%s fallbackVerify=%s batches=%d round1Open=%d round2Open=%d candidates=%d verified=%d connectTimedOut=%d postFilter=%d fallbackChecks=%d helper=%t helperCalls=%d configuredAttempts=%d ctxErr=%v task=%s",
			time.Since(started).Round(time.Millisecond), runtimeDuration.Round(time.Millisecond), discoveryDuration.Round(time.Millisecond),
			discoveryTimings.rounds[0].Round(time.Millisecond), discoveryTimings.rounds[1].Round(time.Millisecond),
			fingerprintDuration.Round(time.Millisecond), verificationDuration.Round(time.Millisecond), len(batches), discoveryTimings.roundOpen[0], discoveryTimings.roundOpen[1], candidates, verified, timedOut, postFilter, fallbackVerifications, runtime.useHelper, discoveryTimings.helperCalls, discoveryConfig.Attempts, contextError(ctx), synTaskID(ctx))
	}()
	for _, batch := range batches {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}

		batchOptions := &gonmap.BatchScanOptions{
			DiscoveryOptions: discoveryConfig.portDiscoveryOptions(time.Second*time.Duration(timeout), workers),
			FingerprintOptions: gonmap.ScanOptions{
				Timeout:  time.Second * time.Duration(timeout),
				Protocol: gonmap.ScanProtocolTCP,
			},
			FingerprintWorkers:      workers,
			SuspiciousHighPortStart: 1024,
		}
		// Reserve 20% for response collection/fingerprinting/verification. This
		// counter is display-only; scanner.go persists a separate safe resume cursor.
		batchReported := int32(0)
		batchOptions.DiscoveryOptions.OnProgress = func(attempted, total int) {
			target := synDisplayWork(batch.Total, attempted, total)
			if target > batchReported {
				reportProgress(completed, target-batchReported, progressCallback)
				batchReported = target
			}
		}
		unreachable := 0
		batchOptions.DiscoveryOptions.OnHostUnreachable = func(host string, err error) {
			unreachable++
			if unreachable <= 5 {
				logger.Warning("SYN 本轮无法送达: task=%s host=%s error=%v", synTaskID(ctx), host, err)
			}
		}
		lastDiagnostic := time.Time{}
		batchOptions.DiscoveryOptions.OnSYNDiagnostics = func(d gonmaproot.SYNDiscoveryDiagnostics) {
			if !d.Done && time.Since(lastDiagnostic) < 10*time.Second {
				return
			}
			lastDiagnostic = time.Now()
			logger.Info("SYN 发包诊断: task=%s attempted=%d total=%d sent=%d failedAttempts=%d skipped=%d unreachable=%d preparedHosts=%d prepareFailures=%d neighborAttempts=%d neighborCacheHits=%d sendHandles=%d elapsed=%s prepare=%s source=%s send=%s route=%s write=%s done=%t error=%q lastError=%q",
				synTaskID(ctx), d.Attempted, d.Total, d.Sent, d.Failed, d.Skipped, d.UnreachableHosts, d.PreparedHosts, d.PrepareFailures, d.NeighborAttempts, d.NeighborCacheHits, d.SendHandles, d.Elapsed.Round(time.Millisecond), d.PrepareDuration.Round(time.Millisecond), d.SourceDuration.Round(time.Millisecond), d.SendDuration.Round(time.Millisecond), d.RouteDuration.Round(time.Millisecond), d.WriteDuration.Round(time.Millisecond), d.Done, d.Error, d.LastError)
		}
		discoveryStarted := time.Now()
		onOpen := func(port gonmap.OpenPort) {
			handleResult(&types.PortInfo{Host: port.Host, Port: port.Port, Transport: "tcp", Protocol: "unknown", DiscoveryStatus: "pending"})
		}
		openPortsByHost, err := discoverOpenPortsForBatch(ctx, scanner, batch.Hosts, batch.Ports, batchOptions.DiscoveryOptions, runtime, discoveryConfig.Attempts, &discoveryTimings, onOpen)
		discoveryDuration += time.Since(discoveryStarted)
		if err != nil {
			return err
		}
		if err := contextError(ctx); err != nil {
			return err
		}
		outcome := fingerprintOpenPortsByHost(ctx, scanner, openPortsByHost, batchOptions)
		fingerprintDuration += outcome.fingerprintDuration
		verificationDuration += outcome.verificationDuration
		candidates += outcome.candidates
		if err := contextError(ctx); err != nil {
			return err
		}
		for _, ports := range outcome.verified {
			verified += len(ports)
		}
		for _, ports := range outcome.timedOut {
			timedOut += len(ports)
		}
		fallbackVerifications += outcome.fallbackVerifications
		for _, host := range batch.Hosts {
			hostResult := outcome.results[host]
			hostPostFilter := 0
			for _, item := range hostResult.Results {
				result := batchPortResultToPortInfoWithVerifiedOpen(host, gonmap.ScanProtocolTCP, item, outcome.verified, outcome.timedOut, hostResult.LikelyNoise)
				if result != nil {
					postFilter++
					hostPostFilter++
				}
				handleResult(result)
			}
			if len(batch.Hosts) <= 4 {
				logger.Info("端口扫描 SYN 主机计数: host=%s candidate=%d verified=%d connectTimedOut=%d postFilter=%d likelyNoise=%t",
					host, len(openPortsByHost[host]), len(outcome.verified[host]), len(outcome.timedOut[host]), hostPostFilter, hostResult.LikelyNoise)
			}
		}
		reportProgress(completed, int32(batch.Total)-batchReported, progressCallback)
	}
	if len(otherGroups) > 0 {
		processAddressGroupsConcurrently(ctx, scanner, otherGroups, workers, timeout, completed, TCPDiscoveryConfig{
			Mode: gonmap.PortDiscoveryModeConnect,
		}, handleResult, progressCallback)
	}
	return nil
}

func splitTCPAddressGroups(groups []addressGroup) ([]addressGroup, []addressGroup) {
	tcpGroups := make([]addressGroup, 0, len(groups))
	otherGroups := make([]addressGroup, 0)
	for _, group := range groups {
		if group.Protocol == gonmap.ScanProtocolTCP {
			tcpGroups = append(tcpGroups, group)
		} else {
			otherGroups = append(otherGroups, group)
		}
	}
	return tcpGroups, otherGroups
}

func buildPortDiscoveryBatches(groups []addressGroup, chunkSize int) []portDiscoveryBatch {
	batchByKey := make(map[string]int)
	batches := make([]portDiscoveryBatch, 0)
	for _, group := range groups {
		for _, ports := range chunkPorts(group.Ports, chunkSize) {
			key := portChunkKey(ports)
			index, ok := batchByKey[key]
			if !ok {
				copiedPorts := append([]int(nil), ports...)
				batches = append(batches, portDiscoveryBatch{Ports: copiedPorts})
				index = len(batches) - 1
				batchByKey[key] = index
			}
			batches[index].Hosts = append(batches[index].Hosts, group.IP)
			batches[index].Total += len(ports)
		}
	}
	return batches
}

func portChunkKey(ports []int) string {
	var builder strings.Builder
	for index, port := range ports {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.Itoa(port))
	}
	return builder.String()
}

func resolveSYNDiscoveryRuntime(ctx context.Context) synDiscoveryRuntime {
	if synpriv.CurrentProcessHasRawSocketCapability() {
		return synDiscoveryRuntime{hasRawSocket: true}
	}
	status := synpriv.Status(ctx)
	return synDiscoveryRuntime{useHelper: status.Usable, helperStatus: status}
}

func discoverOpenPortsForBatch(ctx context.Context, scanner *gonmap.SDK, hosts []string, ports []int, options gonmap.PortDiscoveryOptions, runtime synDiscoveryRuntime, attempts int, timings *synDiscoveryTimings, onOpen func(gonmap.OpenPort)) (map[string][]int, error) {
	if runtime.useHelper {
		openPorts, err := discoverOpenPortsWithHelperAttempts(ctx, hosts, ports, options, runtime.helperStatus, attempts, timings, onOpen)
		if err == nil {
			return groupOpenPortsByHost(hosts, openPorts), nil
		}
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("SYN 授权辅助程序执行失败：%w。请检查 SYN 授权状态后重试", err)
	}

	if !runtime.hasRawSocket {
		return nil, synAuthorizationError(runtime.helperStatus)
	}

	var discoveryErr error
	onDiagnostic := options.OnSYNDiagnostics
	options.OnSYNDiagnostics = func(d gonmaproot.SYNDiscoveryDiagnostics) {
		if d.Done {
			discoveryErr = nil
			if d.Error != "" {
				discoveryErr = fmt.Errorf("SYN discovery: %s", d.Error)
			}
		}
		if onDiagnostic != nil {
			onDiagnostic(d)
		}
	}
	openPorts := discoverOpenPortsWithNativeSYNAttempts(ctx, scanner, hosts, ports, options, attempts, timings, onOpen)
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if discoveryErr != nil {
		return nil, discoveryErr
	}
	return groupOpenPortsByHost(hosts, openPorts), nil
}

func synAuthorizationError(status synpriv.HelperStatus) error {
	if runtime.GOOS == "windows" {
		return errors.New("SYN 扫描不可用：当前进程缺少原始套接字能力。请安装或检查 Npcap 后重试")
	}
	reason := status.Error
	if reason == "" {
		switch {
		case !status.Exists:
			reason = "SYN 授权辅助程序未安装"
		case !status.RootOwned:
			reason = "SYN 授权辅助程序未由 root 持有"
		case !status.Setuid:
			reason = "SYN 授权辅助程序未设置 setuid 权限"
		default:
			reason = "SYN 授权辅助程序不可用"
		}
	}
	return fmt.Errorf("SYN 扫描不可用：%s。请在端口扫描设置中重新授权 SYN 后重试", reason)
}

func discoverOpenPortsWithHelperAttempts(ctx context.Context, hosts []string, ports []int, options gonmap.PortDiscoveryOptions, status synpriv.HelperStatus, attempts int, timings *synDiscoveryTimings, onOpen func(gonmap.OpenPort)) ([]gonmap.OpenPort, error) {
	if attempts <= 0 {
		attempts = 1
	}
	var (
		merged  []gonmap.OpenPort
		lastErr error
	)
	for attempt := 0; attempt < attempts; attempt++ {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		attemptStarted := time.Now()
		openPorts, err := discoverOpenPortsWithHelperStatusCallback(ctx, hosts, ports, options, status, onOpen)
		if timings != nil {
			timings.helperCalls++
			if attempt < len(timings.rounds) {
				timings.rounds[attempt] += time.Since(attemptStarted)
			}
		}
		if err != nil {
			lastErr = err
			continue
		}
		lastErr = nil
		if timings != nil && attempt < len(timings.roundOpen) {
			timings.roundOpen[attempt] += len(openPorts)
		}
		merged = mergeOpenPorts(merged, openPorts)
	}
	if lastErr != nil {
		return merged, lastErr
	}
	return merged, nil
}

func discoverOpenPortsWithNativeSYNAttempts(ctx context.Context, scanner *gonmap.SDK, hosts []string, ports []int, options gonmap.PortDiscoveryOptions, attempts int, timings *synDiscoveryTimings, onOpen func(gonmap.OpenPort)) []gonmap.OpenPort {
	return runNativeSYNAttempts(ctx, hosts, ports, options, attempts, timings, onOpen, scanner.DiscoverOpenPortsWithContextCallback)
}

func runNativeSYNAttempts(ctx context.Context, hosts []string, ports []int, options gonmap.PortDiscoveryOptions, attempts int, timings *synDiscoveryTimings, onOpen func(gonmap.OpenPort), discover func(context.Context, []string, []int, *gonmap.PortDiscoveryOptions, func(gonmap.OpenPort)) []gonmap.OpenPort) []gonmap.OpenPort {
	if attempts <= 0 {
		attempts = 1
	}
	merged := make([]gonmap.OpenPort, 0)
	onProgress := options.OnProgress
	budget := synRetryBudget(len(hosts)*len(ports), options.Rate, options.Timeout)
	for attempt := 0; attempt < attempts; attempt++ {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		attemptStarted := time.Now()
		round := attempt + 1
		roundOptions := options
		roundOptions.OnProgress = func(n, total int) {
			if onProgress != nil {
				onProgress(attempt*total+n, attempts*total)
			}
		}
		logger.Info("SYN 轮次开始: task=%s round=%d/%d probes=%d rate=%d timeout=%s", synTaskID(ctx), round, attempts, len(hosts)*len(ports), options.Rate, options.Timeout)
		stopHeartbeat := startSYNHeartbeat(ctx, round)
		openPorts := func() []gonmap.OpenPort {
			defer stopHeartbeat()
			return discover(ctx, hosts, ports, &roundOptions, onOpen)
		}()
		elapsed := time.Since(attemptStarted)
		logger.Info("SYN 轮次退出: task=%s round=%d elapsed=%s returnedOpen=%d ctxErr=%v", synTaskID(ctx), round, elapsed.Round(time.Millisecond), len(openPorts), contextError(ctx))
		if elapsed > budget && contextError(ctx) == nil {
			logger.Warning("SYN 轮次超出预期: task=%s round=%d elapsed=%s threshold=%s；保留配置的重扫次数，慢速不代表重扫无收益", synTaskID(ctx), round, elapsed.Round(time.Second), budget.Round(time.Second))
		}
		merged = mergeOpenPorts(merged, openPorts)
		if timings != nil && attempt < len(timings.rounds) {
			timings.rounds[attempt] += time.Since(attemptStarted)
			timings.roundOpen[attempt] += len(openPorts)
		}
	}
	return merged
}

func mergeOpenPorts(left, right []gonmap.OpenPort) []gonmap.OpenPort {
	seen := make(map[string]struct{}, len(left)+len(right))
	merged := make([]gonmap.OpenPort, 0, len(left)+len(right))
	add := func(items []gonmap.OpenPort) {
		for _, item := range items {
			key := item.Host + "\x00" + strconv.Itoa(item.Port)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, item)
		}
	}
	add(left)
	add(right)
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Host != merged[j].Host {
			return merged[i].Host < merged[j].Host
		}
		return merged[i].Port < merged[j].Port
	})
	return merged
}

type synConnectVerification struct {
	open     map[string][]int
	timedOut map[string][]int
}

func verifyOpenPortsByConnect(ctx context.Context, openPortsByHost map[string][]int, timeout time.Duration, workers int) synConnectVerification {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = time.Second
	}
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: -1}
	return verifyOpenPortsByConnectWithDial(ctx, openPortsByHost, workers, dialer.DialContext)
}

func verifyOpenPortsByConnectWithDial(ctx context.Context, openPortsByHost map[string][]int, workers int, dial func(context.Context, string, string) (net.Conn, error)) synConnectVerification {
	if ctx == nil {
		ctx = context.Background()
	}
	if workers <= 0 {
		workers = 1
	}

	type job struct {
		host string
		port int
	}
	total := 0
	for _, ports := range openPortsByHost {
		total += len(ports)
	}
	result := synConnectVerification{
		open:     emptyOpenPortsByHost(sortedOpenPortHosts(openPortsByHost)),
		timedOut: emptyOpenPortsByHost(sortedOpenPortHosts(openPortsByHost)),
	}
	if total == 0 {
		return result
	}
	if workers > total {
		workers = total
	}

	jobs := make(chan job, total)
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				if ctx != nil && ctx.Err() != nil {
					return
				}
				conn, err := dial(ctx, "tcp", net.JoinHostPort(item.host, strconv.Itoa(item.port)))
				if err != nil {
					if isSYNConnectTimeout(ctx, err) {
						mu.Lock()
						result.timedOut[item.host] = append(result.timedOut[item.host], item.port)
						mu.Unlock()
					}
					continue
				}
				_ = conn.Close()
				mu.Lock()
				result.open[item.host] = append(result.open[item.host], item.port)
				mu.Unlock()
			}
		}()
	}

	for _, host := range sortedOpenPortHosts(openPortsByHost) {
		for _, port := range openPortsByHost[host] {
			if ctx != nil && ctx.Err() != nil {
				break
			}
			jobs <- job{host: host, port: port}
		}
	}
	close(jobs)
	wg.Wait()

	for host := range result.open {
		sort.Ints(result.open[host])
		sort.Ints(result.timedOut[host])
	}
	return result
}

func isSYNConnectTimeout(ctx context.Context, err error) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func groupOpenPortsByHost(hosts []string, openPorts []gonmap.OpenPort) map[string][]int {
	grouped := emptyOpenPortsByHost(hosts)
	for _, item := range openPorts {
		if _, ok := grouped[item.Host]; !ok {
			continue
		}
		grouped[item.Host] = append(grouped[item.Host], item.Port)
	}
	for host := range grouped {
		sort.Ints(grouped[host])
	}
	return grouped
}

func emptyOpenPortsByHost(hosts []string) map[string][]int {
	grouped := make(map[string][]int, len(hosts))
	for _, host := range hosts {
		grouped[host] = nil
	}
	return grouped
}

func fingerprintOpenPortsByHost(ctx context.Context, scanner *gonmap.SDK, openPortsByHost map[string][]int, options *gonmap.BatchScanOptions) synFingerprintOutcome {
	outcome := synFingerprintOutcome{}
	resultsByHost := make(map[string][]gonmap.BatchPortResult, len(openPortsByHost))
	totalOpenPorts := 0
	for host, ports := range openPortsByHost {
		resultsByHost[host] = nil
		totalOpenPorts += len(ports)
	}
	outcome.candidates = totalOpenPorts
	if totalOpenPorts == 0 {
		outcome.verified = openPortsByHost
		outcome.results = analyzeOpenPortsByHost(openPortsByHost, resultsByHost, options)
		return outcome
	}

	workers := options.FingerprintWorkers
	if workers <= 0 {
		workers = 1
	}
	if workers > totalOpenPorts {
		workers = totalOpenPorts
	}

	type job struct {
		host string
		port int
	}
	jobs := make(chan job, totalOpenPorts)
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	fingerprintStarted := time.Now()
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				if ctx != nil && ctx.Err() != nil {
					return
				}
				status, response := scanner.ScanWithContext(ctx, item.host, item.port, &options.FingerprintOptions)
				mu.Lock()
				resultsByHost[item.host] = append(resultsByHost[item.host], gonmap.BatchPortResult{
					Port:     item.port,
					Status:   status,
					Response: response,
				})
				mu.Unlock()
			}
		}()
	}

	hosts := sortedOpenPortHosts(openPortsByHost)
	for _, host := range hosts {
		for _, port := range openPortsByHost[host] {
			if ctx != nil && ctx.Err() != nil {
				break
			}
			jobs <- job{host: host, port: port}
		}
	}
	close(jobs)
	wg.Wait()
	outcome.fingerprintDuration = time.Since(fingerprintStarted)

	outcome.verified, outcome.timedOut, resultsByHost, outcome.verificationDuration, outcome.fallbackVerifications = verifySYNFingerprintCandidates(ctx, hosts, resultsByHost, options.DiscoveryOptions)
	outcome.results = analyzeOpenPortsByHost(outcome.verified, resultsByHost, options)
	return outcome
}

func verifySYNFingerprintCandidates(ctx context.Context, hosts []string, resultsByHost map[string][]gonmap.BatchPortResult, options gonmap.PortDiscoveryOptions) (map[string][]int, map[string][]int, map[string][]gonmap.BatchPortResult, time.Duration, int) {
	fallbackByHost := emptyOpenPortsByHost(hosts)
	verifiedByHost := emptyOpenPortsByHost(hosts)
	fallbackChecks := 0
	for host, results := range resultsByHost {
		for _, item := range results {
			if item.Status == gonmap.Closed || item.Status == gonmap.Unknown {
				fallbackByHost[host] = append(fallbackByHost[host], item.Port)
				fallbackChecks++
			} else {
				verifiedByHost[host] = append(verifiedByHost[host], item.Port)
			}
		}
	}
	verificationStarted := time.Now()
	fallbackVerified := verifyOpenPortsByConnect(ctx, fallbackByHost, options.Timeout, options.Concurrency)
	verificationDuration := time.Since(verificationStarted)
	for host, ports := range fallbackVerified.open {
		verifiedByHost[host] = append(verifiedByHost[host], ports...)
	}
	for host, ports := range fallbackVerified.timedOut {
		verifiedByHost[host] = append(verifiedByHost[host], ports...)
	}
	for host := range verifiedByHost {
		sort.Ints(verifiedByHost[host])
		verifiedResults := resultsByHost[host][:0]
		for _, item := range resultsByHost[host] {
			if portIsOpenForHost(verifiedByHost, host, item.Port) {
				verifiedResults = append(verifiedResults, item)
			}
		}
		resultsByHost[host] = verifiedResults
	}
	return verifiedByHost, fallbackVerified.timedOut, resultsByHost, verificationDuration, fallbackChecks
}

func analyzeOpenPortsByHost(openPortsByHost map[string][]int, resultsByHost map[string][]gonmap.BatchPortResult, options *gonmap.BatchScanOptions) map[string]gonmap.HostBatchResult {
	hostResults := make(map[string]gonmap.HostBatchResult, len(openPortsByHost))
	for host, results := range resultsByHost {
		sort.Slice(results, func(i, j int) bool {
			return results[i].Port < results[j].Port
		})
		analyzed := gonmap.AnalyzeHostResults(host, openPortsByHost[host], results, options)
		hostResults[host] = analyzed
	}
	return hostResults
}

func sortedOpenPortHosts(openPortsByHost map[string][]int) []string {
	hosts := make([]string, 0, len(openPortsByHost))
	for host := range openPortsByHost {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	return hosts
}

func portIsOpenForHost(openPortsByHost map[string][]int, host string, port int) bool {
	ports, ok := openPortsByHost[host]
	if !ok {
		return false
	}
	index := sort.SearchInts(ports, port)
	return index < len(ports) && ports[index] == port
}

func scanHostPortsWithDiscovery(ctx context.Context, scanner *gonmap.SDK, host string, ports []int, options *gonmap.BatchScanOptions, discoveryConfig TCPDiscoveryConfig) *gonmap.HostBatchResult {
	if shouldUseSYNHelper(discoveryConfig) {
		openPorts, err := discoverOpenPortsWithHelper(ctx, []string{host}, ports, options.DiscoveryOptions)
		if err == nil {
			openPortsByHost := groupOpenPortsByHost([]string{host}, openPorts)
			return fingerprintOpenPorts(ctx, scanner, host, openPortsByHost[host], options)
		}
		return &gonmap.HostBatchResult{Host: host}
	}
	return scanner.ScanHostPortsWithContext(ctx, host, ports, options)
}

func shouldUseSYNHelper(discoveryConfig TCPDiscoveryConfig) bool {
	if discoveryConfig.normalized().Mode != gonmap.PortDiscoveryModeSYN {
		return false
	}
	if synpriv.CurrentProcessHasRawSocketCapability() {
		return false
	}
	return synpriv.Status(context.Background()).Usable
}

func discoverOpenPortsWithHelper(ctx context.Context, hosts []string, ports []int, options gonmap.PortDiscoveryOptions) ([]gonmap.OpenPort, error) {
	return discoverOpenPortsWithHelperStatus(ctx, hosts, ports, options, synpriv.Status(ctx))
}

func discoverOpenPortsWithHelperStatus(ctx context.Context, hosts []string, ports []int, options gonmap.PortDiscoveryOptions, status synpriv.HelperStatus) ([]gonmap.OpenPort, error) {
	timeoutMS := int(options.Timeout / time.Millisecond)
	if timeoutMS <= 0 {
		timeoutMS = 1000
	}
	results, err := synpriv.DiscoverOpenPortsWithStatus(ctx, status, synpriv.DiscoveryRequest{
		Hosts:       hosts,
		Ports:       ports,
		TimeoutMS:   timeoutMS,
		Concurrency: options.Concurrency,
		Rate:        options.Rate,
	})
	if err != nil {
		return nil, err
	}
	openPorts := make([]gonmap.OpenPort, 0, len(results))
	for _, item := range results {
		openPorts = append(openPorts, gonmap.OpenPort{Host: item.Host, Port: item.Port})
	}
	sort.Slice(openPorts, func(i, j int) bool {
		if openPorts[i].Host != openPorts[j].Host {
			return openPorts[i].Host < openPorts[j].Host
		}
		return openPorts[i].Port < openPorts[j].Port
	})
	return openPorts, nil
}

func discoverOpenPortsWithHelperStatusCallback(ctx context.Context, hosts []string, ports []int, options gonmap.PortDiscoveryOptions, status synpriv.HelperStatus, onOpen func(gonmap.OpenPort)) ([]gonmap.OpenPort, error) {
	timeoutMS := int(options.Timeout / time.Millisecond)
	if timeoutMS <= 0 {
		timeoutMS = 1000
	}
	results, err := synpriv.DiscoverOpenPortsWithStatusCallback(ctx, status, synpriv.DiscoveryRequest{
		Hosts: hosts, Ports: ports, TimeoutMS: timeoutMS,
		Concurrency: options.Concurrency, Rate: options.Rate,
	}, func(item synpriv.OpenPort) {
		if onOpen != nil {
			onOpen(gonmap.OpenPort{Host: item.Host, Port: item.Port})
		}
	})
	openPorts := make([]gonmap.OpenPort, 0, len(results))
	for _, item := range results {
		openPorts = append(openPorts, gonmap.OpenPort{Host: item.Host, Port: item.Port})
	}
	return openPorts, err
}

func fingerprintOpenPorts(ctx context.Context, scanner *gonmap.SDK, host string, openPorts []int, options *gonmap.BatchScanOptions) *gonmap.HostBatchResult {
	if len(openPorts) == 0 {
		return &gonmap.HostBatchResult{Host: host}
	}

	workers := options.FingerprintWorkers
	if workers <= 0 {
		workers = 1
	}
	if workers > len(openPorts) {
		workers = len(openPorts)
	}

	jobs := make(chan int, len(openPorts))
	results := make([]gonmap.BatchPortResult, 0, len(openPorts))
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for port := range jobs {
				if ctx != nil && ctx.Err() != nil {
					return
				}
				status, response := scanner.ScanWithContext(ctx, host, port, &options.FingerprintOptions)
				mu.Lock()
				results = append(results, gonmap.BatchPortResult{
					Port:     port,
					Status:   status,
					Response: response,
				})
				mu.Unlock()
			}
		}()
	}

	for _, port := range openPorts {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		jobs <- port
	}
	close(jobs)
	wg.Wait()

	sort.Slice(results, func(i, j int) bool {
		return results[i].Port < results[j].Port
	})

	analyzed := gonmap.AnalyzeHostResults(host, openPorts, results, options)
	return &analyzed
}

func batchPortResultToPortInfo(ip string, transport gonmap.ScanProtocol, item gonmap.BatchPortResult, hostLikelyNoise bool) *types.PortInfo {
	if hostLikelyNoise && (item.Suspicious || item.LikelyWrapped) {
		return nil
	}
	return buildPortInfo(ip, item.Port, transport, item.Status, item.Response)
}

func batchPortResultToPortInfoWithVerifiedOpen(ip string, transport gonmap.ScanProtocol, item gonmap.BatchPortResult, openPortsByHost, timedOutByHost map[string][]int, hostLikelyNoise bool) *types.PortInfo {
	if portIsOpenForHost(timedOutByHost, ip, item.Port) {
		return &types.PortInfo{Host: ip, Port: item.Port, Transport: string(transport), Protocol: "unknown"}
	}
	if hostLikelyNoise && (item.Suspicious || item.LikelyWrapped) {
		return nil
	}
	result := buildPortInfo(ip, item.Port, transport, item.Status, item.Response)
	if result != nil {
		return result
	}
	if item.Status != gonmap.Closed && item.Status != gonmap.Unknown {
		return nil
	}
	if !portIsOpenForHost(openPortsByHost, ip, item.Port) {
		return nil
	}
	return buildPortInfo(ip, item.Port, transport, gonmap.Open, nil)
}

func trimAddressGroupsForResume(groups []addressGroup, resumeCompleted int32) []addressGroup {
	if resumeCompleted <= 0 {
		return groups
	}

	remaining := int(resumeCompleted)
	trimmed := make([]addressGroup, 0, len(groups))
	for _, group := range groups {
		if remaining <= 0 {
			trimmed = append(trimmed, group)
			continue
		}
		if remaining >= len(group.Ports) {
			remaining -= len(group.Ports)
			continue
		}
		group.Ports = append([]int(nil), group.Ports[remaining:]...)
		remaining = 0
		trimmed = append(trimmed, group)
	}
	return trimmed
}

func splitBatchWorkers(workers, groups int) (int, int) {
	if workers <= 0 {
		workers = 1
	}
	if groups <= 0 {
		return 1, workers
	}

	groupWorkers := workers
	if groupWorkers > groups {
		groupWorkers = groups
	}

	batchWorkers := (workers + groupWorkers - 1) / groupWorkers
	if batchWorkers < 1 {
		batchWorkers = 1
	}

	return groupWorkers, batchWorkers
}

func splitDiscoveryWorkers(workers, groups int, discoveryConfig TCPDiscoveryConfig) (int, int) {
	groupWorkers, batchWorkers := splitBatchWorkers(workers, groups)
	if discoveryConfig.normalized().Mode != gonmap.PortDiscoveryModeSYN {
		return groupWorkers, batchWorkers
	}
	if groupWorkers <= maxConcurrentSYNDiscoveryGroups {
		return groupWorkers, batchWorkers
	}

	groupWorkers = maxConcurrentSYNDiscoveryGroups
	if groupWorkers > groups {
		groupWorkers = groups
	}
	batchWorkers = (workers + groupWorkers - 1) / groupWorkers
	if batchWorkers < 1 {
		batchWorkers = 1
	}
	return groupWorkers, batchWorkers
}

func batchPortChunkSize(workers int) int {
	switch {
	case workers <= 0:
		return 512
	case workers*8 < 512:
		return 512
	case workers*8 > 4096:
		return 4096
	default:
		return workers * 8
	}
}

func chunkPorts(ports []int, chunkSize int) [][]int {
	if len(ports) == 0 {
		return nil
	}
	if chunkSize <= 0 || len(ports) <= chunkSize {
		return [][]int{ports}
	}

	chunks := make([][]int, 0, (len(ports)+chunkSize-1)/chunkSize)
	for start := 0; start < len(ports); start += chunkSize {
		end := start + chunkSize
		if end > len(ports) {
			end = len(ports)
		}
		chunks = append(chunks, ports[start:end])
	}
	return chunks
}

func reportProgress(completed *int32, delta int32, progressCallback ProgressCallback) {
	current := atomic.AddInt32(completed, delta)
	if progressCallback != nil {
		progressCallback(current)
	}
}
