package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"hephaestus-backend/handlers/toolbox"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/repeater"
	"hephaestus-backend/pkg/toolguard"
)

const (
	repeaterSendToolName   = "repeater_send"
	repeaterOpenToolName   = "repeater_open"
	repeaterDeleteToolName = "repeater_delete"

	repeaterOpenStorageKey   = "mcp:repeater_open"
	repeaterDeleteStorageKey = "mcp:repeater_delete"
	repeaterRoute            = "/scan/repeater"

	repeaterDefaultTimeout = 60 * time.Second
	repeaterMaxTimeout     = 5 * time.Minute
	repeaterResponseLimit  = 256 << 10 // 256 KiB kept for the tab and the MCP reply
)

// Settles let tests shorten the desktop UI waits.
var (
	repeaterOpenSettle   = 2500 * time.Millisecond
	repeaterDeleteSettle = 2000 * time.Millisecond
	// repeaterOpenConfirmWait is how long we give the webview to load and
	// consume the one-shot open request before deciding the first SetURL was
	// dropped (a window that is still restoring its initial URL can lose it).
	repeaterOpenConfirmWait    = 1200 * time.Millisecond
	repeaterOpenRequestPending = desktopOpenRequestPending
)

var errRepeaterTabMissing = errors.New("repeater tab not found")

