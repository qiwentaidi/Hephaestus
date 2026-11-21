package mcp

import (
	"context"
	"sync"
)

// DesktopUIActions is implemented by the Wails desktop host. It lets an MCP
// caller drive the operator's own window (open a route, persist a small amount
// of webview storage) so a follow-up desktop_screenshot captures the intended
// page. Server-only deployments leave it unset and the dependent tools are not
// advertised.
type DesktopUIActions interface {
	// SetDesktopStorage writes keys into the desktop webview's localStorage and
	// broadcasts the change. Callers must not put secrets here.
	SetDesktopStorage(ctx context.Context, items map[string]string) error
	// OpenDesktopRoute navigates the main window to a same-app route.
	OpenDesktopRoute(ctx context.Context, route string) error
}

var (
	desktopUIActionsMu sync.RWMutex
	desktopUIActions   DesktopUIActions
)

// RegisterDesktopUIActions wires the desktop host into the MCP UI tools. Pass
// nil to detach it (tests, server-only mode).
func RegisterDesktopUIActions(actions DesktopUIActions) {
	desktopUIActionsMu.Lock()
	defer desktopUIActionsMu.Unlock()
	desktopUIActions = actions
}

func currentDesktopUIActions() DesktopUIActions {
	desktopUIActionsMu.RLock()
	defer desktopUIActionsMu.RUnlock()
	return desktopUIActions
}

func hasDesktopUIActions() bool {
	return currentDesktopUIActions() != nil
}
