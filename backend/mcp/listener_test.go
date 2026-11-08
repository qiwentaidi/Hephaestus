package mcp

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"hephaestus-backend/pkg/config"

	"github.com/gin-gonic/gin"
)

// TestMain 将配置目录固定到临时目录，避免测试读写真实配置文件。
// config 包的配置基础目录通过 sync.Once 缓存，必须在任何测试运行前设置。
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "hephaestus-mcp-test-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("HEPHAESTUS_CONFIG_DIR", dir); err != nil {
		panic(err)
	}
	config.SetRuntimeMode(config.RuntimeModeServer)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func TestNormalizeMCPListenHost(t *testing.T) {
	validCases := map[string]string{
		"":           mcpDefaultListenHost,
		"  ":         mcpDefaultListenHost,
		"localhost":  mcpDefaultListenHost,
		" localhost": mcpDefaultListenHost,
		"127.0.0.1":  mcpDefaultListenHost,
		"127.0.0.2":  mcpDefaultListenHost,
		"0.0.0.0":    mcpAllInterfacesListenHost,
	}
	for input, want := range validCases {
		got, err := NormalizeMCPListenHost(input)
		if err != nil {
			t.Fatalf("NormalizeMCPListenHost(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("NormalizeMCPListenHost(%q) = %q, want %q", input, got, want)
		}
	}

	invalidCases := []string{
		"not-an-ip",
		"::1",
		"192.0.2.1", // TEST-NET-1，不应存在于本机网卡
	}
	for _, input := range invalidCases {
		if got, err := NormalizeMCPListenHost(input); err == nil {
			t.Fatalf("NormalizeMCPListenHost(%q) = %q, want error", input, got)
		}
	}
}

func TestNormalizeMCPListenPort(t *testing.T) {
	validCases := map[string]string{
		"1":       "1",
		"8080":    "8080",
		" 18080 ": "18080",
		"65535":   "65535",
		"08080":   "8080",
	}
	for input, want := range validCases {
		got, err := NormalizeMCPListenPort(input)
		if err != nil {
			t.Fatalf("NormalizeMCPListenPort(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("NormalizeMCPListenPort(%q) = %q, want %q", input, got, want)
		}
	}

	invalidCases := []string{"", "0", "65536", "abc", "-1", "8080.5"}
	for _, input := range invalidCases {
		if got, err := NormalizeMCPListenPort(input); err == nil {
			t.Fatalf("NormalizeMCPListenPort(%q) = %q, want error", input, got)
		}
	}
}

func TestGetMCPListenerStateDefaults(t *testing.T) {
	ShutdownMCPListener(context.Background())

	mainPort := freeTCPPort(t)
	state := GetMCPListenerState(mainPort)

	if got := state["mcpListenHost"]; got != mcpDefaultListenHost {
		t.Fatalf("mcpListenHost = %v, want %q", got, mcpDefaultListenHost)
	}
	if got := state["mcpListenPort"]; got != strconv.Itoa(mainPort) {
		t.Fatalf("mcpListenPort = %v, want %d", got, mainPort)
	}
	if got := state["mcpServerURL"]; got != "" {
		t.Fatalf("mcpServerURL = %v, want empty", got)
	}
	if got := state["mcpAccessScope"]; got != "local" {
		t.Fatalf("mcpAccessScope = %v, want local", got)
	}
	if got := state["dedicated"]; got != false {
		t.Fatalf("dedicated = %v, want false", got)
	}
}

func TestApplyMCPListenerDedicatedLifecycle(t *testing.T) {
	router := newTestRouter()
	ctx := context.Background()
	t.Cleanup(func() { ShutdownMCPListener(ctx) })

	mainPort := freeTCPPort(t)
	dedPort := freeTCPPort(t)
	for dedPort == mainPort {
		dedPort = freeTCPPort(t)
	}

	// 1. 启动专用监听
	payload, err := ApplyMCPListener(ctx, router, mainPort, "127.0.0.1", strconv.Itoa(dedPort))
	if err != nil {
		t.Fatalf("ApplyMCPListener() error = %v", err)
	}
	if got := payload["dedicated"]; got != true {
		t.Fatalf("dedicated = %v, want true", got)
	}
	if got := payload["mcpListenPort"]; got != strconv.Itoa(dedPort) {
		t.Fatalf("mcpListenPort = %v, want %d", got, dedPort)
	}
	wantURL := fmt.Sprintf("http://127.0.0.1:%d", dedPort)
	if got := payload["mcpServerURL"]; got != wantURL {
		t.Fatalf("mcpServerURL = %v, want %q", got, wantURL)
	}

	// 2. 验证专用监听上的路由可达
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/ping", dedPort))
	if err != nil {
		t.Fatalf("GET dedicated listener error = %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("unexpected response: status=%d body=%s", resp.StatusCode, body)
	}

	// 3. 相同参数重复 apply 幂等
	payload, err = ApplyMCPListener(ctx, router, mainPort, "127.0.0.1", strconv.Itoa(dedPort))
	if err != nil {
		t.Fatalf("ApplyMCPListener() idempotent error = %v", err)
	}
	if got := payload["dedicated"]; got != true {
		t.Fatalf("dedicated after idempotent apply = %v, want true", got)
	}
	resp, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/ping", dedPort))
	if err != nil {
		t.Fatalf("GET after idempotent apply error = %v", err)
	}
	resp.Body.Close()

	// 4. 切回主端口后专用监听被关闭
	payload, err = ApplyMCPListener(ctx, router, mainPort, "", strconv.Itoa(mainPort))
	if err != nil {
		t.Fatalf("ApplyMCPListener() switch to main port error = %v", err)
	}
	if got := payload["dedicated"]; got != false {
		t.Fatalf("dedicated after switching to main port = %v, want false", got)
	}
	if got := payload["mcpListenHost"]; got != mcpDefaultListenHost {
		t.Fatalf("mcpListenHost = %v, want %q", got, mcpDefaultListenHost)
	}
	if got := payload["mcpAccessScope"]; got != "local" {
		t.Fatalf("mcpAccessScope = %v, want local", got)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, dialErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", dedPort), 200*time.Millisecond)
		if dialErr != nil {
			break
		}
		_ = conn.Close()
		if time.Now().After(deadline) {
			t.Fatalf("dedicated listener on port %d should be closed", dedPort)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestApplyMCPListenerRejectsDesktopMode(t *testing.T) {
	config.SetRuntimeMode(config.RuntimeModeDesktop)
	defer config.SetRuntimeMode(config.RuntimeModeServer)

	_, err := ApplyMCPListener(context.Background(), newTestRouter(), 8080, "127.0.0.1", "18080")
	if err == nil || !strings.Contains(err.Error(), "桌面模式") {
		t.Fatalf("ApplyMCPListener() in desktop mode should fail, err = %v", err)
	}
}

func TestApplyMCPListenerRejectsInvalidInput(t *testing.T) {
	router := newTestRouter()
	mainPort := freeTCPPort(t)

	if _, err := ApplyMCPListener(context.Background(), router, mainPort, "127.0.0.1", ""); err == nil {
		t.Fatal("empty port should be rejected")
	}
	if _, err := ApplyMCPListener(context.Background(), router, mainPort, "999.999.999.999", "18080"); err == nil {
		t.Fatal("invalid host should be rejected")
	}
}

func TestRestoreMCPListenerWithoutConfig(t *testing.T) {
	// 临时配置目录下无持久化配置，恢复应为无操作且状态回到默认值
	if err := RestoreMCPListener(newTestRouter(), freeTCPPort(t)); err != nil {
		t.Fatalf("RestoreMCPListener() error = %v", err)
	}

	mainPort := freeTCPPort(t)
	state := GetMCPListenerState(mainPort)
	if got := state["dedicated"]; got != false {
		t.Fatalf("dedicated = %v, want false", got)
	}
	if got := state["mcpServerURL"]; got != "" {
		t.Fatalf("mcpServerURL = %v, want empty", got)
	}
}

func TestSetAndGetMCPListenConfig(t *testing.T) {
	if err := config.SetMCPListenConfig("127.0.0.1", "18080"); err != nil {
		t.Fatalf("SetMCPListenConfig() error = %v", err)
	}
	host, port := config.GetMCPListenConfig()
	if host != "127.0.0.1" || port != "18080" {
		t.Fatalf("GetMCPListenConfig() = (%q, %q), want (127.0.0.1, 18080)", host, port)
	}

	// 清空后应返回空串
	if err := config.SetMCPListenConfig("", ""); err != nil {
		t.Fatalf("SetMCPListenConfig() clear error = %v", err)
	}
	host, port = config.GetMCPListenConfig()
	if host != "" || port != "" {
		t.Fatalf("GetMCPListenConfig() after clear = (%q, %q), want empty", host, port)
	}
}
