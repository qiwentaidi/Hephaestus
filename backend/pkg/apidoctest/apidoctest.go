package apidoctest

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultMaxEndpoints = 120
	MaxEndpoints        = 500
	MaxInputBytes       = 2 * 1024 * 1024
	MaxResponseBody     = 256 * 1024
	DefaultTimeout      = 10 * time.Second
	MaxRunCases         = 30
)

type Request struct {
	SourceURL      string            `json:"sourceUrl"`
	Input          string            `json:"input"`
	BaseURL        string            `json:"baseUrl"`
	DocumentType   string            `json:"documentType"`
	Headers        map[string]string `json:"headers"`
	MaxEndpoints   int               `json:"maxEndpoints"`
	GenerateCases  bool              `json:"generateCases"`
	RunSafeChecks  bool              `json:"runSafeChecks"`
	TimeoutSeconds int               `json:"timeoutSeconds"`
}

type Result struct {
	DocumentType string     `json:"documentType"`
	Title        string     `json:"title"`
	Version      string     `json:"version"`
	BaseURL      string     `json:"baseUrl"`
	SourceURL    string     `json:"sourceUrl,omitempty"`
	Summary      Summary    `json:"summary"`
	Endpoints    []Endpoint `json:"endpoints"`
	Cases        []TestCase `json:"cases"`
	Findings     []Finding  `json:"findings"`
	Warnings     []string   `json:"warnings,omitempty"`
}

type Summary struct {
	EndpointCount      int `json:"endpointCount"`
	AuthRequiredCount  int `json:"authRequiredCount"`
	SensitiveCount     int `json:"sensitiveCount"`
	GeneratedCaseCount int `json:"generatedCaseCount"`
	ExecutedCaseCount  int `json:"executedCaseCount"`
	FindingCount       int `json:"findingCount"`
}

type Endpoint struct {
	Method       string      `json:"method"`
	URL          string      `json:"url"`
	Path         string      `json:"path"`
	OperationID  string      `json:"operationId,omitempty"`
	Summary      string      `json:"summary,omitempty"`
	RequestBody  string      `json:"requestBody,omitempty"`
	ContentType  string      `json:"contentType,omitempty"`
	AuthRequired bool        `json:"authRequired"`
	Parameters   []Parameter `json:"parameters,omitempty"`
	RiskTags     []string    `json:"riskTags,omitempty"`
}

type Parameter struct {
	Name     string `json:"name"`
	In       string `json:"in"`
	Required bool   `json:"required"`
	Type     string `json:"type,omitempty"`
}

type TestCase struct {
	ID                string              `json:"id"`
	Type              string              `json:"type"`
	EndpointMethod    string              `json:"endpointMethod"`
	EndpointURL       string              `json:"endpointUrl"`
	Method            string              `json:"method"`
	URL               string              `json:"url"`
	Headers           map[string]string   `json:"headers,omitempty"`
	Body              string              `json:"body,omitempty"`
	Reason            string              `json:"reason"`
	RiskLevel         string              `json:"riskLevel"`
	SafeToRun         bool                `json:"safeToRun"`
	Executed          bool                `json:"executed"`
	StatusCode        int                 `json:"statusCode,omitempty"`
	StatusText        string              `json:"statusText,omitempty"`
	ContentType       string              `json:"contentType,omitempty"`
	ResponseHeaders   map[string][]string `json:"responseHeaders,omitempty"`
	ResponseBody      string              `json:"responseBody,omitempty"`
	ResponseTruncated bool                `json:"responseTruncated,omitempty"`
	ResponseLength    int                 `json:"responseLength,omitempty"`
	DurationMS        int64               `json:"durationMs,omitempty"`
	Evidence          string              `json:"evidence,omitempty"`
	Error             string              `json:"error,omitempty"`
}

type RunRequest struct {
	Method         string            `json:"method"`
	URL            string            `json:"url"`
	Headers        map[string]string `json:"headers"`
	Body           string            `json:"body"`
	TimeoutSeconds int               `json:"timeoutSeconds"`
}

