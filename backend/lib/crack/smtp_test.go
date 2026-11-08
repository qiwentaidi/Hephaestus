package crack

import (
	"context"
	"encoding/base64"
	"net"
	"net/textproto"
	"strings"
	"testing"
)

func TestSmtpUnauth(t *testing.T) {
	address := startFakeSMTPServer(t, fakeSMTPOptions{openRelay: true})

	flag, banner, err := SmtpUnauth(address)
	if err != nil {
		t.Fatalf("expected smtp unauth to succeed, got error: %v", err)
	}
	if !flag {
		t.Fatal("expected smtp unauth to return true")
	}
	if !strings.Contains(banner, "relay accepted") {
		t.Fatalf("expected relay response in banner, got %q", banner)
	}
}

func TestSmtpConn(t *testing.T) {
	address := startFakeSMTPServer(t, fakeSMTPOptions{
		authUser: "admin",
		authPass: "password",
	})

	flag, _, err := SmtpConn(address, "admin", "password")
	if err != nil {
		t.Fatalf("expected smtp auth to succeed, got error: %v", err)
	}
	if !flag {
		t.Fatal("expected smtp auth to return true")
	}

	flag, _, err = SmtpConn(address, "admin", "wrong")
	if err == nil {
		t.Fatal("expected wrong smtp password to return an error")
	}
	if flag {
		t.Fatal("expected wrong smtp password to return false")
	}
}

func TestSmtpScanReportsWeakPassword(t *testing.T) {
	address := startFakeSMTPServer(t, fakeSMTPOptions{
		authUser: "admin",
		authPass: "password",
	})

	var got *VulnerabilityInfo
	SmtpScan(context.Background(), address, []string{"admin:wrong", "admin:password"}, func(vuln VulnerabilityInfo) {
		got = &vuln
	})

	if got == nil {
		t.Fatal("expected weak password finding")
	}
	if got.ID != "smtp-weak-password" {
		t.Fatalf("expected smtp weak password finding, got %q", got.ID)
	}
	if got.Extract != "admin/password" {
		t.Fatalf("expected extracted credential, got %q", got.Extract)
	}
}

type fakeSMTPOptions struct {
	openRelay bool
	authUser  string
	authPass  string
}

func startFakeSMTPServer(t *testing.T, opts fakeSMTPOptions) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen fake smtp: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleFakeSMTPConn(conn, opts)
		}
	}()

	return listener.Addr().String()
}

func handleFakeSMTPConn(conn net.Conn, opts fakeSMTPOptions) {
	defer conn.Close()

	tp := textproto.NewConn(conn)
	defer tp.Close()

	_ = tp.PrintfLine("220 fake smtp ready")
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"):
			_ = tp.PrintfLine("250-localhost")
			_ = tp.PrintfLine("250 AUTH PLAIN LOGIN")
		case strings.HasPrefix(upper, "HELO"):
			_ = tp.PrintfLine("250 localhost")
		case strings.HasPrefix(upper, "MAIL FROM"):
			_ = tp.PrintfLine("250 sender ok")
		case strings.HasPrefix(upper, "RCPT TO"):
			if opts.openRelay {
				_ = tp.PrintfLine("250 relay accepted")
			} else {
				_ = tp.PrintfLine("554 relay denied")
			}
		case strings.HasPrefix(upper, "AUTH PLAIN "):
			if smtpPlainAuthMatches(strings.TrimSpace(line[len("AUTH PLAIN "):]), opts.authUser, opts.authPass) {
				_ = tp.PrintfLine("235 authentication successful")
			} else {
				_ = tp.PrintfLine("535 authentication failed")
			}
		case strings.HasPrefix(upper, "RSET"):
			_ = tp.PrintfLine("250 reset ok")
		case strings.HasPrefix(upper, "QUIT"):
			_ = tp.PrintfLine("221 bye")
			return
		default:
			_ = tp.PrintfLine("502 command not implemented")
		}
	}
}

func smtpPlainAuthMatches(encoded, user, pass string) bool {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}
	return string(decoded) == "\x00"+user+"\x00"+pass
}
