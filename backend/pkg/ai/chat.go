package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"hephaestus-backend/pkg/config"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatTool struct {
	Type     string                 `json:"type"`
	Function ChatFunctionDefinition `json:"function"`
}

type ChatFunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ChatToolCall struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ChatCompletionResult struct {
	Content      string
	ToolCalls    []ChatToolCall
	FinishReason string
}

type ChatStreamDelta struct {
	Delta       string
	Accumulated string
}

type ChatStreamHandler func(ChatStreamDelta) error

// GenerateText sends a minimal OpenAI-compatible chat completion request.
func GenerateText(ctx context.Context, cfg config.AIConfig, messages []ChatMessage, maxTokens int, temperature float64) (string, error) {
	result, err := GenerateChatCompletionWithTools(ctx, cfg, messages, nil, maxTokens, temperature)
	if err != nil {
		return "", err
	}
	content := strings.TrimSpace(result.Content)
	if content == "" {
		return "", fmt.Errorf("AI 服务返回内容为空")
	}
	return content, nil
}

// GenerateChatCompletionWithTools sends an OpenAI-compatible chat completion request
// and returns assistant content plus any function tool calls requested by the model.
func GenerateChatCompletionWithTools(ctx context.Context, cfg config.AIConfig, messages []ChatMessage, tools []ChatTool, maxTokens int, temperature float64) (ChatCompletionResult, error) {
	cfg = config.ResolveAIRequestConfig(cfg)
	switch strings.TrimSpace(cfg.Protocol) {
	case config.AIProtocolAnthropic:
		return generateAnthropicChatCompletionWithTools(ctx, cfg, messages, tools, maxTokens, temperature)
	default:
		return generateOpenAIChatCompletionWithTools(ctx, cfg, messages, tools, maxTokens, temperature)
	}
}

func generateOpenAIChatCompletionWithTools(ctx context.Context, cfg config.AIConfig, messages []ChatMessage, tools []ChatTool, maxTokens int, temperature float64) (ChatCompletionResult, error) {
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

	payload := map[string]any{
		"model":       cfg.Model,
		"messages":    messages,
		"max_tokens":  maxTokens,
		"temperature": temperature,
	}
	if len(tools) > 0 {
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ChatCompletionResult{}, fmt.Errorf("序列化 AI 请求失败: %w", err)
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

	endpoints := openAIChatCompletionEndpoints(baseURL.String())
	var lastErr error
	for index, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return ChatCompletionResult{}, fmt.Errorf("创建 AI 请求失败: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Key))

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("AI 服务请求失败: %w", err)
			continue
		}

		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
		_ = resp.Body.Close()
		if readErr != nil {
			return ChatCompletionResult{}, fmt.Errorf("读取 AI 服务响应失败: %w", readErr)
		}

		contentType := resp.Header.Get("Content-Type")
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = formatAIServiceStatusError("AI 服务", resp.Status, contentType, respBody)
			if shouldTryNextAIEndpoint(resp.StatusCode, contentType, respBody) && index < len(endpoints)-1 {
				continue
			}
			return ChatCompletionResult{}, lastErr
		}

		result, err := parseChatCompletionResponse(respBody)
		if err != nil {
			lastErr = formatAIServiceParseError("AI 服务", contentType, respBody, err)
			if shouldTryNextAIEndpoint(resp.StatusCode, contentType, respBody) && index < len(endpoints)-1 {
				continue
			}
			return ChatCompletionResult{}, lastErr
		}
		return result, nil
	}

	if lastErr != nil {
		return ChatCompletionResult{}, lastErr
	}
	return ChatCompletionResult{}, fmt.Errorf("AI 服务请求失败")
}

func parseChatCompletionResponse(body []byte) (ChatCompletionResult, error) {
	var result struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return ChatCompletionResult{}, err
	}

	if len(result.Choices) == 0 {
		return ChatCompletionResult{}, fmt.Errorf("AI 服务响应缺少 choices 字段")
	}

	choice := result.Choices[0]
	out := ChatCompletionResult{
		Content:      strings.TrimSpace(choice.Message.Content),
		FinishReason: strings.TrimSpace(choice.FinishReason),
	}
	for _, tc := range choice.Message.ToolCalls {
		name := strings.TrimSpace(tc.Function.Name)
		if name == "" {
			continue
		}
		out.ToolCalls = append(out.ToolCalls, ChatToolCall{
			ID:        strings.TrimSpace(tc.ID),
			Type:      strings.TrimSpace(tc.Type),
			Name:      name,
			Arguments: normalizeChatToolCallArguments(tc.Function.Arguments),
		})
	}

	return out, nil
}

