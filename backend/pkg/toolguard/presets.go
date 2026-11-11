package toolguard

import (
	"hephaestus-backend/pkg/toolruntime"
	"strings"
)

const CurrentPresetVersion = 6

// Match the SQL UPDATE ... SET header, rather than ordinary API names or
// update-related prose. SQL comments can separate tokens just like whitespace.
const sqlUpdateSeparator = `(?:\s|\\[nrt]|/\*.*?\*/|--[^\r\n]*(?:\r?\n|\\n|$))+`
const sqlUpdateIdentifier = `(?:[\p{L}_][\p{L}\p{N}_$]*|\\?"[^"]+\\?"|` + "`[^`]+`" + `|\[[^\]]+\])`
const sqlUpdatePattern = `(?is)(?:^|[^\p{L}\p{N}_$])(?P<match>update` + sqlUpdateSeparator +
	`(?:(?:only|low_priority|ignore)` + sqlUpdateSeparator + `)*` + sqlUpdateIdentifier +
	`(?:(?:` + sqlUpdateSeparator + `)?\.(?:` + sqlUpdateSeparator + `)?` + sqlUpdateIdentifier + `)*` +
	`(?:` + sqlUpdateSeparator + `(?:as` + sqlUpdateSeparator + `)?` + sqlUpdateIdentifier + `)?` +
	sqlUpdateSeparator + `set\b)`

const (
	previousDiskFormatPattern    = `(?i)(?:^|[^a-z0-9_])(?P<match>(?:mkfs(?:\.[a-z0-9]+)?|wipefs|shred)\s+[^\s;|&]+)`
	previousDiskOverwritePattern = `(?i)(?:^|[^a-z0-9_])(?P<match>dd\b[^\n;&|]*\bof=/dev/[a-z0-9/_-]+)`
	previousSQLDropPattern       = `(?i)(?:^|[^a-z0-9_])(?P<match>drop\s+(?:table|database|schema|view|index|trigger|procedure|function|column|constraint)\b)`
	artexRecursiveDeletePattern  = `(?i)(?:^|[^a-z0-9_])(?P<match>rm\s+(?:-[a-z]*r[a-z]*f[a-z]*|-[a-z]*f[a-z]*r[a-z]*|-[a-z]*r[a-z]*\s+-[a-z]*f[a-z]*|-[a-z]*f[a-z]*\s+-[a-z]*r[a-z]*|--recursive\s+--force|--force\s+--recursive|--recursive|--no-preserve-root))(?:\s|$)`
	artexDiskFormatPattern       = `(?i)(?:^|[^a-z0-9_])(?P<match>mkfs(?:\.[a-z0-9]+)?\b|(?:wipefs|shred)\s+[^\s;|&]+|wipe\s+[^\n;&|]{0,80}/dev/[a-z0-9/_-]+)`
	artexDiskOverwritePattern    = `(?i)(?:^|[^a-z0-9_])(?P<match>dd\b[^\n;&|]*\bof=\s*/dev/[a-z0-9/_-]+)`
	artexSQLDropPattern          = `(?i)(?:^|[^a-z0-9_])(?P<match>drop\s+(?:table|database|schema|view|index|trigger|procedure|function|column|constraint|tablespace|user|role)\b)`
)

// ARTEX presets already covered by existing Hephaestus rules are kept under
// their existing IDs. These are the ARTEX categories without an equivalent.
var artexAdditionalPresetIDs = []string{
	"preset-critical-system-delete",
	"preset-fork-bomb",
	"preset-shutdown-reboot",
	"preset-kill-all-processes",
	"preset-firewall-flush",
	"preset-mongodb-drop",
	"preset-redis-flush",
	"preset-bulk-clear-route",
	"preset-destructive-system-command",
}

