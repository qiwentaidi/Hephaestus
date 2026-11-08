package admin

import (
	"context"
	stdnet "net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	gopsnet "github.com/shirou/gopsutil/v3/net"

	"hephaestus-backend/middleware"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源，生产环境应该限制
	},
}

var serviceStartedAt = time.Now()

// SystemStats 系统性能统计数据
type SystemStats struct {
	CPU           float64 `json:"cpu"`           // CPU 使用率百分比
	Mem           float64 `json:"mem"`           // 内存使用率百分比
	MemUsedGB     float64 `json:"memUsedGB"`     // 已用内存（GB）
	MemTotalGB    float64 `json:"memTotalGB"`    // 总内存（GB）
	Disk          float64 `json:"disk"`          // 硬盘使用率百分比
	NetworkRxMbps float64 `json:"networkRxMbps"` // 网络下行吞吐（Mbps）
	NetworkTxMbps float64 `json:"networkTxMbps"` // 网络上行吞吐（Mbps）
	UptimeSec     uint64  `json:"uptimeSec"`     // 系统运行时长（秒）
	DiskUsedGB    float64 `json:"diskUsedGB"`    // 已用磁盘空间（GB）
	DiskFreeGB    float64 `json:"diskFreeGB"`    // 可用磁盘空间（GB）
	DiskTotalGB   float64 `json:"diskTotalGB"`   // 总磁盘空间（GB）
	Platform      string  `json:"platform"`      // 操作系统平台 (linux, darwin, windows)
	Arch          string  `json:"arch"`          // 系统架构 (amd64, arm64)
	OS            string  `json:"os"`            // 操作系统完整信息
	Hostname      string  `json:"hostname"`      // 主机名
}

type systemStatsHistorySample struct {
	SampledAt     time.Time `json:"sampledAt"`
	CPU           float64   `json:"cpu"`
	Mem           float64   `json:"mem"`
	MemUsedGB     float64   `json:"memUsedGB"`
	MemTotalGB    float64   `json:"memTotalGB"`
	Disk          float64   `json:"disk"`
	NetworkRxMbps float64   `json:"networkRxMbps"`
	NetworkTxMbps float64   `json:"networkTxMbps"`
	UptimeSec     uint64    `json:"uptimeSec"`
	DiskUsedGB    float64   `json:"diskUsedGB"`
	DiskFreeGB    float64   `json:"diskFreeGB"`
	DiskTotalGB   float64   `json:"diskTotalGB"`
}

const (
	systemStatsSampleInterval        = 3 * time.Second
	systemStatsHistoryDefaultLimit   = 100
	systemStatsHistoryMaxLimit       = 180
	systemStatsSampleRetention       = 5 * time.Minute
	systemStatsRetentionCleanupEvery = 1 * time.Minute
)

func bytesToGB(value uint64) float64 {
	return float64(value) / (1024 * 1024 * 1024)
}

func getDiskUsage() *disk.UsageStat {
	candidatePaths := []string{"/"}
	if runtime.GOOS == "windows" {
		candidatePaths = candidatePaths[:0]
		if wd, err := os.Getwd(); err == nil {
			if volume := filepath.VolumeName(wd); volume != "" {
				candidatePaths = append(candidatePaths, volume+"\\")
			}
		}
		candidatePaths = append(candidatePaths, "C:\\")
	}

	for _, path := range candidatePaths {
		usage, err := disk.Usage(path)
		if err == nil && usage != nil {
			return usage
		}
	}

	// 兜底：尝试任意可用挂载点
	parts, err := disk.Partitions(false)
	if err != nil {
		return nil
	}
	for _, part := range parts {
		usage, usageErr := disk.Usage(part.Mountpoint)
		if usageErr == nil && usage != nil {
			return usage
		}
	}

	return nil
}

type systemStatsClient struct {
	conn      *websocket.Conn
	writeMu   sync.Mutex
	closeOnce sync.Once
}

func (c *systemStatsClient) writeJSON(v interface{}) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return c.conn.WriteJSON(v)
}

func (c *systemStatsClient) close() {
	c.closeOnce.Do(func() {
		_ = c.conn.Close()
	})
}

// 全局 WebSocket 连接管理
var (
	systemStatsClients   = make(map[*websocket.Conn]*systemStatsClient)
	systemStatsClientsMu sync.RWMutex
	systemStatsBroadcast = make(chan SystemStats, 10)
	systemStatsCtx       context.Context
	systemStatsCancel    context.CancelFunc
	systemStatsLatestMu  sync.RWMutex
	systemStatsLatest    *SystemStats
	networkSampleMu      sync.Mutex
	lastNetworkSample    *networkTrafficSample
)

