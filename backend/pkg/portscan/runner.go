package portscan

import (
	"context"
	"hephaestus-backend/lib/crack"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/types"
	"log"
	"sync"
)

var crackLoggerOnce sync.Once

func configureCrackLogger() {
	crackLoggerOnce.Do(func() {
		crack.SetLogger(log.New(logger.NewSourceWriter("CRACK", logger.INFO), "", 0))
	})
}

// VulnerabilityCallback 漏洞结果回调函数
type VulnerabilityCallback func(vuln types.VulnerabilityInfo)

func withSSHMaxTimeoutErrors(ctx context.Context, maxErrors int) context.Context {
	return crack.WithSSHMaxTimeoutErrors(ctx, maxErrors)
}

// SupportedCrackProtocols 返回当前支持的弱口令协议列表
func SupportedCrackProtocols() []string {
	return crack.SupportedProtocols()
}

// RunnerWithContext 执行暴破扫描（带 context 支持）
// credentials 格式：[]string，每个元素是 "user:pass" 或 "pass"（无用户名协议）
func RunnerWithContext(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	configureCrackLogger()
	crack.RunWithContext(ctx, host, credentials, func(vuln crack.VulnerabilityInfo) {
		if vulnCallback != nil {
			vulnCallback(types.VulnerabilityInfo{
				TaskId:             taskId,
				ID:                 vuln.ID,
				Name:               vuln.Name,
				Description:        vuln.Description,
				Reference:          vuln.Reference,
				Type:               vuln.Type,
				Severity:           vuln.Severity,
				URL:                vuln.URL,
				Request:            vuln.Request,
				Response:           vuln.Response,
				ResponseTime:       vuln.ResponseTime,
				Extract:            vuln.Extract,
				PrincipleDetection: vuln.PrincipleDetection,
			})
		}
	})
}

// Runner 执行暴破扫描（保留向后兼容）
func Runner(taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	configureCrackLogger()
	crack.Run(host, credentials, func(vuln crack.VulnerabilityInfo) {
		if vulnCallback != nil {
			vulnCallback(types.VulnerabilityInfo{
				TaskId:             taskId,
				ID:                 vuln.ID,
				Name:               vuln.Name,
				Description:        vuln.Description,
				Reference:          vuln.Reference,
				Type:               vuln.Type,
				Severity:           vuln.Severity,
				URL:                vuln.URL,
				Request:            vuln.Request,
				Response:           vuln.Response,
				ResponseTime:       vuln.ResponseTime,
				Extract:            vuln.Extract,
				PrincipleDetection: vuln.PrincipleDetection,
			})
		}
	})
}
