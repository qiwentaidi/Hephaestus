package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"hephaestus-backend/pkg/toolnetwork"
)

func main() {
	if len(os.Args) < 2 {
		panic("serve or relay required")
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve()
	case "relay":
		err = relay(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func serve() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	state := func() (toolnetwork.State, error) { return toolnetwork.ReadState("/policy/state.json") }
	g, err := toolnetwork.NewGateway(ctx, state, toolnetwork.FileAudit("/audit/requests.jsonl"))
	if err != nil {
		return err
	}
	defer g.Close()
	if _, err = state(); err != nil {
		return err
	}
	if err = os.WriteFile("/channel/ca.pem", g.CA.PEM, 0644); err != nil {
		return err
	}
	socket := "/channel/proxy.sock"
	_ = os.Remove(socket)
	unix, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer unix.Close()
	if err = os.Chmod(socket, 0777); err != nil {
		return err
	}
	tcp, err := net.Listen("tcp", ":8080")
	if err != nil {
		return err
	}
	defer tcp.Close()
	unixServer := &http.Server{Handler: g.Handler(false), ReadTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 64 << 10}
	tcpServer := &http.Server{Handler: g.Handler(true), ReadTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 64 << 10}
	defer unixServer.Close()
	defer tcpServer.Close()
	errs := make(chan error, 2)
	go func() { errs <- unixServer.Serve(unix) }()
	go func() { errs <- tcpServer.Serve(tcp) }()
	// Publish readiness only after both sockets and the public CA exist.
	if err = os.WriteFile("/audit/ready.tmp", g.CA.PEM, 0644); err != nil {
		return err
	}
	if err = os.Rename("/audit/ready.tmp", "/audit/ready"); err != nil {
		return err
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errs:
			return err
		case <-ticker.C:
			if _, err := state(); err != nil {
				return err
			}
		}
	}
}
func relay(args []string) error {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return fmt.Errorf("command required")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:18080")
	if err != nil {
		return err
	}
	defer listener.Close()
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer client.Close()
				upstream, err := net.Dial("unix", "/channel/proxy.sock")
				if err != nil {
					return
				}
				defer upstream.Close()
				done := make(chan struct{})
				go func() {
					_, _ = io.Copy(upstream, client)
					if c, ok := upstream.(*net.UnixConn); ok {
						_ = c.CloseWrite()
					}
					close(done)
				}()
				_, _ = io.Copy(client, upstream)
				client.Close()
				<-done
			}()
		}
	}()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cert := "/channel/ca.pem"
	cmd.Env = append(os.Environ(), "HTTP_PROXY=http://127.0.0.1:18080", "HTTPS_PROXY=http://127.0.0.1:18080", "http_proxy=http://127.0.0.1:18080", "https_proxy=http://127.0.0.1:18080", "NO_PROXY=", "no_proxy=", "ALL_PROXY=", "all_proxy=", "SSL_CERT_FILE="+cert, "REQUESTS_CA_BUNDLE="+cert, "CURL_CA_BUNDLE="+cert, "PIP_CERT="+cert, "NODE_EXTRA_CA_CERTS="+cert, "NODE_USE_ENV_PROXY=1")
	if cwd, err := os.Getwd(); err == nil {
		tmp := filepath.Join(cwd, "tmp")
		_ = os.MkdirAll(tmp, 0700)
		cmd.Env = append(cmd.Env, "TMPDIR="+tmp, "TMP="+tmp, "TEMP="+tmp)
	}
	if err = cmd.Run(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			os.Exit(e.ExitCode())
		}
		return err
	}
	return nil
}
