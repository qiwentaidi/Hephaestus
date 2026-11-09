//go:build linux || darwin

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hephaestus-backend/pkg/dockerupdate"
)

func TestRestartRequiresMatchingStagedImage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		phase   string
		version string
		digest  string
		code    int
	}{
		{"malformed version", dockerupdate.Staged, "../v1.2.3", "sha256:" + strings.Repeat("a", 64), http.StatusBadRequest},
		{"different version", dockerupdate.Staged, "v1.2.4", "sha256:" + strings.Repeat("a", 64), http.StatusConflict},
		{"missing digest", dockerupdate.Staged, "v1.2.3", "", http.StatusConflict},
		{"still pulling", dockerupdate.Pulling, "v1.2.3", "", http.StatusConflict},
		{"retry accepted restart", dockerupdate.Restarting, "v1.2.3", "sha256:" + strings.Repeat("a", 64), http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := job{Status: dockerupdate.Status{Version: "v1.2.3", Phase: tc.phase}, Digest: tc.digest}
			u := updater{job: current, stateDir: t.TempDir()}
			request := httptest.NewRequest(http.MethodPost, "/restart", strings.NewReader(`{"version":"`+tc.version+`"}`))
			response := httptest.NewRecorder()

			u.startRestart(response, request)

			if response.Code != tc.code || u.job != current {
				t.Fatalf("status %d, job %+v; want status %d with existing job %+v", response.Code, u.job, tc.code, current)
			}
		})
	}
}
