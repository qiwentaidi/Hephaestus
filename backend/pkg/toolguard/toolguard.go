// Package toolguard applies configurable blocking rules before tool execution.
package toolguard

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"
	"hephaestus-backend/pkg/toolruntime"
)

const (
	MaxRules          = 100
	MaxIDLength       = 128
	MaxNameLength     = 200
	MaxToolNameLength = 255
	MaxPatternLength  = 4096
	MaxMessageLength  = 4096

	defaultMessage                = "识别到 {match}，工具调用已被安全规则「{rule}」拦截。"
	defaultDangerousDeletePattern = `(?i)(?:^|[^a-z0-9_])(?P<match>rm\s+(?:-[a-z]*r[a-z]*f[a-z]*|-[a-z]*f[a-z]*r[a-z]*|-[a-z]*r[a-z]*\s+-[a-z]*f[a-z]*|-[a-z]*f[a-z]*\s+-[a-z]*r[a-z]*|--recursive\s+--force|--force\s+--recursive))(?:\s|$)`
	legacyDangerousDeleteRuleID   = "dangerous-recursive-delete"
)

type Config struct {
	Enabled       bool   `json:"enabled" yaml:"enabled"`
	Rules         []Rule `json:"rules" yaml:"rules"`
	PresetVersion int    `json:"presetVersion,omitempty" yaml:"presetVersion,omitempty"`
}

type Rule struct {
	ID       string   `json:"id" yaml:"id"`
	Name     string   `json:"name" yaml:"name"`
	Enabled  bool     `json:"enabled" yaml:"enabled"`
	Origin   string   `json:"origin,omitempty" yaml:"origin,omitempty"`
	Target   string   `json:"target,omitempty" yaml:"target,omitempty"`
	Priority int      `json:"priority,omitempty" yaml:"priority,omitempty"`
	Tools    []string `json:"tools,omitempty" yaml:"tools,omitempty"`
	Pattern  string   `json:"pattern" yaml:"pattern"`
	Message  string   `json:"message" yaml:"message"`
}

type Match struct {
	RuleID      string `json:"ruleId"`
	RuleName    string `json:"ruleName"`
	MatchedText string `json:"matchedText"`
	Message     string `json:"message"`
}

type compiledRule struct {
	rule       Rule
	pattern    *regexp.Regexp
	matchGroup int
	tools      map[string]struct{}
}

type Policy struct {
	config Config
	rules  []compiledRule
}

type Manager struct {
	policy atomic.Pointer[Policy]
}

type BlockedError struct {
	Match Match
}

func (e *BlockedError) Error() string {
	return FormatBlockedMessage(&e.Match)
}

func DefaultConfig() Config {
	return Config{Enabled: true, Rules: PresetRules(), PresetVersion: CurrentPresetVersion}
}

// EnsureRuleIDs assigns opaque IDs to new rules and migrates the legacy
// built-in identifier. Existing non-legacy IDs stay unchanged so audit links
// remain stable across upgrades.
func EnsureRuleIDs(config Config) Config {
	config = cloneConfig(config)
	seen := make(map[string]struct{}, len(config.Rules))
	for _, rule := range config.Rules {
		id := strings.TrimSpace(rule.ID)
		if id != "" && id != legacyDangerousDeleteRuleID {
			seen[id] = struct{}{}
		}
	}
	for i := range config.Rules {
		id := strings.TrimSpace(config.Rules[i].ID)
		if id != "" && id != legacyDangerousDeleteRuleID {
			continue
		}
		for {
			id = uuid.NewString()
			if _, exists := seen[id]; !exists {
				break
			}
		}
		config.Rules[i].ID = id
		seen[id] = struct{}{}
	}
	return config
}

