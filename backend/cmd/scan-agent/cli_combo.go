package main

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/version"
	"hephaestus-backend/pkg/webscan"
)

// comboStageStatus 组合流水线单阶段执行结果
type comboStageStatus struct {
	name      string
	skipped   bool
	err       error
	startedAt time.Time
}

// cmdCombo 组合任务扫描：端口扫描 → 指纹/Web 漏洞扫描 → 服务爆破 → 目录扫描
// 阶段间自动推导目标（端口扫描结果 → Web 目标 + 爆破目标），单阶段失败不阻塞后续阶段。
func cmdCombo(args []string) int {
	fs := flag.NewFlagSet("combo", flag.ExitOnError)
	hostFlag := fs.String("h", "", "目标主机/IP/CIDR，逗号分隔（如 192.168.1.1 或 192.168.1.0/24）")
	hostFile := fs.String("hf", "", "从文件读取目标（每行一个）")
	portRange := fs.String("p", cliDefaultTopPorts, "端口范围（如 80,443,1000-2000，默认常用端口）")
	threads := fs.Int("t", 600, "端口扫描并发线程数")
	timeout := fs.Int("timeout", 3, "单次探测超时（秒）")
	synMode := fs.Bool("syn", false, "使用 SYN 半开放扫描（需管理员权限；默认 TCP connect）")
	noPing := fs.Bool("np", false, "跳过 ICMP/Ping 存活探测")
	subnetDiscovery := fs.Bool("subnet-discovery", false, "启用大网段抽样探测（IPv4 CIDR 大于 /24）")
	subnetMode := fs.String("subnet-discovery-mode", "full", "网段策略：full 保留全部网段，fast 跳过未命中网段")
	subnetSuffixes := fs.String("subnet-discovery-host-suffixes", "1,2,254", "固定抽样地址尾数，逗号分隔，1-254")
	subnetRandom := fs.Int("subnet-discovery-random-samples", 2, "每段额外随机抽样数，0-32")
	tcpHeuristic := fs.Bool("tcp-heuristic", false, "启用 TCP 启发式主机探测（未命中主机跳过完整端口扫描）")
	tcpHeuristicPorts := fs.String("tcp-heuristic-ports", "80,443,22,3389,445", "启发式 TCP 端口，逗号分隔")
	tcpHeuristicTimeout := fs.Int("tcp-heuristic-timeout", 3000, "启发式 TCP 探测超时（毫秒，100-30000）")
	skipCrack := fs.Bool("skip-crack", false, "跳过弱口令爆破阶段")
	skipWebscan := fs.Bool("skip-webscan", false, "跳过指纹/Web 漏洞扫描阶段")
	skipDirscan := fs.Bool("skip-dirscan", false, "跳过目录扫描阶段")
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

	// PoC 模板签名与缓存初始化（失败不阻塞，仅警告）
	if err := webscan.RegisterCodeTemplateSignerForNuclei(); err != nil {
		logger.Warning("[cli] 初始化 code 模板签名验证器失败: %v", err)
	} else if err := webscan.SignUpdatedCodeTemplatesForNuclei(webscan.GetPoCDir()); err != nil {
		logger.Warning("[cli] 签名 code PoC 模板失败: %v", err)
	}
	if err := webscan.EnsurePOCTemplateCache(); err != nil {
		logger.Warning("[cli] 刷新 PoC 模板缓存失败: %v", err)
	}

	printer, closeOut := newCLIPrinter(*output)
	defer closeOut()

	printer.printf("Hephaestus scan-agent %s | combo 组合扫描 | 目标 %d 个 | 端口 %s", version.Version, len(targets), *portRange)
	printer.printf("流水线: 端口扫描 → 指纹/Web漏洞扫描 → 服务爆破 → 目录扫描")
	printer.printf("开始时间: %s", time.Now().Format("2006-01-02 15:04:05"))

	ctx, cancel := newCLIContext()
	defer cancel()

	stages := make([]comboStageStatus, 0, 4)
	failed := false

	// ── 阶段一：端口扫描（含存活探测与服务识别）──
	stage := comboStageStatus{name: "portscan", startedAt: time.Now()}
	printer.section("[1/4] 端口扫描（存活探测 + 端口 + 服务识别）")
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
	portTaskID, err := createCLITask("portscan", fmt.Sprintf("CLI combo portscan %s", time.Now().Format("0102-150405")), targets, portCfg)
	if err != nil {
		fmt.Printf("[!] %v\n", err)
		return 1
	}
	portErr := runCLITask(ctx, "portscan", portTaskID, targets, portCfg)
	printer.printPortResults(portTaskID)
	stage.err = portErr
	stages = append(stages, stage)

	// 端口扫描失败/中断时无目标依据，直接收尾
	if portErr != nil {
		failed = true
		if ctx.Err() != nil {
			return comboFinish(printer, stages, true)
		}
		printer.printf("[!] 端口扫描失败，流水线终止: %v", portErr)
		return comboFinish(printer, stages, failed)
	}

	// 阶段间目标推导：端口结果 → Web 目标 + 爆破目标
	webTargets, crackTargets := deriveTargetsFromPortResults(portTaskID)
	if len(webTargets) > 0 {
		printer.printf("[*] 推导出 Web 目标 %d 个（进入指纹/Web 漏洞扫描与目录扫描）", len(webTargets))
	} else {
		printer.printf("[*] 未发现 Web 服务，跳过指纹/Web 漏洞扫描与目录扫描")
	}
	if len(crackTargets) > 0 {
		printer.printf("[*] 推导出爆破目标 %d 个（进入服务爆破）", len(crackTargets))
	} else {
		printer.printf("[*] 未发现可爆破服务，跳过服务爆破")
	}

	// ── 阶段二：指纹 + Web 漏洞扫描（webscan 内置指纹识别→POC 匹配）──
	stage = comboStageStatus{name: "webscan", startedAt: time.Now()}
	if *skipWebscan || len(webTargets) == 0 {
		stage.skipped = true
		printer.section("[2/4] 指纹/Web 漏洞扫描（跳过）")
		printer.empty(comboSkipReason(*skipWebscan, len(webTargets) == 0))
	} else {
		printer.section("[2/4] 指纹/Web 漏洞扫描（指纹识别 + nuclei POC）")
		webCfg := map[string]interface{}{
			"threads":       10.0,
			"webscanOption": 3, // 指纹 + 漏洞
			"deepScan":      false,
			"screenshot":    false,
		}
		if *proxy != "" {
			webCfg["proxy"] = *proxy
		}
		webTaskID, err := createCLITask("webscan", fmt.Sprintf("CLI combo webscan %s", time.Now().Format("0102-150405")), webTargets, webCfg)
		if err != nil {
			stage.err = err
			failed = true
			printer.printf("[!] 创建 Web 扫描任务失败: %v", err)
		} else {
			stage.err = runCLITask(ctx, "webscan", webTaskID, webTargets, webCfg)
			printer.printFingerprintResults(webTaskID)
			printer.printVulnResults(webTaskID, "Web 漏洞扫描结果")
			if stage.err != nil {
				failed = true
				if ctx.Err() != nil {
					stages = append(stages, stage)
					return comboFinish(printer, stages, true)
				}
				printer.printf("[!] Web 漏洞扫描失败（继续后续阶段）: %v", stage.err)
			}
		}
	}
	stages = append(stages, stage)

	// ── 阶段三：端口服务弱口令爆破 ──
	stage = comboStageStatus{name: "crack", startedAt: time.Now()}
	if *skipCrack || len(crackTargets) == 0 {
		stage.skipped = true
		printer.section("[3/4] 服务弱口令爆破（跳过）")
		printer.empty(comboSkipReason(*skipCrack, len(crackTargets) == 0))
	} else {
		printer.section(fmt.Sprintf("[3/4] 服务弱口令爆破（目标 %d 个）", len(crackTargets)))
		crackCfg := map[string]interface{}{
			"defaultDict": true,
			"threads":     100,
			"timeout":     10,
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
		crackTaskID, err := createCLITask("crack", fmt.Sprintf("CLI combo crack %s", time.Now().Format("0102-150405")), crackTargets, crackCfg)
		if err != nil {
			stage.err = err
			failed = true
			printer.printf("[!] 创建爆破任务失败: %v", err)
		} else {
			stage.err = runCLITask(ctx, "crack", crackTaskID, crackTargets, crackCfg)
			printer.printVulnResults(crackTaskID, "弱口令爆破结果")
			if stage.err != nil {
				failed = true
				if ctx.Err() != nil {
					stages = append(stages, stage)
					return comboFinish(printer, stages, true)
				}
				printer.printf("[!] 服务爆破失败（继续后续阶段）: %v", stage.err)
			}
		}
	}
	stages = append(stages, stage)

	// ── 阶段四：目录扫描 ──
	stage = comboStageStatus{name: "dirscan", startedAt: time.Now()}
	if *skipDirscan || len(webTargets) == 0 {
		stage.skipped = true
		printer.section("[4/4] 目录扫描（跳过）")
		printer.empty(comboSkipReason(*skipDirscan, len(webTargets) == 0))
	} else {
		printer.section(fmt.Sprintf("[4/4] 目录扫描（目标 %d 个）", len(webTargets)))
		dirCfg := map[string]interface{}{
			"workers":  20.0,
			"timeout":  5.0,
			"redirect": true,
		}
		if *proxy != "" {
			dirCfg["proxy"] = *proxy
		}
		// 预留字典补充钩子；当前 CLI 使用磁盘资源目录。
		applyEmbeddedDirsearchDicts(dirCfg)
		dirTaskID, err := createCLITask("dirscan", fmt.Sprintf("CLI combo dirscan %s", time.Now().Format("0102-150405")), webTargets, dirCfg)
		if err != nil {
			stage.err = err
			failed = true
			printer.printf("[!] 创建目录扫描任务失败: %v", err)
		} else {
			stage.err = runCLITask(ctx, "dirscan", dirTaskID, webTargets, dirCfg)
			printer.printDirResults(dirTaskID)
			if stage.err != nil {
				failed = true
				if ctx.Err() != nil {
					stages = append(stages, stage)
					return comboFinish(printer, stages, true)
				}
				printer.printf("[!] 目录扫描失败: %v", stage.err)
			}
		}
	}
	stages = append(stages, stage)

	return comboFinish(printer, stages, failed)
}

// comboSkipReason 生成阶段跳过原因文案
func comboSkipReason(explicitSkip, noTargets bool) string {
	switch {
	case explicitSkip:
		return "用户指定跳过"
	case noTargets:
		return "无可用目标（上游阶段未产出）"
	default:
		return "未知原因"
	}
}

// comboFinish 输出流水线摘要并返回退出码
func comboFinish(printer *cliPrinter, stages []comboStageStatus, interrupted bool) int {
	printer.section("组合扫描流水线摘要")
	for _, stage := range stages {
		switch {
		case stage.skipped:
			printer.printf("[-] %-10s 跳过", stage.name)
		case stage.err != nil:
			printer.printf("[x] %-10s 失败: %v", stage.name, stage.err)
		default:
			printer.printf("[+] %-10s 完成 (耗时 %s)", stage.name, time.Since(stage.startedAt).Round(time.Second))
		}
	}
	printer.printf("完成时间: %s", time.Now().Format("2006-01-02 15:04:05"))

	if interrupted {
		return 130
	}
	if failed := comboAnyFailed(stages); failed {
		return 1
	}
	return 0
}

func comboAnyFailed(stages []comboStageStatus) bool {
	for _, stage := range stages {
		if stage.err != nil {
			return true
		}
	}
	return false
}
