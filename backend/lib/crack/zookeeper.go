package crack

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func ZookeeperScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	if ctx.Err() != nil {
		warningf("[zookeeper] User exits crack scanning")
		return
	}

	flag, response, err := ZookeeperUnauth(host)
	if flag && err == nil {
		if vulnCallback != nil {
			vulnCallback(VulnerabilityInfo{
				ID:       "zookeeper-unauthorized",
				Name:     "zookeeper unauthorized",
				URL:      host,
				Type:     "Zookeeper",
				Severity: "HIGH",
				Request:  "ruok",
				Response: response,
			})
		}
	}
}

func ZookeeperUnauth(address string) (bool, string, error) {
	for _, command := range []string{"ruok", "stat"} {
		response, err := zookeeperFourLetterCommand(address, command)
		if err != nil {
			continue
		}
		lower := strings.ToLower(response)
		if strings.Contains(lower, "imok") || strings.Contains(lower, "zookeeper version") {
			return true, response, nil
		}
	}
	return false, "", fmt.Errorf("zookeeper unauthorized check failed")
}

func zookeeperFourLetterCommand(address, command string) (string, error) {
	conn, err := WrapperTcpWithTimeout("tcp", address, defaultCrackConnectTimeout)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(defaultCrackIOTimeout)); err != nil {
		return "", err
	}
	if _, err := conn.Write([]byte(command)); err != nil {
		return "", err
	}

	buffer := make([]byte, 4096)
	n, err := conn.Read(buffer)
	if err != nil {
		return "", err
	}
	return string(buffer[:n]), nil
}
