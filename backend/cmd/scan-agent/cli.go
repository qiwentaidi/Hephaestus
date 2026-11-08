package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/portscan"
	"hephaestus-backend/pkg/version"
	"hephaestus-backend/pkg/webscan"
)

// cliDefaultTopPorts scan 命令默认扫描的常用端口（fscan 风格）
const cliDefaultTopPorts = "21,22,23,80,81,110,135,139,443,445,445,1433,1521,2049,2181,2375,2376,3306,3389,4848,5000,5432,5601,5672,5900,5984,6379,6443,7001,7002,7070,7180,8000,8009,8010,8020,8060,8069,8080,8081,8088,8089,8090,8161,8180,8200,8222,8333,8443,8500,8848,8866,8899,9000,9001,9002,9009,9043,9044,9080,9090,9091,9200,9300,9418,9443,9500,9527,9999,10000,10250,11211,15672,18080,18081,20880,27017,28017,50000,50070,61616"

// runCLI CLI 模式入口，返回进程退出码
func runCLI(args []string) int {
	command := strings.ToLower(strings.TrimSpace(args[0]))
	rest := args[1:]

	var code int
	switch command {
	case "help", "-h", "--help":
		printCLIUsage()
	case "version", "-v", "--version":
		fmt.Printf("Hephaestus scan-agent %s (cli mode)\n", version.Version)
	case "combo":
		code = cmdCombo(rest)
	case "scan":
		code = cmdScan(rest)
	case "webscan":
		code = cmdWebscan(rest)
	case "crack":
		code = cmdCrack(rest)
	case "dirscan":
		code = cmdDirscan(rest)
	case "subdomain":
		code = cmdSubdomain(rest)
	case "jsscan":
		code = cmdJsscan(rest)
	default:
		fmt.Printf("[!] 未知命令: %s\n\n", command)
		printCLIUsage()
		code = 2
	}
	return code
}

func addGlobalFlags(fs *flag.FlagSet) *cliGlobalFlags {
	g := &cliGlobalFlags{}
	fs.StringVar(&g.remote.Host, "db-host", "", "远程 PostgreSQL 地址（结果汇总到服务端库，Web 端任务列表实时可见）")
	fs.StringVar(&g.remote.Port, "db-port", "5432", "远程 PostgreSQL 端口")
	fs.StringVar(&g.remote.User, "db-user", "postgres", "远程 PostgreSQL 用户名")
	fs.StringVar(&g.remote.Password, "db-pass", "postgres", "远程 PostgreSQL 密码")
	fs.StringVar(&g.remote.DBName, "db-name", "hephaestus", "远程 PostgreSQL 数据库名")
	fs.StringVar(&g.remote.SSLMode, "db-sslmode", "disable", "远程 PostgreSQL SSL 模式")
	fs.StringVar(&g.serverURL, "server-url", "", "服务端地址（远程模式下用于截图等资产上传，如 http://1.2.3.4:8080）")
	fs.BoolVar(&g.keepDB, "keep-db", false, "保留本地临时数据库文件（调试用）")
	return g
}

// newCLIContext 创建可被 Ctrl+C 中断的扫描上下文
func newCLIContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n[!] 收到中断信号，正在停止已扫描结果仍会输出...")
		cancel()
	}()
	return ctx, cancel
}

func collectTargets(items []string, file string, label string) ([]string, error) {
	var targets []string
	targets = append(targets, items...)
	if strings.TrimSpace(file) != "" {
		lines, err := readLinesFile(file)
		if err != nil {
			return nil, err
		}
		targets = append(targets, lines...)
	}
	targets = dedupeSorted(targets)
	if len(targets) == 0 {
		return nil, fmt.Errorf("未指定任何%s（使用 -h/-u/-d 或 -hf/-l/-df）", label)
	}
	return targets, nil
}

func dedupeSorted(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
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
	sort.Strings(out)
	return out
}

// normalizeWebTargets 为缺少 scheme 的目标补 http://
func normalizeWebTargets(targets []string) []string {
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target != "" && !strings.Contains(target, "://") {
			target = "http://" + target
		}
		out = append(out, target)
	}
	return out
}

