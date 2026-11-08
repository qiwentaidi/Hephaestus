package scan

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

type responseLengthCondition struct {
	kind  string
	value int64
	min   int64
	max   int64
}

var byteSizePattern = regexp.MustCompile(`^\d+(\.\d+)?$`)

var vulnFilterClausePattern = regexp.MustCompile(`^([a-zA-Z0-9_.]+)\s*(>=|<=|==|=|>|<|:|~)\s*(.+)$`)

func splitRuleTokens(raw string) []string {
	replacer := strings.NewReplacer("，", ",", "；", ",", ";", ",", "\n", ",")
	normalized := replacer.Replace(raw)
	parts := strings.Split(normalized, ",")
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens
}

func parseByteSize(raw string) (int64, error) {
	s := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(raw, " ", "")))
	if s == "" {
		return 0, errors.New("为空")
	}

	if strings.HasSuffix(s, "b") {
		s = strings.TrimSuffix(s, "b")
	}

	if s == "" {
		return 0, errors.New("缺少数值")
	}
	if !byteSizePattern.MatchString(s) {
		return 0, fmt.Errorf("仅支持 B 单位或纯数字字节值: %q", raw)
	}

	value, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("无法解析 %q", raw)
	}
	if value < 0 {
		return 0, errors.New("不能为负数")
	}
	return int64(math.Round(value)), nil
}

func parseResponseLengthConditionToken(token string) (responseLengthCondition, error) {
	t := strings.TrimSpace(token)
	if t == "" {
		return responseLengthCondition{}, errors.New("为空")
	}

	if strings.Contains(t, "-") && !strings.HasPrefix(t, ">") && !strings.HasPrefix(t, "<") && !strings.HasPrefix(t, "=") {
		parts := strings.SplitN(t, "-", 2)
		if len(parts) != 2 {
			return responseLengthCondition{}, fmt.Errorf("区间格式错误: %q", token)
		}
		left, err := parseByteSize(parts[0])
		if err != nil {
			return responseLengthCondition{}, err
		}
		right, err := parseByteSize(parts[1])
		if err != nil {
			return responseLengthCondition{}, err
		}
		if left > right {
			left, right = right, left
		}
		return responseLengthCondition{kind: "range", min: left, max: right}, nil
	}

	parseByPrefix := func(prefix string, kind string) (responseLengthCondition, bool, error) {
		if !strings.HasPrefix(t, prefix) {
			return responseLengthCondition{}, false, nil
		}
		value, err := parseByteSize(strings.TrimSpace(strings.TrimPrefix(t, prefix)))
		if err != nil {
			return responseLengthCondition{}, true, err
		}
		return responseLengthCondition{kind: kind, value: value}, true, nil
	}

	for _, rule := range []struct {
		prefix string
		kind   string
	}{
		{prefix: ">=", kind: "gte"},
		{prefix: "<=", kind: "lte"},
		{prefix: "==", kind: "eq"},
		{prefix: ">", kind: "gt"},
		{prefix: "<", kind: "lt"},
		{prefix: "=", kind: "eq"},
	} {
		cond, matched, err := parseByPrefix(rule.prefix, rule.kind)
		if matched {
			return cond, err
		}
	}

	value, err := parseByteSize(t)
	if err != nil {
		return responseLengthCondition{}, err
	}
	return responseLengthCondition{kind: "eq", value: value}, nil
}

func parseResponseLengthConditions(raw string) ([]responseLengthCondition, error) {
	tokens := splitRuleTokens(raw)
	conditions := make([]responseLengthCondition, 0, len(tokens))
	for _, token := range tokens {
		cond, err := parseResponseLengthConditionToken(token)
		if err != nil {
			return nil, fmt.Errorf("规则 %q 无效: %w", token, err)
		}
		conditions = append(conditions, cond)
	}
	return conditions, nil
}