type networkTrafficSample struct {
	names   []string
	rxBytes uint64
	txBytes uint64
	at      time.Time
}

func registerSystemStatsClient(client *systemStatsClient) {
	systemStatsClientsMu.Lock()
	systemStatsClients[client.conn] = client
	systemStatsClientsMu.Unlock()
}

func removeSystemStatsClient(client *systemStatsClient) {
	systemStatsClientsMu.Lock()
	delete(systemStatsClients, client.conn)
	systemStatsClientsMu.Unlock()
	client.close()
}

func init() {
	// 初始化系统监控上下文
	systemStatsCtx, systemStatsCancel = context.WithCancel(context.Background())

	// 启动广播协程
	go handleSystemStatsBroadcast()

	// 启动性能数据采集协程
	go startSystemStatsEmitter()
}

func setLatestSystemStats(stats *SystemStats) {
	if stats == nil {
		return
	}

	copyValue := *stats
	systemStatsLatestMu.Lock()
	systemStatsLatest = &copyValue
	systemStatsLatestMu.Unlock()
}

func getLatestSystemStats() *SystemStats {
	systemStatsLatestMu.RLock()
	defer systemStatsLatestMu.RUnlock()

	if systemStatsLatest == nil {
		return nil
	}

	copyValue := *systemStatsLatest
	return &copyValue
}

func getOrCollectLatestSystemStats() *SystemStats {
	if stats := getLatestSystemStats(); stats != nil {
		return stats
	}

	stats := getSystemStats()
	setLatestSystemStats(stats)
	return stats
}

func bitsPerSecondToMbps(value float64) float64 {
	return value / 1_000_000
}

