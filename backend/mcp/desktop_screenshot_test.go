package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"testing"
	"time"
)

type fakeDesktopScreenshotProvider struct {
	screenshot *DesktopScreenshot
	err        error
}

func (f fakeDesktopScreenshotProvider) CaptureDesktopScreenshot(
	_ context.Context,
	_ DesktopScreenshotOptions,
) (*DesktopScreenshot, error) {
	return f.screenshot, f.err
}

func testDesktopScreenshotPNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buffer.Bytes()
}

func TestDesktopScreenshotToolRequiresDesktopProvider(t *testing.T) {
	RegisterDesktopScreenshotProvider(nil)
	t.Cleanup(func() { RegisterDesktopScreenshotProvider(nil) })

	for _, tool := range mcpToolsForProfile(mcpHephaestusProfile) {
		if tool.Name == desktopScreenshotToolName {
			t.Fatalf("desktop screenshot tool must not be advertised without a provider")
		}
	}

	if _, err := mcpCallDesktopScreenshot(context.Background()); err == nil {
		t.Fatalf("expected an error when no desktop provider is registered")
	}
}

func TestDesktopScreenshotToolReturnsImageContent(t *testing.T) {
	pngBytes := testDesktopScreenshotPNG(t)
	RegisterDesktopScreenshotProvider(fakeDesktopScreenshotProvider{
		screenshot: &DesktopScreenshot{
			PNG:        pngBytes,
			Width:      1360,
			Height:     734,
			Route:      "/scan/repeater",
			Title:      "Hephaestus",
			CapturedAt: time.Date(2026, 11, 20, 0, 16, 3, 0, time.UTC),
		},
	})
	t.Cleanup(func() { RegisterDesktopScreenshotProvider(nil) })

	advertised := false
	for _, tool := range mcpToolsForProfile(mcpHephaestusProfile) {
		if tool.Name == desktopScreenshotToolName {
			advertised = true
			break
		}
	}
	if !advertised {
		t.Fatalf("desktop screenshot tool should be advertised when a provider is registered")
	}

	result, err := mcpCallDesktopScreenshot(context.Background())
	if err != nil {
		t.Fatalf("call desktop screenshot: %v", err)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content blocks = %d, want 2", len(result.Content))
	}
	if result.Content[0].Type != "image" || result.Content[0].MimeType != "image/png" {
		t.Fatalf("first content block = %#v, want image/png", result.Content[0])
	}
	decoded, err := base64.StdEncoding.DecodeString(result.Content[0].Data)
	if err != nil {
		t.Fatalf("decode image content: %v", err)
	}
	if !bytes.Equal(decoded, pngBytes) {
		t.Fatalf("image content does not match the captured PNG")
	}
	if result.Content[1].Type != "text" || result.Content[1].Text == "" {
		t.Fatalf("second content block should summarise the capture: %#v", result.Content[1])
	}

	structured, ok := result.StructuredContent.(map[string]interface{})
	if !ok {
		t.Fatalf("structured content = %#v, want an object", result.StructuredContent)
	}
	if structured["width"] != 1360 || structured["height"] != 734 {
		t.Fatalf("structured width/height = %v/%v, want 1360/734", structured["width"], structured["height"])
	}
	if structured["pixelWidth"] != 2 || structured["pixelHeight"] != 2 {
		t.Fatalf("structured pixel size = %v/%v, want 2/2", structured["pixelWidth"], structured["pixelHeight"])
	}
	if structured["route"] != "/scan/repeater" {
		t.Fatalf("structured route = %v", structured["route"])
	}
	if structured["capturedAt"] != "2026-11-20T00:16:03Z" {
		t.Fatalf("structured capturedAt = %v", structured["capturedAt"])
	}
}

func TestDesktopScreenshotToolRejectsInvalidPNG(t *testing.T) {
	RegisterDesktopScreenshotProvider(fakeDesktopScreenshotProvider{
		screenshot: &DesktopScreenshot{PNG: []byte("not a png")},
	})
	t.Cleanup(func() { RegisterDesktopScreenshotProvider(nil) })

	if _, err := mcpCallDesktopScreenshot(context.Background()); err == nil {
		t.Fatalf("expected invalid PNG error")
	}
}

func TestDesktopScreenshotToolPropagatesProviderError(t *testing.T) {
	RegisterDesktopScreenshotProvider(fakeDesktopScreenshotProvider{err: errors.New("window unavailable")})
	t.Cleanup(func() { RegisterDesktopScreenshotProvider(nil) })

	if _, err := mcpCallDesktopScreenshot(context.Background()); err == nil {
		t.Fatalf("expected provider error")
	}
}

func TestDesktopScreenshotToolCatalogDefinition(t *testing.T) {
	tool := findMCPToolForTest(t, desktopScreenshotToolName)
	if tool.Permission != "read" {
		t.Fatalf("permission = %q, want read", tool.Permission)
	}
	if tool.ModuleKey != "desktop" {
		t.Fatalf("moduleKey = %q, want desktop", tool.ModuleKey)
	}
	if tool.Annotations["readOnlyHint"] != true {
		t.Fatalf("readOnlyHint = %#v, want true", tool.Annotations["readOnlyHint"])
	}
}