// GenerateChatCompletionStreamWithTools sends an OpenAI-compatible streaming
// chat completion request. Content deltas are emitted through onDelta; the
// returned result contains the final accumulated assistant content and any
// function tool calls assembled from streaming tool_call chunks.
// A non-positive maxTokens uses the provider default for OpenAI-compatible
// requests; Anthropic requires an explicit budget and defaults to 4096.
func GenerateChatCompletionStreamWithTools(ctx context.Context, cfg config.AIConfig, messages []ChatMessage, tools []ChatTool, maxTokens int, temperature float64, onDelta ChatStreamHandler) (ChatCompletionResult, error) {
	cfg = config.ResolveAIRequestConfig(cfg)
	switch strings.TrimSpace(cfg.Protocol) {
	case config.AIProtocolAnthropic:
		if maxTokens <= 0 {
			maxTokens = 4096
		}
		result, err := generateAnthropicChatCompletionWithTools(ctx, cfg, messages, tools, maxTokens, temperature)
		if err != nil {
			return ChatCompletionResult{}, err
		}
		if result.Content != "" && onDelta != nil {
			if err := onDelta(ChatStreamDelta{Delta: result.Content, Accumulated: result.Content}); err != nil {
				return ChatCompletionResult{}, err
			}
		}
		return result, nil
	default:
		return generateOpenAIChatCompletionStreamWithTools(ctx, cfg, messages, tools, maxTokens, temperature, onDelta)
	}
}

func generateOpenAIChatCompletionStreamWithTools(ctx context.Context, cfg config.AIConfig, messages []ChatMessage, tools []ChatTool, maxTokens int, temperature float64, onDelta ChatStreamHandler) (ChatCompletionResult, error) {
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
	payload := map[string]any{
		"model":       cfg.Model,
		"messages":    messages,
		"temperature": temperature,
		"stream":      true,
	}
	if maxTokens > 0 {
		payload["max_tokens"] = maxTokens
	}
	if len(tools) > 0 {
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ChatCompletionResult{}, fmt.Errorf("序列化 AI 请求失败: %w", err)
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

	endpoints := openAIChatCompletionEndpoints(baseURL.String())
	var lastErr error
	for index, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return ChatCompletionResult{}, fmt.Errorf("创建 AI 请求失败: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Key))

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("AI 服务请求失败: %w", err)
			continue
		}

		contentType := resp.Header.Get("Content-Type")
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
			_ = resp.Body.Close()
			if readErr != nil {
				return ChatCompletionResult{}, fmt.Errorf("读取 AI 服务响应失败: %w", readErr)
			}
			lastErr = formatAIServiceStatusError("AI 服务", resp.Status, contentType, respBody)
			if shouldTryNextAIEndpoint(resp.StatusCode, contentType, respBody) && index < len(endpoints)-1 {
				continue
			}
			return ChatCompletionResult{}, lastErr
		}

		if strings.Contains(strings.ToLower(contentType), "html") && index < len(endpoints)-1 {
			respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 8192))
			_ = resp.Body.Close()
			if readErr != nil {
				return ChatCompletionResult{}, fmt.Errorf("读取 AI 服务响应失败: %w", readErr)
			}
			lastErr = nonJSONServiceResponseError("AI 服务", contentType, respBody)
			continue
		}

		var result ChatCompletionResult
		if strings.Contains(strings.ToLower(contentType), "application/json") {
			// Some compatible gateways ignore stream=true and return a complete
			// JSON response. Decode that response without issuing a second request.
			var body []byte
			body, err = io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
			if err == nil {
				result, err = parseChatCompletionResponse(body)
			}
			if err == nil && result.Content != "" && onDelta != nil {
				err = onDelta(ChatStreamDelta{Delta: result.Content, Accumulated: result.Content})
			}
		} else {
			result, err = readChatCompletionStream(resp.Body, onDelta)
		}
		_ = resp.Body.Close()
		if err != nil {
			return ChatCompletionResult{}, err
		}
		if strings.TrimSpace(result.Content) == "" && len(result.ToolCalls) == 0 {
			return result, emptyChatCompletionError(result)
		}
		return result, nil
	}

	if lastErr != nil {
		return ChatCompletionResult{}, lastErr
	}
	return ChatCompletionResult{}, fmt.Errorf("AI 服务请求失败")
}

// Preserve the provider's stop reason instead of misreporting exhausted
// reasoning/output tokens as a successful but empty answer.
func emptyChatCompletionError(result ChatCompletionResult) error {
	if result.FinishReason == "length" || result.FinishReason == "max_tokens" {
		return fmt.Errorf("AI 回复达到输出 Token 上限，请提高所选渠道的输出上限后重试")
	}
	if result.FinishReason != "" {
		return fmt.Errorf("AI 服务返回内容为空（finish_reason=%s）", result.FinishReason)
	}
	return fmt.Errorf("AI 服务返回内容为空")
}

