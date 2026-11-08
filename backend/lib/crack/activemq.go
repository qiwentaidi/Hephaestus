// https://github.com/shadow1ng/fscan/blob/main/Plugins/ActiveMQ.go

package crack

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

func ActiveMQScan(ctx context.Context, address string, credentials []string, vulnCallback VulnerabilityCallback) {
	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[activemq] User exits crack scanning")
			return
		}
		// 解析 user:pass 格式
		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}
		user := parts[0]
		pass := parts[1]
		flag, err := ActiveMQConn(address, user, pass)
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "activemq-weak-password",
					Name:     "activemq weak password",
					URL:      address,
					Type:     "ActiveMQ",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
				})
			}
			return
		}
	}
}

// ActiveMQConn 尝试ActiveMQ连接
func ActiveMQConn(address, user, pass string) (bool, error) {
	// 设置超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), defaultCrackConnectTimeout)
	defer cancel()
	dialer := &net.Dialer{Timeout: defaultCrackConnectTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	// 创建结果通道
	resultChan := make(chan struct {
		success bool
		err     error
	}, 1)

	// 在协程中处理认证
	go func() {
		// STOMP协议的CONNECT命令
		stompConnect := fmt.Sprintf("CONNECT\naccept-version:1.0,1.1,1.2\nhost:/\nlogin:%s\npasscode:%s\n\n\x00", user, pass)

		// 发送认证请求
		conn.SetWriteDeadline(time.Now().Add(defaultCrackIOTimeout))
		if _, err := conn.Write([]byte(stompConnect)); err != nil {
			select {
			case <-ctx.Done():
			case resultChan <- struct {
				success bool
				err     error
			}{false, err}:
			}
			return
		}

		// 读取响应
		conn.SetReadDeadline(time.Now().Add(defaultCrackIOTimeout))
		respBuf := make([]byte, 1024)
		n, err := conn.Read(respBuf)
		if err != nil {
			select {
			case <-ctx.Done():
			case resultChan <- struct {
				success bool
				err     error
			}{false, err}:
			}
			return
		}

		// 检查认证结果
		response := string(respBuf[:n])

		var success bool
		var resultErr error

		if strings.Contains(response, "CONNECTED") {
			success = true
			resultErr = nil
		} else if strings.Contains(response, "Authentication failed") || strings.Contains(response, "ERROR") {
			success = false
			resultErr = fmt.Errorf("认证失败")
		} else {
			success = false
			resultErr = fmt.Errorf("未知响应: %s", response)
		}

		select {
		case <-ctx.Done():
		case resultChan <- struct {
			success bool
			err     error
		}{success, resultErr}:
		}
	}()

	// 等待认证结果或上下文取消
	select {
	case result := <-resultChan:
		return result.success, result.err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}
