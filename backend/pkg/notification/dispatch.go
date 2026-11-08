package notification

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/logger"
)

// NotificationEvent is the provider-neutral payload delivered to every
// enabled channel subscribed to EventType.
type NotificationEvent struct {
	EventType  string                 `json:"eventType"`
	Title      string                 `json:"title"`
	Message    string                 `json:"message"`
	Data       map[string]interface{} `json:"data"`
	OccurredAt time.Time              `json:"occurredAt"`
}

func eventEnabled(events []string, eventType string) bool {
	for _, event := range events {
		if event == eventType {
			return true
		}
	}
	return false
}

// DispatchEvent fans out one normalized event to all configured destinations.
func DispatchEvent(event NotificationEvent) error {
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	if event.Data == nil {
		event.Data = map[string]interface{}{}
	}
	settings, err := config.GetNotificationConfig()
	if err != nil {
		return fmt.Errorf("获取统一通知配置失败: %w", err)
	}
	var errors []string
	for _, channel := range settings.Channels {
		if !channel.Enabled || !eventEnabled(channel.Events, event.EventType) {
			continue
		}
		if err := dispatchChannel(channel, event); err != nil {
			name := channel.Name
			if name == "" {
				name = channel.Type
			}
			errors = append(errors, fmt.Sprintf("%s: %v", name, err))
		}
	}
	if len(errors) > 0 {
		logger.Warning("部分统一通知发送失败: %s", strings.Join(errors, "; "))
	}
	return nil
}

func dispatchChannel(channel config.NotificationChannelConfig, event NotificationEvent) error {
	switch channel.Type {
	case config.NotificationChannelDingTalk:
		return sendDingTalkNotificationWithConfig(config.DingTalkConfig{WebhookURL: channel.WebhookURL, Secret: channel.Secret, Enabled: true}, event.Title, event.Message)
	case config.NotificationChannelWeChat:
		return sendWeChatNotificationWithConfig(config.WeChatConfig{WebhookURL: channel.WebhookURL, Enabled: true}, event.Title, event.Message)
	case config.NotificationChannelFeishu:
		return sendFeishuNotificationWithConfig(config.FeishuConfig{WebhookURL: channel.WebhookURL, Secret: channel.Secret, Enabled: true}, event.Title, event.Message)
	case config.NotificationChannelWebhook:
		return sendWebhook(channel, event)
	case config.NotificationChannelJira:
		return sendJira(channel, event)
	case config.NotificationChannelEmail:
		return sendEmail(channel, event)
	default:
		return fmt.Errorf("不支持的通知通道类型: %s", channel.Type)
	}
}

func sendWebhook(channel config.NotificationChannelConfig, event NotificationEvent) error {
	if strings.TrimSpace(channel.WebhookURL) == "" {
		return fmt.Errorf("Webhook URL 不能为空")
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("序列化 Webhook 事件失败: %w", err)
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Post(channel.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("发送 Webhook 事件失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Webhook 返回错误状态码: %d", resp.StatusCode)
	}
	return nil
}

func sendJira(channel config.NotificationChannelConfig, event NotificationEvent) error {
	baseURL := strings.TrimRight(strings.TrimSpace(channel.BaseURL), "/")
	if baseURL == "" || channel.ProjectKey == "" || channel.Username == "" || channel.Token == "" {
		return fmt.Errorf("Jira 地址、项目 Key、用户名和 API Token 均不能为空")
	}
	endpoint := baseURL
	if !strings.HasSuffix(endpoint, "/rest/api/2/issue") {
		endpoint += "/rest/api/2/issue"
	}
	description := event.Message
	payload := map[string]interface{}{"fields": map[string]interface{}{
		"project":     map[string]string{"key": channel.ProjectKey},
		"summary":     event.Title,
		"description": description,
		"issuetype":   map[string]string{"name": valueOrDefault(channel.IssueType, "Task")},
	}}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化 Jira 事件失败: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("创建 Jira 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(channel.Username+":"+channel.Token)))
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("发送 Jira 事件失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Jira 返回错误状态码: %d", resp.StatusCode)
	}
	return nil
}

func sendEmail(channel config.NotificationChannelConfig, event NotificationEvent) error {
	host := strings.TrimSpace(channel.SMTPHost)
	from := strings.TrimSpace(channel.From)
	recipients := splitRecipients(channel.Recipients)
	if host == "" || from == "" || len(recipients) == 0 {
		return fmt.Errorf("SMTP 主机、发件人和收件人不能为空")
	}
	port := channel.SMTPPort
	if port == 0 {
		port = 587
	}
	message := strings.Join([]string{
		"From: " + from,
		"To: " + strings.Join(recipients, ", "),
		"Subject: " + event.Title,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		event.Message,
	}, "\r\n")
	var auth smtp.Auth
	if channel.SMTPUsername != "" {
		auth = smtp.PlainAuth("", channel.SMTPUsername, channel.SMTPPassword, host)
	}
	if err := smtp.SendMail(host+":"+strconv.Itoa(port), auth, from, recipients, []byte(message)); err != nil {
		return fmt.Errorf("发送邮件失败: %w", err)
	}
	return nil
}

func splitRecipients(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' || r == '\n' })
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if address := strings.TrimSpace(part); address != "" {
			result = append(result, address)
		}
	}
	return result
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// SendTest sends a provider-specific test event without requiring the channel
// to be enabled or persisted first.
func SendTest(channel config.NotificationChannelConfig) error {
	if channel.Type == "" {
		return fmt.Errorf("通知通道类型不能为空")
	}
	return dispatchChannel(channel, NotificationEvent{
		EventType:  "test",
		Title:      "Hephaestus 通知通道测试",
		Message:    "这是一条统一通知事件模型的测试消息。",
		Data:       map[string]interface{}{"source": "settings"},
		OccurredAt: time.Now(),
	})
}
