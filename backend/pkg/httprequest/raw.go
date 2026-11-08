package httprequest

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"sort"
	"strings"
)

type RawRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    string
}

type MultipartField struct {
	Name  string
	Value string
}

type MultipartFile struct {
	FieldName   string
	FileName    string
	ContentType string
	Content     []byte
}

func BuildRawRequest(req RawRequest) string {
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}

	parsed, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil {
		return buildRawRequestWithTarget(method, strings.TrimSpace(req.URL), "", req.Headers, req.Body)
	}

	target := parsed.RequestURI()
	if target == "" {
		target = "/"
	}
	return buildRawRequestWithTarget(method, target, parsed.Host, req.Headers, req.Body)
}

func BuildMultipartBody(fields []MultipartField, files []MultipartFile, boundary string) (string, string, error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	if strings.TrimSpace(boundary) != "" {
		if err := writer.SetBoundary(strings.TrimSpace(boundary)); err != nil {
			return "", "", err
		}
	}

	for _, field := range fields {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			continue
		}
		if err := writer.WriteField(name, field.Value); err != nil {
			return "", "", err
		}
	}

	for _, file := range files {
		fieldName := strings.TrimSpace(file.FieldName)
		if fieldName == "" {
			fieldName = "file"
		}
		fileName := strings.TrimSpace(file.FileName)
		if fileName == "" {
			fileName = "sample.txt"
		}
		contentType := strings.TrimSpace(file.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeMultipartQuote(fieldName), escapeMultipartQuote(fileName)))
		header.Set("Content-Type", contentType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return "", "", err
		}
		if _, err := part.Write(file.Content); err != nil {
			return "", "", err
		}
	}

	if err := writer.Close(); err != nil {
		return "", "", err
	}
	return buffer.String(), writer.FormDataContentType(), nil
}

func BuildMultipartRawRequest(method string, targetURL string, headers map[string]string, fields []MultipartField, files []MultipartFile, boundary string) (string, string, error) {
	body, contentType, err := BuildMultipartBody(fields, files, boundary)
	if err != nil {
		return "", "", err
	}
	nextHeaders := cloneHeaders(headers)
	nextHeaders["Content-Type"] = contentType
	raw := BuildRawRequest(RawRequest{
		Method:  method,
		URL:     targetURL,
		Headers: nextHeaders,
		Body:    body,
	})
	return raw, contentType, nil
}

func buildRawRequestWithTarget(method string, target string, host string, headers map[string]string, body string) string {
	if strings.TrimSpace(target) == "" {
		target = "/"
	}
	lines := []string{method + " " + target + " HTTP/1.1"}
	if strings.TrimSpace(host) != "" {
		lines = append(lines, "Host: "+strings.TrimSpace(host))
	}

	headerKeys := sortedHeaderKeys(headers)
	hasTransferEncoding := false
	for _, key := range headerKeys {
		if strings.EqualFold(key, "Host") || strings.EqualFold(key, "Content-Length") {
			continue
		}
		if strings.EqualFold(key, "Transfer-Encoding") {
			hasTransferEncoding = true
		}
		value := strings.TrimSpace(headers[key])
		if value == "" {
			continue
		}
		lines = append(lines, strings.TrimSpace(key)+": "+value)
	}
	if body != "" && !hasTransferEncoding {
		lines = append(lines, fmt.Sprintf("Content-Length: %d", len([]byte(body))))
	}
	return strings.Join(lines, "\n") + "\n\n" + body
}

func sortedHeaderKeys(headers map[string]string) []string {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		if strings.TrimSpace(key) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneHeaders(headers map[string]string) map[string]string {
	out := make(map[string]string, len(headers)+1)
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func escapeMultipartQuote(value string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(value)
}
