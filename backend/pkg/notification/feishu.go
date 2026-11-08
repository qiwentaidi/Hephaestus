package notification

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/logger"
)

// calculateFeishuSign 计算飞书 Webhook 签名
// 算法：timestamp + "\n" + secret -> HmacSHA256 -> Base64
func calculateFeishuSign(timestamp int64, secret string) string {
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)
	mac := hmac.New(sha256.New, []byte(stringToSign))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func sendFeishuNotificationWithConfig(feishuConfig config.FeishuConfig, title, content string) error {
	// 检查是否启用
	if !feishuConfig.Enabled || feishuConfig.WebhookURL == "" {
		return nil // 未启用，静默返回
	}

	// 生成时间戳（秒）
	timestamp := time.Now().Unix()

	// 构建消息体
	message := map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"header": map[string]interface{}{
				"title": map[string]interface{}{
					"tag":     "plain_text",
					"content": title,
				},
				"template": "blue",
			},
			"elements": []map[string]interface{}{
				{
					"tag":     "markdown",
					"content": content,
				},
			},
		},
	}

	// 如果配置了签名密钥，添加签名
	if feishuConfig.Secret != "" {
		sign := calculateFeishuSign(timestamp, feishuConfig.Secret)
		message["timestamp"] = strconv.FormatInt(timestamp, 10)
		message["sign"] = sign
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	// 发送 HTTP 请求
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Post(feishuConfig.WebhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("发送飞书通知失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("飞书通知返回错误状态码: %d", resp.StatusCode)
	}

	return nil
}

// SendFeishuNotification 发送飞书通知（使用系统配置）
func SendFeishuNotification(title, content string) error {
	feishuConfig, err := config.GetFeishuConfig()
	if err != nil {
		return fmt.Errorf("获取飞书配置失败: %w", err)
	}

	return sendFeishuNotificationWithConfig(feishuConfig, title, content)
}

// SendFeishuTestNotification 测试飞书通知
func SendFeishuTestNotification(webhookURL, secret string) error {
	if webhookURL == "" {
		return fmt.Errorf("Webhook URL 不能为空")
	}

	testConfig := config.FeishuConfig{
		WebhookURL: webhookURL,
		Secret:     secret,
		Enabled:    true,
	}

	title := "🔔 飞书通知通道测试"
	content := fmt.Sprintf(`测试消息发送成功

时间: %s
说明: 该消息用于验证飞书机器人配置是否可用。`,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	return sendFeishuNotificationWithConfig(testConfig, title, content)
}

// NotifyFeishuVulnerabilityFound 通知飞书发现漏洞
func NotifyFeishuVulnerabilityFound(taskID, taskName, vulnName, severity, vulnURL string) error {
	feishuConfig, err := config.GetFeishuConfig()
	if err != nil {
		return fmt.Errorf("获取飞书配置失败: %w", err)
	}

	// 检查是否启用漏洞发现通知
	if !feishuConfig.Enabled || !feishuConfig.NotifyOnVulnFound {
		return nil
	}

	// 严重程度颜色映射
	severityEmoji := map[string]string{
		"critical": "🔴",
		"high":     "🟠",
		"medium":   "🟡",
		"low":      "🔵",
		"info":     "🟢",
	}
	emoji := severityEmoji[severity]
	if emoji == "" {
		emoji = "⚪"
	}

	title := "🔍 发现漏洞"
	content := fmt.Sprintf(`**任务名称:** %s
**任务ID:** %s
**漏洞名称:** %s
**严重程度:** %s %s
**漏洞URL:** %s

> 时间: %s`,
		taskName,
		taskID,
		vulnName,
		emoji,
		severity,
		vulnURL,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	if err := SendFeishuNotification(title, content); err != nil {
		logger.Warning("发送飞书漏洞通知失败: %v", err)
		return err
	}

	return nil
}

// NotifyFeishuScanComplete 通知飞书扫描完成
func NotifyFeishuScanComplete(taskID, taskName string, totalVulns int, status string) error {
	feishuConfig, err := config.GetFeishuConfig()
	if err != nil {
		return fmt.Errorf("获取飞书配置失败: %w", err)
	}

	// 检查是否启用扫描完成通知
	if !feishuConfig.Enabled || !feishuConfig.NotifyOnScanComplete {
		return nil
	}

	statusEmoji := "✅"
	if status == "failed" {
		statusEmoji = "❌"
	} else if status == "cancelled" {
		statusEmoji = "⏹️"
	}

	title := fmt.Sprintf("%s 扫描任务完成", statusEmoji)
	content := fmt.Sprintf(`**任务名称:** %s
**任务ID:** %s
**状态:** %s
**发现漏洞数:** %d

> 完成时间: %s`,
		taskName,
		taskID,
		status,
		totalVulns,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	if err := SendFeishuNotification(title, content); err != nil {
		logger.Warning("发送飞书扫描完成通知失败: %v", err)
		return err
	}

	return nil
}
