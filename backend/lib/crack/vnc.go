package crack

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/mitchellh/go-vnc"
)

func VncScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {

	for _, credential := range credentials {
		if ctx.Err() != nil {
			warningf("[vnc] User exits crack scanning")
			return
		}
		pass := vncPasswordFromCredential(credential)
		pass = strings.Replace(pass, "{user}", "vnc", -1)
		flag, err := VncConnWithContext(ctx, host, pass)
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "vnc-weak-password",
					Name:     "vnc weak password",
					URL:      host,
					Type:     "VNC",
					Severity: "HIGH",
					Extract:  pass,
				})
			}
			return
		}
	}
}

func vncPasswordFromCredential(credential string) string {
	if _, pass, ok := strings.Cut(credential, ":"); ok {
		return pass
	}
	return credential
}

func VncConn(host, pass string) (flag bool, err error) {
	return VncConnWithContext(context.Background(), host, pass)
}

func VncConnWithContext(ctx context.Context, host, pass string) (flag bool, err error) {
	flag = false
	connTimeout := defaultCrackConnectTimeout + defaultCrackIOTimeout
	connCtx, cancel := context.WithTimeout(ctx, connTimeout)
	defer cancel()

	dialer := &net.Dialer{Timeout: defaultCrackConnectTimeout}
	conn, err := dialer.DialContext(connCtx, "tcp", host)
	if err != nil {
		return flag, err
	}

	var closeOnce sync.Once
	closeConn := func() {
		closeOnce.Do(func() {
			_ = conn.Close()
		})
	}
	defer closeConn()

	done := make(chan struct{})
	go func() {
		select {
		case <-connCtx.Done():
			closeConn()
		case <-done:
		}
	}()
	defer close(done)

	if err = conn.SetDeadline(time.Now().Add(connTimeout)); err != nil {
		conn.Close()
		return flag, err
	}
	defer conn.SetDeadline(time.Time{})
	// Create a VNC client connection
	cfg := &vnc.ClientConfig{
		Auth: []vnc.ClientAuth{&vnc.PasswordAuth{Password: pass}},
	}
	client, err := vnc.Client(conn, cfg)
	if err != nil {
		return flag, err
	}
	// Use the client connection to interact with the VNC server
	// ...

	// Close the client connection when done
	client.Close()
	flag = true
	return flag, err
}
