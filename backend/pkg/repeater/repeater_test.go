package repeater

import "testing"

func TestParseRawRequestKeepsBodyWithoutContentLength(t *testing.T) {
	raw := "POST /ccat/step/getStepParams HTTP/1.1\r\n" +
		"Host: prfm.caocaokeji.cn\r\n" +
		"Content-Type: application/json\r\n" +
		"\r\n" +
		"{\"caseId\":\"1\",\"httpReqType\":\"query\",\"stepId\":\"1\"}"

	req, err := parseRawRequest(raw)
	if err != nil {
		t.Fatalf("parseRawRequest returned error: %v", err)
	}

	expectedBody := "{\"caseId\":\"1\",\"httpReqType\":\"query\",\"stepId\":\"1\"}"
	if string(req.Body) != expectedBody {
		t.Fatalf("unexpected body: got %q want %q", string(req.Body), expectedBody)
	}

	if req.Host != "prfm.caocaokeji.cn" {
		t.Fatalf("unexpected host: got %q", req.Host)
	}
}

func TestBuildFullURLSupportsAbsoluteTarget(t *testing.T) {
	req := &rawRequest{
		Method:        "POST",
		RequestTarget: "http://example.com/api/test?id=1",
		Host:          "ignored.example",
	}

	got := buildFullURL(req, "https")
	want := "http://example.com/api/test?id=1"
	if got != want {
		t.Fatalf("unexpected url: got %q want %q", got, want)
	}
}
