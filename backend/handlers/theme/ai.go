package theme

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"hephaestus-backend/pkg/ai"
	"hephaestus-backend/pkg/config"

	"github.com/gin-gonic/gin"
)

const (
	codeAIUnavailable   = "ai_unavailable"
	codeAIRequestFailed = "ai_request_failed"
	codeAIInvalidReply  = "invalid_ai_response"
	codeInvalidPrompt   = "invalid_prompt"

	maxPromptRunes = 2000
)

type generateThemeRequest struct {
	Prompt string `json:"prompt"`
	Base   string `json:"base,omitempty"`
}

type generatedTheme struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Definition  Definition `json:"definition"`
}

type aiThemeEnvelope struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Base        string            `json:"base"`
	Radius      string            `json:"radius"`
	Definition  *Definition       `json:"definition"`
	Colors      *Colors           `json:"colors"`
	Overrides   map[string]string `json:"overrides"`
}

// GenerateCustomThemeHandler turns a natural-language style description into
// a validated theme definition using the configured AI provider.
func GenerateCustomThemeHandler(c *gin.Context) {
	var req generateThemeRequest
	if err := decodeJSON(c, &req); err != nil {
		writeThemeError(c, http.StatusBadRequest, codeInvalidPayload, "无效的请求参数")
		return
	}

	prompt := strings.TrimSpace(req.Prompt)
	if utf8.RuneCountInString(prompt) < 3 {
		writeThemeError(c, http.StatusBadRequest, codeInvalidPrompt, "请描述你想要的风格")
		return
	}
	if utf8.RuneCountInString(prompt) > maxPromptRunes {
		writeThemeError(c, http.StatusBadRequest, codeInvalidPrompt, "风格描述过长")
		return
	}

	base := strings.ToLower(strings.TrimSpace(req.Base))
	if base != "" && base != "dark" && base != "light" {
		writeThemeError(c, http.StatusBadRequest, codeInvalidPrompt, "基调必须是 dark 或 light")
		return
	}

	aiConfig, err := config.GetAIConfig()
	if err != nil {
		log.Printf("theme ai config load failed: %v", err)
		writeThemeError(c, http.StatusServiceUnavailable, codeAIUnavailable, "AI 配置不可用")
		return
	}
	aiConfig = config.ResolveAIRequestConfig(aiConfig)
	if !aiConfig.Enabled || strings.TrimSpace(aiConfig.Key) == "" || strings.TrimSpace(aiConfig.Model) == "" {
		writeThemeError(c, http.StatusServiceUnavailable, codeAIUnavailable, "AI 未启用或未配置模型")
		return
	}

	content, err := ai.GenerateText(
		c.Request.Context(),
		aiConfig,
		buildThemeGenerationMessages(prompt, base),
		2400,
		0.65,
	)
	if err != nil {
		log.Printf("theme ai generation failed: %v", err)
		writeThemeError(c, http.StatusBadGateway, codeAIRequestFailed, "AI 生成失败")
		return
	}

	result, err := parseAIThemeResponse(content, base)
	if err != nil {
		log.Printf("theme ai response invalid: %v", err)
		writeThemeError(c, http.StatusBadGateway, codeAIInvalidReply, "AI 返回的配色格式无效")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func buildThemeGenerationMessages(prompt string, base string) []ai.ChatMessage {
	system := `你是 Hephaestus 安全平台的主题设计师。根据用户的中文或英文描述，生成一套可直接使用的界面配色。
只输出一个 JSON 对象，不要 Markdown 代码块，不要解释、前缀、后缀或注释。
JSON 结构必须严格为：
{
  "name": "简短主题名",
  "description": "一句话说明风格",
  "definition": {
    "schemaVersion": 1,
    "base": "dark 或 light",
    "radius": "0.5rem",
    "colors": {
      "background": "#RRGGBB",
      "foreground": "#RRGGBB",
      "card": "#RRGGBB",
      "primary": "#RRGGBB",
      "primaryForeground": "#RRGGBB",
      "muted": "#RRGGBB",
      "mutedForeground": "#RRGGBB",
      "border": "#RRGGBB",
      "sidebar": "#RRGGBB",
      "sidebarForeground": "#RRGGBB",
      "success": "#RRGGBB",
      "warning": "#RRGGBB",
      "info": "#RRGGBB",
      "destructive": "#RRGGBB",
      "destructiveForeground": "#RRGGBB",
      "ring": "#RRGGBB"
    },
      "overrides": {
        "--card": "#RRGGBB",
        "--muted": "#RRGGBB",
        "--border": "#RRGGBB",
        "--sidebar": "#RRGGBB",
        "--sidebar-accent": "#RRGGBB",
        "--input-panel-bg": "#RRGGBB",
        "--tone-purple": "#RRGGBB",
        "--tone-orange": "#RRGGBB",
        "--tone-slate": "#RRGGBB"
      }
    }
  }
}
规则：
- 所有颜色必须是 #RRGGBB 十六进制，不要使用 rgba/hsl/oklch/CSS 变量。
- success/warning/info/destructive 是状态色，需与背景和卡片表面协调。
- 必须同时生成 8 到 24 个 overrides，优先覆盖表面层级、侧边栏、输入面板和辅助状态色阶；只能使用这些 Token：--background、--foreground、--card、--card-foreground、--popover、--popover-foreground、--muted、--muted-foreground、--secondary、--secondary-foreground、--accent、--accent-foreground、--border、--input、--input-panel-bg、--input-panel-bg-hover、--input-panel-border、--sidebar、--sidebar-foreground、--sidebar-primary、--sidebar-primary-foreground、--sidebar-accent、--sidebar-accent-foreground、--sidebar-border、--sidebar-ring、--sidebar-hover、--tone-purple、--tone-purple-soft、--tone-purple-border、--tone-orange、--tone-orange-soft、--tone-orange-border、--tone-slate、--tone-slate-soft、--tone-slate-border。
- 背景、卡片、弱化表面、侧边栏至少形成 3 个可感知的明度或色相层级，不要把它们简单复制成同一个颜色。
- 主题需要有明确的设计特征：可使用互补色、冷暖对比、复古低饱和、金属感、纸张感或荧光强调，但不要只生成单一色相的纯色主题。
- 避免直接复用常见默认蓝、默认紫或默认灰作为唯一视觉特征；主色之外至少提供一个辅助色彩关系。
- overrides 只用于细化视觉层级，不要覆盖语义状态色为难以辨认的颜色。
- 正文与背景、主色与主色文字必须有足够对比度。
- 基调由描述决定；没有明确说明时选择 dark。
- radius 只能使用 rem 或 px，范围 0 到 1.5rem。
- 不要输出 JSON 之外的任何字符。`

	userPrompt := "风格描述：" + prompt
	if base != "" {
		userPrompt += "\n优先基调：" + base + "（如果描述没有更强要求，请使用该基调）"
	}

	return []ai.ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: userPrompt},
	}
}

