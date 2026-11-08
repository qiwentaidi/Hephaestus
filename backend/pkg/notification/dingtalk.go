package notification

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/logger"
)

// calculateDingTalkSign 计算钉钉 Webhook 签名
// 算法：timestamp + "\n" + secret -> HmacSHA256 -> Base64 -> URL编码
func calculateDingTalkSign(timestamp int64, secret string) string {
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stringToSign))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return url.QueryEscape(sign)
}

// buildDingTalkWebhookURL 构建带签名的钉钉 Webhook URL
func buildDingTalkWebhookURL(webhookURL, secret string) (string, error) {
	if secret == "" {
		// 没有密钥，直接返回原 URL
		return webhookURL, nil
	}

	// 解析 URL
	u, err := url.Parse(webhookURL)
	if err != nil {
		return "", fmt.Errorf("解析 Webhook URL 失败: %w", err)
	}

	// 生成时间戳（毫秒）
	timestamp := time.Now().UnixNano() / 1e6

	// 计算签名
	sign := calculateDingTalkSign(timestamp, secret)

	// 添加签名参数
	q := u.Query()
	q.Set("timestamp", strconv.FormatInt(timestamp, 10))
	q.Set("sign", sign)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func sendDingTalkNotificationWithConfig(dingTalkConfig config.DingTalkConfig, title, content string) error {
	// 检查是否启用
	if !dingTalkConfig.Enabled || dingTalkConfig.WebhookURL == "" {
		return nil // 未启用，静默返回
	}

	// 构建带签名的 Webhook URL
	webhookURL, err := buildDingTalkWebhookURL(dingTalkConfig.WebhookURL, dingTalkConfig.Secret)
	if err != nil {
		return fmt.Errorf("构建 Webhook URL 失败: %w", err)
	}

	// 构建消息体
	message := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  content,
		},
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	// 发送 HTTP 请求
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("发送钉钉通知失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("钉钉通知返回错误状态码: %d", resp.StatusCode)
	}

	return nil
}

// SendDingTalkNotification 发送钉钉通知（使用系统配置）
func SendDingTalkNotification(title, content string) error {
	dingTalkConfig, err := config.GetDingTalkConfig()
	if err != nil {
		return fmt.Errorf("获取钉钉配置失败: %w", err)
	}

	return sendDingTalkNotificationWithConfig(dingTalkConfig, title, content)
}