// cmdScan fscan 风格主命令：存活探测 + 端口扫描 + 服务识别（默认附带弱口令爆破）
func cmdScan(args []string) int {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	hostFlag := fs.String("h", "", "目标主机/IP/CIDR，逗号分隔（如 192.168.1.1 或 192.168.1.0/24）")
	hostFile := fs.String("hf", "", "从文件读取目标（每行一个）")
	portRange := fs.String("p", cliDefaultTopPorts, "端口范围（如 80,443,1000-2000，默认常用端口）")
	threads := fs.Int("t", 600, "并发线程数")
	timeout := fs.Int("timeout", 3, "单次探测超时（秒）")
	synMode := fs.Bool("syn", false, "使用 SYN 半开放扫描（需管理员权限，速度快；默认 TCP connect 兼容性最好）")
	noPing := fs.Bool("np", false, "跳过 ICMP/Ping 存活探测")
	subnetDiscovery := fs.Bool("subnet-discovery", false, "启用大网段抽样探测（IPv4 CIDR 大于 /24）")
	subnetMode := fs.String("subnet-discovery-mode", "full", "网段策略：full 保留全部网段，fast 跳过未命中网段")
	subnetSuffixes := fs.String("subnet-discovery-host-suffixes", "1,2,254", "固定抽样地址尾数，逗号分隔，1-254")
	subnetRandom := fs.Int("subnet-discovery-random-samples", 2, "每段额外随机抽样数，0-32")
	tcpHeuristic := fs.Bool("tcp-heuristic", false, "启用 TCP 启发式主机探测（未命中主机跳过完整端口扫描）")
	tcpHeuristicPorts := fs.String("tcp-heuristic-ports", "80,443,22,3389,445", "启发式 TCP 端口，逗号分隔")
	tcpHeuristicTimeout := fs.Int("tcp-heuristic-timeout", 3000, "启发式 TCP 探测超时（毫秒，100-30000）")
	noCrack := fs.Bool("no-crack", false, "禁用弱口令爆破（默认对发现的服务爆破）")
	crackUser := fs.String("user", "", "爆破用户名，逗号分隔（默认使用内置字典）")
	crackPass := fs.String("pw", "", "爆破密码，逗号分隔（默认使用内置字典）")
	proxy := fs.String("proxy", "", "代理地址（socks5://... 或 http://...）")
	output := fs.String("o", "", "结果输出文件")
	global := addGlobalFlags(fs)
	_ = fs.Parse(args)

	targets, err := collectTargets(splitComma(*hostFlag), *hostFile, "目标主机")
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 2
	}

	cleanup, err := initCLIRuntime(*global)
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 1
	}
	defer cleanup()

	printer, closeOut := newCLIPrinter(*output)
	defer closeOut()

	printer.printf("Hephaestus scan-agent %s | 目标 %d 个 | 端口 %s | 线程 %d", version.Version, len(targets), *portRange, *threads)
	printer.printf("开始时间: %s", time.Now().Format("2006-01-02 15:04:05"))

	ctx, cancel := newCLIContext()
	defer cancel()

	// 阶段一：端口扫描（含存活探测与服务识别）
	portCfg := map[string]interface{}{
		"threads":                      *threads,
		"timeout":                      *timeout,
		"portRange":                    *portRange,
		"aliveDetection":               !*noPing,
		"subnetDiscoveryEnabled":       *subnetDiscovery,
		"subnetDiscoveryMode":          *subnetMode,
		"subnetDiscoveryHostSuffixes":  *subnetSuffixes,
		"subnetDiscoveryRandomSamples": *subnetRandom,
		"tcpHeuristicEnabled":          *tcpHeuristic || *subnetDiscovery,
		"tcpHeuristicPorts":            *tcpHeuristicPorts,
		"tcpHeuristicTimeoutMs":        *tcpHeuristicTimeout,
	}
	if *synMode {
		portCfg["portDiscoveryMode"] = "syn"
	} else {
		portCfg["portDiscoveryMode"] = "connect"
	}
	if *proxy != "" {
		portCfg["proxy"] = *proxy
	}
	portTaskID, err := createCLITask("portscan", fmt.Sprintf("CLI scan %s", time.Now().Format("0102-150405")), targets, portCfg)
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 1
	}
	portErr := runCLITask(ctx, "portscan", portTaskID, targets, portCfg)
	printer.printPortResults(portTaskID)

	// 阶段二：弱口令爆破
	if !*noCrack && portErr == nil {
		webTargets, crackTargets := deriveTargetsFromPortResults(portTaskID)
		if len(webTargets) > 0 {
			printer.section("发现的 Web 服务（可用 scan-agent webscan -u <url> 继续漏洞扫描）")
			for _, url := range webTargets {
				printer.printf("[*] %s", url)
			}
		}
		if len(crackTargets) > 0 {
			crackCfg := map[string]interface{}{
				"defaultDict": true,
				"threads":     *threads,
				"timeout":     *timeout,
			}
			if *proxy != "" {
				crackCfg["proxy"] = *proxy
			}
			if len(splitComma(*crackUser)) > 0 {
				crackCfg["username"] = strings.Join(splitComma(*crackUser), "\n")
				crackCfg["defaultDict"] = false
			}
			if len(splitComma(*crackPass)) > 0 {
				crackCfg["password"] = strings.Join(splitComma(*crackPass), "\n")
				crackCfg["defaultDict"] = false
			}
			crackTaskID, err := createCLITask("crack", fmt.Sprintf("CLI crack %s", time.Now().Format("0102-150405")), crackTargets, crackCfg)
			if err != nil {
				logger.Warning("[cli] 创建爆破任务失败: %v", err)
			} else {
				printer.printf("[*] 开始弱口令爆破，目标 %d 个...", len(crackTargets))
				_ = runCLITask(ctx, "crack", crackTaskID, crackTargets, crackCfg)
				printer.printVulnResults(crackTaskID, "弱口令爆破结果")
			}
		}
	}

	printer.printf("完成时间: %s", time.Now().Format("2006-01-02 15:04:05"))
	if portErr != nil {
		if ctx.Err() != nil {
			return 130
		}
		fmt.Printf("[!] 端口扫描失败: %v\n", portErr)
		return 1
	}
	return 0
}

