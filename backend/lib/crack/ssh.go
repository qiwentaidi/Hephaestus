package crack

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

type task struct {
	User string
	Pass string
}

type sshMaxTimeoutErrorsContextKey struct{}

const defaultSSHMaxTimeoutErrors = 5

var errSSHPasswordAuthNotConfirmed = errors.New("ssh password authentication was not confirmed")

// WithSSHMaxTimeoutErrors sets the per-target SSH slow timeout error limit.
// A zero or negative value disables this early-stop guard.
func WithSSHMaxTimeoutErrors(ctx context.Context, maxErrors int) context.Context {
	return context.WithValue(ctx, sshMaxTimeoutErrorsContextKey{}, maxErrors)
}

// SshScan SSH 暴力破解
// credentials 格式：[]string，每个元素是 "user:pass"
func SshScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	scanCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	maxTimeoutErrors := sshMaxTimeoutErrorsFromContext(ctx)
	var timeoutErrors int32
	var attempted, authFailures, connectionErrors, verified int32
	var firstConnectionError sync.Once
	defer func() {
		infof("ssh target %s summary: credentials=%d attempted=%d verified=%d auth_failures=%d timeout_errors=%d connection_errors=%d unattempted=%d",
			host, len(credentials), attempted, verified, authFailures, timeoutErrors, connectionErrors, len(credentials)-int(attempted))
	}()
	taskChan := make(chan task)
	doneChan := make(chan struct{})

	// 启动多线程暴破
	for range 5 {
		go func() {
			for t := range taskChan {
				// 检查是否需要停止
				if scanCtx.Err() != nil {
					continue
				}

				atomic.AddInt32(&attempted, 1)
				flag, err := SshConnContext(scanCtx, host, t.User, t.Pass)
				if err != nil {
					switch {
					case isSSHTimeoutError(err):
						newCount := atomic.AddInt32(&timeoutErrors, 1)
						if maxTimeoutErrors > 0 && newCount >= int32(maxTimeoutErrors) {
							if newCount == int32(maxTimeoutErrors) {
								warningf("skip ssh target %s after %d timeout errors", host, maxTimeoutErrors)
							}
							cancel()
							continue
						}
					case strings.Contains(err.Error(), "unable to authenticate"):
						atomic.AddInt32(&authFailures, 1)
					case scanCtx.Err() == nil:
						atomic.AddInt32(&connectionErrors, 1)
						firstConnectionError.Do(func() {
							warningf("ssh target %s connection or session error (first occurrence): %v", host, err)
						})
					}
				}
				if flag && err == nil {
					atomic.AddInt32(&verified, 1)
					result, err := ExecuteSshCommandContext(scanCtx, host, t.User, t.Pass, "whoami")
					if err != nil {
						result = err.Error()
					}
					if vulnCallback != nil {
						vulnCallback(VulnerabilityInfo{
							ID:       "ssh-weak-password",
							Name:     "ssh weak password",
							URL:      host,
							Type:     "SSH",
							Severity: "HIGH",
							Extract:  t.User + "/" + t.Pass,
							Request:  "[Command] whoami",
							Response: result,
						})
					}
					cancel()
				}
			}
			doneChan <- struct{}{}
		}()
	}

	// 分发任务
	go func() {
		for _, cred := range credentials {
			// 如果中途终止，就不要继续塞任务了
			if scanCtx.Err() != nil {
				break
			}
			// 解析 user:pass 格式
			parts := strings.SplitN(cred, ":", 2)
			if len(parts) != 2 {
				continue
			}
			user := parts[0]
			pass := parts[1]
			select {
			case <-scanCtx.Done():
				close(taskChan)
				return
			case taskChan <- task{User: user, Pass: pass}:
			}
		}
		close(taskChan)
	}()

	// 等待所有worker退出
	for range 5 {
		<-doneChan
	}
}

func sshMaxTimeoutErrorsFromContext(ctx context.Context) int {
	if value, ok := ctx.Value(sshMaxTimeoutErrorsContextKey{}).(int); ok {
		return value
	}
	return defaultSSHMaxTimeoutErrors
}

func isSSHTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	errText := strings.ToLower(err.Error())
	return strings.Contains(errText, "i/o timeout") ||
		strings.Contains(errText, "timeout") ||
		strings.Contains(errText, "deadline exceeded")
}