func parseAIThemeResponse(content string, fallbackBase string) (generatedTheme, error) {
	raw, err := extractJSONObject(content)
	if err != nil {
		return generatedTheme{}, err
	}

	var envelope aiThemeEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return generatedTheme{}, err
	}

	definition := Definition{}
	switch {
	case envelope.Definition != nil && envelope.Definition.Colors != (Colors{}):
		definition = *envelope.Definition
	case envelope.Colors != nil:
		definition = Definition{
			SchemaVersion: DefinitionSchemaVersion,
			Base:          envelope.Base,
			Radius:        envelope.Radius,
			Colors:        *envelope.Colors,
			Overrides:     envelope.Overrides,
		}
	default:
		return generatedTheme{}, errInvalidAITheme
	}
	if len(definition.Overrides) == 0 && len(envelope.Overrides) > 0 {
		definition.Overrides = envelope.Overrides
	}

	definition.Base = strings.ToLower(strings.TrimSpace(definition.Base))
	if definition.Base == "" {
		definition.Base = strings.ToLower(strings.TrimSpace(envelope.Base))
	}
	if definition.Base != "dark" && definition.Base != "light" {
		definition.Base = strings.ToLower(strings.TrimSpace(fallbackBase))
		if definition.Base != "dark" && definition.Base != "light" {
			definition.Base = "dark"
		}
	}
	if strings.TrimSpace(definition.Radius) == "" {
		definition.Radius = strings.TrimSpace(envelope.Radius)
	}

	normalized, err := NormalizeDefinition(definition)
	if err != nil {
		return generatedTheme{}, err
	}

	name := strings.TrimSpace(envelope.Name)
	name = strings.Trim(name, `"'“”`)
	if name == "" {
		name = "AI 主题"
	}
	if utf8.RuneCountInString(name) > MaxNameRunes {
		name = string([]rune(name)[:MaxNameRunes])
	}
	normalizedName, _, err := NormalizeName(name)
	if err != nil {
		normalizedName = "AI 主题"
	}

	description := strings.TrimSpace(envelope.Description)
	if utf8.RuneCountInString(description) > 300 {
		description = string([]rune(description)[:300])
	}

	return generatedTheme{
		Name:        normalizedName,
		Description: description,
		Definition:  normalized,
	}, nil
}

func extractJSONObject(content string) (string, error) {
	cleaned := strings.TrimSpace(content)
	if strings.HasPrefix(cleaned, "```") {
		if newline := strings.IndexByte(cleaned, '\n'); newline >= 0 {
			cleaned = cleaned[newline+1:]
		}
		cleaned = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(cleaned), "```"))
	}

	start := strings.IndexByte(cleaned, '{')
	end := strings.LastIndexByte(cleaned, '}')
	if start < 0 || end <= start {
		return "", errInvalidAITheme
	}
	return cleaned[start : end+1], nil
}

var errInvalidAITheme = &themeParseError{"AI 返回内容不是有效的主题 JSON"}

type themeParseError struct {
	message string
}

func (e *themeParseError) Error() string {
	return e.message
}