type RunResult struct {
	Method            string              `json:"method"`
	URL               string              `json:"url"`
	StatusCode        int                 `json:"statusCode,omitempty"`
	StatusText        string              `json:"statusText,omitempty"`
	ContentType       string              `json:"contentType,omitempty"`
	ResponseHeaders   map[string][]string `json:"responseHeaders,omitempty"`
	ResponseBody      string              `json:"responseBody,omitempty"`
	ResponseTruncated bool                `json:"responseTruncated,omitempty"`
	ResponseLength    int                 `json:"responseLength,omitempty"`
	DurationMS        int64               `json:"durationMs,omitempty"`
	Error             string              `json:"error,omitempty"`
}

type Finding struct {
	Title      string `json:"title"`
	Endpoint   string `json:"endpoint"`
	RiskLevel  string `json:"riskLevel"`
	Confidence string `json:"confidence"`
	Summary    string `json:"summary"`
	Evidence   string `json:"evidence,omitempty"`
}

func AnalyzeAndTest(ctx context.Context, req Request) (Result, error) {
	input := strings.TrimSpace(req.Input)
	headers := sanitizeHeaders(req.Headers)
	if input == "" && strings.TrimSpace(req.SourceURL) != "" {
		fetched, err := fetchDocument(ctx, strings.TrimSpace(req.SourceURL), headers, timeout(req.TimeoutSeconds))
		if err != nil {
			return Result{}, err
		}
		input = fetched
	}
	if input == "" {
		return Result{}, fmt.Errorf("input 或 sourceUrl 不能为空")
	}
	if len(input) > MaxInputBytes {
		return Result{}, fmt.Errorf("接口文档超过最大长度 %d bytes", MaxInputBytes)
	}
	limit := req.MaxEndpoints
	if limit <= 0 {
		limit = DefaultMaxEndpoints
	}
	if limit > MaxEndpoints {
		limit = MaxEndpoints
	}
	docType := detectDocumentType(req.DocumentType, req.SourceURL, input)
	result := Result{
		DocumentType: docType,
		BaseURL:      strings.TrimSpace(req.BaseURL),
		SourceURL:    strings.TrimSpace(req.SourceURL),
	}
	var err error
	switch docType {
	case "openapi", "swagger":
		result, err = analyzeOpenAPI(input, req.BaseURL, req.SourceURL, limit)
	case "wsdl":
		result, err = analyzeWSDL(input, req.BaseURL, req.SourceURL, limit)
	case "graphql":
		result, err = analyzeGraphQL(input, req.BaseURL, req.SourceURL, limit)
	default:
		result, err = analyzeText(input, req.BaseURL, req.SourceURL, limit)
	}
	if err != nil {
		return Result{}, err
	}
	result.SourceURL = strings.TrimSpace(req.SourceURL)
	result.DocumentType = docType
	if result.BaseURL == "" {
		result.BaseURL = strings.TrimSpace(req.BaseURL)
	}
	if req.GenerateCases || req.RunSafeChecks {
		result.Cases = GenerateCases(result.Endpoints, headers)
	}
	if req.RunSafeChecks {
		executeSafeCases(ctx, &result, timeout(req.TimeoutSeconds))
	}
	finalizeSummary(&result)
	return result, nil
}

func RunEndpoint(ctx context.Context, req RunRequest) (RunResult, error) {
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	targetURL := strings.TrimSpace(req.URL)
	if targetURL == "" {
		return RunResult{}, fmt.Errorf("url 不能为空")
	}
	tc := TestCase{
		Type:      "manual_endpoint_test",
		Method:    method,
		URL:       targetURL,
		Headers:   sanitizeHeaders(req.Headers),
		Body:      req.Body,
		SafeToRun: isSafeHTTPMethod(method),
	}
	runCase(ctx, &tc, timeout(req.TimeoutSeconds))
	return RunResult{
		Method:            tc.Method,
		URL:               tc.URL,
		StatusCode:        tc.StatusCode,
		StatusText:        tc.StatusText,
		ContentType:       tc.ContentType,
		ResponseHeaders:   tc.ResponseHeaders,
		ResponseBody:      tc.ResponseBody,
		ResponseTruncated: tc.ResponseTruncated,
		ResponseLength:    tc.ResponseLength,
		DurationMS:        tc.DurationMS,
		Error:             tc.Error,
	}, nil
}