func getSelectedNetworkInterfaces() []string {
	ifaceName := strings.TrimSpace(os.Getenv("DEFAULT_NETWORK"))
	if ifaceName != "" && !strings.EqualFold(ifaceName, "auto") {
		return []string{ifaceName}
	}

	ifaces, err := stdnet.Interfaces()
	if err != nil {
		return nil
	}

	selected := make([]string, 0)
	for _, iface := range ifaces {
		if iface.Flags&stdnet.FlagUp == 0 || iface.Flags&stdnet.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		hasIPv4 := false
		for _, addr := range addrs {
			if ipNet, ok := addr.(*stdnet.IPNet); ok && ipNet.IP.To4() != nil {
				hasIPv4 = true
				break
			}
		}

		if hasIPv4 {
			selected = append(selected, iface.Name)
		}
	}

	return selected
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func measureNetworkThroughput(now time.Time) (float64, float64) {
	selectedNames := getSelectedNetworkInterfaces()
	if len(selectedNames) == 0 {
		return 0, 0
	}

	counters, err := gopsnet.IOCounters(true)
	if err != nil {
		return 0, 0
	}

	selectedSet := make(map[string]struct{}, len(selectedNames))
	for _, name := range selectedNames {
		selectedSet[name] = struct{}{}
	}

	var rxBytes uint64
	var txBytes uint64
	for _, counter := range counters {
		if _, ok := selectedSet[counter.Name]; !ok {
			continue
		}
		rxBytes += counter.BytesRecv
		txBytes += counter.BytesSent
	}

	networkSampleMu.Lock()
	defer networkSampleMu.Unlock()

	if lastNetworkSample == nil || !sameStringSlice(lastNetworkSample.names, selectedNames) {
		lastNetworkSample = &networkTrafficSample{
			names:   append([]string(nil), selectedNames...),
			rxBytes: rxBytes,
			txBytes: txBytes,
			at:      now,
		}
		return 0, 0
	}

	elapsedSeconds := now.Sub(lastNetworkSample.at).Seconds()
	if elapsedSeconds <= 0 {
		return 0, 0
	}

	rxDelta := float64(0)
	txDelta := float64(0)
	if rxBytes >= lastNetworkSample.rxBytes {
		rxDelta = float64(rxBytes - lastNetworkSample.rxBytes)
	}
	if txBytes >= lastNetworkSample.txBytes {
		txDelta = float64(txBytes - lastNetworkSample.txBytes)
	}

	lastNetworkSample = &networkTrafficSample{
		names:   append([]string(nil), selectedNames...),
		rxBytes: rxBytes,
		txBytes: txBytes,
		at:      now,
	}

	return bitsPerSecondToMbps((rxDelta * 8) / elapsedSeconds), bitsPerSecondToMbps((txDelta * 8) / elapsedSeconds)
}

// SystemStatsWebSocketHandler WebSocket 处理器
func SystemStatsWebSocketHandler(c *gin.Context) {
	if !websocket.IsWebSocketUpgrade(c.Request) {
		c.Status(http.StatusBadRequest)
		return
	}

	// 从 query 参数获取 token（WebSocket 无法设置 Authorization Header）
	tokenString := c.Query("token")
	if tokenString == "" {
		logger.Warning("[SystemStats] WebSocket 连接失败: 未提供 token")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供 token"})
		return
	}
	// 验证 token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return middleware.GetJWTKey(), nil
	})

	if err != nil || !token.Valid {
		logger.Warning("[SystemStats] WebSocket Token 验证失败: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token 无效"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Warning("[SystemStats] 升级 WebSocket 失败: %v", err)
		return
	}
	client := &systemStatsClient{conn: conn}

	// 注册客户端
	registerSystemStatsClient(client)

	// 清理时移除客户端
	defer func() {
		removeSystemStatsClient(client)
	}()

	// 立即发送一次当前数据
	if stats := getOrCollectLatestSystemStats(); stats != nil {
		if err := client.writeJSON(stats); err != nil {
			logger.Warning("[SystemStats] 发送初始系统性能数据失败: %v", err)
			return
		}
	}

	// 保持连接，监听客户端消息（ping/pong）
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// GetSystemStatsHistoryHandler 获取系统性能历史采样
func GetSystemStatsHistoryHandler(c *gin.Context) {
	limit := systemStatsHistoryDefaultLimit
	if rawLimit := c.Query("limit"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
			if parsed > systemStatsHistoryMaxLimit {
				parsed = systemStatsHistoryMaxLimit
			}
			limit = parsed
		}
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库连接失败"})
		return
	}

	var records []database.SystemStatsSample
	if err := db.
		Order("sampled_at DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取历史采样失败"})
		return
	}

	samples := make([]systemStatsHistorySample, 0, len(records))
	for i := len(records) - 1; i >= 0; i-- {
		record := records[i]
		samples = append(samples, systemStatsHistorySample{
			SampledAt:     record.SampledAt,
			CPU:           record.CPU,
			Mem:           record.Mem,
			MemUsedGB:     record.MemUsedGB,
			MemTotalGB:    record.MemTotalGB,
			Disk:          record.Disk,
			NetworkRxMbps: record.NetworkRxMbps,
			NetworkTxMbps: record.NetworkTxMbps,
			UptimeSec:     record.UptimeSec,
			DiskUsedGB:    record.DiskUsedGB,
			DiskFreeGB:    record.DiskFreeGB,
			DiskTotalGB:   record.DiskTotalGB,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"samples": samples,
			"latest":  getOrCollectLatestSystemStats(),
		},
	})
}

// handleSystemStatsBroadcast 处理广播消息
func handleSystemStatsBroadcast() {
	for {
		select {
		case <-systemStatsCtx.Done():
			return
		case stats := <-systemStatsBroadcast:
			systemStatsClientsMu.RLock()
			clients := make([]*systemStatsClient, 0, len(systemStatsClients))
			for _, client := range systemStatsClients {
				clients = append(clients, client)
			}
			systemStatsClientsMu.RUnlock()

			for _, client := range clients {
				if err := client.writeJSON(stats); err != nil {
					logger.Warning("[SystemStats] 发送数据失败: %v", err)
					removeSystemStatsClient(client)
				}
			}
		}
	}
}