// SendDingTalkTestNotification 测试钉钉通知（使用用户提供配置）
func SendDingTalkTestNotification(webhookURL, secret string) error {
	if webhookURL == "" {
		return fmt.Errorf("Webhook URL 不能为空")
	}

	testConfig := config.DingTalkConfig{
		WebhookURL: webhookURL,
		Secret:     secret,
		Enabled:    true,
	}

	title := "🔔 钉钉通知通道测试"
	content := fmt.Sprintf(`测试消息发送成功

时间: %s
说明: 该消息用于验证钉钉机器人配置是否可用。`,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	return sendDingTalkNotificationWithConfig(testConfig, title, content)
}

// NotifyVulnerabilityFound 通知发现漏洞（统一调用所有平台）
func NotifyVulnerabilityFound(taskID, taskName, vulnName, severity, vulnURL string) error {
	return DispatchEvent(NotificationEvent{
		EventType: config.NotificationEventVulnerabilityFound,
		Title:     "🔍 发现漏洞",
		Message:   fmt.Sprintf("任务名称: %s\n任务 ID: %s\n漏洞名称: %s\n严重程度: %s\n漏洞 URL: %s", taskName, taskID, vulnName, severity, vulnURL),
		Data: map[string]interface{}{
			"taskId": taskID, "taskName": taskName, "vulnerability": vulnName,
			"severity": severity, "url": vulnURL,
		},
	})
}

// AIFindingEvent builds the vulnerability_found event for a finding recorded by
// the AI workbench. Only the title, severity and target are reported.
func AIFindingEvent(title, severity, target string) NotificationEvent {
	title = strings.TrimSpace(title)
	severity = strings.ToLower(strings.TrimSpace(severity))
	target = strings.TrimSpace(target)
	return NotificationEvent{
		EventType: config.NotificationEventVulnerabilityFound,
		Title:     "🔍 发现漏洞",
		Message:   fmt.Sprintf("漏洞标题: %s\n风险级别: %s\n目标: %s", title, severity, target),
		Data: map[string]interface{}{
			"source":   "ai_workbench",
			"title":    title,
			"severity": severity,
			"target":   target,
		},
	}
}

// NotifyAIFindingRecorded dispatches an AI workbench finding through the same
// vulnerability_found subscription that scan results use.
func NotifyAIFindingRecorded(title, severity, target string) error {
	return DispatchEvent(AIFindingEvent(title, severity, target))
}

// notifyDingTalkVulnerabilityFound 钉钉漏洞通知（内部函数）
func notifyDingTalkVulnerabilityFound(taskID, taskName, vulnName, severity, vulnURL string) error {
	dingTalkConfig, err := config.GetDingTalkConfig()
	if err != nil {
		return fmt.Errorf("获取钉钉配置失败: %w", err)
	}

	// 检查是否启用漏洞发现通知
	if !dingTalkConfig.Enabled || !dingTalkConfig.NotifyOnVulnFound {
		return nil
	}

	// 严重程度颜色映射
	severityColor := map[string]string{
		"critical": "#FF0000", // 红色
		"high":     "#FF6600", // 橙色
		"medium":   "#FFAA00", // 黄色
		"low":      "#00AAFF", // 蓝色
		"info":     "#00AA00", // 绿色
	}
	color := severityColor[severity]
	if color == "" {
		color = "#666666" // 默认灰色
	}

	title := "🔍 发现漏洞"
	content := fmt.Sprintf(`## 🔍 发现漏洞

**任务名称:** %s
**任务ID:** %s
**漏洞名称:** %s
**严重程度:** <font color="%s">%s</font>
**漏洞URL:** %s

> 时间: %s`,
		taskName,
		taskID,
		vulnName,
		color,
		severity,
		vulnURL,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	if err := SendDingTalkNotification(title, content); err != nil {
		logger.Warning("发送钉钉漏洞通知失败: %v", err)
		return err
	}

	return nil
}

// NotifyScanComplete 通知扫描完成（统一调用所有平台）
func NotifyScanComplete(taskID, taskName string, totalVulns int, status string) error {
	return DispatchEvent(NotificationEvent{
		EventType: config.NotificationEventScanComplete,
		Title:     "✅ 扫描任务完成",
		Message:   fmt.Sprintf("任务名称: %s\n任务 ID: %s\n状态: %s\n发现漏洞数: %d", taskName, taskID, status, totalVulns),
		Data: map[string]interface{}{
			"taskId": taskID, "taskName": taskName, "status": status,
			"totalVulnerabilities": totalVulns,
		},
	})
}

// notifyDingTalkScanComplete 钉钉扫描完成通知（内部函数）
func notifyDingTalkScanComplete(taskID, taskName string, totalVulns int, status string) error {
	dingTalkConfig, err := config.GetDingTalkConfig()
	if err != nil {
		return fmt.Errorf("获取钉钉配置失败: %w", err)
	}

	// 检查是否启用扫描完成通知
	if !dingTalkConfig.Enabled || !dingTalkConfig.NotifyOnScanComplete {
		return nil
	}

	title := "✅ 扫描任务完成"

	statusEmoji := "✅"
	if status == "failed" {
		statusEmoji = "❌"
	} else if status == "cancelled" {
		statusEmoji = "⏹️"
	}

	content := fmt.Sprintf(`## %s 扫描任务完成

**任务名称:** %s
**任务ID:** %s
**状态:** %s
**发现漏洞数:** %d

> 完成时间: %s`,
		statusEmoji,
		taskName,
		taskID,
		status,
		totalVulns,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	if err := SendDingTalkNotification(title, content); err != nil {
		logger.Warning("发送钉钉扫描完成通知失败: %v", err)
		return err
	}

	return nil
}
