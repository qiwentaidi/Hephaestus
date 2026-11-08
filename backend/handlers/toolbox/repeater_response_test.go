package toolbox

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestInferRepeaterResponseLength(t *testing.T) {
	tests := []struct {
		name string
		resp *http.Response
		want int64
	}{
		{
			name: "nil response",
			resp: nil,
			want: 0,
		},
		{
			name: "known content length",
			resp: &http.Response{ContentLength: 5237762},
			want: 5237762,
		},
		{
			name: "unknown content length",
			resp: &http.Response{ContentLength: -1},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inferRepeaterResponseLength(tt.resp); got != tt.want {
				t.Fatalf("inferRepeaterResponseLength() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestShouldHandleRepeaterResponseAsBinaryByContentType(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{
			"Content-Type": []string{"application/zip; charset=UTF-8"},
		},
	}

	if !shouldHandleRepeaterResponseAsBinary(resp) {
		t.Fatal("expected zip response to be treated as binary")
	}
}

func TestShouldHandleRepeaterResponseAsBinaryByAttachment(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{
			"Content-Type":        []string{"text/plain; charset=utf-8"},
			"Content-Disposition": []string{"attachment; filename=\"sample.txt\""},
		},
	}

	if !shouldHandleRepeaterResponseAsBinary(resp) {
		t.Fatal("expected attachment response to be treated as binary")
	}
}

func TestDumpRepeaterBinaryResponseIncludesHeadersAndPreviewMeta(t *testing.T) {
	resp := &http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"Content-Type":        []string{"application/zip"},
			"Content-Disposition": []string{"attachment; filename=\"sample.zip\""},
			"Content-Length":      []string{"9"},
		},
		Body: io.NopCloser(strings.NewReader("PK\x03\x04hello")),
	}

	got, err := dumpRepeaterBinaryResponse(resp)
	if err != nil {
		t.Fatalf("dumpRepeaterBinaryResponse returned error: %v", err)
	}

	for _, want := range []string{
		"HTTP/1.1 200 OK",
		"Content-Type: application/zip",
		"Content-Disposition: attachment; filename=\"sample.zip\"",
		"Content-Length: 9",
		"PK",
		"hello",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got: %s", want, got)
		}
	}
}

func TestDumpRepeaterBinaryResponseDoesNotSynthesizeContentLengthFromResponseField(t *testing.T) {
	resp := &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		ProtoMajor:    1,
		ProtoMinor:    1,
		ContentLength: 5237762,
		Header: http.Header{
			"Content-Type": []string{"application/x-msdownload; charset=UTF-8"},
			"Server":       []string{"SY8045"},
		},
		Body: io.NopCloser(strings.NewReader("PK\x03\x04preview")),
	}

	got, err := dumpRepeaterBinaryResponse(resp)
	if err != nil {
		t.Fatalf("dumpRepeaterBinaryResponse returned error: %v", err)
	}

	if strings.Contains(got, "Content-Length: 5237762") {
		t.Fatalf("expected output not to synthesize Content-Length from response field, got: %s", got)
	}
}

func TestDumpRepeaterBinaryResponseDoesNotSynthesizeContentLengthFromBodyLength(t *testing.T) {
	resp := &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		ProtoMajor:    1,
		ProtoMinor:    1,
		ContentLength: -1,
		Header: http.Header{
			"Content-Type": []string{"application/x-msdownload; charset=UTF-8"},
			"Server":       []string{"SY8045"},
		},
		Body: io.NopCloser(strings.NewReader("PK\x03\x04hello")),
	}

	got, err := dumpRepeaterBinaryResponse(resp)
	if err != nil {
		t.Fatalf("dumpRepeaterBinaryResponse returned error: %v", err)
	}

	if strings.Contains(got, "Content-Length: 9") {
		t.Fatalf("expected output not to synthesize Content-Length from body length, got: %s", got)
	}
}

func TestDumpRepeaterBinaryResponseEscapesNullBytes(t *testing.T) {
	resp := &http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"Content-Type": []string{"application/octet-stream"},
		},
		Body: io.NopCloser(strings.NewReader("PK\x00\x03\x04hello")),
	}

	got, err := dumpRepeaterBinaryResponse(resp)
	if err != nil {
		t.Fatalf("dumpRepeaterBinaryResponse returned error: %v", err)
	}

	if !strings.Contains(got, `PK\x00`) {
		t.Fatalf("expected output to escape null byte, got: %s", got)
	}
}

func TestDumpRepeaterBinaryResponseTruncatesLargePreview(t *testing.T) {
	largeBody := strings.Repeat("A", repeaterBinaryPreviewLimit+32)
	resp := &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		ProtoMajor:    1,
		ProtoMinor:    1,
		ContentLength: -1,
		Header: http.Header{
			"Content-Type": []string{"application/octet-stream"},
		},
		Body: io.NopCloser(strings.NewReader(largeBody)),
	}

	got, err := dumpRepeaterBinaryResponse(resp)
	if err != nil {
		t.Fatalf("dumpRepeaterBinaryResponse returned error: %v", err)
	}

	if strings.Contains(got, "Content-Length: 4128") {
		t.Fatalf("expected output not to synthesize unknown large content length, got: %s", got)
	}
	if !strings.Contains(got, strings.Repeat("A", repeaterBinaryPreviewLimit)) {
		t.Fatalf("expected output to include truncated preview content")
	}
}