func SshConn(host, user, pass string) (bool, error) {
	return SshConnContext(context.Background(), host, user, pass)
}

func SshConnContext(ctx context.Context, host, user, pass string) (bool, error) {
	passwordAuthAttempted := false
	config := newSSHPasswordClientConfig(user, pass, &passwordAuthAttempted)

	client, err := dialSSHWithContext(ctx, host, config, defaultCrackConnectTimeout)
	if err != nil {
		return false, err
	}
	defer client.Close()

	if !passwordAuthAttempted {
		return false, errSSHPasswordAuthNotConfirmed
	}

	session, err := client.NewSession()
	if err != nil {
		return false, err
	}
	defer session.Close()

	return true, nil
}

func ExecuteSshCommand(host, username, password, command string) (string, error) {
	return ExecuteSshCommandContext(context.Background(), host, username, password, command)
}

func ExecuteSshCommandContext(ctx context.Context, host, username, password, command string) (string, error) {
	passwordAuthAttempted := false
	config := newSSHPasswordClientConfig(username, password, &passwordAuthAttempted)

	// Connect to the SSH server
	client, err := dialSSHWithContext(ctx, host, config, defaultCrackConnectTimeout)
	if err != nil {
		return "", fmt.Errorf("failed to dial: %v", err)
	}
	defer client.Close()

	if !passwordAuthAttempted {
		return "", errSSHPasswordAuthNotConfirmed
	}

	// Create a session to execute the command
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	// Run the command and capture the output
	stopClose := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = client.Close()
		case <-stopClose:
		}
	}()
	output, err := session.CombinedOutput(command)
	close(stopClose)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("failed to run command: %v", err)
	}

	return string(output), nil
}

func newSSHPasswordClientConfig(user, pass string, passwordAuthAttempted *bool) *ssh.ClientConfig {
	return &ssh.ClientConfig{
		Config: ssh.Config{
			KeyExchanges: sshCrackKeyExchanges(),
			MACs:         sshCrackMACs(),
		},
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PasswordCallback(func() (string, error) {
				*passwordAuthAttempted = true
				return pass, nil
			}),
			ssh.KeyboardInteractive(func(_ string, _ string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i, question := range questions {
					if i < len(echos) && !echos[i] && isSSHPasswordQuestion(question) {
						answers[i] = pass
						*passwordAuthAttempted = true
					}
				}
				return answers, nil
			}),
		},
		Timeout:         defaultCrackConnectTimeout,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
}

func sshCrackKeyExchanges() []string {
	supported := ssh.SupportedAlgorithms()
	insecure := ssh.InsecureAlgorithms()
	return appendUniqueSSHAlgorithms(supported.KeyExchanges, insecure.KeyExchanges...)
}

func sshCrackMACs() []string {
	supported := ssh.SupportedAlgorithms()
	insecure := ssh.InsecureAlgorithms()
	return appendUniqueSSHAlgorithms(supported.MACs, insecure.MACs...)
}

func appendUniqueSSHAlgorithms(base []string, extra ...string) []string {
	result := append([]string{}, base...)
	seen := make(map[string]struct{}, len(result)+len(extra))
	for _, algo := range result {
		seen[algo] = struct{}{}
	}
	for _, algo := range extra {
		if _, ok := seen[algo]; ok {
			continue
		}
		result = append(result, algo)
		seen[algo] = struct{}{}
	}
	return result
}

func isSSHPasswordQuestion(question string) bool {
	question = strings.ToLower(question)
	return strings.Contains(question, "pass") ||
		strings.Contains(question, "password") ||
		strings.Contains(question, "passcode") ||
		strings.Contains(question, "pin")
}

func dialSSHWithContext(ctx context.Context, host string, config *ssh.ClientConfig, timeout time.Duration) (*ssh.Client, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if timeout <= 0 {
		timeout = defaultCrackConnectTimeout
	}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(dialCtx, "tcp", host)
	if err != nil {
		return nil, err
	}

	_ = conn.SetDeadline(time.Now().Add(timeout))
	stopClose := make(chan struct{})
	go func() {
		select {
		case <-dialCtx.Done():
			_ = conn.Close()
		case <-stopClose:
		}
	}()

	clientConn, chans, reqs, err := ssh.NewClientConn(conn, host, config)
	close(stopClose)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})

	return ssh.NewClient(clientConn, chans, reqs), nil
}