type streamingToolCallBuilder struct {
	id        string
	callType  string
	name      string
	arguments strings.Builder
}

func readChatCompletionStream(r io.Reader, onDelta ChatStreamHandler) (ChatCompletionResult, error) {
	reader := bufio.NewReaderSize(r, 64*1024)
	var accumulated string
	var finishReason string
	toolCalls := map[int]*streamingToolCallBuilder{}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errorsIsEOF(err) {
			return ChatCompletionResult{}, fmt.Errorf("读取 AI 流式响应失败: %w", err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				break
			}
			if data != "" {
				delta, nextFinishReason, err := parseChatCompletionStreamData(data, toolCalls)
				if err != nil {
					return ChatCompletionResult{}, err
				}
				if nextFinishReason != "" {
					finishReason = nextFinishReason
				}
				if delta != "" {
					next, normalizedDelta := NormalizeStreamingDelta(accumulated, delta)
					accumulated = next
					if normalizedDelta != "" && onDelta != nil {
						if err := onDelta(ChatStreamDelta{Delta: normalizedDelta, Accumulated: accumulated}); err != nil {
							return ChatCompletionResult{}, err
						}
					}
				}
			}
		}
		if err == io.EOF {
			break
		}
	}

	out := ChatCompletionResult{
		Content:      strings.TrimSpace(accumulated),
		FinishReason: strings.TrimSpace(finishReason),
	}
	for index := 0; index < len(toolCalls); index++ {
		builder := toolCalls[index]
		if builder == nil || strings.TrimSpace(builder.name) == "" {
			continue
		}
		out.ToolCalls = append(out.ToolCalls, ChatToolCall{
			ID:        strings.TrimSpace(builder.id),
			Type:      strings.TrimSpace(builder.callType),
			Name:      strings.TrimSpace(builder.name),
			Arguments: normalizeChatToolCallArguments(json.RawMessage(strings.TrimSpace(builder.arguments.String()))),
		})
	}
	return out, nil
}

func parseChatCompletionStreamData(data string, toolCalls map[int]*streamingToolCallBuilder) (string, string, error) {
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content   any `json:"content"`
				ToolCalls []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return "", "", fmt.Errorf("解析 AI 流式响应失败: %w", err)
	}
	if len(chunk.Choices) == 0 {
		return "", "", nil
	}
	choice := chunk.Choices[0]
	for _, tc := range choice.Delta.ToolCalls {
		builder := toolCalls[tc.Index]
		if builder == nil {
			builder = &streamingToolCallBuilder{}
			toolCalls[tc.Index] = builder
		}
		if strings.TrimSpace(tc.ID) != "" {
			builder.id = strings.TrimSpace(tc.ID)
		}
		if strings.TrimSpace(tc.Type) != "" {
			builder.callType = strings.TrimSpace(tc.Type)
		}
		if strings.TrimSpace(tc.Function.Name) != "" {
			builder.name += strings.TrimSpace(tc.Function.Name)
		}
		if tc.Function.Arguments != "" {
			builder.arguments.WriteString(tc.Function.Arguments)
		}
	}
	return chatStreamContentToString(choice.Delta.Content), strings.TrimSpace(choice.FinishReason), nil
}

func chatStreamContentToString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		var b strings.Builder
		for _, item := range v {
			switch typed := item.(type) {
			case string:
				b.WriteString(typed)
			case map[string]any:
				if text, ok := typed["text"].(string); ok {
					b.WriteString(text)
				}
			}
		}
		return b.String()
	default:
		return fmt.Sprint(v)
	}
}

func NormalizeStreamingDelta(current, incoming string) (next string, delta string) {
	if incoming == "" {
		return current, ""
	}
	if current == "" {
		return incoming, incoming
	}
	if strings.HasPrefix(incoming, current) {
		return incoming, incoming[len(current):]
	}
	if strings.HasPrefix(current, incoming) {
		return current, ""
	}
	maxOverlap := 0
	max := len(current)
	if len(incoming) < max {
		max = len(incoming)
	}
	for n := 1; n <= max; n++ {
		if strings.HasSuffix(current, incoming[:n]) {
			maxOverlap = n
		}
	}
	if maxOverlap > 0 {
		tail := incoming[maxOverlap:]
		return current + tail, tail
	}
	return current + incoming, incoming
}

func errorsIsEOF(err error) bool {
	return err == io.EOF
}

func normalizeChatToolCallArguments(raw json.RawMessage) json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(`{}`)
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil && strings.TrimSpace(s) != "" {
			s = strings.TrimSpace(s)
			if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
				return json.RawMessage(s)
			}
			wrapped, err := json.Marshal(map[string]string{"raw": s})
			if err == nil {
				return wrapped
			}
		}
	}
	return raw
}
