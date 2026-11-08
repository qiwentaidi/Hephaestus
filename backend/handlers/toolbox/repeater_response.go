package toolbox

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

const repeaterBinaryPreviewLimit = 4096

func inferRepeaterResponseLength(resp *http.Response) int64 {
	if resp == nil {
		return 0
	}
	if resp.ContentLength > 0 {
		return resp.ContentLength
	}
	return 0
}

func shouldHandleRepeaterResponseAsBinary(resp *http.Response) bool {
	if resp == nil {
		return false
	}

	if isAttachmentDisposition(resp.Header.Get("Content-Disposition")) {
		return true
	}

	contentType := resp.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.ToLower(strings.Split(contentType, ";")[0]))
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType == "" {
		return false
	}

	if strings.HasPrefix(mediaType, "text/") {
		return false
	}

	switch mediaType {
	case "application/json",
		"application/ld+json",
		"application/problem+json",
		"application/xml",
		"application/xhtml+xml",
		"application/javascript",
		"application/x-javascript",
		"application/x-www-form-urlencoded",
		"application/graphql-response+json",
		"image/svg+xml":
		return false
	default:
		return true
	}
}

func isAttachmentDisposition(disposition string) bool {
	if disposition == "" {
		return false
	}
	value, _, err := mime.ParseMediaType(disposition)
	if err != nil {
		value = strings.TrimSpace(strings.ToLower(strings.Split(disposition, ";")[0]))
	}
	return strings.EqualFold(value, "attachment")
}

func dumpRepeaterBinaryResponse(resp *http.Response) (string, error) {
	if resp == nil {
		return "", fmt.Errorf("响应为空")
	}

	var previewBytes []byte
	if resp.Body != nil {
		var err error
		previewBytes, err = io.ReadAll(io.LimitReader(resp.Body, repeaterBinaryPreviewLimit+1))
		if err != nil {
			return "", fmt.Errorf("读取二进制响应预览失败: %w", err)
		}
		if len(previewBytes) > repeaterBinaryPreviewLimit {
			previewBytes = previewBytes[:repeaterBinaryPreviewLimit]
		}
	}

	var builder strings.Builder
	statusText := resp.Status
	if statusText == "" {
		statusText = strconv.Itoa(resp.StatusCode)
	}
	builder.WriteString(fmt.Sprintf("HTTP/%d.%d %s\r\n", resp.ProtoMajor, resp.ProtoMinor, statusText))

	if err := resp.Header.WriteSubset(&builder, map[string]bool{}); err != nil {
		return "", fmt.Errorf("写入响应头失败: %w", err)
	}

	builder.WriteString("\r\n")
	builder.WriteString(binaryPreviewToRawString(previewBytes))

	return builder.String(), nil
}

func binaryPreviewToRawString(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(data))
	for len(data) > 0 {
		if data[0] == 0x00 {
			builder.WriteString(`\x00`)
			data = data[1:]
			continue
		}

		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError && size == 1 {
			builder.WriteString(fmt.Sprintf(`\x%02x`, data[0]))
			data = data[1:]
			continue
		}
		builder.WriteRune(r)
		data = data[size:]
	}
	return builder.String()
}
