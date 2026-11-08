package crack

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

func SmtpScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	flag, banner, err := SmtpUnauth(host)
	if flag && err == nil {
		if vulnCallback != nil {
			vulnCallback(VulnerabilityInfo{
				ID:       "smtp-unauthorized",
				Name:     "smtp unauthorized",
				URL:      host,
				Type:     "SMTP",
				Severity: "HIGH",
				Response: banner,
			})
		}
		return
	}

	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[smtp] User exits crack scanning")
			return
		}

		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}
		user := parts[0]
		pass := parts[1]

		flag, banner, err = SmtpConn(host, user, pass)
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "smtp-weak-password",
					Name:     "smtp weak password",
					URL:      host,
					Type:     "SMTP",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
					Response: banner,
				})
			}
			return
		}
	}
}

func SmtpUnauth(address string) (bool, string, error) {
	session, err := newSMTPSession(address)
	if err != nil {
		return false, "", err
	}
	defer session.close()

	if err := session.ehlo(); err != nil {
		return false, session.banner, err
	}

	if err := session.expectCommand(250, "MAIL FROM:<probe@example.com>"); err != nil {
		return false, session.banner, err
	}
	code, line, err := session.command("RCPT TO:<probe@example.org>")
	if err != nil {
		return false, session.banner, err
	}
	_ = session.printfLine("RSET")
	_ = session.printfLine("QUIT")

	if code == 250 || code == 251 {
		return true, session.banner + "\n" + line, nil
	}
	return false, session.banner + "\n" + line, fmt.Errorf("smtp relay denied: %s", line)
}

func SmtpConn(address, user, pass string) (bool, string, error) {
	session, err := newSMTPSession(address)
	if err != nil {
		return false, "", err
	}
	defer session.close()

	if err := session.ehlo(); err != nil {
		return false, session.banner, err
	}
	if session.supportsStartTLS() {
		if err := session.startTLS(); err != nil {
			return false, session.banner, err
		}
		if err := session.ehlo(); err != nil {
			return false, session.banner, err
		}
	}

	if err := session.authPlain(user, pass); err == nil {
		_ = session.printfLine("QUIT")
		return true, session.banner, nil
	}
	if err := session.authLogin(user, pass); err == nil {
		_ = session.printfLine("QUIT")
		return true, session.banner, nil
	}
	_ = session.printfLine("QUIT")
	return false, session.banner, fmt.Errorf("smtp auth failed")
}

type smtpSession struct {
	address   string
	host      string
	conn      net.Conn
	tp        *textproto.Conn
	banner    string
	ehloLines []string
}

func newSMTPSession(address string) (*smtpSession, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	dialer := &net.Dialer{Timeout: defaultCrackConnectTimeout}
	var conn net.Conn
	if p, _ := strconv.Atoi(port); p == 465 {
		conn, err = tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true,
		})
	} else {
		conn, err = dialer.Dial("tcp", address)
	}
	if err != nil {
		return nil, err
	}

	if err := conn.SetDeadline(time.Now().Add(defaultCrackIOTimeout)); err != nil {
		conn.Close()
		return nil, err
	}

	session := &smtpSession{
		address: address,
		host:    host,
		conn:    conn,
		tp:      textproto.NewConn(conn),
	}
	code, line, err := session.readResponse()
	if err != nil {
		session.close()
		return nil, err
	}
	session.banner = line
	if code != 220 {
		session.close()
		return nil, fmt.Errorf("unexpected smtp banner: %s", line)
	}
	return session, nil
}

func (s *smtpSession) close() {
	if s.tp != nil {
		_ = s.tp.Close()
		return
	}
	if s.conn != nil {
		_ = s.conn.Close()
	}
}

func (s *smtpSession) ehlo() error {
	code, line, err := s.command("EHLO localhost")
	if err != nil {
		return err
	}
	if code != 250 {
		code, line, err = s.command("HELO localhost")
		if err != nil {
			return err
		}
		if code != 250 {
			return fmt.Errorf("smtp helo failed: %s", line)
		}
	}
	s.ehloLines = strings.Split(line, "\n")
	return nil
}

func (s *smtpSession) supportsStartTLS() bool {
	for _, line := range s.ehloLines {
		if strings.Contains(strings.ToUpper(line), "STARTTLS") {
			return true
		}
	}
	return false
}

func (s *smtpSession) startTLS() error {
	code, line, err := s.command("STARTTLS")
	if err != nil {
		return err
	}
	if code != 220 {
		return fmt.Errorf("smtp starttls failed: %s", line)
	}
	tlsConn := tls.Client(s.conn, &tls.Config{
		ServerName:         s.host,
		InsecureSkipVerify: true,
	})
	if err := tlsConn.Handshake(); err != nil {
		return err
	}
	s.conn = tlsConn
	s.tp = textproto.NewConn(tlsConn)
	return nil
}

func (s *smtpSession) authPlain(user, pass string) error {
	payload := base64.StdEncoding.EncodeToString([]byte("\x00" + user + "\x00" + pass))
	code, line, err := s.command("AUTH PLAIN " + payload)
	if err != nil {
		return err
	}
	if code != 235 {
		return fmt.Errorf("smtp auth plain failed: %s", line)
	}
	return nil
}

func (s *smtpSession) authLogin(user, pass string) error {
	code, line, err := s.command("AUTH LOGIN")
	if err != nil {
		return err
	}
	if code != 334 {
		return fmt.Errorf("smtp auth login user prompt failed: %s", line)
	}
	code, line, err = s.command(base64.StdEncoding.EncodeToString([]byte(user)))
	if err != nil {
		return err
	}
	if code != 334 {
		return fmt.Errorf("smtp auth login password prompt failed: %s", line)
	}
	code, line, err = s.command(base64.StdEncoding.EncodeToString([]byte(pass)))
	if err != nil {
		return err
	}
	if code != 235 {
		return fmt.Errorf("smtp auth login failed: %s", line)
	}
	return nil
}

func (s *smtpSession) expectCommand(expectedCode int, command string) error {
	code, line, err := s.command(command)
	if err != nil {
		return err
	}
	if code != expectedCode {
		return fmt.Errorf("smtp command failed: %s", line)
	}
	return nil
}

func (s *smtpSession) command(command string) (int, string, error) {
	if err := s.printfLine("%s", command); err != nil {
		return 0, "", err
	}
	return s.readResponse()
}

func (s *smtpSession) printfLine(format string, args ...interface{}) error {
	if err := s.conn.SetDeadline(time.Now().Add(defaultCrackIOTimeout)); err != nil {
		return err
	}
	return s.tp.PrintfLine(format, args...)
}

func (s *smtpSession) readResponse() (int, string, error) {
	if err := s.conn.SetDeadline(time.Now().Add(defaultCrackIOTimeout)); err != nil {
		return 0, "", err
	}
	return s.tp.ReadResponse(0)
}
