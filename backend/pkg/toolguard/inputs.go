package toolguard

import (
	"encoding/json"
	"strings"
)

func normalizedToolArguments(arguments any) map[string]any {
	raw, err := json.Marshal(arguments)
	if err != nil {
		return nil
	}
	var fields map[string]any
	if json.Unmarshal(raw, &fields) != nil {
		return nil
	}
	return fields
}

// Read only fields that can execute a command or carry a request payload.
// Descriptions, fact bodies and reports may legitimately quote dangerous syntax.
func executableInputs(toolName string, arguments any) []string {
	normalized := normalizedToolArguments(arguments)
	if normalized == nil {
		return nil
	}
	name := strings.ToLower(strings.TrimSpace(toolName))
	executionTool := IsExecutionTool(name)
	requestTool := name == "http-framework-test" || strings.Contains(name, "http") || strings.Contains(name, "request")
	sqlTool := strings.Contains(name, "sql") || strings.Contains(name, "database")
	var inputs []string
	var collectStrings func(any)
	collectStrings = func(value any) {
		switch value := value.(type) {
		case string:
			inputs = append(inputs, value)
		case []any:
			for _, item := range value {
				collectStrings(item)
			}
		case map[string]any:
			for _, item := range value {
				collectStrings(item)
			}
		}
	}
	var walk func(any)
	walk = func(value any) {
		fields, ok := value.(map[string]any)
		if !ok {
			return
		}
		for key, item := range fields {
			switch strings.ToLower(key) {
			case "command", "cmd", "code", "script", "sql", "statement", "payload", "payloads", "rawrequest", "raw_request", "requestbody", "request_body":
				collectStrings(item)
			case "text":
				if name == "shell_send" {
					collectStrings(item)
				}
			case "args", "additional_args":
				if executionTool {
					collectStrings(item)
				}
			case "url", "data", "body", "headers", "request":
				if requestTool {
					collectStrings(item)
				}
			case "query":
				if sqlTool {
					collectStrings(item)
				}
			case "config", "options", "parameters", "input":
				walk(item)
			}
		}
	}
	walk(normalized)
	if executionTool {
		addStructuredCommandCandidate(func(value string) { inputs = append(inputs, value) }, normalized)
	}
	return inputs
}

// IsExecutionTool reports whether a tool runs a command, script, Python code
// or an interactive shell. Runtime call interception is intentionally scoped
// to these tools; MCP control-plane tools and scan-task creation must not be
// blocked by command-oriented rules.
func IsExecutionTool(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "bash" || name == "monitor" || name == "exec" || name == "exec_run" || name == "python_execute" || name == "execute-python-script" || name == "install-python-package" || strings.Contains(name, "shell")
}

// IsInterceptionTool reports whether the runtime call guard should inspect the
// call. Native HTTP requests are checked like command/script HTTP egress, while
// MCP control-plane tools and attack-surface task creation are excluded.
func IsInterceptionTool(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return IsExecutionTool(name) || name == "http-framework-test"
}
