package crack

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/qiwentaidi/clients"
)

const defaultAliveURL = "http://www.baidu.com"

func Socks5Scan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	hostwithoutport := strings.Split(host, ":")[0]
	port, err := strconv.Atoi(strings.Split(host, ":")[1])
	if err != nil {
		errorf("socks5://%s is invalid port", host)
		return
	}
	flag := Socks5Conn(hostwithoutport, port, 5, "", "", defaultAliveURL)
	if flag {
		if vulnCallback != nil {
			vulnCallback(VulnerabilityInfo{
				ID:       "socks5-unauthorized",
				Name:     "socks5 unauthorized",
				URL:      host,
				Type:     "socks5",
				Severity: "HIGH",
			})
		}
		return
	}
	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[socks5] User exits crack scanning")
			return
		}
		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}
		user := parts[0]
		pass := parts[1]
		flag = Socks5Conn(hostwithoutport, port, 3, user, pass, defaultAliveURL)
		if flag {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "socks5-weak-password",
					Name:     "socks5 weak password",
					URL:      host,
					Type:     "SOCKS5",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
				})
			}
			return
		}
	}
}

func Socks5Conn(ip string, port, timeout int, username, password, aliveURL string) bool {
	socks5URL := fmt.Sprintf("socks5://%s:%s@%s:%d", username, password, ip, port)
	client := clients.NewRestyClientWithProxy(nil, true, socks5URL)
	_, err := clients.DoRequest("GET", aliveURL, nil, nil, timeout, client)
	return err == nil
}
