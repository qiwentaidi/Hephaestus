package mcp

import (
	"encoding/json"
	"errors"
	"strings"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/miniprogramtool"
)

const (
	miniProgramListToolName      = "miniprogram_list_programs"
	miniProgramDecompileToolName = "miniprogram_decompile"
)

type miniProgramListArgs struct {
	AppletPath string `json:"applet_path"`
}

type miniProgramDecompileArgs struct {
	AppID      string `json:"app_id"`
	Version    string `json:"version"`
	AppletPath string `json:"applet_path"`
}

func isMiniProgramTool(name string) bool {
	return name == miniProgramListToolName || name == miniProgramDecompileToolName
}

func ensureMiniProgramDesktop() error {
	if config.GetRuntimeMode() != config.RuntimeModeDesktop {
		return errors.New("miniprogram decompile is only available in the Hephaestus desktop client")
	}
	return nil
}

func mcpCallMiniProgramList(rawArgs json.RawMessage) (mcpToolCallResult, error) {
	if err := ensureMiniProgramDesktop(); err != nil {
		return mcpToolCallResult{}, err
	}
	var args miniProgramListArgs
	if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
		return mcpToolCallResult{}, err
	}
	result, err := miniprogramtool.ScanPrograms(strings.TrimSpace(args.AppletPath))
	if err != nil {
		return mcpToolCallResult{}, err
	}
	return newMCPToolResult(result), nil
}

func mcpCallMiniProgramDecompile(rawArgs json.RawMessage) (mcpToolCallResult, error) {
	if err := ensureMiniProgramDesktop(); err != nil {
		return mcpToolCallResult{}, err
	}
	var args miniProgramDecompileArgs
	if err := decodeMCPToolArgs(rawArgs, &args); err != nil {
		return mcpToolCallResult{}, err
	}
	result, err := miniprogramtool.Decompile(
		strings.TrimSpace(args.AppID),
		strings.TrimSpace(args.Version),
		strings.TrimSpace(args.AppletPath),
	)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	return newMCPToolResult(result), nil
}