// cmdWebscan Web 漏洞扫描（指纹识别 + nuclei POC）
func cmdWebscan(args []string) int {
	fs := flag.NewFlagSet("webscan", flag.ExitOnError)
	urlFlag := fs.String("u", "", "目标 URL，逗号分隔（如 http://target.com）")
	urlFile := fs.String("l", "", "从文件批量读取 URL（每行一个）")
	mode := fs.Int("mode", 3, "扫描模式: 1 指纹 2 全指纹 3 指纹+漏洞 4 专项漏洞")
	tags := fs.String("tags", "", "nuclei 模板标签过滤，逗号分隔")
	threads := fs.Int("t", 10, "并发线程数")
	deep := fs.Bool("deep", false, "深度扫描")
	screenshot := fs.Bool("screenshot", false, "网页截图")
	proxy := fs.String("proxy", "", "代理地址")
	output := fs.String("o", "", "结果输出文件")
	global := addGlobalFlags(fs)
	_ = fs.Parse(args)

	rawTargets, err := collectTargets(splitComma(*urlFlag), *urlFile, "目标 URL")
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 2
	}
	targets := normalizeWebTargets(rawTargets)

	cleanup, err := initCLIRuntime(*global)
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 1
	}
	defer cleanup()

	// PoC 模板签名与缓存初始化（失败不阻塞，仅警告）
	if err := webscan.RegisterCodeTemplateSignerForNuclei(); err != nil {
		logger.Warning("[cli] 初始化 code 模板签名验证器失败: %v", err)
	} else if err := webscan.SignUpdatedCodeTemplatesForNuclei(webscan.GetPoCDir()); err != nil {
		logger.Warning("[cli] 签名 code PoC 模板失败: %v", err)
	}
	if err := webscan.RefreshPOCTemplateCache(); err != nil {
		logger.Warning("[cli] 刷新 PoC 模板缓存失败: %v", err)
	}

	printer, closeOut := newCLIPrinter(*output)
	defer closeOut()

	printer.printf("Hephaestus scan-agent %s | webscan | 目标 %d 个 | 模式 %d", version.Version, len(targets), *mode)

	ctx, cancel := newCLIContext()
	defer cancel()

	cfg := map[string]interface{}{
		"threads":       float64(*threads),
		"webscanOption": *mode,
		"deepScan":      *deep,
		"screenshot":    *screenshot,
	}
	if *proxy != "" {
		cfg["proxy"] = *proxy
	}
	if tagList := splitComma(*tags); len(tagList) > 0 {
		tagValues := make([]interface{}, 0, len(tagList))
		for _, tag := range tagList {
			tagValues = append(tagValues, tag)
		}
		cfg["tags"] = tagValues
	}

	taskID, err := createCLITask("webscan", fmt.Sprintf("CLI webscan %s", time.Now().Format("0102-150405")), targets, cfg)
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 1
	}
	scanErr := runCLITask(ctx, "webscan", taskID, targets, cfg)

	printer.printFingerprintResults(taskID)
	printer.printVulnResults(taskID, "漏洞扫描结果")

	if scanErr != nil {
		if ctx.Err() != nil {
			return 130
		}
		fmt.Printf("[!] 扫描失败: %v\n", scanErr)
		return 1
	}
	return 0
}

