package crack

import (
	"context"
	"regexp"
	"time"
)

var rmiVulRegexp = regexp.MustCompile(`^N[\s\S]{1,2}\d*\.\d*\.\d*\.\d*`)

func RmiScan(ctx context.Context, address string, credentials []string, vulnCallback VulnerabilityCallback) {
	// 使用10秒超时建立TCP连接
	client, err := WrapperTcpWithTimeout("tcp", address, defaultCrackConnectTimeout)
	defer func() {
		if client != nil {
			client.Close()
		}
	}()

	if err == nil {
		err = client.SetDeadline(time.Now().Add(defaultCrackIOTimeout))
		if err == nil {
			// RMI协议握手包
			handshake := []byte{
				0x4a, 0x52, 0x4d, 0x49, // "JRMI"
				0x00, 0x02, // 协议版本
				0x4b, // 握手标识
			}

			_, err = client.Write(handshake)
			if err == nil {
				rev := make([]byte, 1024)
				n, err := client.Read(rev)
				if err == nil {
					// 检查返回的数据是否包含RMI响应特征
					result := rmiVulRegexp.Find(rev)
					if result != nil {
						if vulnCallback != nil {
							vulnCallback(VulnerabilityInfo{
								ID:                 "rmi-unauthorized",
								Name:               "rmi unauthorized",
								URL:                address,
								Type:               "RMI",
								Severity:           "CRITICAL",
								Request:            string(handshake),
								Response:           string(rev[:n]),
								PrincipleDetection: true,
							})
						}
						return
					}
				}
			}
		}
	}
}
