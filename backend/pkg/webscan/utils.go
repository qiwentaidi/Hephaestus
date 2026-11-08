package webscan

import "hephaestus-backend/pkg/types"

// VulnerabilityCallback 漏洞扫描结果回调函数
type VulnerabilityCallback func(vuln types.VulnerabilityInfo)

// NucleiJobDoneCallback 每个 nuclei 任务执行完成后的回调
type NucleiJobDoneCallback func(url string)
