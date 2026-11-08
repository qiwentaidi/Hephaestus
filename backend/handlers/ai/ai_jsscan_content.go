package ai

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"unicode"
)

// Rune offsets are stable for UTF-8 source, including non-ASCII identifiers.
func jsContentIndex(content []rune, query string, offset int) int {
	needle := []rune(strings.Map(unicode.ToLower, query))
	if len(needle) == 0 {
		return -1
	}
	for i := offset; i+len(needle) <= len(content); i++ {
		matched := true
		for j, c := range needle {
			if unicode.ToLower(content[i+j]) != c {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}
	return -1
}

func jsContentMatches(content, query string) []map[string]any {
	matches := []map[string]any{}
	if query == "" {
		return matches
	}
	runes := []rune(content)
	offset := 0
	for len(matches) < 3 {
		index := jsContentIndex(runes, query, offset)
		if index < 0 {
			break
		}
		start := max(0, index-120)
		end := min(len(runes), max(start+400, index+len([]rune(query))))
		// Queries can be arbitrarily long; keep every returned context bounded.
		end = min(end, start+1200)
		matches = append(matches, map[string]any{"matchOffset": index, "offset": start, "endOffset": end, "content": string(runes[start:end])})
		offset = index + max(1, len([]rune(query)))
	}
	return matches
}

func jsContentWindow(content string, args pentestJSScanEvidenceArgs, defaultLimit int) (map[string]any, error) {
	runes := []rune(content)
	if args.Offset < 0 || args.Length < 0 {
		return nil, fmt.Errorf("offset 和 length 必须非负（Unicode 字符偏移）")
	}
	if args.Offset > len(runes) {
		return nil, fmt.Errorf("offset 超出资源长度 %d", len(runes))
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
	if args.ContentHash != "" && args.ContentHash != hash {
		return nil, fmt.Errorf("JS 内容已变化，请重新检索资源并从新 contentHash 读取")
	}
	length := args.Length
	if length == 0 {
		length = defaultLimit
	}
	length = min(length, 16000)
	offset := args.Offset
	matchOffset := -1
	if args.Query != "" {
		matchOffset = jsContentIndex(runes, args.Query, offset)
		if matchOffset < 0 {
			return map[string]any{"content": "", "matchFound": false, "offset": offset, "nextOffset": len(runes), "totalLength": len(runes), "hasMore": false, "truncated": false, "contentHash": hash, "offsetUnit": "unicode_code_points"}, nil
		}
		offset = max(offset, matchOffset-min(300, length/4))
	}
	end := offset + min(length, len(runes)-offset)
	result := map[string]any{"content": string(runes[offset:end]), "offset": offset, "nextOffset": end, "totalLength": len(runes), "hasMore": end < len(runes), "truncated": offset > 0 || end < len(runes), "contentHash": hash, "offsetUnit": "unicode_code_points"}
	if args.Query != "" {
		result["matchFound"] = true
		result["matchOffset"] = matchOffset
	}
	return result, nil
}
