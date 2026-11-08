package asset

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Finding targets can contain multiple URLs with explanatory text. Only this
// explicit target field is mined; normal chat and tool prose are not.
var findingURLPattern = regexp.MustCompile("(?i)https?://[^\\s<>\"'`，。；（）【】「」]+")

func AIFindingURLs(target string) []string {
	urls := []string{}
	seen := map[string]bool{}
	for _, candidate := range findingURLPattern.FindAllString(target, -1) {
		candidate = strings.TrimRight(candidate, ".,;!")
		for _, pair := range [][2]string{{"(", ")"}, {"[", "]"}, {"{", "}"}} {
			for strings.HasSuffix(candidate, pair[1]) && strings.Count(candidate, pair[1]) > strings.Count(candidate, pair[0]) {
				candidate = strings.TrimSuffix(candidate, pair[1])
			}
		}
		if _, err := WebsiteOrigin(candidate); err != nil || seen[candidate] {
			continue
		}
		seen[candidate] = true
		urls = append(urls, candidate)
	}
	return urls
}

// DiscoverAIURLs reads successful tool results, not assistant prose or unexecuted
// command arguments. It accepts structured URL fields, JSON/JSONL tool output,
// and standalone URL lines (for example a crawler's output).
func DiscoverAIURLs(tool string, args map[string]any, result any) []string {
	raw, err := json.Marshal(result)
	if err != nil {
		return nil
	}
	var root any
	if json.Unmarshal(raw, &root) != nil {
		return nil
	}
	seen := map[string]bool{}
	urls := []string{}
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if _, err := WebsiteOrigin(raw); err != nil || seen[raw] {
			return
		}
		seen[raw] = true
		urls = append(urls, raw)
	}
	var walk func(any, string, int)
	walk = func(value any, key string, depth int) {
		if depth > 20 {
			return
		}
		switch v := value.(type) {
		case map[string]any:
			for _, k := range []string{"ok", "success"} {
				if flag, ok := v[k].(bool); ok && !flag {
					return
				}
			}
			if flag, _ := v["isError"].(bool); flag {
				return
			}
			if code, ok := v["exitCode"].(float64); ok && code != 0 {
				return
			}
			for k, item := range v {
				walk(item, strings.ToLower(k), depth+1)
			}
		case []any:
			for _, item := range v {
				walk(item, key, depth+1)
			}
		case string:
			switch key {
			case "", "url", "urls", "target_url", "targeturl", "final_url", "finalurl", "discovered_urls", "endpoints", "target":
				add(v)
			case "output", "stdout", "text", "content":
				var decoded any
				if json.Unmarshal([]byte(v), &decoded) == nil {
					walk(decoded, "", depth+1)
					return
				}
				for _, line := range strings.Split(v, "\n") {
					line = strings.TrimSpace(line)
					if json.Unmarshal([]byte(line), &decoded) == nil {
						walk(decoded, "", depth+1)
						continue
					}
					// Don't mine prose, HTML or credential-bearing text for incidental links.
					if !strings.ContainsAny(line, " \t\r\"<>") {
						add(line)
					}
				}
			}
		}
	}
	walk(root, "", 0)
	// HTTP clients may return only a response; the successful request URL is
	// evidence only when an actual HTTP status was returned.
	var hasHTTPStatus func(any) bool
	hasHTTPStatus = func(v any) bool {
		m, ok := v.(map[string]any)
		if !ok {
			return false
		}
		for _, k := range []string{"ok", "success"} {
			if flag, ok := m[k].(bool); ok && !flag {
				return false
			}
		}
		for _, k := range []string{"status", "statusCode", "status_code", "responseCode"} {
			if code, ok := m[k].(float64); ok && code >= 100 && code <= 599 {
				return true
			}
		}
		for _, k := range []string{"result", "response", "data"} {
			if hasHTTPStatus(m[k]) {
				return true
			}
		}
		return false
	}
	if strings.Contains(strings.ToLower(tool), "http") && hasHTTPStatus(root) {
		for _, key := range []string{"url", "target_url", "target", "base_url"} {
			if value, ok := args[key].(string); ok {
				add(value)
			}
		}
	}
	return urls
}
