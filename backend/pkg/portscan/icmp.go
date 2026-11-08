package portscan

import (
	"bytes"
	"context"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/icmp"

	"hephaestus-backend/pkg/logger"
)

// CheckLive 检测主机存活状态
func CheckLive(hostslist []string, Ping bool) []string {
	return CheckLiveWithContext(context.Background(), hostslist, Ping)
}

// CheckLiveWithContext 检测主机存活状态，支持任务暂停/取消时尽快退出。
func CheckLiveWithContext(ctx context.Context, hostslist []string, Ping bool) []string {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return nil
	}

	return collectAliveHosts(hostslist, func(chanHosts chan string) {
		// 根据Ping参数选择检测方式
		if Ping {
			// 使用ping方式探测
			RunPingWithContext(ctx, hostslist, chanHosts)
			return
		}
		probeWithICMP(hostslist, chanHosts)
	})
}

// collectAliveHosts keeps every liveness probe's result state scoped to that
// invocation. Port scan tasks run concurrently, so package-level result state
// would allow one task's live hosts to become another task's scan targets.
func collectAliveHosts(hostslist []string, probe func(chan string)) []string {
	aliveHosts := make([]string, 0, len(hostslist))
	existHosts := make(map[string]struct{}, len(hostslist))

	// 创建主机通道
	chanHosts := make(chan string, len(hostslist))
	aliveAliases := buildAliveHostAliases(hostslist)
	handlerDone := make(chan struct{})

	// 处理存活主机
	go func() {
		defer close(handlerDone)
		handleAliveHosts(chanHosts, aliveAliases, &aliveHosts, existHosts)
	}()

	probe(chanHosts)
	close(chanHosts)
	<-handlerDone

	// 输出存活统计信息
	return aliveHosts
}

func buildAliveHostAliases(hostslist []string) map[string]string {
	aliases := make(map[string]string, len(hostslist))
	for _, host := range hostslist {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		if _, ok := aliases[host]; !ok {
			aliases[host] = host
		}
		ips, err := net.LookupIP(host)
		if err != nil {
			continue
		}
		for _, ip := range ips {
			if ip4 := ip.To4(); ip4 != nil {
				aliases[ip4.String()] = host
			}
		}
	}
	return aliases
}

func handleAliveHosts(chanHosts chan string, aliveAliases map[string]string, aliveHosts *[]string, existHosts map[string]struct{}) {
	for detected := range chanHosts {
		aliveHost, ok := aliveAliases[strings.TrimSpace(detected)]
		if ok {
			if _, exists := existHosts[aliveHost]; !exists {
				existHosts[aliveHost] = struct{}{}
				*aliveHosts = append(*aliveHosts, aliveHost)
				logger.Info("%s is alive!", aliveHost)
			}
		}
	}
}

// probeWithICMP 使用ICMP方式探测
func probeWithICMP(hostslist []string, chanHosts chan string) {
	// 尝试监听本地ICMP
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err == nil {
		RunIcmp1(hostslist, conn, chanHosts)
		return
	}

	logger.Error("%s", "icmp_listen_failed"+err.Error())
	logger.Info("trying_no_listen_icmp")

	// 尝试无监听ICMP探测
	conn2, err := net.DialTimeout("ip4:icmp", "127.0.0.1", 3*time.Second)
	if err == nil {
		defer conn2.Close()
		RunIcmp2(hostslist, chanHosts)
		return
	}

	// 降级使用ping探测
	RunPing(hostslist, chanHosts)
}

// RunIcmp1 使用ICMP批量探测主机存活(监听模式)
func RunIcmp1(hostslist []string, conn *icmp.PacketConn, chanHosts chan string) {
	var detectedMu sync.Mutex
	detectedHosts := make(map[string]struct{}, len(hostslist))
	listenerDone := make(chan struct{})

	// 启动监听协程
	go func() {
		defer close(listenerDone)
		for {
			// 接收ICMP响应
			msg := make([]byte, 100)
			_, sourceIP, err := conn.ReadFrom(msg)
			if err != nil {
				return
			}
			if sourceIP != nil {
				detectedMu.Lock()
				detectedHosts[sourceIP.String()] = struct{}{}
				detectedMu.Unlock()
				chanHosts <- sourceIP.String()
			}
		}
	}()

	// 发送ICMP请求
	for _, host := range hostslist {
		dst, _ := net.ResolveIPAddr("ip", host)
		IcmpByte := makemsg(host)
		conn.WriteTo(IcmpByte, dst)
	}

	// 等待响应
	wait := 6 * time.Second
	if len(hostslist) <= 256 {
		wait = 3 * time.Second
	}
	deadline := time.NewTimer(wait)
	poll := time.NewTicker(10 * time.Millisecond)
	defer deadline.Stop()
	defer poll.Stop()

	waiting := true
	for {
		select {
		case <-deadline.C:
			waiting = false
		case <-poll.C:
			detectedMu.Lock()
			allDetected := len(detectedHosts) >= len(hostslist)
			detectedMu.Unlock()
			if allDetected {
				waiting = false
			}
		}
		if !waiting {
			break
		}
	}

	_ = conn.Close()
	<-listenerDone
}