// PresetRules are initial editable rules. After migration they live only in
// persisted Config.Rules; deleting one never causes it to be recreated.
func PresetRules() []Rule {
	return []Rule{
		{ID: "preset-high-risk-api-operation", Name: "[内置] 高危接口操作", Enabled: true, Origin: "preset", Target: "http_method", Priority: 90, Pattern: `(?i)(?:^|[^\p{L}\p{M}\p{N}_.-])(?P<match>(?:(?:update|delete|remove|destroy|batchdelete|deleteall|delete-all|removeall|remove-all|destroyall|destroy-all|stop|quit)[\p{L}\p{M}\p{N}_.-]*|(?:import|export|download|clear|drop|truncate|purge|reset|batch|batchdelete|update|delete|remove|destroy|modify|edit|create|add|save)(?:all|-all)|[\p{L}\p{M}\p{N}_-]+-all|all))(?:$|[^\p{L}\p{M}\p{N}_.-])`, Message: "检测到高危接口操作 {match}，工具调用已拦截。"},
		{ID: "preset-recursive-delete", Name: "[内置] 递归强制删除 rm -rf", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 100, Pattern: artexRecursiveDeletePattern, Message: "检测到递归强制删除 {match}，工具调用已拦截。"},
		{ID: "preset-critical-system-delete", Name: "[内置] 删除系统关键目录", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 100, Pattern: `(?i)\brm\b[^\n;&|]{0,100}\s(?P<match>/(?:etc|bin|usr|boot|var|lib|sys|proc|dev|sbin|root)(?:/|\b)|/(?:\s|$))`, Message: "检测到删除系统关键路径 {match}，工具调用已拦截。"},
		{ID: "preset-disk-format", Name: "[内置] 磁盘格式化或擦除", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 100, Pattern: artexDiskFormatPattern, Message: "检测到磁盘格式化或擦除命令 {match}，工具调用已拦截。"},
		{ID: "preset-disk-overwrite", Name: "[内置] 覆写磁盘设备 dd", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 100, Pattern: artexDiskOverwritePattern, Message: "检测到磁盘覆写命令 {match}，工具调用已拦截。"},
		{ID: "preset-fork-bomb", Name: "[内置] Fork 炸弹", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 100, Pattern: `:\(\)\s*\{[^}]*:\|:`, Message: "检测到 Fork 炸弹，工具调用已拦截。"},
		{ID: "preset-shutdown-reboot", Name: "[内置] 关机 / 重启", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 100, Pattern: `(?i)\b(?P<match>shutdown|reboot|halt|poweroff|init\s+[06])\b`, Message: "检测到关机或重启命令 {match}，工具调用已拦截。"},
		{ID: "preset-kill-all-processes", Name: "[内置] 杀死全部进程", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 100, Pattern: `(?i)\b(?P<match>kill\s+-9\s+-1|killall\s+-9)\b`, Message: "检测到批量终止进程命令 {match}，工具调用已拦截。"},
		{ID: "preset-firewall-flush", Name: "[内置] 清空防火墙规则", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 100, Pattern: `(?i)\b(?P<match>iptables\s+(?:-F|--flush)\b|nft\s+flush\s+ruleset\b)`, Message: "检测到清空防火墙规则命令 {match}，工具调用已拦截。"},
		// ARTEX also has a composite destructive-command rule. Only its cases not
		// covered by the specific rules above are included here.
		{ID: "preset-destructive-system-command", Name: "[内置] 破坏性系统命令", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 100, Pattern: `(?i)(?P<match>\bdd\s+if=|>\s*/dev/sd[a-z0-9]*)`, Message: "检测到破坏性系统命令 {match}，工具调用已拦截。"},
		{ID: "preset-sql-drop", Name: "[内置] SQL DROP", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 90, Pattern: artexSQLDropPattern, Message: "检测到 SQL DROP 操作 {match}，工具调用已拦截。"},
		{ID: "preset-sql-delete", Name: "[内置] SQL DELETE FROM", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 90, Pattern: `(?i)(?:^|[^a-z0-9_])(?P<match>delete\s+from\b)`, Message: "检测到 SQL DELETE 操作 {match}，工具调用已拦截。"},
		{ID: "preset-sql-update", Name: "[内置] SQL UPDATE", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 90, Pattern: sqlUpdatePattern, Message: "检测到 SQL UPDATE 操作 {match}，工具调用已拦截，避免篡改目标数据。"},
		{ID: "preset-sql-truncate", Name: "[内置] SQL TRUNCATE", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 90, Pattern: `(?i)(?:^|[^a-z0-9_])(?P<match>truncate\s+(?:table\s+)?[a-z_][a-z0-9_]*)`, Message: "检测到 SQL TRUNCATE 操作 {match}，工具调用已拦截。"},
		{ID: "preset-mongodb-drop", Name: "[内置] MongoDB drop / dropDatabase", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 90, Pattern: `(?i)(?P<match>\.(?:dropDatabase|dropCollection|drop)\s*\()`, Message: "检测到 MongoDB drop 操作 {match}，工具调用已拦截。"},
		{ID: "preset-redis-flush", Name: "[内置] Redis FLUSHALL / FLUSHDB", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 90, Pattern: `(?i)\b(?P<match>FLUSHALL|FLUSHDB)\b`, Message: "检测到 Redis 清空数据命令 {match}，工具调用已拦截。"},
		{ID: "preset-http-post-delete-route", Name: "[内置] 写接口删除动作", Enabled: true, Origin: "preset", Target: "http_method", Priority: 80, Pattern: `(?i)^(?:POST|PUT|PATCH)\s+/[^\s?]*(?:/|[._-])(?:delete|remove|destroy|batchdelete|deleteall)(?:/|$)`, Message: "检测到写接口删除动作 {match}，工具调用已拦截。"},
		{ID: "preset-curl-delete", Name: "[内置] curl / wget DELETE", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 80, Pattern: `(?i)(?:^|[\s;&|])(?P<match>curl\b[^\n;&|]{0,1000}?(?:-X\s*['"]?DELETE\b|--request(?:=|\s+)['"]?DELETE\b)|wget\b[^\n;&|]{0,1000}?--method(?:=|\s+)['"]?DELETE\b)`, Message: "检测到 curl 或 wget HTTP DELETE 请求 {match}，工具调用已拦截。"},
		{ID: "preset-client-delete", Name: "[内置] Python / HTTP 客户端 DELETE", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 80, Pattern: `(?i)(?P<match>\.\s*delete\s*\(|(?:\.\s*(?:request|urlopen|build_request)\s*\(|\bRequest\s*\()\s*['"]DELETE['"]|\b_?method\s*[:=]\s*['"]?DELETE\b|\bx-http-method-override\s*:\s*DELETE\b|\b(?:http|https|xh)\s+DELETE\s+\S+)`, Message: "检测到客户端 HTTP DELETE 请求 {match}，工具调用已拦截。"},
		{ID: "preset-raw-http-delete", Name: "[内置] 原始 HTTP DELETE 请求", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 80, Pattern: `(?im)^\s*(?P<match>DELETE\s+\S+\s+HTTP/[0-9.]+)`, Message: "检测到原始 HTTP DELETE 请求 {match}，工具调用已拦截。"},
		{ID: "preset-bulk-clear-route", Name: "[内置] 批量清空 / 清除接口路径", Enabled: true, Origin: "preset", Target: "executable_input", Priority: 80, Pattern: `(?i)(?P<match>/(?:clear|wipe|flush|purge|truncate|drop|destroy|factory[-_]reset|reset[-_]all)(?:[/?#"'\s]|$))`, Message: "检测到批量清空或销毁接口路径 {match}，工具调用已拦截。"},
	}
}

