package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"hephaestus-backend/pkg/config"
)

const anthropicVersion = "2023-06-01"

func generateAnthropicChatCompletionWithTools(ctx context.Context, cfg config.AIConfig, messages []ChatMessage, tools []ChatTool, maxTokens int, temperature float64) (ChatCompletionResult, error) {
	if !cfg.Enabled {
		return ChatCompletionResult{}, fmt.Errorf("AI 未启用")
	}

	baseURL, err := validateAIBaseURL(cfg.API)
	if err != nil {
		return ChatCompletionResult{}, err
	}
	if strings.TrimSpace(cfg.Key) == "" {
		return ChatCompletionResult{}, fmt.Errorf("API Key 不能为空")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return ChatCompletionResult{}, fmt.Errorf("模型不能为空")
	}
	if len(messages) == 0 {
		return ChatCompletionResult{}, fmt.Errorf("消息不能为空")
	}
	if maxTokens <= 0 {
		maxTokens = 1200
	}

	system, anthropicMessages := convertAnthropicMessages(messages)
	if len(anthropicMessages) == 0 {
		return ChatCompletionResult{}, fmt.Errorf("消息不能为空")
	}

	payload := map[string]any{
		"model":       strings.TrimSpace(cfg.Model),
		"messages":    anthropicMessages,
		"max_tokens":  maxTokens,
		"temperature": temperature,
	}
	if system != "" {
		payload["system"] = system
	}
	if len(tools) > 0 {
		payload["tools"] = convertAnthropicTools(tools)
		payload["tool_choice"] = map[string]string{"type": "auto"}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ChatCompletionResult{}, fmt.Errorf("序列化 Claude 请求失败: %w", err)
	}

	client := &http.Client{
		Transport: newModelRequestTransport(cfg),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("重定向次数过多")
			}
			_, err := validateAIBaseURL(req.URL.String())
			return err
		},
	}

	endpoints := anthropicMessagesEndpoints(baseURL.String())
	var lastErr error
	for index, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return ChatCompletionResult{}, fmt.Errorf("创建 Claude 请求失败: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", strings.TrimSpace(cfg.Key))
		req.Header.Set("anthropic-version", anthropicVersion)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("Claude 服务请求失败: %w", err)
			continue
		}

		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
		_ = resp.Body.Close()
		if readErr != nil {
			return ChatCompletionResult{}, fmt.Errorf("读取 Claude 服务响应失败: %w", readErr)
		}
		contentType := resp.Header.Get("Content-Type")
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = formatAIServiceStatusError("Claude 服务", resp.Status, contentType, respBody)
			if shouldTryNextAIEndpoint(resp.StatusCode, contentType, respBody) && index < len(endpoints)-1 {
				continue
			}
			return ChatCompletionResult{}, lastErr
		}

		result, err := parseAnthropicMessageResponse(respBody)
		if err != nil {
			lastErr = formatAIServiceParseError("Claude 服务", contentType, respBody, err)
			if shouldTryNextAIEndpoint(resp.StatusCode, contentType, respBody) && index < len(endpoints)-1 {
				continue
			}
			return ChatCompletionResult{}, lastErr
		}
		if strings.TrimSpace(result.Content) == "" && len(result.ToolCalls) == 0 {
			return result, emptyChatCompletionError(result)
		}
		return result, nil
	}

	if lastErr != nil {
		return ChatCompletionResult{}, lastErr
	}
	return ChatCompletionResult{}, fmt.Errorf("Claude 服务请求失败")
}

func anthropicMessagesEndpoint(base string) string {
	endpoints := anthropicMessagesEndpoints(base)
	if len(endpoints) == 0 {
		return ""
	}
	return endpoints[0]
}

func convertAnthropicMessages(messages []ChatMessage) (string, []map[string]string) {
	var systemParts []string
	out := make([]map[string]string, 0, len(messages))

	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		if role == "system" {
			systemParts = append(systemParts, content)
			continue
		}
		if role != "assistant" {
			role = "user"
		}
		if len(out) > 0 && out[len(out)-1]["role"] == role {
			out[len(out)-1]["content"] += "\n\n" + content
			continue
		}
		out = append(out, map[string]string{
			"role":    role,
			"content": content,
		})
	}

	return strings.Join(systemParts, "\n\n"), out
}

func convertAnthropicTools(tools []ChatTool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Function.Name)
		if name == "" {
			continue
		}
		out = append(out, map[string]any{
			"name":         name,
			"description":  strings.TrimSpace(tool.Function.Description),
			"input_schema": tool.Function.Parameters,
		})
	}
	return out
}

func parseAnthropicMessageResponse(body []byte) (ChatCompletionResult, error) {
	var raw struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		if isLikelyNonJSONResponse("", body) {
			return ChatCompletionResult{}, nonJSONServiceResponseError("Claude 服务", "", body)
		}
		return ChatCompletionResult{}, fmt.Errorf("解析 Claude 服务响应失败: %w", err)
	}

	var textParts []string
	out := ChatCompletionResult{FinishReason: strings.TrimSpace(raw.StopReason)}
	for _, item := range raw.Content {
		switch item.Type {
		case "text":
			if strings.TrimSpace(item.Text) != "" {
				textParts = append(textParts, strings.TrimSpace(item.Text))
			}
		case "tool_use":
			if strings.TrimSpace(item.Name) == "" {
				continue
			}
			args := item.Input
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			out.ToolCalls = append(out.ToolCalls, ChatToolCall{
				ID:        strings.TrimSpace(item.ID),
				Type:      "function",
				Name:      strings.TrimSpace(item.Name),
				Arguments: normalizeChatToolCallArguments(args),
			})
		}
	}
	out.Content = strings.TrimSpace(strings.Join(textParts, "\n\n"))
	return out, nil
}
