package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/logger"
)

func sendWeChatNotificationWithConfig(wechatConfig config.WeChatConfig, title, content string) error {
	// 检查是否启用
	if !wechatConfig.Enabled || wechatConfig.WebhookURL == "" {
		return nil // 未启用，静默返回
	}

	// 构建消息体
	message := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": fmt.Sprintf("## %s\n\n%s", title, content),
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

	resp, err := client.Post(wechatConfig.WebhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("发送企业微信通知失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("企业微信通知返回错误状态码: %d", resp.StatusCode)
	}

	return nil
}

// SendWeChatNotification 发送企业微信通知（使用系统配置）
func SendWeChatNotification(title, content string) error {
	wechatConfig, err := config.GetWeChatConfig()
	if err != nil {
		return fmt.Errorf("获取企业微信配置失败: %w", err)
	}

	return sendWeChatNotificationWithConfig(wechatConfig, title, content)
}

// SendWeChatTestNotification 测试企业微信通知
func SendWeChatTestNotification(webhookURL string) error {
	if webhookURL == "" {
		return fmt.Errorf("Webhook URL 不能为空")
	}

	testConfig := config.WeChatConfig{
		WebhookURL: webhookURL,
		Enabled:    true,
	}

	title := "🔔 企业微信通知通道测试"
	content := fmt.Sprintf(`测试消息发送成功

时间: %s
说明: 该消息用于验证企业微信机器人配置是否可用。`,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	return sendWeChatNotificationWithConfig(testConfig, title, content)
}

// NotifyWeChatVulnerabilityFound 通知企业微信发现漏洞
func NotifyWeChatVulnerabilityFound(taskID, taskName, vulnName, severity, vulnURL string) error {
	wechatConfig, err := config.GetWeChatConfig()
	if err != nil {
		return fmt.Errorf("获取企业微信配置失败: %w", err)
	}

	// 检查是否启用漏洞发现通知
	if !wechatConfig.Enabled || !wechatConfig.NotifyOnVulnFound {
		return nil
	}

	// 严重程度颜色映射
	severityColor := map[string]string{
		"critical": "warning", // 红色
		"high":     "warning", // 橙色
		"medium":   "warning", // 黄色
		"low":      "info",    // 蓝色
		"info":     "info",    // 绿色
	}
	color := severityColor[severity]
	if color == "" {
		color = "info"
	}

	title := "🔍 发现漏洞"
	content := fmt.Sprintf(`> 任务名称: <font color="%s">%s</font>
> 任务ID: %s
> 漏洞名称: <font color="%s">%s</font>
> 严重程度: <font color="%s">%s</font>
> 漏洞URL: %s
> 
> 时间: %s`,
		color,
		taskName,
		taskID,
		color,
		vulnName,
		color,
		severity,
		vulnURL,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	if err := SendWeChatNotification(title, content); err != nil {
		logger.Warning("发送企业微信漏洞通知失败: %v", err)
		return err
	}

	return nil
}

// NotifyWeChatScanComplete 通知企业微信扫描完成
func NotifyWeChatScanComplete(taskID, taskName string, totalVulns int, status string) error {
	wechatConfig, err := config.GetWeChatConfig()
	if err != nil {
		return fmt.Errorf("获取企业微信配置失败: %w", err)
	}

	// 检查是否启用扫描完成通知
	if !wechatConfig.Enabled || !wechatConfig.NotifyOnScanComplete {
		return nil
	}

	statusEmoji := "✅"
	statusColor := "info"
	if status == "failed" {
		statusEmoji = "❌"
		statusColor = "warning"
	} else if status == "cancelled" {
		statusEmoji = "⏹️"
		statusColor = "comment"
	}

	title := fmt.Sprintf("%s 扫描任务完成", statusEmoji)
	content := fmt.Sprintf(`> 任务名称: <font color="%s">%s</font>
> 任务ID: %s
> 状态: <font color="%s">%s</font>
> 发现漏洞数: **%d**
> 
> 完成时间: %s`,
		statusColor,
		taskName,
		taskID,
		statusColor,
		status,
		totalVulns,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	if err := SendWeChatNotification(title, content); err != nil {
		logger.Warning("发送企业微信扫描完成通知失败: %v", err)
		return err
	}

	return nil
}