func IsLegacyDefaultRule(rule Rule) bool {
	return rule.Name == "高危递归删除命令" &&
		rule.Pattern == defaultDangerousDeletePattern &&
		rule.Message == "检测到高危递归删除命令 {match}，工具调用已被安全策略拦截。请检查命令影响范围后再试。" &&
		strings.Join(rule.Tools, ",") == "exec,exec_run,python_execute,execute-python-script"
}

// MigratePresetRules runs once per persisted configuration. It preserves all
// user-created rules and reuses the legacy default rule ID for its replacement.
func MigratePresetRules(config Config) (Config, bool) {
	config, changed := migratePresetRules(config)
	config = cloneConfig(config)
	for i, rule := range config.Rules {
		names, updated := toolruntime.MigrateDisabled(rule.Tools)
		if updated {
			config.Rules[i].Tools = names
			changed = true
		}
	}
	return config, changed
}

func migratePresetRules(config Config) (Config, bool) {
	if config.PresetVersion >= CurrentPresetVersion {
		return config, false
	}
	if config.PresetVersion == 1 {
		config = cloneConfig(config)
		// Add each preset introduced after v1 without restoring other rules
		// that an administrator removed from the saved configuration.
		appendMissingPreset(&config, "preset-http-post-delete-route")
		config.PresetVersion = 2
	}
	if config.PresetVersion == 2 {
		config = cloneConfig(config)
		appendMissingPreset(&config, "preset-high-risk-api-operation")
		config.PresetVersion = 3
	}
	if config.PresetVersion == 3 {
		config = cloneConfig(config)
		removePresetRule(&config, "preset-http-method-delete")
		config.PresetVersion = 4
	}
	if config.PresetVersion == 4 {
		config = cloneConfig(config)
		upgradeUnmodifiedARTEXEquivalents(&config)
		for _, id := range artexAdditionalPresetIDs {
			appendMissingPreset(&config, id)
		}
		config.PresetVersion = 5
	}
	if config.PresetVersion == 5 {
		config = cloneConfig(config)
		appendMissingPreset(&config, "preset-sql-update")
		config.PresetVersion = CurrentPresetVersion
		return config, true
	}
	config = cloneConfig(config)
	presets := PresetRules()
	for i, rule := range config.Rules {
		if IsLegacyDefaultRule(rule) {
			for j := range presets {
				if presets[j].ID == "preset-recursive-delete" {
					presets[j].ID = rule.ID
					presets[j].Enabled = rule.Enabled
					break
				}
			}
			config.Rules = append(config.Rules[:i], config.Rules[i+1:]...)
			break
		}
	}
	next := make([]Rule, 0, len(presets)+len(config.Rules))
	for _, preset := range presets {
		if !hasEquivalentPreset(config.Rules, preset) {
			next = append(next, preset)
		}
	}
	config.Rules = append(next, config.Rules...)
	removePresetRule(&config, "preset-http-method-delete")
	config.PresetVersion = CurrentPresetVersion
	return config, true
}