// cmdCrack 弱口令爆破
func cmdCrack(args []string) int {
	fs := flag.NewFlagSet("crack", flag.ExitOnError)
	targetFlag := fs.String("h", "", "爆破目标 protocol://host:port，逗号分隔（如 ssh://1.1.1.1:22,mysql://2.2.2.2:3306）")
	targetFile := fs.String("hf", "", "从文件读取目标（每行一个）")
	crackUser := fs.String("user", "", "用户名，逗号分隔（默认使用内置字典）")
	crackPass := fs.String("pw", "", "密码，逗号分隔（默认使用内置字典）")
	noDefaultDict := fs.Bool("no-default-dict", false, "禁用内置字典")
	threads := fs.Int("t", 100, "并发线程数")
	timeout := fs.Int("timeout", 10, "单目标超时（秒）")
	proxy := fs.String("proxy", "", "代理地址")
	output := fs.String("o", "", "结果输出文件")
	global := addGlobalFlags(fs)
	_ = fs.Parse(args)

	rawTargets, err := collectTargets(splitComma(*targetFlag), *targetFile, "爆破目标")
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 2
	}
	targets := normalizeCrackTargets(rawTargets)

	cleanup, err := initCLIRuntime(*global)
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 1
	}
	defer cleanup()

	printer, closeOut := newCLIPrinter(*output)
	defer closeOut()

	printer.printf("Hephaestus scan-agent %s | crack | 目标 %d 个", version.Version, len(targets))

	ctx, cancel := newCLIContext()
	defer cancel()

	cfg := map[string]interface{}{
		"defaultDict": !*noDefaultDict,
		"threads":     *threads,
		"timeout":     *timeout,
	}
	if *proxy != "" {
		cfg["proxy"] = *proxy
	}
	if userList := splitComma(*crackUser); len(userList) > 0 {
		cfg["username"] = strings.Join(userList, "\n")
		// 显式指定用户名/密码时替换内置字典（fscan 语义）
		cfg["defaultDict"] = false
	}
	if passList := splitComma(*crackPass); len(passList) > 0 {
		cfg["password"] = strings.Join(passList, "\n")
		cfg["defaultDict"] = false
	}

	taskID, err := createCLITask("crack", fmt.Sprintf("CLI crack %s", time.Now().Format("0102-150405")), targets, cfg)
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 1
	}
	scanErr := runCLITask(ctx, "crack", taskID, targets, cfg)
	printer.printVulnResults(taskID, "弱口令爆破结果")

	if scanErr != nil {
		if ctx.Err() != nil {
			return 130
		}
		fmt.Printf("[!] 爆破失败: %v\n", scanErr)
		return 1
	}
	return 0
}

