package crack

import (
	"context"
)

func JdwpScan(ctx context.Context, address string, credentials []string, vulnCallback VulnerabilityCallback) {
	if vulnCallback != nil {
		vulnCallback(VulnerabilityInfo{
			ID:          "jdwp-exposed",
			Name:        "JDWP Debug Port Exposed",
			URL:         address,
			Type:        "JDWP",
			Severity:    "MEDIUM",
			Request:     "JDWP-Handshake",
			Response:    "JDWP-Handshake",
			Description: "检测到JDWP端口开放, 自行尝试是否存在命令执行漏洞",
			Reference:   "https://forum.butian.net/share/1232,https://github.com/l3yx/jdwp-codeifier",
		})

	}
}