func analyzeOpenAPI(input string, baseURL string, sourceURL string, limit int) (Result, error) {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(input), &doc); err != nil {
		return Result{}, fmt.Errorf("解析 OpenAPI/Swagger 失败: %w", err)
	}
	info := asMap(doc["info"])
	resolvedBaseURL := resolveOpenAPIBaseURL(doc, baseURL, sourceURL)
	docType := "openapi"
	version := str(doc["openapi"])
	if version == "" {
		docType = "swagger"
		version = str(doc["swagger"])
	}
	result := Result{
		DocumentType: docType,
		Title:        str(info["title"]),
		Version:      version,
		BaseURL:      resolvedBaseURL,
	}
	globalSecurity := hasNonEmpty(doc["security"])
	paths := asMap(doc["paths"])
	pathKeys := sortedKeys(paths)
	for _, path := range pathKeys {
		if len(result.Endpoints) >= limit {
			break
		}
		pathItem := asMap(paths[path])
		pathParams := parseOpenAPIParameters(pathItem["parameters"])
		for _, method := range []string{"get", "post", "put", "patch", "delete", "head", "options"} {
			if len(result.Endpoints) >= limit {
				break
			}
			op := asMap(pathItem[method])
			if len(op) == 0 {
				continue
			}
			params := append([]Parameter{}, pathParams...)
			params = append(params, parseOpenAPIParameters(op["parameters"])...)
			authRequired := globalSecurity || hasNonEmpty(op["security"])
			endpointURL := resolveEndpointURL(resolvedBaseURL, path)
			summary := str(op["summary"])
			if summary == "" {
				summary = str(op["description"])
			}
			requestBody, contentType := buildOpenAPIRequestBody(op)
			result.Endpoints = append(result.Endpoints, Endpoint{
				Method:       strings.ToUpper(method),
				URL:          endpointURL,
				Path:         path,
				OperationID:  str(op["operationId"]),
				Summary:      summary,
				RequestBody:  requestBody,
				ContentType:  contentType,
				AuthRequired: authRequired,
				Parameters:   params,
				RiskTags:     riskTagsForEndpoint(strings.ToUpper(method), path, params),
			})
		}
	}
	return result, nil
}

type wsdlDefinitions struct {
	XMLName   xml.Name       `xml:"definitions"`
	TargetNS  string         `xml:"targetNamespace,attr"`
	Services  []wsdlService  `xml:"service"`
	Bindings  []wsdlBinding  `xml:"binding"`
	PortTypes []wsdlPortType `xml:"portType"`
}

type wsdlService struct {
	Name  string     `xml:"name,attr"`
	Ports []wsdlPort `xml:"port"`
}

type wsdlPort struct {
	Name    string `xml:"name,attr"`
	Address []struct {
		Location string `xml:"location,attr"`
	} `xml:",any"`
}

type wsdlBinding struct {
	Name       string          `xml:"name,attr"`
	Type       string          `xml:"type,attr"`
	Operations []wsdlOperation `xml:"operation"`
}

type wsdlPortType struct {
	Name       string          `xml:"name,attr"`
	Operations []wsdlOperation `xml:"operation"`
}

type wsdlOperation struct {
	Name string `xml:"name,attr"`
}

func analyzeWSDL(input string, baseURL string, sourceURL string, limit int) (Result, error) {
	var doc wsdlDefinitions
	decoder := xml.NewDecoder(strings.NewReader(input))
	if err := decoder.Decode(&doc); err != nil {
		return Result{}, fmt.Errorf("解析 WSDL 失败: %w", err)
	}
	serviceName := ""
	serviceURL := firstNonEmpty(baseURL, sourceURL)
	for _, service := range doc.Services {
		if serviceName == "" {
			serviceName = service.Name
		}
		for _, port := range service.Ports {
			for _, addr := range port.Address {
				if strings.TrimSpace(addr.Location) != "" {
					serviceURL = strings.TrimSpace(addr.Location)
					break
				}
			}
		}
	}
	result := Result{
		DocumentType: "wsdl",
		Title:        serviceName,
		Version:      "wsdl",
		BaseURL:      serviceURL,
	}
	seen := map[string]bool{}
	addOperation := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] || len(result.Endpoints) >= limit {
			return
		}
		seen[name] = true
		result.Endpoints = append(result.Endpoints, Endpoint{
			Method:       "POST",
			URL:          serviceURL,
			Path:         name,
			OperationID:  name,
			Summary:      "SOAP operation " + name,
			AuthRequired: false,
			RiskTags:     riskTagsForEndpoint("POST", name, nil),
		})
	}
	for _, binding := range doc.Bindings {
		for _, op := range binding.Operations {
			addOperation(op.Name)
		}
	}
	for _, portType := range doc.PortTypes {
		for _, op := range portType.Operations {
			addOperation(op.Name)
		}
	}
	return result, nil
}

