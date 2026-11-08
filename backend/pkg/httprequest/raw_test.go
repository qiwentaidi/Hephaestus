package httprequest

import (
	"strings"
	"testing"
)

func TestBuildRawRequest(t *testing.T) {
	got := BuildRawRequest(RawRequest{
		Method: "post",
		URL:    "http://example.com/api/login?debug=1",
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Host":         "ignored.example",
		},
		Body: `{"username":"test"}`,
	})

	want := "POST /api/login?debug=1 HTTP/1.1\n" +
		"Host: example.com\n" +
		"Content-Type: application/json\n" +
		"Content-Length: 19\n\n" +
		`{"username":"test"}`
	if got != want {
		t.Fatalf("raw request mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildMultipartRawRequest(t *testing.T) {
	raw, contentType, err := BuildMultipartRawRequest(
		"POST",
		"http://example.com/upload",
		map[string]string{"User-Agent": "test-agent"},
		[]MultipartField{{Name: "name", Value: "demo"}},
		[]MultipartFile{{
			FieldName:   "file",
			FileName:    "sample.txt",
			ContentType: "text/plain",
			Content:     []byte("hello"),
		}},
		"HephaestusBoundary",
	)
	if err != nil {
		t.Fatalf("BuildMultipartRawRequest returned error: %v", err)
	}
	if contentType != "multipart/form-data; boundary=HephaestusBoundary" {
		t.Fatalf("content type = %q", contentType)
	}
	for _, expected := range []string{
		"POST /upload HTTP/1.1",
		"Host: example.com",
		"Content-Type: multipart/form-data; boundary=HephaestusBoundary",
		"User-Agent: test-agent",
		`Content-Disposition: form-data; name="name"`,
		`Content-Disposition: form-data; name="file"; filename="sample.txt"`,
		"Content-Type: text/plain",
		"hello",
		"--HephaestusBoundary--",
	} {
		if !strings.Contains(raw, expected) {
			t.Fatalf("raw request missing %q:\n%s", expected, raw)
		}
	}
}