func upgradeUnmodifiedARTEXEquivalents(config *Config) {
	oldPatterns := map[string]string{
		"preset-recursive-delete": defaultDangerousDeletePattern,
		"preset-disk-format":      previousDiskFormatPattern,
		"preset-disk-overwrite":   previousDiskOverwritePattern,
		"preset-sql-drop":         previousSQLDropPattern,
	}
	newPatterns := map[string]string{
		"preset-recursive-delete": artexRecursiveDeletePattern,
		"preset-disk-format":      artexDiskFormatPattern,
		"preset-disk-overwrite":   artexDiskOverwritePattern,
		"preset-sql-drop":         artexSQLDropPattern,
	}
	for i := range config.Rules {
		rule := &config.Rules[i]
		if old, ok := oldPatterns[rule.ID]; ok && rule.Pattern == old {
			rule.Pattern = newPatterns[rule.ID]
		}
	}
}

func appendMissingPreset(config *Config, id string) {
	var preset *Rule
	for _, rule := range PresetRules() {
		if rule.ID == id {
			preset = &rule
			break
		}
	}
	if preset == nil {
		return
	}
	if hasEquivalentPreset(config.Rules, *preset) {
		return
	}
	config.Rules = append(config.Rules, *preset)
}

func hasEquivalentPreset(rules []Rule, preset Rule) bool {
	for _, rule := range rules {
		if rule.ID == preset.ID || rule.Name == preset.Name || (rule.Target == preset.Target && rule.Pattern == preset.Pattern) {
			return true
		}
	}
	return false
}

func removePresetRule(config *Config, id string) {
	rules := config.Rules[:0]
	for _, rule := range config.Rules {
		if rule.ID != id {
			rules = append(rules, rule)
		}
	}
	config.Rules = rules
}

func executableInputCandidates(toolName string, arguments any) []string {
	var candidates []string
	seen := make(map[string]struct{})
	for _, input := range executableInputs(toolName, arguments) {
		for round := 0; round <= 3; round++ {
			for _, value := range []string{input, strings.ReplaceAll(input, "+", " ")} {
				if _, exists := seen[value]; !exists {
					seen[value] = struct{}{}
					candidates = append(candidates, value)
				}
			}
			decoded := decodePercentEscapes(input)
			if decoded == input {
				break
			}
			input = decoded
		}
	}
	return candidates
}