func analyzeGraphQL(input string, baseURL string, sourceURL string, limit int) (Result, error) {
	endpointURL := firstNonEmpty(baseURL, sourceURL)
	if endpointURL == "" {
		endpointURL = "/graphql"
	}
	result := Result{
		DocumentType: "graphql",
		Title:        "GraphQL Schema",
		Version:      "graphql",
		BaseURL:      endpointURL,
	}
	names := extractGraphQLOperationNames(input, limit)
	if len(names) == 0 {
		names = []string{"introspection"}
	}
	for _, name := range names {
		result.Endpoints = append(result.Endpoints, Endpoint{
			Method:       "POST",
			URL:          endpointURL,
			Path:         name,
			OperationID:  name,
			Summary:      "GraphQL operation " + name,
			AuthRequired: false,
			RiskTags:     riskTagsForEndpoint("POST", name, nil),
		})
	}
	return result, nil
}

func analyzeText(input string, baseURL string, sourceURL string, limit int) (Result, error) {
	result := Result{
		DocumentType: "text",
		Title:        "API Text",
		Version:      "text",
		BaseURL:      firstNonEmpty(baseURL, sourceURL),
	}
	re := regexp.MustCompile(`(?im)\b(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\s+([^\s"'<>]+)|["']((?:https?://|/)[^"'\s<>]+)["']`)
	seen := map[string]bool{}
	for _, match := range re.FindAllStringSubmatch(input, -1) {
		if len(result.Endpoints) >= limit {
			break
		}
		method := strings.ToUpper(match[1])
		raw := match[2]
		if raw == "" {
			method = "GET"
			raw = match[3]
		}
		endpointURL := resolveEndpointURL(result.BaseURL, raw)
		if endpointURL == "" {
			continue
		}
		key := method + " " + endpointURL
		if seen[key] {
			continue
		}
		seen[key] = true
		result.Endpoints = append(result.Endpoints, Endpoint{
			Method:   method,
			URL:      endpointURL,
			Path:     raw,
			RiskTags: riskTagsForEndpoint(method, raw, nil),
		})
	}
	return result, nil
}

func GenerateCases(endpoints []Endpoint, headers map[string]string) []TestCase {
	cases := make([]TestCase, 0)
	for index, endpoint := range endpoints {
		method := strings.ToUpper(endpoint.Method)
		if endpoint.AuthRequired || hasAuthHeaders(headers) {
			cases = append(cases, TestCase{
				ID:             fmt.Sprintf("unauth-%d", index+1),
				Type:           "unauthorized_access",
				EndpointMethod: endpoint.Method,
				EndpointURL:    endpoint.URL,
				Method:         safeMethod(method),
				URL:            endpoint.URL,
				Headers:        removeAuthHeaders(headers),
				Reason:         "移除认证信息后进行只读访问对比，识别接口文档暴露出的未授权访问风险。",
				RiskLevel:      "medium",
				SafeToRun:      isSafeHTTPMethod(method),
			})
		}
		if contains(endpoint.RiskTags, "sensitive_operation") || contains(endpoint.RiskTags, "admin") {
			cases = append(cases, TestCase{
				ID:             fmt.Sprintf("sensitive-%d", index+1),
				Type:           "sensitive_endpoint_review",
				EndpointMethod: endpoint.Method,
				EndpointURL:    endpoint.URL,
				Method:         method,
				URL:            endpoint.URL,
				Reason:         "接口路径或操作名包含敏感管理、导出、删除、上传等语义，建议结合授权上下文专项复核。",
				RiskLevel:      "medium",
				SafeToRun:      false,
			})
		}
		if contains(endpoint.RiskTags, "idor_candidate") {
			cases = append(cases, TestCase{
				ID:             fmt.Sprintf("idor-%d", index+1),
				Type:           "idor_candidate",
				EndpointMethod: endpoint.Method,
				EndpointURL:    endpoint.URL,
				Method:         method,
				URL:            endpoint.URL,
				Reason:         "路径或参数包含对象 ID，适合在双账号或授权样本下做越权差异测试。",
				RiskLevel:      "medium",
				SafeToRun:      false,
			})
		}
		if method == "GET" || method == "HEAD" {
			cases = append(cases, TestCase{
				ID:             fmt.Sprintf("method-options-%d", index+1),
				Type:           "method_options_probe",
				EndpointMethod: endpoint.Method,
				EndpointURL:    endpoint.URL,
				Method:         "OPTIONS",
				URL:            endpoint.URL,
				Headers:        headers,
				Reason:         "使用 OPTIONS 获取允许方法，辅助识别方法暴露和跨域策略。",
				RiskLevel:      "low",
				SafeToRun:      true,
			})
		}
	}
	return cases
}

