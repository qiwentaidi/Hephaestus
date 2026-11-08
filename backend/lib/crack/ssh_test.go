package crack

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestSSHMaxTimeoutErrorsFromContext(t *testing.T) {
	if got := sshMaxTimeoutErrorsFromContext(context.Background()); got != 5 {
		t.Fatalf("expected default ssh timeout error limit 5, got %d", got)
	}

	ctx := WithSSHMaxTimeoutErrors(context.Background(), 0)
	if got := sshMaxTimeoutErrorsFromContext(ctx); got != 0 {
		t.Fatalf("expected configured ssh timeout error limit 0, got %d", got)
	}
}

func TestIsSSHTimeoutError(t *testing.T) {
	if !isSSHTimeoutError(context.DeadlineExceeded) {
		t.Fatal("expected context deadline exceeded to be a timeout error")
	}
	if !isSSHTimeoutError(errors.New("ssh: handshake failed: read tcp 127.0.0.1:22: i/o timeout")) {
		t.Fatal("expected ssh i/o timeout text to be a timeout error")
	}
	if isSSHTimeoutError(errors.New("ssh: unable to authenticate, attempted methods [none password], no supported methods remain")) {
		t.Fatal("expected normal auth failure not to be a timeout error")
	}
}

func TestSshConnContextRejectsNoneAuthSuccess(t *testing.T) {
	addr := startSSHTestServer(t, &ssh.ServerConfig{
		NoClientAuth: true,
	})

	ok, err := SshConnContext(context.Background(), addr, "root", "any-password")
	if ok {
		t.Fatal("expected none-authenticated connection not to verify an SSH password")
	}
	if !errors.Is(err, errSSHPasswordAuthNotConfirmed) {
		t.Fatalf("expected password auth confirmation error, got %v", err)
	}
}

func TestSshConnContextSupportsLegacyAlgorithms(t *testing.T) {
	config := &ssh.ServerConfig{
		Config: ssh.Config{
			KeyExchanges: []string{ssh.InsecureKeyExchangeDH1SHA1},
			MACs:         []string{ssh.InsecureHMACSHA196},
		},
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if conn.User() == "root" && string(password) == "toor" {
				return nil, nil
			}
			return nil, errors.New("invalid password")
		},
	}
	addr := startSSHTestServer(t, config)

	ok, err := SshConnContext(context.Background(), addr, "root", "toor")
	if err != nil {
		t.Fatalf("expected legacy SSH password auth to succeed, got %v", err)
	}
	if !ok {
		t.Fatal("expected valid legacy SSH password to verify")
	}
}

func TestSshScanFindsPasswordInLargeDictionary(t *testing.T) {
	addr := startSSHTestServer(t, &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if conn.User() == "root" && string(password) == "2" {
				return nil, nil
			}
			return nil, errors.New("invalid password")
		},
	})
	for _, position := range []int{0, 1, 276} {
		t.Run(fmt.Sprintf("valid-password-at-%d", position+1), func(t *testing.T) {
			credentials := make([]string, 277)
			for i := range credentials {
				credentials[i] = fmt.Sprintf("root:wrong-%d", i)
			}
			credentials[position] = "root:2"
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			findings := make(chan VulnerabilityInfo, 5)
			RunWithContext(ctx, "ssh://"+addr, credentials, func(vuln VulnerabilityInfo) {
				findings <- vuln
			})
			select {
			case vuln := <-findings:
				if vuln.Extract != "root/2" || vuln.Response != "root\n" {
					t.Fatalf("expected confirmed root/2 login and command result, got %#v", vuln)
				}
			default:
				t.Fatalf("valid credential at position %d was not reported", position+1)
			}
		})
	}
}

func TestSshScanReportsConnectionRejections(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
	})

	var logs bytes.Buffer
	previous := packageLogger
	SetLogger(log.New(&logs, "", 0))
	t.Cleanup(func() { SetLogger(previous) })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	SshScan(ctx, listener.Addr().String(), []string{"root:secret-one", "root:secret-two"}, func(vuln VulnerabilityInfo) {
		t.Errorf("a rejected connection must not be reported as a valid password: %#v", vuln)
	})
	output := logs.String()
	if strings.Count(output, "connection or session error (first occurrence)") != 1 {
		t.Fatalf("expected one actionable connection error, got %s", output)
	}
	if !strings.Contains(output, "credentials=2 attempted=2 verified=0 auth_failures=0 timeout_errors=0 connection_errors=2 unattempted=0") {
		t.Fatalf("expected connection rejections to be distinguished from bad passwords: %s", output)
	}
	if strings.Contains(output, "secret-one") || strings.Contains(output, "secret-two") {
		t.Fatalf("diagnostic logs must not include candidate passwords: %s", output)
	}
}

func startSSHTestServer(t *testing.T, config *ssh.ServerConfig) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate SSH test key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatalf("failed to create SSH test signer: %v", err)
	}
	config.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen for SSH test server: %v", err)
	}

	var wg sync.WaitGroup
	t.Cleanup(func() {
		_ = listener.Close()
		wg.Wait()
	})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()

				_, chans, reqs, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)

				for newChannel := range chans {
					if newChannel.ChannelType() != "session" {
						_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported channel type")
						continue
					}
					channel, requests, err := newChannel.Accept()
					if err != nil {
						continue
					}
					go handleSSHTestSession(channel, requests)
				}
			}()
		}
	}()

	return listener.Addr().String()
}

func handleSSHTestSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()
	for req := range requests {
		switch req.Type {
		case "exec":
			_, _ = channel.Write([]byte("root\n"))
			_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct {
				Status uint32
			}{Status: 0}))
			_ = req.Reply(true, nil)
			return
		default:
			_ = req.Reply(true, nil)
		}
	}
}
