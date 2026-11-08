package repeater

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExecuteHTTPTestSendsRequestAndFiltersResponse(t *testing.T) {
	var receivedCookie string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Method; got != http.MethodPost {
			t.Errorf("method = %q, want POST", got)
		}
		if got := r.Header.Get("X-Test"); got != "value" {
			t.Errorf("X-Test = %q", got)
		}
		receivedCookie = r.Header.Get("Cookie")
		_, _ = w.Write([]byte("ignore\ntoken=expected\nignore"))
	}))
	defer server.Close()

	result, err := ExecuteHTTPTest(context.Background(), HTTPTestOptions{
		URL:            server.URL,
		Method:         http.MethodPost,
		Data:           "payload",
		Headers:        map[string]string{"X-Test": "value"},
		Cookies:        "session=abc",
		ResponseFilter: "token=",
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("ExecuteHTTPTest() error = %v", err)
	}
	if receivedCookie != "session=abc" {
		t.Fatalf("Cookie = %q", receivedCookie)
	}
	if len(result.Responses) != 1 {
		t.Fatalf("responses = %d, want 1", len(result.Responses))
	}
	if got := result.Responses[0].Body; got != "token=expected" {
		t.Fatalf("body = %q", got)
	}
}

func TestHTTPBeforeRequestChecksEachRepeat(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits++ }))
	defer server.Close()
	checks := 0
	blocked := errors.New("policy changed")
	_, err := ExecuteHTTPTest(context.Background(), HTTPTestOptions{
		URL: server.URL, Repeat: 3,
		BeforeRequest: func(req *http.Request) error {
			checks++
			if req.Method != http.MethodGet {
				t.Errorf("guard saw non-effective method %q", req.Method)
			}
			if checks == 2 {
				return blocked
			}
			return nil
		},
	})
	if !errors.Is(err, blocked) || checks != 2 || hits != 1 {
		t.Fatalf("repeat escaped policy: checks=%d hits=%d err=%v", checks, hits, err)
	}
}

func TestExecuteHTTPTestRepeatAndBodyCap(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte("one\ntwo\nthree"))
	}))
	defer server.Close()

	result, err := ExecuteHTTPTest(context.Background(), HTTPTestOptions{
		URL:              server.URL,
		Repeat:           2,
		ResponseMaxLines: 2,
	})
	if err != nil {
		t.Fatalf("ExecuteHTTPTest() error = %v", err)
	}
	if requests != 2 || len(result.Responses) != 2 {
		t.Fatalf("requests/responses = %d/%d, want 2/2", requests, len(result.Responses))
	}
	if !result.Responses[0].BodyTruncated || strings.Count(result.Responses[0].Body, "\n") != 1 {
		t.Fatalf("unexpected capped body: %#v", result.Responses[0])
	}
}

func TestExecuteHTTPTestRejectsInvalidURL(t *testing.T) {
	_, err := ExecuteHTTPTest(context.Background(), HTTPTestOptions{URL: "example.com"})
	if err == nil {
		t.Fatal("expected invalid URL error")
	}
}