func Compile(config Config) (*Policy, error) {
	if len(config.Rules) > MaxRules {
		return nil, fmt.Errorf("调用拦截规则最多允许 %d 条", MaxRules)
	}
	policy := &Policy{config: cloneConfig(config)}
	ids := make(map[string]struct{}, len(config.Rules))
	for i, rule := range policy.config.Rules {
		prefix := fmt.Sprintf("调用拦截规则 %d", i+1)
		if strings.TrimSpace(rule.ID) == "" || len(rule.ID) > MaxIDLength {
			return nil, fmt.Errorf("%s: id 不能为空且不能超过 %d 字节", prefix, MaxIDLength)
		}
		if rule.ID != strings.TrimSpace(rule.ID) {
			return nil, fmt.Errorf("%s: id 不能包含首尾空格", prefix)
		}
		if _, exists := ids[rule.ID]; exists {
			return nil, fmt.Errorf("%s: id %q 重复", prefix, rule.ID)
		}
		ids[rule.ID] = struct{}{}
		if strings.TrimSpace(rule.Name) == "" || len(rule.Name) > MaxNameLength {
			return nil, fmt.Errorf("%s: 名称不能为空且不能超过 %d 字节", prefix, MaxNameLength)
		}
		if strings.TrimSpace(rule.Pattern) == "" || len(rule.Pattern) > MaxPatternLength {
			return nil, fmt.Errorf("%s: 正则不能为空且不能超过 %d 字节", prefix, MaxPatternLength)
		}
		if len(rule.Message) > MaxMessageLength {
			return nil, fmt.Errorf("%s: 提醒不能超过 %d 字节", prefix, MaxMessageLength)
		}
		if rule.Target != "" && rule.Target != "tool_input" && rule.Target != "executable_input" && rule.Target != "http_method" {
			return nil, fmt.Errorf("%s: 无效的匹配目标 %q", prefix, rule.Target)
		}
		if rule.Priority < -1000 || rule.Priority > 1000 {
			return nil, fmt.Errorf("%s: 优先级须在 -1000 到 1000 之间", prefix)
		}
		pattern, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return nil, fmt.Errorf("%s (%s): 正则无效: %w", prefix, rule.ID, err)
		}
		compiled := compiledRule{rule: rule, pattern: pattern, matchGroup: pattern.SubexpIndex("match")}
		if len(rule.Tools) > 0 {
			compiled.tools = make(map[string]struct{}, len(rule.Tools))
			seenTools := make(map[string]struct{}, len(rule.Tools))
			tools, _ := toolruntime.MigrateDisabled(rule.Tools)
			tools = append(tools, rule.Tools...) // Historical aliases still match imported rules.
			for _, rawTool := range tools {
				tool := strings.TrimSpace(rawTool)
				if tool == "" || len(tool) > MaxToolNameLength {
					return nil, fmt.Errorf("%s: 工具名不能为空且不能超过 %d 字节", prefix, MaxToolNameLength)
				}
				if _, exists := seenTools[tool]; exists {
					return nil, fmt.Errorf("%s: 工具名 %q 重复", prefix, tool)
				}
				seenTools[tool] = struct{}{}
				compiled.tools[tool] = struct{}{}
			}
		}
		if rule.Enabled {
			policy.rules = append(policy.rules, compiled)
		}
	}
	sort.SliceStable(policy.rules, func(i, j int) bool {
		return policy.rules[i].rule.Priority > policy.rules[j].rule.Priority
	})
	return policy, nil
}

func (p *Policy) Check(toolName string, arguments any) *Match {
	return p.check(toolName, arguments, toolName)
}

// CheckToolCall applies the runtime interception scope: command, Python and
// shell execution tools plus the native HTTP test tool are inspected. MCP
// catalog tools, attack-surface task creation and direct MCP HTTP tools keep
// their own authorization and scope checks instead of being blocked by
// command-oriented rules.
func (p *Policy) CheckToolCall(toolName string, arguments any) *Match {
	if !IsInterceptionTool(strings.TrimSpace(toolName)) {
		return nil
	}
	return p.Check(toolName, arguments)
}

// CheckRequest preserves rule tool scopes while inspecting an actual network
// request using HTTP fields, regardless of which executor generated it.
func (p *Policy) CheckRequest(toolName string, arguments any) *Match {
	return p.check(toolName, arguments, "http-framework-test")
}

func (p *Policy) check(toolName string, arguments any, inputTool string) *Match {
	if p == nil || !p.config.Enabled || len(p.rules) == 0 {
		return nil
	}
	toolName = strings.TrimSpace(toolName)
	var allCandidates, executableCandidates, methodCandidates []string
	var allReady, executableReady, methodReady bool
	for _, rule := range p.rules {
		if len(rule.tools) > 0 {
			if _, allowed := rule.tools[toolName]; !allowed {
				continue
			}
		}
		var candidates []string
		switch rule.rule.Target {
		case "executable_input":
			if !executableReady {
				executableCandidates = executableInputCandidates(inputTool, arguments)
				executableReady = true
			}
			candidates = executableCandidates
		case "http_method":
			if !methodReady {
				methodCandidates = httpMethodCandidates(inputTool, arguments)
				methodReady = true
			}
			candidates = methodCandidates
		default:
			if !allReady {
				allCandidates = candidateTexts(toolName, arguments)
				allReady = true
			}
			candidates = allCandidates
		}
		for _, candidate := range candidates {
			indices := rule.pattern.FindStringSubmatchIndex(candidate)
			if indices == nil {
				continue
			}
			start, end := indices[0], indices[1]
			if group := rule.matchGroup; group > 0 && indices[group*2] >= 0 {
				start, end = indices[group*2], indices[group*2+1]
			}
			matched := candidate[start:end]
			message := strings.TrimSpace(rule.rule.Message)
			if message == "" {
				message = defaultMessage
			}
			message = strings.NewReplacer("{match}", matched, "{tool}", toolName, "{rule}", rule.rule.Name).Replace(message)
			return &Match{RuleID: rule.rule.ID, RuleName: rule.rule.Name, MatchedText: matched, Message: message}
		}
	}
	return nil
}

