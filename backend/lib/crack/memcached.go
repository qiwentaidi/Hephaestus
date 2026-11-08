package crack

import (
	"context"
	"strings"
	"time"
)

func MemcachedScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	// Memcached 通常无需认证，credentials 参数保留以保持接口一致性
	client, err := WrapperTcpWithTimeout("tcp", host, defaultCrackConnectTimeout)
	defer func() {
		if client != nil {
			client.Close()
		}
	}()
	if err == nil {
		err = client.SetDeadline(time.Now().Add(defaultCrackIOTimeout))
		if err == nil {
			_, err = client.Write([]byte("stats\n")) //Set the key randomly to prevent the key on the server from being overwritten
			if err == nil {
				rev := make([]byte, 1024)
				n, err := client.Read(rev)
				if err == nil {
					if strings.Contains(string(rev[:n]), "STAT") {
						if vulnCallback != nil {
							vulnCallback(VulnerabilityInfo{
								ID:       "memcached-unauthorized",
								Name:     "memcached unauthorized",
								URL:      host,
								Type:     "Memcached",
								Severity: "HIGH",
								Request:  "stats\n",
								Response: string(rev[:n]),
							})
						}
						return
					}
				}
			}
		}
	}
}