type mcpRepeaterSendArgs struct {
	RawRequest     string `json:"rawRequest"`
	Name           string `json:"name"`
	ForceHTTPS     bool   `json:"forceHttps"`
	Redirect       bool   `json:"redirect"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

type mcpRepeaterOpenArgs struct {
	TabID string `json:"tabId"`
}

// Seams so repeatable tests never need a live application database.
var (
	repeaterSaveTab   = saveRepeaterTabToDB
	repeaterLoadTab   = loadRepeaterTabFromDB
	repeaterDeleteTab = deleteRepeaterTabFromDB
)

// mcpCallRepeaterSend validates and sends a raw POC request, stores the
// request/response as a Repeater tab, and returns the response so the caller can
// judge whether the POC is correct before touching the operator's window. A
// failed send never creates a tab.
func mcpCallRepeaterSend(c *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
	var args mcpRepeaterSendArgs
	if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
		return repeaterToolError(err.Error()), nil
	}
	raw := strings.TrimSpace(args.RawRequest)
	if raw == "" {
		return repeaterToolError("rawRequest 不能为空。请传入完整的 HTTP 请求报文，例如 “GET /path HTTP/1.1\\r\\nHost: example.com\\r\\n\\r\\n”。"), nil
	}

	method, target, err := repeater.ParseRawRequestTarget(raw, args.ForceHTTPS)
	if err != nil {
		return repeaterToolError("解析 POC 请求失败：" + err.Error() + "；未发送、未写入标签页。"), nil
	}
	// Reuse the native HTTP tool's guard rules (dangerous methods, URL scope,
	// etc.). The generic tool-call guard skips this tool name by design.
	if match := toolguard.CheckToolCall("http-framework-test", map[string]any{
		"url": target, "method": method, "request": raw,
	}); match != nil {
		message := toolguard.FormatBlockedMessage(match)
		return mcpToolCallResult{
			Content:           []mcpToolContent{{Type: "text", Text: message}},
			StructuredContent: map[string]any{"blocked": true, "match": match, "saved": false, "opened": false},
			IsError:           true,
			Blocked:           true,
		}, nil
	}

	timeout := repeaterDefaultTimeout
	if args.TimeoutSeconds > 0 {
		timeout = time.Duration(args.TimeoutSeconds) * time.Second
		if timeout > repeaterMaxTimeout {
			timeout = repeaterMaxTimeout
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	started := time.Now()
	resp, duration, err := repeater.SendRequestWithRaw(ctx, raw, args.ForceHTTPS, args.Redirect, "")
	if err != nil {
		return repeaterToolError("POC 发送失败：" + err.Error() + "；未写入请求转发标签页，也未打开界面。请修正请求或目标后重试。"), nil
	}
	if resp.RawResponse != nil && resp.RawResponse.Body != nil {
		defer resp.RawResponse.Body.Close()
	}
	statusCode := resp.StatusCode()
	statusText := ""
	if resp.RawResponse != nil {
		statusText = resp.RawResponse.Status
	}
	durationMs := duration
	if durationMs <= 0 {
		durationMs = time.Since(started).Milliseconds()
	}

	responseText, responseLength, dumpErr := toolbox.DumpRepeaterResponse(resp.RawResponse)
	if dumpErr != nil {
		responseText = "读取响应体失败: " + dumpErr.Error()
		responseLength = 0
	}
	storedResponse, responseTruncated := limitRepeaterText(responseText, repeaterResponseLimit)

	hasRedirect := (statusCode == 301 || statusCode == 302 || statusCode == 303 ||
		statusCode == 307 || statusCode == 308) && !args.Redirect

	tab := database.RepeaterTab{
		ID:             uuid.NewString(),
		Name:           repeaterTabName(args.Name, method, target),
		Method:         method,
		URL:            target,
		RawRequest:     raw,
		Response:       storedResponse,
		ResponseLength: int(responseLength),
		ResponseTime:   int(durationMs),
		HasRedirect:    hasRedirect,
		ForceHttps:     args.ForceHTTPS,
	}
	saveErr := repeaterSaveTab(&tab)

	summary := fmt.Sprintf("POC 已发送：%s %s → HTTP %d（%s），用时 %.2fs，响应 %d 字节。",
		method, target, statusCode, strings.TrimSpace(statusText), float64(durationMs)/1000, responseLength)
	if saveErr != nil {
		summary += "\n写入请求转发标签页失败：" + saveErr.Error() + "。本次响应仍可用于判断 POC，但无法用 repeater_open 打开。"
	} else {
		summary += fmt.Sprintf("\n已写入请求转发标签页「%s」（tabId=%s）。确认响应符合预期后，可调用 %s 打开该标签页，再用 desktop_screenshot 截图。",
			tab.Name, tab.ID, repeaterOpenToolName)
	}
	if responseTruncated {
		summary += fmt.Sprintf("\n响应超过 %d KiB，标签页与本次返回均已截断。", repeaterResponseLimit>>10)
	}

	content := []mcpToolContent{{Type: "text", Text: summary + "\n\n" + storedResponse}}
	return mcpToolCallResult{
		Content: content,
		StructuredContent: map[string]any{
			"tabId":          tab.ID,
			"name":           tab.Name,
			"method":         method,
			"url":            target,
			"saved":          saveErr == nil,
			"statusCode":     statusCode,
			"status":         statusText,
			"responseTimeMs": durationMs,
			"responseLength": responseLength,
			"hasRedirect":    hasRedirect,
			"truncated":      responseTruncated,
		},
		IsError: saveErr != nil,
	}, nil
}

// mcpCallRepeaterOpen brings a previously stored Repeater tab to the front in
// the desktop client so the next desktop_screenshot captures it. It never sends
// a request.
func mcpCallRepeaterOpen(c *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
	var args mcpRepeaterOpenArgs
	if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
		return repeaterToolError(err.Error()), nil
	}
	tabID := strings.TrimSpace(args.TabID)
	if tabID == "" {
		return repeaterToolError("tabId 不能为空；请先调用 repeater_send 获取 tabId。"), nil
	}
	actions := currentDesktopUIActions()
	if actions == nil {
		return repeaterToolError("repeater_open 仅在 Hephaestus 桌面客户端中可用；本次未打开界面，也未改变任何数据。"), nil
	}
	tab, err := repeaterLoadTab(tabID)
	if err != nil {
		return repeaterToolError("读取请求转发标签页失败：" + err.Error() + "；未打开界面。"), nil
	}
	payload, marshalErr := json.Marshal(map[string]string{"tabId": tab.ID})
	if marshalErr != nil {
		return repeaterToolError("生成打开请求失败：" + marshalErr.Error()), nil
	}
	// One-shot request consumed by the Repeater editor: it selects the tab and
	// clears the key so a later restart does not steal the operator's own
	// active tab.
	if err := actions.SetDesktopStorage(c.Request.Context(), map[string]string{repeaterOpenStorageKey: string(payload)}); err != nil {
		return repeaterToolError("写入桌面端标签页选择失败：" + err.Error() + "；未打开界面。"), nil
	}
	if err := actions.OpenDesktopRoute(c.Request.Context(), repeaterRoute); err != nil {
		return repeaterToolError("打开请求转发页面失败：" + err.Error()), nil
	}
	// The page reloads and fetches tabs from the backend; give it a moment so an
	// immediate desktop_screenshot does not capture the previous page. If the
	// one-shot key was not consumed, the window was still initialising and
	// dropped the first load: repeat it while the key is still present.
	waitStarted := time.Now()
	confirm := repeaterOpenConfirmWait
	if confirm > repeaterOpenSettle {
		confirm = repeaterOpenSettle
	}
	waitForContext(c.Request.Context(), confirm)
	retried := false
	if repeaterOpenRequestPending() {
		if err := actions.OpenDesktopRoute(c.Request.Context(), repeaterRoute); err == nil {
			retried = true
			waitForContext(c.Request.Context(), repeaterOpenSettle-confirm)
		}
	}
	settled := time.Since(waitStarted)

	return mcpToolCallResult{
		Content: []mcpToolContent{{Type: "text", Text: fmt.Sprintf(
			"已在桌面端打开请求转发页面 %s，并选中标签页「%s」（tabId=%s）。现在可以调用 desktop_screenshot 截图核对。",
			repeaterRoute, tab.Name, tab.ID)}},
		StructuredContent: map[string]any{
			"opened":   true,
			"route":    repeaterRoute,
			"tabId":    tab.ID,
			"name":     tab.Name,
			"waitedMs": settled.Milliseconds(),
			"retried":  retried,
		},
	}, nil
}

// mcpCallRepeaterDelete removes a stored Repeater tab. It is idempotent: a
// missing tab reports success without touching the UI. On the desktop client it
// also reloads the Repeater page so the open editor drops the row immediately;
// without that reload the editor would keep showing a stale tab until the next
// navigation because desktop storage is only hydrated on page load.
func mcpCallRepeaterDelete(c *gin.Context, rawArgs json.RawMessage) (mcpToolCallResult, error) {
	var args mcpRepeaterOpenArgs
	if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
		return repeaterToolError(err.Error()), nil
	}
	tabID := strings.TrimSpace(args.TabID)
	if tabID == "" {
		return repeaterToolError("tabId 不能为空。"), nil
	}
	tab, err := repeaterLoadTab(tabID)
	if err != nil {
		if errors.Is(err, errRepeaterTabMissing) {
			return mcpToolCallResult{
				Content:           []mcpToolContent{{Type: "text", Text: fmt.Sprintf("标签页 %s 不存在或已删除，无需操作。", tabID)}},
				StructuredContent: map[string]any{"deleted": false, "alreadyMissing": true, "tabId": tabID},
			}, nil
		}
		return repeaterToolError("读取请求转发标签页失败：" + err.Error()), nil
	}

	actions := currentDesktopUIActions()
	uiReloaded := false
	if actions != nil {
		if payload, marshalErr := json.Marshal(map[string]string{"tabId": tabID}); marshalErr == nil {
			// Best effort: the editor clears this key when it loads the page; it
			// is not needed if the reload below already removes the row.
			_ = actions.SetDesktopStorage(c.Request.Context(), map[string]string{repeaterDeleteStorageKey: string(payload)})
		}
	}
	if err := repeaterDeleteTab(tabID); err != nil {
		return repeaterToolError("删除请求转发标签页失败：" + err.Error()), nil
	}
	if actions != nil {
		if err := actions.OpenDesktopRoute(c.Request.Context(), repeaterRoute); err == nil {
			uiReloaded = true
			timer := time.NewTimer(repeaterDeleteSettle)
			select {
			case <-c.Request.Context().Done():
			case <-timer.C:
			}
			timer.Stop()
		}
	}

	text := fmt.Sprintf("已删除请求转发标签页「%s」（tabId=%s）。", tab.Name, tab.ID)
	if uiReloaded {
		text += "桌面端已刷新请求转发页面，标签页已从界面移除。"
	} else if actions != nil {
		text += "桌面端刷新失败；界面可能仍显示旧标签页，下一次进入请求转发时会消失。"
	}
	return mcpToolCallResult{
		Content: []mcpToolContent{{Type: "text", Text: text}},
		StructuredContent: map[string]any{
			"deleted":    true,
			"tabId":      tab.ID,
			"name":       tab.Name,
			"uiReloaded": uiReloaded,
		},
	}, nil
}

func waitForContext(ctx context.Context, duration time.Duration) {
	if duration <= 0 {
		return
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// desktopOpenRequestPending reports whether the one-shot open key is still in
// the persisted desktop storage. Its absence means the Repeater editor loaded
// the page and consumed it.
func desktopOpenRequestPending() bool {
	raw, err := database.GetSetting(database.SettingKeyDesktopConfig)
	if err != nil || raw == nil {
		return false
	}
	items, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	_, pending := items[repeaterOpenStorageKey]
	return pending
}

func repeaterToolError(text string) mcpToolCallResult {
	return mcpToolCallResult{
		Content:           []mcpToolContent{{Type: "text", Text: text}},
		StructuredContent: map[string]any{"saved": false, "opened": false},
		IsError:           true,
	}
}

func repeaterTabName(requested, method, target string) string {
	name := strings.TrimSpace(requested)
	if name == "" {
		name = fmt.Sprintf("MCP POC %s", time.Now().Format("2006-01-02 15:04:05"))
	}
	name = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, name)
	if utf8.RuneCountInString(name) > 80 {
		name = string([]rune(name)[:80])
	}
	if name == "" {
		name = method + " " + target
	}
	return name
}

func limitRepeaterText(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut] + "\n…[truncated by Hephaestus MCP]", true
}

func saveRepeaterTabToDB(tab *database.RepeaterTab) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	var minOrder int
	if err := db.Model(&database.RepeaterTab{}).Select("COALESCE(MIN(sort_order), 0)").Scan(&minOrder).Error; err != nil {
		return err
	}
	tab.SortOrder = minOrder - 1
	now := time.Now()
	tab.CreatedAt = now
	tab.UpdatedAt = now
	return db.Create(tab).Error
}

func loadRepeaterTabFromDB(id string) (database.RepeaterTab, error) {
	db, err := database.GetDB()
	if err != nil {
		return database.RepeaterTab{}, err
	}
	var tab database.RepeaterTab
	if err := db.Where("id = ?", strings.TrimSpace(id)).First(&tab).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return database.RepeaterTab{}, fmt.Errorf("%w: 标签页 %s 不存在或已被删除", errRepeaterTabMissing, id)
		}
		return database.RepeaterTab{}, err
	}
	return tab, nil
}

func deleteRepeaterTabFromDB(id string) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	return db.Where("id = ?", strings.TrimSpace(id)).Delete(&database.RepeaterTab{}).Error
}