func NewManager(config Config) (*Manager, error) {
	manager := &Manager{}
	if err := manager.Update(config); err != nil {
		return nil, err
	}
	return manager, nil
}

func (m *Manager) Update(config Config) error {
	policy, err := Compile(config)
	if err != nil {
		return err
	}
	m.policy.Store(policy)
	return nil
}

func (m *Manager) Config() Config {
	if m == nil {
		return Config{}
	}
	if policy := m.policy.Load(); policy != nil {
		return cloneConfig(policy.config)
	}
	return Config{}
}

func (m *Manager) Check(toolName string, arguments any) *Match {
	if m == nil {
		return nil
	}
	policy := m.policy.Load()
	if policy == nil {
		return nil
	}
	return policy.Check(toolName, arguments)
}

func (m *Manager) CheckToolCall(toolName string, arguments any) *Match {
	if m == nil {
		return nil
	}
	policy := m.policy.Load()
	if policy == nil {
		return nil
	}
	return policy.CheckToolCall(toolName, arguments)
}

func FormatBlockedMessage(match *Match) string {
	if match == nil {
		return "工具调用已被安全规则拦截"
	}
	message := "工具调用已被安全规则拦截"
	if strings.TrimSpace(match.Message) != "" {
		message += "：" + strings.TrimSpace(match.Message)
	}
	return fmt.Sprintf("%s\n规则: %s (%s)\n匹配内容: %q", message, match.RuleName, match.RuleID, match.MatchedText)
}

func NewBlockedError(match *Match) error {
	if match == nil {
		return nil
	}
	return &BlockedError{Match: *match}
}

func AsBlocked(err error) (*Match, bool) {
	var blocked *BlockedError
	if !errors.As(err, &blocked) || blocked == nil {
		return nil, false
	}
	match := blocked.Match
	return &match, true
}

func cloneConfig(config Config) Config {
	if config.Rules == nil {
		return config
	}
	rules := make([]Rule, len(config.Rules))
	for i, rule := range config.Rules {
		rules[i] = rule
		rules[i].Tools = append([]string(nil), rule.Tools...)
	}
	config.Rules = rules
	return config
}

func candidateTexts(toolName string, arguments any) []string {
	var candidates []string
	seen := make(map[string]struct{})
	add := func(value string) {
		for round := 0; round <= 3; round++ {
			if _, exists := seen[value]; !exists {
				seen[value] = struct{}{}
				candidates = append(candidates, value)
			}
			decoded := decodePercentEscapes(value)
			if decoded == value {
				break
			}
			value = decoded
		}
	}
	add(toolName)

	var walk func(any, int)
	walk = func(value any, remainingDepth int) {
		if remainingDepth <= 0 {
			return
		}
		switch value := value.(type) {
		case string:
			add(value)
		case map[string]any:
			keys := make([]string, 0, len(value))
			for key := range value {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				add(key)
				walk(value[key], remainingDepth-1)
			}
		case []any:
			for _, item := range value {
				walk(item, remainingDepth-1)
			}
		}
	}

	if raw, err := json.Marshal(arguments); err == nil {
		var normalized any
		if json.Unmarshal(raw, &normalized) == nil {
			walk(normalized, 10001)
			if values, ok := normalized.(map[string]any); ok {
				addStructuredCommandCandidate(add, values)
			}
		}
		add(string(raw))
	} else {
		walk(arguments, 128)
	}
	return candidates
}

func addStructuredCommandCandidate(add func(string), arguments map[string]any) {
	command, _ := arguments["command"].(string)
	command = strings.TrimSpace(command)
	if command == "" {
		return
	}
	parts := []string{command}
	if rawArgs, ok := arguments["args"].([]any); ok {
		for _, rawArg := range rawArgs {
			if arg, ok := rawArg.(string); ok {
				parts = append(parts, arg)
			}
		}
	}
	add(strings.Join(parts, " "))
}

func decodePercentEscapes(value string) string {
	if !strings.Contains(value, "%") {
		return value
	}
	var output strings.Builder
	output.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] == '%' && i+2 < len(value) {
			hi, okHi := hexValue(value[i+1])
			lo, okLo := hexValue(value[i+2])
			if okHi && okLo {
				output.WriteByte(hi<<4 | lo)
				i += 2
				continue
			}
		}
		output.WriteByte(value[i])
	}
	return output.String()
}

func hexValue(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}

var globalManager = mustDefaultManager()

func mustDefaultManager() *Manager {
	manager, err := NewManager(DefaultConfig())
	if err != nil {
		panic(err)
	}
	return manager
}

func Global() *Manager {
	return globalManager
}

func UpdateGlobal(config Config) error {
	return globalManager.Update(config)
}

func Check(toolName string, arguments any) *Match {
	return globalManager.Check(toolName, arguments)
}

// CheckToolCall is the runtime entry point used before dispatching a tool.
func CheckToolCall(toolName string, arguments any) *Match {
	return globalManager.CheckToolCall(toolName, arguments)
}
