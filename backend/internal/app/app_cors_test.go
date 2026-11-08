package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildRouterAllowsPatchCORSPreflight(t *testing.T) {
	router := New().BuildRouter(false)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/ai/projects/project-1", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", http.MethodPatch)
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if methods := recorder.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(methods, http.MethodPatch) {
		t.Fatalf("Access-Control-Allow-Methods = %q, want PATCH", methods)
	}
}

func TestBuildRouterAllowsSSECursorCORSPreflight(t *testing.T) {
	router := New().BuildRouter(false)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, "/api/v1/ai/sessions/session-1/stream", nil)
			req.Header.Set("Origin", "http://localhost:3000")
			req.Header.Set("Access-Control-Request-Method", method)
			req.Header.Set("Access-Control-Request-Headers", "authorization,content-type,last-event-id")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusNoContent {
				t.Fatalf("preflight status = %d, want %d", recorder.Code, http.StatusNoContent)
			}
			headers := strings.ToLower(recorder.Header().Get("Access-Control-Allow-Headers"))
			for _, header := range []string{"authorization", "content-type", "last-event-id"} {
				if !strings.Contains(headers, header) {
					t.Fatalf("Access-Control-Allow-Headers = %q, missing %s", headers, header)
				}
			}
		})
	}
}