// cmdDirscan 目录扫描
func cmdDirscan(args []string) int {
	fs := flag.NewFlagSet("dirscan", flag.ExitOnError)
	urlFlag := fs.String("u", "", "目标 URL，逗号分隔（如 http://target.com）")
	urlFile := fs.String("l", "", "从文件批量读取 URL（每行一个）")
	workers := fs.Int("t", 20, "并发数")
	timeout := fs.Int("timeout", 5, "单请求超时（秒）")
	proxy := fs.String("proxy", "", "代理地址")
	output := fs.String("o", "", "结果输出文件")
	global := addGlobalFlags(fs)
	_ = fs.Parse(args)

	rawTargets, err := collectTargets(splitComma(*urlFlag), *urlFile, "目标 URL")
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 2
	}
	targets := normalizeWebTargets(rawTargets)

	cleanup, err := initCLIRuntime(*global)
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 1
	}
	defer cleanup()

	printer, closeOut := newCLIPrinter(*output)
	defer closeOut()

	printer.printf("Hephaestus scan-agent %s | dirscan | 目标 %d 个", version.Version, len(targets))

	ctx, cancel := newCLIContext()
	defer cancel()

	cfg := map[string]interface{}{
		"workers":  float64(*workers),
		"timeout":  float64(*timeout),
		"redirect": true,
	}
	if *proxy != "" {
		cfg["proxy"] = *proxy
	}
	// 预留字典补充钩子；当前 CLI 使用磁盘资源目录。
	applyEmbeddedDirsearchDicts(cfg)

	taskID, err := createCLITask("dirscan", fmt.Sprintf("CLI dirscan %s", time.Now().Format("0102-150405")), targets, cfg)
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 1
	}
	scanErr := runCLITask(ctx, "dirscan", taskID, targets, cfg)
	printer.printDirResults(taskID)

	if scanErr != nil {
		if ctx.Err() != nil {
			return 130
		}
		fmt.Printf("[!] 扫描失败: %v\n", scanErr)
		return 1
	}
	return 0
}

// cmdSubdomain 子域名收集
func cmdSubdomain(args []string) int {
	fs := flag.NewFlagSet("subdomain", flag.ExitOnError)
	domainFlag := fs.String("d", "", "主域名，逗号分隔（如 example.com）")
	domainFile := fs.String("df", "", "从文件读取域名（每行一个）")
	thread := fs.Int("t", 50, "并发数")
	timeout := fs.Int("timeout", 5, "超时（秒）")
	output := fs.String("o", "", "结果输出文件")
	global := addGlobalFlags(fs)
	_ = fs.Parse(args)

	targets, err := collectTargets(splitComma(*domainFlag), *domainFile, "域名")
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 2
	}

	cleanup, err := initCLIRuntime(*global)
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 1
	}
	defer cleanup()

	printer, closeOut := newCLIPrinter(*output)
	defer closeOut()

	printer.printf("Hephaestus scan-agent %s | subdomain | 域名 %d 个", version.Version, len(targets))

	ctx, cancel := newCLIContext()
	defer cancel()

	cfg := map[string]interface{}{
		"thread":  *thread,
		"timeout": *timeout,
		"mode":    1,
	}

	taskID, err := createCLITask("subdomain", fmt.Sprintf("CLI subdomain %s", time.Now().Format("0102-150405")), targets, cfg)
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 1
	}
	scanErr := runCLITask(ctx, "subdomain", taskID, targets, cfg)
	printer.printSubdomainResults(taskID)

	if scanErr != nil {
		if ctx.Err() != nil {
			return 130
		}
		fmt.Printf("[!] 扫描失败: %v\n", scanErr)
		return 1
	}
	return 0
}

