package apidoctest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnalyzeOpenAPI(t *testing.T) {
	input := `
openapi: 3.0.1
info:
  title: Demo API
  version: 1.0.0
servers:
  - url: https://example.com
security:
  - bearerAuth: []
paths:
  /api/users/{id}:
    get:
      operationId: getUser
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
  /api/admin/export:
    get:
      operationId: exportUsers
`
	result, err := AnalyzeAndTest(context.Background(), Request{
		Input:         input,
		DocumentType:  "auto",
		GenerateCases: true,
	})
	if err != nil {
		t.Fatalf("AnalyzeAndTest returned error: %v", err)
	}
	if result.DocumentType != "openapi" {
		t.Fatalf("document type = %q, want openapi", result.DocumentType)
	}
	if got := len(result.Endpoints); got != 2 {
		t.Fatalf("endpoint count = %d, want 2", got)
	}
	if result.Endpoints[0].URL != "https://example.com/api/admin/export" {
		t.Fatalf("first endpoint URL = %q", result.Endpoints[0].URL)
	}
	if result.Summary.AuthRequiredCount != 2 {
		t.Fatalf("auth required count = %d, want 2", result.Summary.AuthRequiredCount)
	}
	if len(result.Cases) == 0 {
		t.Fatal("expected generated cases")
	}
}

func TestAnalyzeSwaggerInheritsSourceSchemeWhenSchemesMissing(t *testing.T) {
	input := `
swagger: "2.0"
info:
  title: Legacy API
  version: "2.0"
host: bdcdj.taixing.cn:8896
basePath: /bdc
paths:
  /business/info/getCompleteList:
    post:
      operationId: getCompleteListUsingPOST
`
	result, err := AnalyzeAndTest(context.Background(), Request{
		Input:        input,
		SourceURL:    "http://bdcdj.taixing.cn:8896/v2/api-docs",
		DocumentType: "auto",
	})
	if err != nil {
		t.Fatalf("AnalyzeAndTest returned error: %v", err)
	}
	if got := result.Endpoints[0].URL; got != "http://bdcdj.taixing.cn:8896/bdc/business/info/getCompleteList" {
		t.Fatalf("endpoint URL = %q", got)
	}
}

func TestAnalyzeSwaggerPrefersSourceSchemeOverDocumentSchemes(t *testing.T) {
	input := `
swagger: "2.0"
info:
  title: Legacy API
  version: "2.0"
host: bdcdj.taixing.cn:8896
basePath: /bdc
schemes:
  - https
  - http
paths:
  /business/info/certificateAgainPrint:
    post:
      operationId: certificateAgainPrintUsingPOST
`
	result, err := AnalyzeAndTest(context.Background(), Request{
		Input:        input,
		SourceURL:    "http://bdcdj.taixing.cn:8896/v2/api-docs",
		DocumentType: "auto",
	})
	if err != nil {
		t.Fatalf("AnalyzeAndTest returned error: %v", err)
	}
	if got := result.Endpoints[0].URL; got != "http://bdcdj.taixing.cn:8896/bdc/business/info/certificateAgainPrint" {
		t.Fatalf("endpoint URL = %q", got)
	}
}

func TestAnalyzeOpenAPIExtractsJSONRequestBody(t *testing.T) {
	input := `
openapi: 3.0.1
info:
  title: Demo API
  version: 1.0.0
servers:
  - url: http://example.com
paths:
  /api/users:
    post:
      summary: 新增用户
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                username:
                  type: string
                age:
                  type: integer
`
	result, err := AnalyzeAndTest(context.Background(), Request{
		Input:        input,
		DocumentType: "auto",
	})
	if err != nil {
		t.Fatalf("AnalyzeAndTest returned error: %v", err)
	}
	endpoint := result.Endpoints[0]
	if endpoint.ContentType != "application/json" {
		t.Fatalf("content type = %q, want application/json", endpoint.ContentType)
	}
	if endpoint.RequestBody == "" {
		t.Fatal("expected generated request body")
	}
	if !strings.Contains(endpoint.RequestBody, `"username": "test"`) || !strings.Contains(endpoint.RequestBody, `"age": 1`) {
		t.Fatalf("request body = %s", endpoint.RequestBody)
	}
}