func responseLengthClause(cond responseLengthCondition) (string, []interface{}) {
	switch cond.kind {
	case "gt":
		return "response_length > ?", []interface{}{cond.value}
	case "gte":
		return "response_length >= ?", []interface{}{cond.value}
	case "lt":
		return "response_length < ?", []interface{}{cond.value}
	case "lte":
		return "response_length <= ?", []interface{}{cond.value}
	case "range":
		return "response_length BETWEEN ? AND ?", []interface{}{cond.min, cond.max}
	default:
		return "response_length = ?", []interface{}{cond.value}
	}
}

func applyResponseLengthInclude(query *gorm.DB, conditions []responseLengthCondition) *gorm.DB {
	if len(conditions) == 0 {
		return query
	}
	clauses := make([]string, 0, len(conditions))
	args := make([]interface{}, 0, len(conditions)*2)
	for _, cond := range conditions {
		clause, clauseArgs := responseLengthClause(cond)
		clauses = append(clauses, "("+clause+")")
		args = append(args, clauseArgs...)
	}
	return query.Where(strings.Join(clauses, " OR "), args...)
}

func applyResponseLengthExclude(query *gorm.DB, conditions []responseLengthCondition) *gorm.DB {
	if len(conditions) == 0 {
		return query
	}
	for _, cond := range conditions {
		clause, args := responseLengthClause(cond)
		query = query.Not(clause, args...)
	}
	return query
}

type vulnerabilityFilterExpression struct {
	Keyword        string
	Type           string
	Level          string
	DisposalStatus string
	Method         string
	ResponseType   string
	AIVerified     *bool
	ResponseLength []responseLengthCondition
	ExcludeLength  []responseLengthCondition
	OrGroups       []vulnerabilityFilterExpression
}

func normalizeVulnLevel(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	switch strings.ToLower(trimmed) {
	case "all":
		return "all"
	case "critical":
		return "critical"
	case "high":
		return "high"
	case "medium":
		return "medium"
	case "low":
		return "low"
	case "info":
		return "info"
	}

	switch trimmed {
	case "严重":
		return "critical"
	case "高危":
		return "high"
	case "中危":
		return "medium"
	case "低危":
		return "low"
	case "信息":
		return "info"
	}

	return strings.ToLower(trimmed)
}

func normalizeJSVulnDisposalStatus(raw string) string {
	if status, ok := normalizeVulnerabilityDisposalStatus(raw); ok {
		return status
	}

	switch strings.TrimSpace(raw) {
	case "待处理", "待处置":
		return "open"
	case "处理中", "跟进中":
		return "in_progress"
	case "已修复":
		return "fixed"
	case "误报":
		return "false_positive"
	case "已忽略":
		return "ignored"
	default:
		return ""
	}
}

func splitClausesByAnd(raw string) []string {
	clauses := make([]string, 0, 8)
	var builder strings.Builder
	quote := rune(0)
	runes := []rune(raw)

	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if quote != 0 {
			builder.WriteRune(ch)
			if ch == quote && (i == 0 || runes[i-1] != '\\') {
				quote = 0
			}
			continue
		}

		if ch == '"' || ch == '\'' {
			quote = ch
			builder.WriteRune(ch)
			continue
		}

		if i+2 < len(runes) {
			maybeAnd := strings.ToUpper(string(runes[i : i+3]))
			prevIsSpace := i > 0 && (runes[i-1] == ' ' || runes[i-1] == '\t')
			nextIsSpace := i+3 < len(runes) && (runes[i+3] == ' ' || runes[i+3] == '\t')
			if maybeAnd == "AND" && prevIsSpace && nextIsSpace {
				clause := strings.TrimSpace(builder.String())
				if clause != "" {
					clauses = append(clauses, clause)
				}
				builder.Reset()
				i += 2
				continue
			}
		}

		builder.WriteRune(ch)
	}

	last := strings.TrimSpace(builder.String())
	if last != "" {
		clauses = append(clauses, last)
	}
	return clauses
}

