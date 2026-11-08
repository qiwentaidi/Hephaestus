package crack

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func RedisScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	flag, err := RedisUnauth(host)
	if flag && err == nil {
		if vulnCallback != nil {
			vulnCallback(VulnerabilityInfo{
				ID:       "redis-unauthorized",
				Name:     "redis unauthorized",
				URL:      host,
				Type:     "Redis",
				Severity: "HIGH",
			})
		}
		return
	}
	for _, pass := range credentials {
		if ctx.Err() != nil {
			warningf("[redis] User exits crack scanning")
			return
		}

		flag, err := RedisConn(host, pass)
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "redis-weak-password",
					Name:     "redis weak password",
					URL:      host,
					Type:     "Redis",
					Severity: "HIGH",
					Extract:  pass,
				})
			}
			return
		}
	}
}

func RedisConn(address, password string) (flag bool, err error) {
	flag = false
	conn, err := WrapperTcpWithTimeout("tcp", address, defaultCrackConnectTimeout)
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()
	if err != nil {
		return flag, err
	}
	err = conn.SetReadDeadline(time.Now().Add(defaultCrackIOTimeout))
	if err != nil {
		return flag, err
	}
	_, err = fmt.Fprintf(conn, "auth %s\r\n", password)
	if err != nil {
		return flag, err
	}
	buffer := make([]byte, 4096)
	n, err := conn.Read(buffer)
	if err != nil {
		return flag, err
	}
	if strings.Contains(string(buffer[:n]), "+OK") {
		flag = true
	}
	return flag, err
}

func RedisUnauth(address string) (flag bool, err error) {
	flag = false
	conn, err := WrapperTcpWithTimeout("tcp", address, defaultCrackConnectTimeout)
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()
	if err != nil {
		return flag, err
	}
	err = conn.SetReadDeadline(time.Now().Add(defaultCrackIOTimeout))
	if err != nil {
		return flag, err
	}
	_, err = conn.Write([]byte("info\r\n"))
	if err != nil {
		return flag, err
	}
	buffer := make([]byte, 4096)
	n, err := conn.Read(buffer)
	if err != nil {
		return flag, err
	}
	if strings.Contains(string(buffer[:n]), "redis_version") {
		flag = true
	}
	return flag, err
}