func TestAnalyzeSwaggerExtractsFormDataRequestBody(t *testing.T) {
	input := `
swagger: "2.0"
info:
  title: Legacy API
  version: "2.0"
host: example.com
schemes:
  - http
paths:
  /login:
    post:
      parameters:
        - name: account
          in: formData
          type: string
        - name: password
          in: formData
          type: string
`
	result, err := AnalyzeAndTest(context.Background(), Request{
		Input:        input,
		DocumentType: "auto",
	})
	if err != nil {
		t.Fatalf("AnalyzeAndTest returned error: %v", err)
	}
	endpoint := result.Endpoints[0]
	if endpoint.ContentType != "application/x-www-form-urlencoded" {
		t.Fatalf("content type = %q, want form urlencoded", endpoint.ContentType)
	}
	if endpoint.RequestBody != "account=test&password=Password123%21" {
		t.Fatalf("request body = %q", endpoint.RequestBody)
	}
}

func TestAnalyzeOpenAPIServerURLPrefersSourceSchemeForSameHost(t *testing.T) {
	input := `
openapi: 3.0.1
info:
  title: Demo API
  version: 1.0.0
servers:
  - url: https://bdcdj.taixing.cn:8896/bdc
paths:
  /business/info/certificateAgainPrint:
    post:
      operationId: certificateAgainPrintUsingPOST
`
	result, err := AnalyzeAndTest(context.Background(), Request{
		Input:        input,
		SourceURL:    "http://bdcdj.taixing.cn:8896/v3/api-docs",
		DocumentType: "auto",
	})
	if err != nil {
		t.Fatalf("AnalyzeAndTest returned error: %v", err)
	}
	if got := result.Endpoints[0].URL; got != "http://bdcdj.taixing.cn:8896/bdc/business/info/certificateAgainPrint" {
		t.Fatalf("endpoint URL = %q", got)
	}
}

func TestAnalyzeWSDL(t *testing.T) {
	input := `<?xml version="1.0"?>
<definitions name="DemoService" targetNamespace="urn:demo"
  xmlns="http://schemas.xmlsoap.org/wsdl/"
  xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/">
  <service name="DemoService">
    <port name="DemoPort">
      <soap:address location="https://example.com/soap"/>
    </port>
  </service>
  <binding name="DemoBinding">
    <operation name="GetUser"/>
    <operation name="ExportUsers"/>
  </binding>
</definitions>`
	result, err := AnalyzeAndTest(context.Background(), Request{
		Input:         input,
		DocumentType:  "auto",
		GenerateCases: true,
	})
	if err != nil {
		t.Fatalf("AnalyzeAndTest returned error: %v", err)
	}
	if result.DocumentType != "wsdl" {
		t.Fatalf("document type = %q, want wsdl", result.DocumentType)
	}
	if got := len(result.Endpoints); got != 2 {
		t.Fatalf("endpoint count = %d, want 2", got)
	}
	if result.Endpoints[0].URL != "https://example.com/soap" {
		t.Fatalf("endpoint URL = %q", result.Endpoints[0].URL)
	}
}

func TestRunEndpointReturnsResponseDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") != "yes" {
			t.Fatalf("missing X-Test header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Trace-ID", "trace-1")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	result, err := RunEndpoint(context.Background(), RunRequest{
		Method:  http.MethodGet,
		URL:     server.URL,
		Headers: map[string]string{"X-Test": "yes"},
	})
	if err != nil {
		t.Fatalf("RunEndpoint returned error: %v", err)
	}
	if result.StatusCode != http.StatusCreated {
		t.Fatalf("status code = %d, want %d", result.StatusCode, http.StatusCreated)
	}
	if result.ContentType != "application/json" {
		t.Fatalf("content type = %q, want application/json", result.ContentType)
	}
	if got := result.ResponseHeaders["X-Trace-Id"]; len(got) != 1 || got[0] != "trace-1" {
		t.Fatalf("response header X-Trace-ID = %v, want trace-1", got)
	}
	if result.ResponseBody != `{"ok":true}` {
		t.Fatalf("response body = %q", result.ResponseBody)
	}
}
