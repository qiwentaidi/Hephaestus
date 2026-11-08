package scan

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/projectdiscovery/interactsh/pkg/client"
	"github.com/projectdiscovery/interactsh/pkg/server"
)

type testWebScanOOBRequest struct {
	InteractshURL   string `json:"interactshURL" binding:"required"`
	InteractshToken string `json:"interactshToken"`
}

type oobInteractionResult struct {
	Protocol string
	URL      string
}

// TestWebScanOOBHandler 测试 OOB（Interactsh）可用性（注册 + 轮询 + 回连）
func TestWebScanOOBHandler(c *gin.Context) {
	var req testWebScanOOBRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的请求参数: " + err.Error()})
		return
	}

	targetURL, err := normalizeOOBURL(req.InteractshURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "无效的 Interactsh URL: " + err.Error()})
		return
	}

	result, err := probeInteractshWorkflow(targetURL, strings.TrimSpace(req.InteractshToken))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": "OOB 测试失败: " + err.Error()})
		return
	}

	message := fmt.Sprintf("测试成功：已完成注册与轮询，并捕获到 %s 回连", strings.ToUpper(result.Protocol))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": message,
		"data": gin.H{
			"url":           targetURL,
			"interactURL":   result.URL,
			"interaction":   result.Protocol,
			"workflowCheck": true,
		},
	})
}

func normalizeOOBURL(raw string) (string, error) {
	target := strings.TrimSpace(raw)
	if target == "" {
		return "", fmt.Errorf("URL 不能为空")
	}

	if !strings.Contains(target, "://") {
		target = "https://" + target
	}

	parsed, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("缺少主机名")
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("仅支持 http/https 协议")
	}

	// Interactsh ServerURL 只需要协议 + 主机(含端口)，避免路径影响 /register /poll 路由
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return strings.TrimRight(parsed.String(), "/"), nil
}

func probeInteractshWorkflow(serverURL, token string) (*oobInteractionResult, error) {
	cli, err := client.New(&client.Options{
		ServerURL:           serverURL,
		Token:               token,
		DisableHTTPFallback: true,
		KeepAliveInterval:   0,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = cli.Close()
	}()

	interactURL := cli.URL()
	if interactURL == "" {
		return nil, fmt.Errorf("未获取到 Interactsh 回连域名")
	}

	resultCh := make(chan string, 1)
	if err := cli.StartPolling(1*time.Second, func(interaction *server.Interaction) {
		select {
		case resultCh <- strings.ToLower(strings.TrimSpace(interaction.Protocol)):
		default:
		}
	}); err != nil {
		return nil, fmt.Errorf("轮询启动失败: %w", err)
	}
	defer func() {
		_ = cli.StopPolling()
	}()

	if err := triggerInteraction(interactURL); err != nil {
		return nil, err
	}

	select {
	case protocol := <-resultCh:
		if protocol == "" {
			protocol = "unknown"
		}
		return &oobInteractionResult{
			Protocol: protocol,
			URL:      interactURL,
		}, nil
	case <-time.After(12 * time.Second):
		return nil, fmt.Errorf("未在超时时间内收到回连，请检查 Interactsh 服务、DNS 解析或 Token 权限")
	}
}

func triggerInteraction(interactURL string) error {
	var errs []string

	// 1) 优先触发 DNS 回连
	dnsHost := normalizeHostForLookup(interactURL)
	if err := lookupDNS(dnsHost); err != nil {
		errs = append(errs, "DNS触发失败: "+err.Error())
	}

	// 2) 再尝试触发 HTTP 回连（即使 DNS 失败，也可能直接成功）
	if err := hitHTTP(interactURL); err != nil {
		errs = append(errs, "HTTP触发失败: "+err.Error())
	}

	if len(errs) == 2 {
		return errors.New(strings.Join(errs, "；"))
	}
	return nil
}

func normalizeHostForLookup(input string) string {
	host := strings.TrimSpace(input)
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func lookupDNS(host string) error {
	if host == "" {
		return fmt.Errorf("回连域名为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	_, err := net.DefaultResolver.LookupHost(ctx, host)
	return err
}

func hitHTTP(host string) error {
	if host == "" {
		return fmt.Errorf("回连域名为空")
	}
	req, err := http.NewRequest(http.MethodGet, "http://"+host, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Hephaestus-OOB-Check/1.0")

	httpClient := &http.Client{Timeout: 6 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
