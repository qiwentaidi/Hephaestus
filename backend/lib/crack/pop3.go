package crack

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

func Pop3Scan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	flag, banner, err := Pop3Conn(host, "", "")
	if flag && err == nil {
		if vulnCallback != nil {
			vulnCallback(VulnerabilityInfo{
				ID:       "pop3-unauthorized",
				Name:     "pop3 unauthorized",
				URL:      host,
				Type:     "POP3",
				Severity: "HIGH",
				Response: banner,
			})
		}
		return
	}

	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[pop3] User exits crack scanning")
			return
		}

		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}

		user := parts[0]
		pass := parts[1]
		flag, banner, err = Pop3Conn(host, user, pass)
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "pop3-weak-password",
					Name:     "pop3 weak password",
					URL:      host,
					Type:     "POP3",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
					Response: banner,
				})
			}
			return
		}
	}
}

func Pop3Conn(address, user, pass string) (bool, string, error) {
	conn, err := dialPOP3(address)
	if err != nil {
		return false, "", err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(defaultCrackIOTimeout)); err != nil {
		return false, "", err
	}

	tp := textproto.NewConn(conn)
	defer tp.Close()

	banner, err := tp.ReadLine()
	if err != nil {
		return false, "", err
	}
	if !strings.HasPrefix(strings.ToUpper(banner), "+OK") {
		return false, banner, fmt.Errorf("unexpected banner: %s", banner)
	}

	if err := sendPOP3Command(tp, fmt.Sprintf("USER %s", user)); err != nil {
		return false, banner, err
	}

	if err := sendPOP3Command(tp, fmt.Sprintf("PASS %s", pass)); err != nil {
		return false, banner, err
	}

	_ = tp.PrintfLine("STAT")
	if line, statErr := tp.ReadLine(); statErr == nil && strings.TrimSpace(line) != "" {
		banner = banner + "\n" + line
	}
	_ = tp.PrintfLine("QUIT")

	return true, banner, nil
}

func sendPOP3Command(tp *textproto.Conn, command string) error {
	if err := tp.PrintfLine("%s", command); err != nil {
		return err
	}
	line, err := tp.ReadLine()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(strings.ToUpper(line), "+OK") {
		return fmt.Errorf("pop3 auth failed: %s", line)
	}
	return nil
}

func dialPOP3(address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	dialer := &net.Dialer{Timeout: defaultCrackConnectTimeout}
	if p, _ := strconv.Atoi(port); p == 995 {
		return tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(host, port), &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true,
		})
	}

	return dialer.Dial("tcp", address)
}
