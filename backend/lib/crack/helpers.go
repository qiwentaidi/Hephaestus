package crack

import (
	"context"
	"net"
	"time"
)

type crackFunc func(context.Context, string, []string, VulnerabilityCallback)

const (
	defaultCrackConnectTimeout = 3 * time.Second
	defaultCrackIOTimeout      = 3 * time.Second
)

// VulnerabilityCallback reports one crack finding.
type VulnerabilityCallback func(vuln VulnerabilityInfo)

func WrapperTcpWithTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: timeout}
	return WrapperTCP(network, address, dialer)
}

func WrapperTCP(network, address string, forward *net.Dialer) (net.Conn, error) {
	return forward.Dial(network, address)
}
