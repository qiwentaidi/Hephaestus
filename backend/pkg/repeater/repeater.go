package repeater

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/qiwentaidi/clients"
)

type rawRequest struct {
	Method        string
	RequestTarget string
	Host          string
	Headers       map[string]string
	Body          []byte
}

func SendRequestWithRaw(ctx context.Context, raw string, forceHttps, redirect bool, proxyURL string) (*resty.Response, int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	req, err := parseRawRequest(raw)
	if err != nil {
		return nil, 0, fmt.Errorf("解析HTTP请求失败: %v. 请确保请求格式正确，包含请求行（如 GET /path HTTP/1.1）和必要的头部（如 Host）", err)
	}
	scheme := "http"
	if forceHttps {
		scheme = "https"
	}
	client := clients.NewRestyClientWithProxy(nil, redirect, proxyURL)

	// 不跟随重定向
	if !redirect {
		client.SetRedirectPolicy(resty.RedirectPolicyFunc(func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}))
	}

	// ===== 记录开始时间 =====
	start := time.Now()
	resp, err := ConvertRawRequestToResty(ctx, client, req, scheme)
	if err != nil {
		return nil, 0, err
	}
	// ===== 计算耗时 =====
	duration := time.Since(start).Milliseconds()
	return resp, duration, nil
}

func parseRawRequest(raw string) (*rawRequest, error) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	parts := strings.SplitN(normalized, "\n\n", 2)
	headerPart := strings.Trim(parts[0], "\n")
	if strings.TrimSpace(headerPart) == "" {
		return nil, fmt.Errorf("请求头为空")
	}

	bodyPart := ""
	if len(parts) == 2 {
		bodyPart = parts[1]
	}

	lines := strings.Split(headerPart, "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("缺少请求行")
	}

	requestLine := strings.TrimSpace(lines[0])
	requestParts := strings.Fields(requestLine)
	if len(requestParts) < 3 {
		return nil, fmt.Errorf("无效请求行: %s", requestLine)
	}

	mimeHeader, err := textproto.NewReader(bufio.NewReader(strings.NewReader(strings.Join(lines[1:], "\r\n") + "\r\n\r\n"))).ReadMIMEHeader()
	if err != nil {
		return nil, fmt.Errorf("解析请求头失败: %w", err)
	}

	headers := make(map[string]string, len(mimeHeader))
	for key, values := range mimeHeader {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	host := headers["Host"]
	if host == "" {
		host = headers["host"]
	}

	req := &rawRequest{
		Method:        requestParts[0],
		RequestTarget: requestParts[1],
		Host:          host,
		Headers:       headers,
		Body:          []byte(bodyPart),
	}

	if req.Method == "" {
		return nil, fmt.Errorf("请求方法不能为空")
	}
	if req.RequestTarget == "" {
		return nil, fmt.Errorf("请求路径不能为空")
	}

	return req, nil
}

func ConvertRawRequestToResty(ctx context.Context, client *resty.Client, req *rawRequest, scheme string) (*resty.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	client.SetDoNotParseResponse(true) // 禁用自动解析响应
	headers := make(map[string]string, len(req.Headers))
	for key, value := range req.Headers {
		headers[key] = value
	}

	rawURL := buildFullURL(req, scheme)
	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}

	if len(req.Body) > 0 && !strings.EqualFold(headers["Transfer-Encoding"], "chunked") &&
		!strings.EqualFold(req.Method, http.MethodGet) && !strings.EqualFold(req.Method, http.MethodHead) {
		headers["Content-Length"] = strconv.Itoa(len(req.Body))
	}
	if len(req.Body) == 0 {
		delete(headers, "Content-Length")
	}

	request := client.R().SetContext(ctx)

	if headers != nil {
		request.SetHeaders(headers)
	}

	if body != nil {
		data, err := io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("read body failed: %w", err)
		}
		request.SetBody(data)
	}

	switch req.Method {
	case http.MethodGet:
		return request.Get(rawURL)
	case http.MethodPost:
		return request.Post(rawURL)
	case http.MethodPut:
		return request.Put(rawURL)
	case http.MethodDelete:
		return request.Delete(rawURL)
	case http.MethodPatch:
		return request.Patch(rawURL)
	case http.MethodOptions:
		return request.Options(rawURL)
	case http.MethodHead:
		return request.Head(rawURL)
	default:
		return nil, fmt.Errorf("unsupported method: %s", req.Method)
	}
}

// buildFullURL 根据原始请求和 scheme 构造完整 URL
func buildFullURL(req *rawRequest, scheme string) string {
	if parsed, err := url.Parse(req.RequestTarget); err == nil && parsed.IsAbs() {
		return parsed.String()
	}

	host := strings.TrimSpace(req.Host)
	if host == "" {
		return req.RequestTarget
	}

	return scheme + "://" + host + req.RequestTarget
}