func executeSafeCases(ctx context.Context, result *Result, timeoutValue time.Duration) {
	executed := 0
	for i := range result.Cases {
		if executed >= MaxRunCases {
			return
		}
		if !result.Cases[i].SafeToRun {
			continue
		}
		executed++
		runCase(ctx, &result.Cases[i], timeoutValue)
		if result.Cases[i].StatusCode > 0 {
			result.Findings = append(result.Findings, findingForCase(result.Cases[i])...)
		}
	}
}

func runCase(ctx context.Context, tc *TestCase, timeoutValue time.Duration) {
	reqCtx, cancel := context.WithTimeout(ctx, timeoutValue)
	defer cancel()
	body := strings.NewReader(tc.Body)
	req, err := http.NewRequestWithContext(reqCtx, tc.Method, tc.URL, body)
	if err != nil {
		tc.Error = err.Error()
		return
	}
	for key, value := range tc.Headers {
		req.Header.Set(key, value)
	}
	client := &http.Client{
		Timeout: timeoutValue,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	started := time.Now()
	resp, err := client.Do(req)
	tc.DurationMS = time.Since(started).Milliseconds()
	tc.Executed = true
	if err != nil {
		tc.Error = err.Error()
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBody+1))
	truncated := len(data) > MaxResponseBody
	if truncated {
		data = data[:MaxResponseBody]
	}
	tc.StatusCode = resp.StatusCode
	tc.StatusText = resp.Status
	tc.ContentType = resp.Header.Get("Content-Type")
	tc.ResponseHeaders = resp.Header
	tc.ResponseBody = string(data)
	tc.ResponseTruncated = truncated
	tc.ResponseLength = len(data)
	tc.Evidence = fmt.Sprintf("%s %s -> HTTP %d，响应长度 %d，耗时 %dms", tc.Method, tc.URL, tc.StatusCode, tc.ResponseLength, tc.DurationMS)
}

func findingForCase(tc TestCase) []Finding {
	if tc.Type == "unauthorized_access" && tc.StatusCode >= 200 && tc.StatusCode < 300 {
		return []Finding{{
			Title:      "接口文档候选未授权访问",
			Endpoint:   tc.EndpointURL,
			RiskLevel:  "medium",
			Confidence: "low",
			Summary:    "移除认证信息后仍返回 2xx。该结果仅说明存在未授权候选，需要结合响应语义和业务权限进一步确认。",
			Evidence:   tc.Evidence,
		}}
	}
	return nil
}

func finalizeSummary(result *Result) {
	authRequired := 0
	sensitive := 0
	for _, endpoint := range result.Endpoints {
		if endpoint.AuthRequired {
			authRequired++
		}
		if contains(endpoint.RiskTags, "sensitive_operation") || contains(endpoint.RiskTags, "admin") {
			sensitive++
		}
	}
	executed := 0
	for _, tc := range result.Cases {
		if tc.Executed {
			executed++
		}
	}
	result.Summary = Summary{
		EndpointCount:      len(result.Endpoints),
		AuthRequiredCount:  authRequired,
		SensitiveCount:     sensitive,
		GeneratedCaseCount: len(result.Cases),
		ExecutedCaseCount:  executed,
		FindingCount:       len(result.Findings),
	}
}

