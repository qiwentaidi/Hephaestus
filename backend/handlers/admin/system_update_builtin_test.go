package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestValidateUpdateDownloadURL(t *testing.T) {
	for _, raw := range []string{
		"https://github.com/example/project/releases/download/v1/app.zip",
		"https://objects.githubusercontent.com/example/app.zip",
		"https://gitee.com/example/project/releases/download/v1/app.zip",
	} {
		if _, err := validateUpdateDownloadURL(raw); err != nil {
			t.Errorf("trusted URL %q rejected: %v", raw, err)
		}
	}
	for _, raw := range []string{
		"http://127.0.0.1:2375/version",
		"http://169.254.169.254/latest/meta-data/",
		"http://attacker.invalid/update.zip",
		"file:///etc/passwd",
	} {
		if _, err := validateUpdateDownloadURL(raw); err == nil {
			t.Errorf("untrusted URL %q was accepted", raw)
		}
	}
}

func TestUpdateServerHandlersUseBuiltinSource(t *testing.T) {
	gin.SetMode(gin.TestMode)

	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/config/update-server", nil)
	getRecorder := httptest.NewRecorder()
	getContext, _ := gin.CreateTestContext(getRecorder)
	getContext.Request = getRequest
	GetUpdateServerHandler(getContext)
	if getRecorder.Code != http.StatusOK || !bytes.Contains(getRecorder.Body.Bytes(), []byte(DefaultUpdateServer)) {
		t.Fatalf("GET returned %d %q, expected built-in update server", getRecorder.Code, getRecorder.Body.String())
	}

	putRequest := httptest.NewRequest(http.MethodPut, "/api/v1/config/update-server", bytes.NewBufferString(`{"updateServer":"http://attacker.invalid"}`))
	putRecorder := httptest.NewRecorder()
	putContext, _ := gin.CreateTestContext(putRecorder)
	putContext.Request = putRequest
	UpdateUpdateServerHandler(putContext)
	if putRecorder.Code != http.StatusForbidden {
		t.Fatalf("PUT returned %d, expected %d", putRecorder.Code, http.StatusForbidden)
	}
}

func TestSelectPublishedDownloadRequiresCurrentOfficialRelease(t *testing.T) {
	info := UpdateInfo{
		Version:     "v1.2.3",
		DownloadURL: "https://github.com/example/project/releases/download/v1.2.3/app.zip",
		MirrorURL:   "https://gitee.com/example/project/releases/download/v1.2.3/app.zip",
	}
	for _, link := range []string{info.DownloadURL, info.MirrorURL} {
		if got, err := selectPublishedDownload(info, link, info.Version); err != nil || got != link {
			t.Fatalf("published link rejected: %q, %v", got, err)
		}
	}
	if _, err := selectPublishedDownload(info, info.DownloadURL, "v1.2.2"); err == nil {
		t.Fatal("accepted a stale version")
	}
	if _, err := selectPublishedDownload(info, "https://github.com/another/project/app.zip", info.Version); err == nil {
		t.Fatal("accepted a link not advertised by the release API")
	}
}
