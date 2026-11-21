package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// SetDesktopStorage implements mcp.DesktopUIActions. It mirrors the desktop
// storage API: persist the keys, then push them into the live webview so the
// UI can react without a restart.
func (d *DesktopService) SetDesktopStorage(ctx context.Context, items map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	clean := make(map[string]string, len(items))
	for key, value := range items {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		clean[key] = value
	}
	if len(clean) == 0 {
		return nil
	}
	if err := mergeDesktopStorage(clean); err != nil {
		return fmt.Errorf("写入桌面端存储失败: %w", err)
	}
	d.broadcastDesktopStorageItems(clean)
	return nil
}

// OpenDesktopRoute implements mcp.DesktopUIActions. It loads the route through
// the window's native URL handler instead of ExecJS: ExecJS is only flushed
// once the Wails JS runtime reports ready, which the external Next dev server
// never does, so a queued script would silently do nothing in `wails3 dev`.
// A native load works in both dev and packaged builds and guarantees the target
// page fetches its data before the caller screenshots it.
func (d *DesktopService) OpenDesktopRoute(ctx context.Context, route string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	route = strings.TrimSpace(route)
	if !strings.HasPrefix(route, "/") || strings.HasPrefix(route, "//") {
		return fmt.Errorf("无效的应用内路由: %q", route)
	}
	if strings.ContainsAny(route, "\r\n\t ") {
		return fmt.Errorf("无效的应用内路由: %q", route)
	}
	window := d.getWindow()
	if window == nil {
		return errors.New("桌面客户端窗口尚未就绪")
	}
	window.SetURL(d.windowURL(route))
	return nil
}