func stripWrappingQuotes(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) < 2 {
		return trimmed
	}
	first := trimmed[0]
	last := trimmed[len(trimmed)-1]
	if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
		return trimmed[1 : len(trimmed)-1]
	}
	return trimmed
}

func newVulnerabilityFilterExpression() vulnerabilityFilterExpression {
	return vulnerabilityFilterExpression{
		ResponseLength: make([]responseLengthCondition, 0, 2),
		ExcludeLength:  make([]responseLengthCondition, 0, 2),
		OrGroups:       make([]vulnerabilityFilterExpression, 0, 2),
	}
}

func parseVulnerabilityAndFilterExpression(raw string) (vulnerabilityFilterExpression, error) {
	result := newVulnerabilityFilterExpression()
	text := strings.TrimSpace(raw)
	if text == "" {
		return result, nil
	}

	keywordParts := make([]string, 0, 4)
	clauses := splitClausesByAnd(text)

	for _, clause := range clauses {
		match := vulnFilterClausePattern.FindStringSubmatch(clause)
		if len(match) != 4 {
			keywordParts = append(keywordParts, stripWrappingQuotes(clause))
			continue
		}

		field := strings.ToLower(strings.TrimSpace(match[1]))
		op := strings.TrimSpace(match[2])
		valueRaw := stripWrappingQuotes(match[3])
		if valueRaw == "" {
			return result, fmt.Errorf("语法错误: %s", clause)
		}

		switch field {
		case "vuln.type":
			if op != "=" && op != ":" {
				return result, fmt.Errorf("字段 %s 仅支持 = 或 :", field)
			}
			result.Type = valueRaw
		case "vuln.level":
			if op != "=" && op != ":" {
				return result, fmt.Errorf("字段 %s 仅支持 = 或 :", field)
			}
			result.Level = normalizeVulnLevel(valueRaw)
		case "vuln.status", "vuln.disposal", "disposal.status":
			if op != "=" && op != ":" {
				return result, fmt.Errorf("字段 %s 仅支持 = 或 :", field)
			}
			normalized := normalizeJSVulnDisposalStatus(valueRaw)
			if normalized == "" {
				return result, fmt.Errorf("字段 %s 仅支持 open/in_progress/fixed/false_positive/ignored", field)
			}
			result.DisposalStatus = normalized
		case "vuln.method":
			if op != "=" && op != ":" {
				return result, fmt.Errorf("字段 %s 仅支持 = 或 :", field)
			}
			result.Method = strings.ToUpper(valueRaw)
		case "response.type", "response.contenttype", "response.content_type", "contenttype":
			if op != "=" && op != ":" && op != "~" {
				return result, fmt.Errorf("字段 %s 仅支持 =、:、~", field)
			}
			result.ResponseType = valueRaw
		case "ai.verified":
			if op != "=" && op != ":" {
				return result, fmt.Errorf("字段 %s 仅支持 = 或 :", field)
			}
			normalized := strings.ToLower(valueRaw)
			switch normalized {
			case "1", "true", "yes":
				v := true
				result.AIVerified = &v
			case "0", "false", "no":
				v := false
				result.AIVerified = &v
			default:
				return result, fmt.Errorf("字段 %s 仅支持 true/false", field)
			}
		case "response.length", "response.length.exclude":
			normalizedOp := op
			if op == ":" {
				normalizedOp = "="
			}
			if normalizedOp == "~" {
				return result, fmt.Errorf("字段 %s 不支持 ~", field)
			}

			ruleToken := valueRaw
			if normalizedOp != "=" {
				ruleToken = normalizedOp + valueRaw
			}

			condition, err := parseResponseLengthConditionToken(ruleToken)
			if err != nil {
				return result, fmt.Errorf("长度规则错误: %w", err)
			}
			if field == "response.length" {
				result.ResponseLength = append(result.ResponseLength, condition)
			} else {
				result.ExcludeLength = append(result.ExcludeLength, condition)
			}
		case "vuln.url", "vuln.title", "vuln.description", "keyword", "vuln.keyword":
			keywordParts = append(keywordParts, valueRaw)
		default:
			return result, fmt.Errorf("不支持的字段: %s", field)
		}
	}

	if len(keywordParts) > 0 {
		result.Keyword = strings.Join(keywordParts, " ")
	}
	return result, nil
}