// cmdJsscan JS 信息/漏洞扫描
func cmdJsscan(args []string) int {
	fs := flag.NewFlagSet("jsscan", flag.ExitOnError)
	urlFlag := fs.String("u", "", "目标 URL，逗号分隔（如 http://target.com）")
	urlFile := fs.String("l", "", "从文件批量读取 URL（每行一个）")
	proxy := fs.String("proxy", "", "代理地址")
	output := fs.String("o", "", "结果输出文件")
	global := addGlobalFlags(fs)
	_ = fs.Parse(args)

	rawTargets, err := collectTargets(splitComma(*urlFlag), *urlFile, "目标 URL")
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 2
	}
	targets := normalizeWebTargets(rawTargets)

	cleanup, err := initCLIRuntime(*global)
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 1
	}
	defer cleanup()

	printer, closeOut := newCLIPrinter(*output)
	defer closeOut()

	printer.printf("Hephaestus scan-agent %s | jsscan | 目标 %d 个", version.Version, len(targets))

	ctx, cancel := newCLIContext()
	defer cancel()

	cfg := map[string]interface{}{}
	if *proxy != "" {
		cfg["proxy"] = *proxy
	}

	taskID, err := createCLITask("jsscan", fmt.Sprintf("CLI jsscan %s", time.Now().Format("0102-150405")), targets, cfg)
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 1
	}
	scanErr := runCLITask(ctx, "jsscan", taskID, targets, cfg)
	printer.printJSResults(taskID)

	if scanErr != nil {
		if ctx.Err() != nil {
			return 130
		}
		fmt.Printf("[!] 扫描失败: %v\n", scanErr)
		return 1
	}
	return 0
}

// deriveTargetsFromPortResults 从端口扫描结果推导 Web 目标与爆破目标
func deriveTargetsFromPortResults(taskID string) (webTargets []string, crackTargets []string) {
	db, err := database.GetDB()
	if err != nil {
		return nil, nil
	}
	var results []database.PortScanResult
	if err := db.Where("task_id = ? AND version = ?", taskID, 1).Order("host, port").Find(&results).Error; err != nil {
		return nil, nil
	}

	crackProtos := make(map[string]struct{})
	for _, proto := range portscan.SupportedCrackProtocols() {
		crackProtos[strings.ToLower(proto)] = struct{}{}
	}

	for _, row := range results {
		if scheme, ok := classifyWebProtocol(row.Protocol, row.Port); ok {
			webTargets = append(webTargets, fmt.Sprintf("%s://%s:%d", scheme, row.Host, row.Port))
		}
		proto := strings.ToLower(strings.TrimSpace(row.Protocol))
		if _, ok := crackProtos[proto]; ok {
			crackTargets = append(crackTargets, fmt.Sprintf("%s://%s:%d", proto, row.Host, row.Port))
			continue
		}
		switch row.Port {
		case 110, 995:
			crackTargets = append(crackTargets, fmt.Sprintf("pop3://%s:%d", row.Host, row.Port))
		}
	}
	return dedupeSorted(webTargets), dedupeSorted(crackTargets)
}

var cliHTTPPortHints = map[int]string{
	80: "http", 81: "http", 3000: "http", 7001: "http", 8000: "http", 8009: "http",
	8080: "http", 8081: "http", 8088: "http", 8888: "http", 9080: "http", 9090: "http",
	443: "https", 8443: "https", 9443: "https", 10443: "https",
}

func classifyWebProtocol(proto string, port int) (string, bool) {
	p := strings.ToLower(proto)
	switch {
	case strings.Contains(p, "https"):
		return "https", true
	case strings.Contains(p, "http"):
		return "http", true
	}
	if scheme, ok := cliHTTPPortHints[port]; ok {
		return scheme, true
	}
	return "", false
}

