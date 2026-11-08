package toolguard

import (
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

var (
	requestURLLiteral  = regexp.MustCompile(`(?i)https?://[^\s"'` + "`" + `<>\\(){}\[\],;]+`)
	requestPathLiteral = regexp.MustCompile(`["'](/[^\s"'<>\\]*)["']`)
	rawRequestLine     = regexp.MustCompile(`(?im)^\s*(GET|HEAD|POST|PUT|PATCH|DELETE|OPTIONS|TRACE|CONNECT)\s+(\S+)\s+HTTP/[0-9.]+`)
	// Only explicit operation selectors, not prose or arbitrary body values.
	requestOperationLiteral = regexp.MustCompile(`(?i)(?:^|[\s?&;,{'"(])(?:action|operation|op|_method|http_method)["']?\s*[:=]\s*["']?([a-z][a-z0-9_.-]*)`)
)

// http_method rules inspect methods, request paths and explicit operation
// selectors. Executable fields are inspected for request literals as well.
// This is static inspection, not evaluation of code or files referenced by it.
func httpMethodCandidates(toolName string, arguments any) []string {
	fields := normalizedToolArguments(arguments)
	if fields == nil {
		return nil
	}
	name := strings.ToLower(strings.TrimSpace(toolName))
	executionTool := IsExecutionTool(name)
	requestTool := name == "http-framework-test" || strings.Contains(name, "http") || strings.Contains(name, "request")
	requestCarrier := requestTool || strings.HasPrefix(name, "external_mcp_") || strings.HasPrefix(name, "attack_surface_create_")
	if !requestCarrier && !executionTool {
		return nil
	}
	var candidates []string
	seen := map[string]bool{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			candidates = append(candidates, value)
		}
	}
	variants := func(value string, visit func(string)) {
		for round := 0; round <= 3; round++ {
			visit(value)
			decoded := decodePercentEscapes(value)
			if decoded == value {
				break
			}
			value = decoded
		}
	}
	addOperations := func(value string) {
		variants(value, func(value string) {
			for _, match := range requestOperationLiteral.FindAllStringSubmatch(value, -1) {
				add(match[1])
			}
		})
	}
	addURL := func(value, method string) {
		variants(value, func(value string) {
			parsed, err := url.Parse(value)
			if err != nil {
				return
			}
			// Path rules must not depend on the caller explicitly supplying a
			// method. The native HTTP executor defaults an omitted method to GET.
			variants(parsed.Path, func(path string) {
				add(path)
				if method != "" {
					add(strings.ToUpper(strings.TrimSpace(method)) + " " + path)
				}
			})
			addOperations(parsed.RawQuery)
		})
	}
	var addPayload func(any)
	addPayload = func(value any) {
		switch value := value.(type) {
		case string:
			var decoded any
			if json.Unmarshal([]byte(value), &decoded) == nil {
				// JSON strings may contain ordinary prose. Inspect structured
				// bodies only; never recursively parse a scalar JSON string.
				switch decoded.(type) {
				case map[string]any, []any:
					addPayload(decoded)
				}
				return
			}
			addOperations(value)
		case map[string]any:
			for _, key := range sortedInputKeys(value) {
				item := value[key]
				switch strings.ToLower(key) {
				case "action", "operation", "op", "_method", "http_method":
					if text, ok := item.(string); ok {
						variants(text, add)
					}
				default:
					switch item.(type) {
					case map[string]any, []any:
						addPayload(item)
					}
				}
			}
		case []any:
			for _, item := range value {
				addPayload(item)
			}
		}
	}
	var walk func(any)
	walk = func(value any) {
		if list, ok := value.([]any); ok {
			for _, item := range list {
				walk(item)
			}
			return
		}
		values, ok := value.(map[string]any)
		if !ok {
			return
		}
		var urls, methods []string
		for _, key := range sortedInputKeys(values) {
			value := values[key]
			switch strings.ToLower(key) {
			case "url", "uri", "endpoint", "urls", "targets":
				switch value := value.(type) {
				case string:
					urls = append(urls, value)
				case []any:
					for _, item := range value {
						if text, ok := item.(string); ok {
							urls = append(urls, text)
						}
					}
				}
			case "method", "httpmethod", "http_method":
				if text, ok := value.(string); ok {
					methods = append(methods, strings.TrimSpace(text))
				}
			case "request", "requests", "input", "options", "config", "parameters":
				walk(value)
			}
		}
		if !requestTool && len(urls) == 0 {
			return
		}
		for _, method := range methods {
			add(method)
		}
		for _, value := range urls {
			addURL(value, "")
			for _, method := range methods {
				addURL(value, method)
			}
		}
		for _, key := range sortedInputKeys(values) {
			value := values[key]
			switch strings.ToLower(key) {
			case "headers":
				if headers, ok := value.(map[string]any); ok {
					for _, header := range sortedInputKeys(headers) {
						if key := strings.ToLower(strings.TrimSpace(header)); key == "x-http-method-override" || key == "x-method-override" {
							switch value := headers[header].(type) {
							case string:
								add(value)
							case []any:
								for _, item := range value {
									if text, ok := item.(string); ok {
										add(text)
									}
								}
							}
						}
					}
				}
			case "data", "body", "requestbody", "request_body", "query", "params":
				addPayload(value)
			}
		}
	}
	if requestCarrier {
		walk(fields)
	}
	// Apply literal extraction only to executable/request fields. Reports,
	// facts, descriptions and tool metadata do not become request candidates.
	for _, input := range executableInputCandidates(toolName, arguments) {
		for _, match := range requestURLLiteral.FindAllString(input, -1) {
			addURL(match, "")
		}
		for _, match := range requestPathLiteral.FindAllStringSubmatch(input, -1) {
			addURL(match[1], "")
		}
		for _, match := range rawRequestLine.FindAllStringSubmatch(input, -1) {
			add(match[1])
			addURL(match[2], match[1])
		}
		if executionTool {
			addOperations(input)
		}
	}
	return candidates
}

func sortedInputKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