func parseVulnerabilityFilterExpression(raw string) (vulnerabilityFilterExpression, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return newVulnerabilityFilterExpression(), nil
	}

	orClauses := splitDirResultClausesByOr(text)
	if len(orClauses) <= 1 {
		return parseVulnerabilityAndFilterExpression(text)
	}

	result := newVulnerabilityFilterExpression()
	for _, clause := range orClauses {
		group, err := parseVulnerabilityAndFilterExpression(clause)
		if err != nil {
			return result, err
		}
		result.OrGroups = append(result.OrGroups, group)
	}
	return result, nil
}

func vulnerabilityFilterSQL(expr vulnerabilityFilterExpression) (string, []interface{}) {
	clauses := make([]string, 0, 10)
	args := make([]interface{}, 0, 14)

	if expr.Level != "" && expr.Level != "all" {
		clauses = append(clauses, "level = ?")
		args = append(args, expr.Level)
	}
	if expr.Type != "" && expr.Type != "all" {
		clauses = append(clauses, "type = ?")
		args = append(args, expr.Type)
	}
	if expr.Method != "" && expr.Method != "all" {
		clauses = append(clauses, "method = ?")
		args = append(args, expr.Method)
	}
	if expr.ResponseType != "" {
		like := "%" + strings.ToLower(expr.ResponseType) + "%"
		clauses = append(clauses, "LOWER(response_type) LIKE ?")
		args = append(args, like)
	}
	if normalizedDisposalStatus := normalizeJSVulnDisposalStatus(expr.DisposalStatus); normalizedDisposalStatus != "" {
		clauses = append(clauses, "COALESCE(disposal_status, 'open') = ?")
		args = append(args, normalizedDisposalStatus)
	}
	if expr.Keyword != "" {
		like := "%" + expr.Keyword + "%"
		clauses = append(clauses, "(title LIKE ? OR type LIKE ? OR url LIKE ? OR description LIKE ? OR method LIKE ? OR response_type LIKE ?)")
		args = append(args, like, like, like, like, like, like)
	}
	if expr.AIVerified != nil {
		clauses = append(clauses, "ai_verified = ?")
		args = append(args, *expr.AIVerified)
	}
	if len(expr.ResponseLength) > 0 {
		includeClauses := make([]string, 0, len(expr.ResponseLength))
		for _, cond := range expr.ResponseLength {
			clause, clauseArgs := responseLengthClause(cond)
			includeClauses = append(includeClauses, "("+clause+")")
			args = append(args, clauseArgs...)
		}
		clauses = append(clauses, "("+strings.Join(includeClauses, " OR ")+")")
	}
	for _, cond := range expr.ExcludeLength {
		clause, clauseArgs := responseLengthClause(cond)
		clauses = append(clauses, "NOT ("+clause+")")
		args = append(args, clauseArgs...)
	}

	return strings.Join(clauses, " AND "), args
}

func applyVulnerabilityOrGroups(query *gorm.DB, groups []vulnerabilityFilterExpression) *gorm.DB {
	if len(groups) == 0 {
		return query
	}

	groupClauses := make([]string, 0, len(groups))
	args := make([]interface{}, 0, len(groups)*4)
	for _, group := range groups {
		clause, clauseArgs := vulnerabilityFilterSQL(group)
		if clause == "" {
			continue
		}
		groupClauses = append(groupClauses, "("+clause+")")
		args = append(args, clauseArgs...)
	}
	if len(groupClauses) == 0 {
		return query
	}
	return query.Where("("+strings.Join(groupClauses, " OR ")+")", args...)
}