func fetchDocument(ctx context.Context, sourceURL string, headers map[string]string, timeoutValue time.Duration) (string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeoutValue)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", fmt.Errorf("构造接口文档请求失败: %w", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	client := &http.Client{
		Timeout: timeoutValue,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("获取接口文档失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("获取接口文档返回 HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxInputBytes+1))
	if err != nil {
		return "", fmt.Errorf("读取接口文档失败: %w", err)
	}
	if len(data) > MaxInputBytes {
		return "", fmt.Errorf("接口文档超过最大长度 %d bytes", MaxInputBytes)
	}
	return string(data), nil
}

func detectDocumentType(preferred string, sourceURL string, input string) string {
	preferred = strings.ToLower(strings.TrimSpace(preferred))
	if preferred != "" && preferred != "auto" {
		return preferred
	}
	lowerURL := strings.ToLower(sourceURL)
	lower := strings.ToLower(strings.TrimSpace(input))
	switch {
	case strings.Contains(lowerURL, "graphql") || strings.Contains(lower, "type query") || strings.Contains(lower, "__schema"):
		return "graphql"
	case strings.Contains(lowerURL, "wsdl") || strings.Contains(lower, "<definitions") || strings.Contains(lower, "soap:"):
		return "wsdl"
	case strings.Contains(lower, "swagger:") || strings.Contains(lower, `"swagger"`):
		return "swagger"
	case strings.Contains(lower, "openapi:") || strings.Contains(lower, `"openapi"`):
		return "openapi"
	default:
		return "text"
	}
}

func resolveOpenAPIBaseURL(doc map[string]any, baseURL string, sourceURL string) string {
	if strings.TrimSpace(baseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}

	if servers, ok := doc["servers"].([]any); ok && len(servers) > 0 {
		server := asMap(servers[0])
		if serverURL := str(server["url"]); serverURL != "" {
			resolved := resolveEndpointURL(sourceURL, serverURL)
			return strings.TrimRight(preferSourceSchemeForSameHost(resolved, sourceURL), "/")
		}
	}

	host := str(doc["host"])
	if host != "" {
		scheme := sourceScheme(sourceURL)
		if scheme == "" {
			scheme = firstOpenAPIScheme(doc)
		}
		if scheme == "" {
			scheme = "https"
		}
		basePath := str(doc["basePath"])
		return strings.TrimRight(scheme+"://"+host+basePath, "/")
	}
	if sourceURL != "" {
		parsed, err := url.Parse(sourceURL)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" {
			return parsed.Scheme + "://" + parsed.Host
		}
	}
	return ""
}

func firstOpenAPIScheme(doc map[string]any) string {
	if schemes, ok := doc["schemes"].([]any); ok && len(schemes) > 0 {
		for _, raw := range schemes {
			scheme := str(raw)
			if scheme == "http" || scheme == "https" {
				return scheme
			}
		}
	}
	return ""
}

func preferSourceSchemeForSameHost(targetURL string, sourceURL string) string {
	sourceParsed, sourceErr := url.Parse(sourceURL)
	targetParsed, targetErr := url.Parse(targetURL)
	if sourceErr != nil || targetErr != nil {
		return targetURL
	}
	if sourceParsed.Scheme == "" || sourceParsed.Host == "" || targetParsed.Scheme == "" || targetParsed.Host == "" {
		return targetURL
	}
	if sourceParsed.Host != targetParsed.Host {
		return targetURL
	}
	if sourceParsed.Scheme != "http" && sourceParsed.Scheme != "https" {
		return targetURL
	}
	targetParsed.Scheme = sourceParsed.Scheme
	return targetParsed.String()
}

func sourceScheme(sourceURL string) string {
	if sourceURL == "" {
		return ""
	}
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return ""
	}
	if parsed.Scheme == "http" || parsed.Scheme == "https" {
		return parsed.Scheme
	}
	return ""
}

func parseOpenAPIParameters(value any) []Parameter {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	params := make([]Parameter, 0, len(items))
	for _, item := range items {
		param := asMap(item)
		if len(param) == 0 {
			continue
		}
		paramType := str(param["type"])
		if paramType == "" {
			schema := asMap(param["schema"])
			paramType = str(schema["type"])
		}
		params = append(params, Parameter{
			Name:     str(param["name"]),
			In:       str(param["in"]),
			Required: boolValue(param["required"]),
			Type:     paramType,
		})
	}
	return params
}