// normalizeCrackTargets 补全爆破目标协议（host:port → protocol://host:port）
func normalizeCrackTargets(targets []string) []string {
	supported := make(map[string]struct{})
	for _, proto := range portscan.SupportedCrackProtocols() {
		supported[strings.ToLower(proto)] = struct{}{}
	}
	portProtoHints := map[int]string{
		21: "ftp", 22: "ssh", 1433: "mssql", 3306: "mysql", 3389: "rdp",
		5432: "postgres", 6379: "redis", 27017: "mongodb", 445: "smb",
		11211: "memcached", 9200: "elasticsearch",
	}
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if strings.Contains(target, "://") {
			out = append(out, target)
			continue
		}
		host, portStr, err := splitHostPort(target)
		if err != nil {
			logger.Warning("[cli] 无法识别爆破目标格式（应为 protocol://host:port）: %s", target)
			continue
		}
		var port int
		if _, scanErr := fmt.Sscanf(portStr, "%d", &port); scanErr != nil {
			logger.Warning("[cli] 端口无法解析: %s", target)
			continue
		}
		proto, ok := portProtoHints[port]
		if !ok {
			logger.Warning("[cli] 端口 %d 无对应爆破协议，请显式指定（如 ssh://%s）", port, target)
			continue
		}
		if _, ok := supported[proto]; !ok {
			logger.Warning("[cli] 协议 %s（端口 %d）不支持爆破，已跳过", proto, port)
			continue
		}
		out = append(out, fmt.Sprintf("%s://%s:%d", proto, host, port))
	}
	return dedupeSorted(out)
}

func splitHostPort(target string) (string, string, error) {
	idx := strings.LastIndex(target, ":")
	if idx <= 0 || idx == len(target)-1 {
		return "", "", fmt.Errorf("无效目标: %s", target)
	}
	host := strings.Trim(target[:idx], "[]")
	port := target[idx+1:]
	if host == "" || port == "" {
		return "", "", fmt.Errorf("无效目标: %s", target)
	}
	return host, port, nil
}

func printCLIUsage() {
	fmt.Print(`Hephaestus scan-agent - CLI 扫描模式（fscan 风格）

用法:
  scan-agent <命令> [参数]

命令:
  combo       组合扫描流水线：端口扫描 → 指纹/Web漏洞扫描 → 服务爆破 → 目录扫描
  scan        内网综合扫描：存活探测 + 端口扫描 + 服务识别 + 弱口令爆破
  webscan     Web 漏洞扫描：指纹识别 + nuclei POC
  crack       弱口令爆破（ssh/mysql/redis/ftp/mssql/postgres/mongodb/rdp 等）
  dirscan     目录扫描
  subdomain   子域名收集
  jsscan      JS 信息收集与漏洞扫描
  help        显示帮助
  supervise   托管节点运行及平台更新（由安装器启动）
  version     显示版本

示例:
  scan-agent combo -h 192.168.1.0/24
  scan-agent combo -h 192.168.1.1 -p 1-65535 -skip-crack -o result.txt
  scan-agent combo -hf targets.txt -user root,admin -pw pass.txt
  scan-agent scan -h 192.168.1.0/24
  scan-agent scan -h 192.168.1.1 -p 1-65535 -t 1000 -no-crack
  scan-agent scan -hf targets.txt -o result.txt
  scan-agent webscan -u http://target.com -mode 3
  scan-agent webscan -l urls.txt -tags cve,rce
  scan-agent crack -h ssh://1.1.1.1:22,mysql://2.2.2.2:3306
  scan-agent crack -hf crack.txt -user root,admin -pw pass.txt
  scan-agent dirscan -u http://target.com
  scan-agent subdomain -d example.com
  scan-agent jsscan -u http://target.com

通用参数（所有命令可用）:
  -o file           结果输出文件（同时输出终端）
  -proxy url        代理（socks5://127.0.0.1:1080 或 http://...）
  -db-host host     远程 PostgreSQL，结果直接写入服务端库，Web 端任务列表实时可见
  -db-port port     远程 PostgreSQL 端口（默认 5432）
  -db-user user     远程 PostgreSQL 用户（默认 postgres）
  -db-pass pass     远程 PostgreSQL 密码（默认 postgres）
  -db-name name     远程 PostgreSQL 库名（默认 hephaestus）
  -db-sslmode mode  SSL 模式（默认 disable）
  -server-url url   服务端地址（远程模式下用于截图等资产上传）
  -keep-db          保留本地临时数据库（调试用）

说明:
  - 默认使用本地临时 SQLite 库，扫描结束自动清理
  - 指定 -db-host 即为远程汇总模式，任务与结果直接进入服务端，可在 Web 端查看
  - PoC/指纹等资源默认读取二进制同级 config/ 目录（scan-agent deploy 部署布局）
`)
}
