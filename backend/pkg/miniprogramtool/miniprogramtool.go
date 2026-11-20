// Package miniprogramtool exposes the mini-program decompile feature to the AI
// and MCP tool layers without depending on the HTTP handlers.
package miniprogramtool

import (
	"encoding/json"
	"os"
	"path/filepath"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/miniprogram"
)

type Settings struct {
	AppletPath string                  `json:"appletPath"`
	RuleGroups []miniprogram.RuleGroup `json:"ruleGroups"`
	Rules      []string                `json:"rules,omitempty"`
}

type ProgramList struct {
	AppletPath string                    `json:"appletPath"`
	OutputRoot string                    `json:"outputRoot"`
	Items      []miniprogram.MiniProgram `json:"items"`
}

type DecompileResponse struct {
	miniprogram.DecompileResult
	OutputRoot string `json:"outputRoot"`
}

// OutputRoot returns the directory that stores decompiled mini-program output.
func OutputRoot() string {
	outputRoot := filepath.Join(config.GetDataDir(), "mini-program-decompile")
	_ = os.MkdirAll(outputRoot, 0755)
	return outputRoot
}

// LoadSettings reads the saved mini-program decompile settings, falling back to
// the built-in defaults when the user has not configured them yet.
func LoadSettings() (Settings, error) {
	settings := Settings{
		AppletPath: miniprogram.DefaultAppletPath(),
		RuleGroups: miniprogram.CloneRuleGroups(miniprogram.DefaultRuleGroups),
	}

	raw, err := database.GetSetting(database.SettingKeyMiniProgramDecompile)
	if err != nil || raw == nil {
		return settings, err
	}

	payload, err := json.Marshal(raw)
	if err != nil {
		return settings, err
	}
	var stored Settings
	if err := json.Unmarshal(payload, &stored); err != nil {
		return settings, err
	}
	settings.AppletPath = stored.AppletPath
	settings.RuleGroups = stored.RuleGroups
	settings.Rules = stored.Rules
	settings.AppletPath = miniprogram.ResolveAppletPath(settings.AppletPath)
	if settings.AppletPath == "" {
		settings.AppletPath = miniprogram.DefaultAppletPath()
	}
	if len(settings.RuleGroups) == 0 {
		settings.RuleGroups = miniprogram.RuleGroupsFromLegacyRules(settings.Rules)
	}
	settings.RuleGroups = miniprogram.NormalizeRuleGroups(settings.RuleGroups)
	settings.Rules = miniprogram.FlattenEnabledRulePatterns(settings.RuleGroups)
	return settings, nil
}

func buildService(settings Settings) *miniprogram.Service {
	return miniprogram.NewServiceWithRuleGroups(settings.AppletPath, OutputRoot(), settings.RuleGroups)
}

// ScanPrograms lists the mini programs found in the local WeChat cache. An
// optional appletPath overrides the saved settings for this call.
func ScanPrograms(appletPath string) (ProgramList, error) {
	settings, err := LoadSettings()
	if err != nil {
		return ProgramList{}, err
	}
	if appletPath != "" {
		settings.AppletPath = miniprogram.ResolveAppletPath(appletPath)
	}
	items, err := buildService(settings).ScanPrograms()
	if err != nil {
		return ProgramList{}, err
	}
	return ProgramList{AppletPath: settings.AppletPath, OutputRoot: OutputRoot(), Items: items}, nil
}

// Decompile decompiles one mini-program version and returns the extracted
// inventory plus matched sensitive values.
func Decompile(appID, version, appletPath string) (DecompileResponse, error) {
	settings, err := LoadSettings()
	if err != nil {
		return DecompileResponse{}, err
	}
	if appletPath != "" {
		settings.AppletPath = miniprogram.ResolveAppletPath(appletPath)
	}
	result, err := buildService(settings).Decompile(appID, version)
	if err != nil {
		return DecompileResponse{}, err
	}
	return DecompileResponse{DecompileResult: *result, OutputRoot: OutputRoot()}, nil
}
