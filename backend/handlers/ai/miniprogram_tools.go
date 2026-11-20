package ai

import (
	"errors"
	"strings"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/miniprogramtool"
)

type miniProgramListArgs struct {
	AppletPath string `json:"applet_path"`
}

type miniProgramDecompileArgs struct {
	AppID      string `json:"app_id"`
	Version    string `json:"version"`
	AppletPath string `json:"applet_path"`
}

func pentestMiniProgramToolSchemas() []pentest.AgentToolSchema {
	return []pentest.AgentToolSchema{
		{
			Type: "function",
			Function: pentest.AgentToolFunction{
				Name:        "miniprogram_list_programs",
				Description: "列出本机微信小程序缓存中的小程序、版本和反编译状态，用于在反编译前确定 app_id 与 version。仅在 Hephaestus 桌面客户端可用。",
				Parameters: pentestNativeObjectSchema(map[string]any{
					"applet_path": map[string]any{"type": "string", "description": "可选，微信小程序缓存目录；留空使用设置中的路径。"},
				}),
			},
		},
		{
			Type: "function",
			Function: pentest.AgentToolFunction{
				Name:        "miniprogram_decompile",
				Description: "反编译指定微信小程序版本，导出代码并按规则提取敏感信息（云凭证、API Key、URL、账号信息等）。请先调用 miniprogram_list_programs 获取 app_id 和 version。仅在 Hephaestus 桌面客户端可用。",
				Parameters: pentestNativeObjectSchema(map[string]any{
					"app_id":      map[string]any{"type": "string", "description": "目标小程序的 AppID，例如 wx1234567890abcdef。"},
					"version":     map[string]any{"type": "string", "description": "目标版本号，来自 miniprogram_list_programs 的返回结果；root 表示根目录版本。"},
					"applet_path": map[string]any{"type": "string", "description": "可选，微信小程序缓存目录；留空使用设置中的路径。"},
				}, "app_id", "version"),
			},
		},
	}
}

func ensureMiniProgramDesktop() error {
	if config.GetRuntimeMode() != config.RuntimeModeDesktop {
		return errors.New("小程序反编译仅在 Hephaestus 桌面客户端中可用")
	}
	return nil
}

func runMiniProgramList(args map[string]any) (any, bool, error) {
	if err := ensureMiniProgramDesktop(); err != nil {
		return nil, true, err
	}
	var parsed miniProgramListArgs
	if err := decodePentestToolArgs(args, &parsed); err != nil {
		return nil, true, err
	}
	result, err := miniprogramtool.ScanPrograms(strings.TrimSpace(parsed.AppletPath))
	return result, true, err
}

func runMiniProgramDecompile(args map[string]any) (any, bool, error) {
	if err := ensureMiniProgramDesktop(); err != nil {
		return nil, true, err
	}
	var parsed miniProgramDecompileArgs
	if err := decodePentestToolArgs(args, &parsed); err != nil {
		return nil, true, err
	}
	result, err := miniprogramtool.Decompile(
		strings.TrimSpace(parsed.AppID),
		strings.TrimSpace(parsed.Version),
		strings.TrimSpace(parsed.AppletPath),
	)
	return result, true, err
}