func buildOpenAPIRequestBody(op map[string]any) (string, string) {
	if requestBody := asMap(op["requestBody"]); len(requestBody) > 0 {
		content := asMap(requestBody["content"])
		contentType := preferredContentType(content)
		if contentType != "" {
			mediaType := asMap(content[contentType])
			body := encodeSampleBody(sampleValueForSchema(asMap(mediaType["schema"]), "body"), contentType)
			return body, contentType
		}
	}

	parameters, ok := op["parameters"].([]any)
	if !ok {
		return "", ""
	}
	formValues := url.Values{}
	for _, item := range parameters {
		param := asMap(item)
		switch str(param["in"]) {
		case "body":
			schema := asMap(param["schema"])
			body := encodeSampleBody(sampleValueForSchema(schema, str(param["name"])), "application/json")
			return body, "application/json"
		case "formData":
			name := str(param["name"])
			if name == "" {
				continue
			}
			formValues.Set(name, fmt.Sprint(sampleValueForParameter(param)))
		}
	}
	if len(formValues) > 0 {
		return formValues.Encode(), "application/x-www-form-urlencoded"
	}
	return "", ""
}

func preferredContentType(content map[string]any) string {
	if len(content) == 0 {
		return ""
	}
	for _, candidate := range []string{"application/json", "application/x-www-form-urlencoded", "multipart/form-data"} {
		if _, ok := content[candidate]; ok {
			return candidate
		}
	}
	keys := sortedKeys(content)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

func encodeSampleBody(value any, contentType string) string {
	if strings.Contains(contentType, "json") {
		data, err := json.MarshalIndent(value, "", "  ")
		if err == nil {
			return string(data)
		}
	}
	if form, ok := value.(url.Values); ok {
		return form.Encode()
	}
	if mapped, ok := value.(map[string]any); ok && (strings.Contains(contentType, "form") || strings.Contains(contentType, "multipart")) {
		values := url.Values{}
		for key, val := range mapped {
			values.Set(key, fmt.Sprint(val))
		}
		return values.Encode()
	}
	return fmt.Sprint(value)
}

func sampleValueForSchema(schema map[string]any, fallbackName string) any {
	if len(schema) == 0 {
		return sampleScalarValue(fallbackName, "string", "")
	}
	if example, ok := schema["example"]; ok {
		return example
	}
	if defaultValue, ok := schema["default"]; ok {
		return defaultValue
	}
	if enumValues, ok := schema["enum"].([]any); ok && len(enumValues) > 0 {
		return enumValues[0]
	}

	schemaType := str(schema["type"])
	if schemaType == "" && len(asMap(schema["properties"])) > 0 {
		schemaType = "object"
	}
	switch schemaType {
	case "object":
		out := map[string]any{}
		properties := asMap(schema["properties"])
		for _, key := range sortedKeys(properties) {
			out[key] = sampleValueForSchema(asMap(properties[key]), key)
		}
		if len(out) == 0 {
			out[fallbackNameOrDefault(fallbackName)] = "test"
		}
		return out
	case "array":
		return []any{sampleValueForSchema(asMap(schema["items"]), fallbackName)}
	default:
		return sampleScalarValue(fallbackName, schemaType, str(schema["format"]))
	}
}

func sampleValueForParameter(param map[string]any) any {
	paramType := str(param["type"])
	if paramType == "" {
		paramType = str(asMap(param["schema"])["type"])
	}
	return sampleScalarValue(str(param["name"]), paramType, str(param["format"]))
}

func sampleScalarValue(name string, typ string, format string) any {
	lowerName := strings.ToLower(name)
	lowerType := strings.ToLower(typ)
	switch {
	case lowerType == "integer" || lowerType == "number" || strings.Contains(lowerName, "id"):
		return 1
	case lowerType == "boolean":
		return true
	case strings.Contains(lowerName, "page"):
		return 1
	case strings.Contains(lowerName, "size") || strings.Contains(lowerName, "limit"):
		return 10
	case strings.Contains(lowerName, "date") || format == "date":
		return time.Now().Format("2006-01-02")
	case strings.Contains(lowerName, "email") || format == "email":
		return "test@example.com"
	case strings.Contains(lowerName, "password"):
		return "Password123!"
	case strings.Contains(lowerName, "phone"):
		return "13800000000"
	default:
		return "test"
	}
}

func fallbackNameOrDefault(name string) string {
	if strings.TrimSpace(name) == "" {
		return "value"
	}
	return strings.TrimSpace(name)
}

func riskTagsForEndpoint(method string, path string, params []Parameter) []string {
	text := strings.ToLower(method + " " + path)
	tags := []string{}
	add := func(tag string) {
		if !contains(tags, tag) {
			tags = append(tags, tag)
		}
	}
	if strings.Contains(text, "admin") || strings.Contains(text, "manage") {
		add("admin")
	}
	for _, marker := range []string{"delete", "remove", "upload", "export", "download", "debug", "config", "backup"} {
		if strings.Contains(text, marker) {
			add("sensitive_operation")
		}
	}
	if strings.Contains(text, "{id}") || strings.Contains(text, "/id") || strings.Contains(text, "userid") || strings.Contains(text, "user_id") || strings.Contains(text, "tenant") || strings.Contains(text, "orgid") || strings.Contains(text, "org_id") {
		add("idor_candidate")
	}
	if strings.Contains(text, "url") || strings.Contains(text, "redirect") || strings.Contains(text, "callback") {
		add("ssrf_candidate")
	}
	for _, param := range params {
		name := strings.ToLower(param.Name)
		if strings.Contains(name, "id") || strings.Contains(name, "tenant") || strings.Contains(name, "org") {
			add("idor_candidate")
		}
		if name == "url" || strings.Contains(name, "redirect") || strings.Contains(name, "callback") {
			add("ssrf_candidate")
		}
		if strings.Contains(name, "token") || strings.Contains(name, "key") || strings.Contains(name, "secret") {
			add("credential_parameter")
		}
	}
	sort.Strings(tags)
	return tags
}

func resolveEndpointURL(baseURL string, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	if baseURL == "" {
		return raw
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return raw
	}
	rel, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if strings.HasPrefix(raw, "/") && base.Path != "" && base.Path != "/" {
		base.Path = strings.TrimRight(base.Path, "/") + rel.Path
		base.RawQuery = rel.RawQuery
		base.Fragment = rel.Fragment
		return base.String()
	}
	return base.ResolveReference(rel).String()
}

func extractGraphQLOperationNames(input string, limit int) []string {
	re := regexp.MustCompile(`(?m)\b(?:type|input|interface|enum)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	seen := map[string]bool{}
	names := []string{}
	for _, match := range re.FindAllStringSubmatch(input, -1) {
		name := match[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
		if len(names) >= limit {
			break
		}
	}
	return names
}

func asMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	if typed, ok := value.(map[any]any); ok {
		out := make(map[string]any, len(typed))
		for key, val := range typed {
			out[fmt.Sprint(key)] = val
		}
		return out
	}
	return nil
}

func sortedKeys(input map[string]any) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func str(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(typed, "true")
	default:
		return false
	}
}

func hasNonEmpty(value any) bool {
	switch typed := value.(type) {
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	case nil:
		return false
	default:
		return true
	}
}

func sanitizeHeaders(headers map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func hasAuthHeaders(headers map[string]string) bool {
	for key := range headers {
		normalized := strings.ToLower(key)
		if normalized == "authorization" || normalized == "cookie" || normalized == "x-api-key" || normalized == "x-auth-token" {
			return true
		}
	}
	return false
}

func removeAuthHeaders(headers map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range headers {
		normalized := strings.ToLower(key)
		if normalized == "authorization" || normalized == "cookie" || normalized == "x-api-key" || normalized == "x-auth-token" {
			continue
		}
		out[key] = value
	}
	return out
}

func safeMethod(method string) string {
	if isSafeHTTPMethod(method) {
		return method
	}
	return "GET"
}

func isSafeHTTPMethod(method string) bool {
	method = strings.ToUpper(method)
	return method == "GET" || method == "HEAD" || method == "OPTIONS"
}

func timeout(seconds int) time.Duration {
	if seconds <= 0 {
		return DefaultTimeout
	}
	if seconds > 30 {
		seconds = 30
	}
	return time.Duration(seconds) * time.Second
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func PrettyJSON(value any) string {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
	return strings.TrimSpace(buf.String())
}
