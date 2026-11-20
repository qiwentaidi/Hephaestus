package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"hephaestus-backend/mcp"

	"github.com/gin-gonic/gin"
)

func testDesktopScreenshotPNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buffer.Bytes()
}

func TestDecodeDesktopScreenshotDataURL(t *testing.T) {
	pngBytes := testDesktopScreenshotPNG(t)
	dataURL := desktopScreenshotDataURLPrefix + base64.StdEncoding.EncodeToString(pngBytes)

	decoded, width, height, err := decodeDesktopScreenshotDataURL(dataURL)
	if err != nil {
		t.Fatalf("decode screenshot: %v", err)
	}
	if !bytes.Equal(decoded, pngBytes) {
		t.Fatalf("decoded bytes differ from the encoded PNG")
	}
	if width != 4 || height != 3 {
		t.Fatalf("decoded size = %d×%d, want 4×3", width, height)
	}
}

func TestDecodeDesktopScreenshotDataURLRejectsInvalidInput(t *testing.T) {
	validPNG := base64.StdEncoding.EncodeToString(testDesktopScreenshotPNG(t))
	cases := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "wrong prefix", value: "data:image/jpeg;base64," + validPNG},
		{name: "invalid base64", value: desktopScreenshotDataURLPrefix + "%%%"},
		{name: "not png", value: desktopScreenshotDataURLPrefix + base64.StdEncoding.EncodeToString([]byte("not png"))},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, _, _, err := decodeDesktopScreenshotDataURL(testCase.value); err == nil {
				t.Fatalf("expected an error")
			}
		})
	}
}

func TestCompleteDesktopScreenshotResolvesPendingRequest(t *testing.T) {
	outcome := desktopScreenshotOutcome{screenshot: &mcp.DesktopScreenshot{Width: 12, Height: 34}}
	channel := make(chan desktopScreenshotOutcome, 1)
	service := &DesktopService{
		screenshotPending: map[string]chan desktopScreenshotOutcome{"request-1": channel},
	}

	if !service.completeDesktopScreenshot("request-1", outcome) {
		t.Fatalf("completeDesktopScreenshot should resolve the pending request")
	}
	select {
	case resolved := <-channel:
		if resolved.screenshot == nil || resolved.screenshot.Width != 12 || resolved.screenshot.Height != 34 {
			t.Fatalf("resolved outcome = %#v", resolved)
		}
	default:
		t.Fatalf("outcome was not delivered")
	}

	if service.completeDesktopScreenshot("request-1", outcome) {
		t.Fatalf("a completed request must not resolve twice")
	}
}

func TestCompleteDesktopScreenshotRejectsUnknownRequest(t *testing.T) {
	service := &DesktopService{}
	if service.completeDesktopScreenshot("missing", desktopScreenshotOutcome{}) {
		t.Fatalf("unknown request should not resolve")
	}
}

func TestCaptureDesktopScreenshotRequiresWindow(t *testing.T) {
	service := &DesktopService{}
	if _, err := service.CaptureDesktopScreenshot(context.Background(), mcp.DesktopScreenshotOptions{}); err == nil {
		t.Fatalf("capture should fail when the main window is unavailable")
	}
}

func TestHandleDesktopScreenshotCompleteRejectsInvalidPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	service := &DesktopService{}
	router.POST("/complete", service.handleDesktopScreenshotComplete)

	request := httptest.NewRequest(http.MethodPost, "/complete", bytes.NewBufferString(`{"requestId":""}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestHandleDesktopScreenshotCompleteDeliversCapture(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	channel := make(chan desktopScreenshotOutcome, 1)
	service := &DesktopService{
		screenshotPending: map[string]chan desktopScreenshotOutcome{"route-request": channel},
	}
	router.POST("/complete", service.handleDesktopScreenshotComplete)

	pngBytes := testDesktopScreenshotPNG(t)
	payload := map[string]interface{}{
		"requestId": "route-request",
		"dataUrl":   desktopScreenshotDataURLPrefix + base64.StdEncoding.EncodeToString(pngBytes),
		"width":     1360,
		"height":    734,
		"route":     "/scan/repeater",
		"title":     "Hephaestus",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/complete", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	select {
	case outcome := <-channel:
		if outcome.err != nil {
			t.Fatalf("outcome error = %v", outcome.err)
		}
		if outcome.screenshot == nil || outcome.screenshot.Width != 1360 || outcome.screenshot.Height != 734 {
			t.Fatalf("outcome screenshot = %#v", outcome.screenshot)
		}
		if outcome.screenshot.Route != "/scan/repeater" || outcome.screenshot.Title != "Hephaestus" {
			t.Fatalf("outcome metadata = %#v", outcome.screenshot)
		}
		if !bytes.Equal(outcome.screenshot.PNG, pngBytes) {
			t.Fatalf("outcome PNG does not match the posted payload")
		}
	default:
		t.Fatalf("screenshot was not delivered to the pending request")
	}
}

func TestPrepareNativeScreenshotDownscalesForInlineCapture(t *testing.T) {
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 8, 6))); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	result, err := prepareNativeScreenshot(buffer.Bytes(), 4, 3, 1)
	if err != nil {
		t.Fatalf("prepare native screenshot: %v", err)
	}
	if result.Width != 4 || result.Height != 3 || result.Scale != 1 {
		t.Fatalf("result metadata = %dx%d scale %.2f, want 4x3 scale 1", result.Width, result.Height, result.Scale)
	}
	config, err := png.DecodeConfig(bytes.NewReader(result.PNG))
	if err != nil {
		t.Fatalf("decode resized png: %v", err)
	}
	if config.Width != 4 || config.Height != 3 {
		t.Fatalf("resized png = %dx%d, want 4x3", config.Width, config.Height)
	}
}

func TestPrepareNativeScreenshotKeepsHighResolutionForButton(t *testing.T) {
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 8, 6))); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	result, err := prepareNativeScreenshot(buffer.Bytes(), 4, 3, 2)
	if err != nil {
		t.Fatalf("prepare native screenshot: %v", err)
	}
	if result.Width != 4 || result.Height != 3 || result.Scale != 2 {
		t.Fatalf("result metadata = %dx%d scale %.2f, want 4x3 scale 2", result.Width, result.Height, result.Scale)
	}
	config, err := png.DecodeConfig(bytes.NewReader(result.PNG))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if config.Width != 8 || config.Height != 6 {
		t.Fatalf("png = %dx%d, want 8x6", config.Width, config.Height)
	}
}

func TestPrepareNativeScreenshotUsesImageSizeWhenWindowIsUnknown(t *testing.T) {
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 8, 6))); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	result, err := prepareNativeScreenshot(buffer.Bytes(), 0, 0, 1)
	if err != nil {
		t.Fatalf("prepare native screenshot: %v", err)
	}
	if result.Width != 8 || result.Height != 6 {
		t.Fatalf("result size = %dx%d, want 8x6", result.Width, result.Height)
	}
}

func TestHandleDesktopScreenshotCaptureRequiresWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	service := &DesktopService{}
	router.POST("/capture", service.handleDesktopScreenshotCapture)

	request := httptest.NewRequest(http.MethodPost, "/capture", bytes.NewBufferString(`{"scale":2}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusServiceUnavailable, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"fallback":"dom"`)) {
		t.Fatalf("response should advertise the DOM fallback: %s", recorder.Body.String())
	}
}