// getSystemStats 获取系统性能数据
func getSystemStats() *SystemStats {
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err != nil || len(cpuPercent) == 0 {
		cpuPercent = []float64{0}
	}

	memStat, err := mem.VirtualMemory()
	memPercent := float64(0)
	memUsedGB := float64(0)
	memTotalGB := float64(0)
	if err == nil && memStat != nil {
		memPercent = memStat.UsedPercent
		memUsedGB = bytesToGB(memStat.Used)
		memTotalGB = bytesToGB(memStat.Total)
	}
	diskUsage := getDiskUsage()
	diskPercent := float64(0)
	diskUsedGB := float64(0)
	diskFreeGB := float64(0)
	diskTotalGB := float64(0)
	if diskUsage != nil {
		diskPercent = diskUsage.UsedPercent
		diskUsedGB = bytesToGB(diskUsage.Used)
		diskFreeGB = bytesToGB(diskUsage.Free)
		diskTotalGB = bytesToGB(diskUsage.Total)
	}
	now := time.Now()
	networkRxMbps, networkTxMbps := measureNetworkThroughput(now)

	// 获取主机信息
	hostInfo, err := host.Info()
	platform := runtime.GOOS
	arch := runtime.GOARCH
	osInfo := platform
	hostname := "unknown"
	uptimeSec := uint64(time.Since(serviceStartedAt).Seconds())

	if err == nil && hostInfo != nil {
		platform = hostInfo.Platform
		osInfo = hostInfo.Platform + " " + hostInfo.PlatformVersion
		hostname = hostInfo.Hostname
	}

	return &SystemStats{
		CPU:           cpuPercent[0],
		Mem:           memPercent,
		MemUsedGB:     memUsedGB,
		MemTotalGB:    memTotalGB,
		Disk:          diskPercent,
		NetworkRxMbps: networkRxMbps,
		NetworkTxMbps: networkTxMbps,
		UptimeSec:     uptimeSec,
		DiskUsedGB:    diskUsedGB,
		DiskFreeGB:    diskFreeGB,
		DiskTotalGB:   diskTotalGB,
		Platform:      platform,
		Arch:          arch,
		OS:            osInfo,
		Hostname:      hostname,
	}
}

func persistSystemStatsSample(stats *SystemStats) {
	if stats == nil {
		return
	}

	db, err := database.GetDB()
	if err != nil {
		logger.Warning("[SystemStats] 获取数据库连接失败: %v", err)
		return
	}

	record := database.SystemStatsSample{
		CPU:           stats.CPU,
		Mem:           stats.Mem,
		MemUsedGB:     stats.MemUsedGB,
		MemTotalGB:    stats.MemTotalGB,
		Disk:          stats.Disk,
		NetworkRxMbps: stats.NetworkRxMbps,
		NetworkTxMbps: stats.NetworkTxMbps,
		UptimeSec:     stats.UptimeSec,
		DiskUsedGB:    stats.DiskUsedGB,
		DiskFreeGB:    stats.DiskFreeGB,
		DiskTotalGB:   stats.DiskTotalGB,
		SampledAt:     time.Now(),
	}

	if err := db.Create(&record).Error; err != nil {
		logger.Warning("[SystemStats] 保存历史采样失败: %v", err)
	}
}

func pruneSystemStatsSamples() {
	db, err := database.GetDB()
	if err != nil {
		logger.Warning("[SystemStats] 获取数据库连接失败: %v", err)
		return
	}

	expireBefore := time.Now().Add(-systemStatsSampleRetention)
	if err := db.
		Where("sampled_at < ?", expireBefore).
		Delete(&database.SystemStatsSample{}).Error; err != nil {
		logger.Warning("[SystemStats] 清理历史采样失败: %v", err)
	}
}

// startSystemStatsEmitter 启动性能数据采集和推送
func startSystemStatsEmitter() {
	ticker := time.NewTicker(systemStatsSampleInterval)
	defer ticker.Stop()
	lastCleanupAt := time.Time{}

	for {
		select {
		case <-systemStatsCtx.Done():
			return
		case <-ticker.C:
			stats := getSystemStats()
			if stats != nil {
				setLatestSystemStats(stats)
				persistSystemStatsSample(stats)

				if lastCleanupAt.IsZero() || time.Since(lastCleanupAt) >= systemStatsRetentionCleanupEvery {
					pruneSystemStatsSamples()
					lastCleanupAt = time.Now()
				}

				systemStatsClientsMu.RLock()
				hasClients := len(systemStatsClients) > 0
				systemStatsClientsMu.RUnlock()

				if hasClients {
					select {
					case systemStatsBroadcast <- *stats:
					default:
						// 广播通道已满，跳过本次更新
					}
				}
			}
		}
	}
}

// StopSystemStatsEmitter 停止性能数据采集（用于优雅关闭）
func StopSystemStatsEmitter() {
	if systemStatsCancel != nil {
		systemStatsCancel()
	}

	// 关闭所有客户端连接
	systemStatsClientsMu.Lock()
	clients := make([]*systemStatsClient, 0, len(systemStatsClients))
	for conn, client := range systemStatsClients {
		clients = append(clients, client)
		delete(systemStatsClients, conn)
	}
	systemStatsClientsMu.Unlock()

	for _, client := range clients {
		client.close()
	}
}