// RunIcmp2 使用ICMP并发探测主机存活(无监听模式)
func RunIcmp2(hostslist []string, chanHosts chan string) {
	// 控制并发数
	num := 1000
	if len(hostslist) < num {
		num = len(hostslist)
	}

	var wg sync.WaitGroup
	limiter := make(chan struct{}, num)

	// 并发探测
	for _, host := range hostslist {
		wg.Add(1)
		limiter <- struct{}{}

		go func(host string) {
			defer func() {
				<-limiter
				wg.Done()
			}()

			if icmpalive(host) {
				chanHosts <- host
			}
		}(host)
	}

	wg.Wait()
	close(limiter)
}

// icmpalive 检测主机ICMP是否存活
func icmpalive(host string) bool {
	startTime := time.Now()

	// 建立ICMP连接
	conn, err := net.DialTimeout("ip4:icmp", host, 6*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	// 设置超时时间
	if err := conn.SetDeadline(startTime.Add(6 * time.Second)); err != nil {
		return false
	}

	// 构造并发送ICMP请求
	msg := makemsg(host)
	if _, err := conn.Write(msg); err != nil {
		return false
	}

	// 接收ICMP响应
	receive := make([]byte, 60)
	if _, err := conn.Read(receive); err != nil {
		return false
	}

	return true
}

func RunPing(hostslist []string, chanHosts chan string) {
	RunPingWithContext(context.Background(), hostslist, chanHosts)
}

func RunPingWithContext(ctx context.Context, hostslist []string, chanHosts chan string) {
	if ctx == nil {
		ctx = context.Background()
	}

	var wg sync.WaitGroup
	limiter := make(chan struct{}, 50)
loop:
	for _, host := range hostslist {
		select {
		case <-ctx.Done():
			break loop
		case limiter <- struct{}{}:
		}

		wg.Add(1)
		go func(host string) {
			defer func() {
				<-limiter
				wg.Done()
			}()

			if ExecCommandPingWithContext(ctx, host) {
				select {
				case <-ctx.Done():
					return
				default:
					chanHosts <- host
				}
			}
		}(host)
	}
	wg.Wait()
}

// ExecCommandPing 执行系统Ping命令检测主机存活
func ExecCommandPing(ip string) bool {
	return ExecCommandPingWithContext(context.Background(), ip)
}

// ExecCommandPingWithContext 执行系统Ping命令检测主机存活，支持取消正在执行的 ping。
func ExecCommandPingWithContext(ctx context.Context, ip string) bool {
	if ctx == nil {
		ctx = context.Background()
	}

	// 过滤黑名单字符
	forbiddenChars := []string{";", "&", "|", "`", "$", "\\", "'", "%", "\"", "\n"}
	for _, char := range forbiddenChars {
		if strings.Contains(ip, char) {
			return false
		}
	}

	var command *exec.Cmd
	// 根据操作系统选择不同的ping命令
	switch runtime.GOOS {
	case "windows":
		command = exec.CommandContext(ctx, "ping", "-n", "1", "-w", "1000", ip)
	case "darwin":
		command = exec.CommandContext(ctx, "ping", "-c", "1", "-W", "1", ip)
	default: // linux
		command = exec.CommandContext(ctx, "ping", "-c", "1", "-w", "1", ip)
	}

	preparePingCommand(command)

	// 捕获命令输出
	var outinfo bytes.Buffer
	command.Stdout = &outinfo

	// 执行命令
	if err := command.Start(); err != nil {
		return false
	}

	if err := command.Wait(); err != nil {
		return false
	}

	return ctx.Err() == nil
}

func makemsg(host string) []byte {
	msg := make([]byte, 40)
	id0, id1 := genIdentifier(host)
	msg[0] = 8
	msg[1] = 0
	msg[2] = 0
	msg[3] = 0
	msg[4], msg[5] = id0, id1
	msg[6], msg[7] = genSequence(1)
	check := checkSum(msg[0:40])
	msg[2] = byte(check >> 8)
	msg[3] = byte(check & 255)
	return msg
}

func checkSum(msg []byte) uint16 {
	sum := 0
	length := len(msg)
	for i := 0; i < length-1; i += 2 {
		sum += int(msg[i])*256 + int(msg[i+1])
	}
	if length%2 == 1 {
		sum += int(msg[length-1]) * 256
	}
	sum = (sum >> 16) + (sum & 0xffff)
	sum = sum + (sum >> 16)
	answer := uint16(^sum)
	return answer
}

func genSequence(v int16) (byte, byte) {
	ret1 := byte(v >> 8)
	ret2 := byte(v & 255)
	return ret1, ret2
}

func genIdentifier(host string) (byte, byte) {
	return host[0], host[1]
}
