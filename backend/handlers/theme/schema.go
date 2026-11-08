// Package theme 管理自定义界面主题的持久化与校验。
package theme

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	// DefinitionSchemaVersion 是当前支持的主题定义版本号。
	DefinitionSchemaVersion = 1
	// MaxNameRunes 限制主题名称长度。
	MaxNameRunes = 60
	// MaxDefinitionBytes 限制单条主题定义大小。
	MaxDefinitionBytes = 32 * 1024
	// MaxImportThemes 限制一次导入的主题数量。
	MaxImportThemes = 50
	// MaxRequestBodyBytes 限制请求体大小。
	MaxRequestBodyBytes = 512 * 1024
)

// Definition 是自定义主题的完整定义。
// 颜色只保存核心 token，其余 token 由前端按 base/核心色派生。
type Definition struct {
	SchemaVersion int    `json:"schemaVersion"`
	Base          string `json:"base"`
	Radius        string `json:"radius"`
	Colors        Colors `json:"colors"`
	// Overrides 允许按 CSS 变量名覆盖派生颜色；值为十六进制颜色。
	Overrides map[string]string `json:"overrides,omitempty"`
}

// Colors 是用户可以编辑的核心颜色集合。
type Colors struct {
	Background        string `json:"background"`
	Foreground        string `json:"foreground"`
	Card              string `json:"card"`
	Primary           string `json:"primary"`
	PrimaryForeground string `json:"primaryForeground"`
	Muted             string `json:"muted"`
	MutedForeground   string `json:"mutedForeground"`
	Border            string `json:"border"`
	Sidebar           string `json:"sidebar"`
	SidebarForeground string `json:"sidebarForeground"`
	Success           string `json:"success"`
	Warning           string `json:"warning"`
	Info              string `json:"info"`
	Destructive       string `json:"destructive"`
	// DestructiveForeground 是危险色上的文字颜色。
	DestructiveForeground string `json:"destructiveForeground"`
	Ring                  string `json:"ring"`
}

var (
	hexColorPattern   = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	radiusPattern     = regexp.MustCompile(`^(?:0|[0-9]+(?:\.[0-9]+)?(?:px|rem|em))$`)
	themeTokenPattern = regexp.MustCompile(`^--[a-z0-9-]{1,62}$`)
)

// NormalizeName 清理主题名称并生成大小写/空白不敏感的唯一键。
func NormalizeName(raw string) (name string, key string, err error) {
	cleaned := strings.Join(strings.Fields(raw), " ")
	if cleaned == "" {
		return "", "", fmt.Errorf("主题名称不能为空")
	}
	if utf8.RuneCountInString(cleaned) > MaxNameRunes {
		return "", "", fmt.Errorf("主题名称不能超过 %d 个字符", MaxNameRunes)
	}
	return cleaned, strings.ToLower(cleaned), nil
}

// NormalizeDefinition 校验并规范化主题定义。
func NormalizeDefinition(def Definition) (Definition, error) {
	if def.SchemaVersion == 0 {
		def.SchemaVersion = DefinitionSchemaVersion
	}
	if def.SchemaVersion != DefinitionSchemaVersion {
		return def, fmt.Errorf("不支持的主题定义版本: %d", def.SchemaVersion)
	}

	def.Base = strings.ToLower(strings.TrimSpace(def.Base))
	if def.Base != "dark" && def.Base != "light" {
		return def, fmt.Errorf("主题基调必须是 dark 或 light")
	}

	def.Radius = strings.TrimSpace(def.Radius)
	if def.Radius == "" {
		def.Radius = "0.5rem"
	}
	if len(def.Radius) > 24 || !radiusPattern.MatchString(def.Radius) {
		return def, fmt.Errorf("圆角必须是 0 或合法的 px/rem/em 数值")
	}

	if def.Colors.Success == "" {
		if def.Base == "dark" {
			def.Colors.Success = "#34d399"
		} else {
			def.Colors.Success = "#18794e"
		}
	}
	if def.Colors.Warning == "" {
		if def.Base == "dark" {
			def.Colors.Warning = "#fbbf24"
		} else {
			def.Colors.Warning = "#946800"
		}
	}
	if def.Colors.Info == "" {
		if def.Base == "dark" {
			def.Colors.Info = "#60a5fa"
		} else {
			def.Colors.Info = "#0b6bcb"
		}
	}

	colorFields := []struct {
		label string
		value *string
	}{
		{"background", &def.Colors.Background},
		{"foreground", &def.Colors.Foreground},
		{"card", &def.Colors.Card},
		{"primary", &def.Colors.Primary},
		{"primaryForeground", &def.Colors.PrimaryForeground},
		{"muted", &def.Colors.Muted},
		{"mutedForeground", &def.Colors.MutedForeground},
		{"border", &def.Colors.Border},
		{"sidebar", &def.Colors.Sidebar},
		{"sidebarForeground", &def.Colors.SidebarForeground},
		{"success", &def.Colors.Success},
		{"warning", &def.Colors.Warning},
		{"info", &def.Colors.Info},
		{"destructive", &def.Colors.Destructive},
		{"destructiveForeground", &def.Colors.DestructiveForeground},
		{"ring", &def.Colors.Ring},
	}
	for _, field := range colorFields {
		*field.value = strings.ToLower(strings.TrimSpace(*field.value))
		if !hexColorPattern.MatchString(*field.value) {
			return def, fmt.Errorf("%s 必须是 #RGB/#RRGGBB/#RRGGBBAA 形式的颜色", field.label)
		}
	}

	if len(def.Overrides) > 160 {
		return def, fmt.Errorf("主题变量覆盖数量过多")
	}
	if len(def.Overrides) > 0 {
		normalized := make(map[string]string, len(def.Overrides))
		for key, value := range def.Overrides {
			token := strings.TrimSpace(key)
			if !themeTokenPattern.MatchString(token) || token == "--radius" {
				return def, fmt.Errorf("无效的主题变量 %s", key)
			}
			color := strings.ToLower(strings.TrimSpace(value))
			if !hexColorPattern.MatchString(color) {
				return def, fmt.Errorf("%s 必须是十六进制颜色", key)
			}
			normalized[token] = color
		}
		def.Overrides = normalized
	}

	return def, nil
}

// EncodeDefinition 将校验后的定义序列化为数据库存储的 JSON。
func EncodeDefinition(def Definition) (string, error) {
	payload, err := json.Marshal(def)
	if err != nil {
		return "", fmt.Errorf("序列化主题定义失败: %w", err)
	}
	if len(payload) > MaxDefinitionBytes {
		return "", fmt.Errorf("主题定义不能超过 %d 字节", MaxDefinitionBytes)
	}
	return string(payload), nil
}

// DecodeDefinition 解析数据库中的主题定义。
func DecodeDefinition(raw string) (Definition, error) {
	var def Definition
	if err := json.Unmarshal([]byte(raw), &def); err != nil {
		return def, fmt.Errorf("解析主题定义失败: %w", err)
	}
	return NormalizeDefinition(def)
}
