package repeater

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// HTTPTestOptions describes a bounded, dependency-free HTTP request replay.
// It intentionally accepts the same common request controls used by the
// Repeater UI so callers do not need a Python runtime for API verification.
type HTTPTestOptions struct {
	// BeforeRequest runs immediately before each outbound request, including
	// redirects and repeats. An error prevents the request from reaching the
	// transport. AI callers supply their tool policy; manual replay need not.
	BeforeRequest            func(*http.Request) error
	Transport                http.RoundTripper // Trusted caller-supplied enforced egress; never populated from JSON.
	URL                      string
	Method                   string
	Data                     string
	Headers                  map[string]string
	Cookies                  string
	UserAgent                string
	Proxy                    string
	Timeout                  time.Duration
	Repeat                   int
	Delay                    time.Duration
	FollowRedirects          bool
	AllowInsecure            bool
	ResponseFilter           string
	ResponseFilterIgnoreCase bool
	ResponseMaxLines         int
	ResponseMaxBytes         int
}

type HTTPTestResponse struct {
	URL           string              `json:"url"`
	StatusCode    int                 `json:"statusCode"`
	Status        string              `json:"status"`
	Headers       map[string][]string `json:"headers"`
	Body          string              `json:"body"`
	BodyTruncated bool                `json:"bodyTruncated"`
	ElapsedMS     int64               `json:"elapsedMs"`
}

type HTTPTestResult struct {
	Responses []HTTPTestResponse `json:"responses"`
}

// ExecuteHTTPTest performs one or more HTTP requests using Go's native HTTP
// stack. The body retained in the result is bounded to protect agent context;
// callers may further reduce it with ResponseMaxBytes or ResponseMaxLines.
func ExecuteHTTPTest(ctx context.Context, opts HTTPTestOptions) (HTTPTestResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := url.ParseRequestURI(strings.TrimSpace(opts.URL)); err != nil {
		return HTTPTestResult{}, fmt.Errorf("invalid url: %w", err)
	}
	parsed, _ := url.Parse(opts.URL)
	if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
		return HTTPTestResult{}, fmt.Errorf("url must be an absolute http or https URL")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: opts.AllowInsecure} // #nosec G402 -- explicit testing option
	if strings.TrimSpace(opts.Proxy) != "" {
		proxyURL, err := url.Parse(opts.Proxy)
		if err != nil || proxyURL.Scheme == "" || proxyURL.Host == "" {
			return HTTPTestResult{}, fmt.Errorf("invalid proxy url")
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	var effective http.RoundTripper = transport
	if opts.Transport != nil {
		effective = opts.Transport
	}
	client := &http.Client{Transport: effective}
	if opts.BeforeRequest != nil {
		client.Transport = guardedHTTPTestTransport{base: effective, before: opts.BeforeRequest}
	}
	if !opts.FollowRedirects {
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}

	repeat := opts.Repeat
	if repeat < 1 {
		repeat = 1
	}
	if repeat > 20 {
		return HTTPTestResult{}, fmt.Errorf("repeat must not exceed 20")
	}
	method := strings.ToUpper(strings.TrimSpace(opts.Method))
	if method == "" {
		method = http.MethodGet
	}
	filter, err := compileHTTPTestFilter(opts.ResponseFilter, opts.ResponseFilterIgnoreCase)
	if err != nil {
		return HTTPTestResult{}, err
	}

	result := HTTPTestResult{Responses: make([]HTTPTestResponse, 0, repeat)}
	for i := 0; i < repeat; i++ {
		if i > 0 && opts.Delay > 0 {
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(opts.Delay):
			}
		}
		requestCtx := ctx
		cancel := func() {}
		if opts.Timeout > 0 {
			requestCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		}
		response, err := executeHTTPTestRequest(requestCtx, client, method, opts, filter)
		cancel()
		if err != nil {
			return result, err
		}
		result.Responses = append(result.Responses, response)
	}
	return result, nil
}

type guardedHTTPTestTransport struct {
	base   http.RoundTripper
	before func(*http.Request) error
}

func (t guardedHTTPTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.before(req); err != nil {
		// RoundTripper owns the body even when it declines to send a request.
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	return t.base.RoundTrip(req)
}

func executeHTTPTestRequest(ctx context.Context, client *http.Client, method string, opts HTTPTestOptions, filter *regexp.Regexp) (HTTPTestResponse, error) {
	req, err := http.NewRequestWithContext(ctx, method, opts.URL, strings.NewReader(opts.Data))
	if err != nil {
		return HTTPTestResponse{}, err
	}
	for key, value := range opts.Headers {
		req.Header.Set(key, value)
	}
	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	}
	for _, item := range strings.Split(opts.Cookies, ";") {
		name, value, found := strings.Cut(strings.TrimSpace(item), "=")
		if found && name != "" {
			req.AddCookie(&http.Cookie{Name: name, Value: strings.TrimSpace(value)})
		}
	}

	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return HTTPTestResponse{}, err
	}
	defer resp.Body.Close()
	const maxCapturedBodyBytes = 4 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCapturedBodyBytes+1))
	if err != nil {
		return HTTPTestResponse{}, err
	}
	truncated := len(body) > maxCapturedBodyBytes
	if truncated {
		body = body[:maxCapturedBodyBytes]
	}
	output, outputTruncated := formatHTTPTestBody(string(body), filter, opts.ResponseMaxLines, opts.ResponseMaxBytes)
	return HTTPTestResponse{
		URL:           resp.Request.URL.String(),
		StatusCode:    resp.StatusCode,
		Status:        resp.Status,
		Headers:       resp.Header,
		Body:          output,
		BodyTruncated: truncated || outputTruncated,
		ElapsedMS:     time.Since(started).Milliseconds(),
	}, nil
}

func compileHTTPTestFilter(pattern string, ignoreCase bool) (*regexp.Regexp, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, nil
	}
	if ignoreCase {
		pattern = "(?i)" + pattern
	}
	filter, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid response filter: %w", err)
	}
	return filter, nil
}

func formatHTTPTestBody(body string, filter *regexp.Regexp, maxLines, maxBytes int) (string, bool) {
	if filter != nil {
		matches := make([]string, 0)
		for _, line := range strings.Split(body, "\n") {
			if filter.MatchString(line) {
				matches = append(matches, line)
			}
		}
		body = strings.Join(matches, "\n")
	}
	truncated := false
	if maxLines > 0 {
		lines := strings.Split(body, "\n")
		if len(lines) > maxLines {
			body = strings.Join(lines[:maxLines], "\n")
			truncated = true
		}
	}
	if maxBytes > 0 && len(body) > maxBytes {
		body = body[:maxBytes]
		truncated = true
	}
	return body, truncated
}
