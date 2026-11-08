package scan

import (
	"fmt"
	"hephaestus-backend/pkg/database"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type dirResultNumericCondition struct {
	kind  string
	value int
}

type dirResultTextCondition struct {
	column string
	value  string
	exact  bool
}

type dirResultFilterExpression struct {
	Keyword       string
	ResponseType  string
	Status        []dirResultNumericCondition
	Recursion     []dirResultNumericCondition
	LengthInclude []responseLengthCondition
	LengthExclude []responseLengthCondition
	Text          []dirResultTextCondition
	OrGroups      []dirResultFilterExpression
}

func parseDirResultNumericCondition(op, raw string) (dirResultNumericCondition, error) {
	normalizedOp := strings.TrimSpace(op)
	if normalizedOp == ":" || normalizedOp == "==" {
		normalizedOp = "="
	}
	if normalizedOp == "~" {
		return dirResultNumericCondition{}, fmt.Errorf("不支持的操作符 %s", op)
	}

	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return dirResultNumericCondition{}, fmt.Errorf("数值无效: %s", raw)
	}

	switch normalizedOp {
	case ">", ">=", "<", "<=", "=":
		return dirResultNumericCondition{kind: normalizedOp, value: value}, nil
	default:
		return dirResultNumericCondition{}, fmt.Errorf("不支持的操作符 %s", op)
	}
}

func dirResultNumericClause(column string, cond dirResultNumericCondition) (string, []interface{}) {
	switch cond.kind {
	case ">":
		return fmt.Sprintf("%s > ?", column), []interface{}{cond.value}
	case ">=":
		return fmt.Sprintf("%s >= ?", column), []interface{}{cond.value}
	case "<":
		return fmt.Sprintf("%s < ?", column), []interface{}{cond.value}
	case "<=":
		return fmt.Sprintf("%s <= ?", column), []interface{}{cond.value}
	default:
		return fmt.Sprintf("%s = ?", column), []interface{}{cond.value}
	}
}

func applyDirResultNumericCondition(query *gorm.DB, column string, conditions []dirResultNumericCondition) *gorm.DB {
	if len(conditions) == 0 {
		return query
	}
	for _, cond := range conditions {
		clause, args := dirResultNumericClause(column, cond)
		query = query.Where(clause, args...)
	}
	return query
}

func dirResultTextClause(cond dirResultTextCondition) (string, []interface{}) {
	if cond.exact {
		return fmt.Sprintf("LOWER(%s) = ?", cond.column), []interface{}{strings.ToLower(cond.value)}
	}
	return fmt.Sprintf("LOWER(%s) LIKE ?", cond.column), []interface{}{"%" + strings.ToLower(cond.value) + "%"}
}

func applyDirResultTextConditions(query *gorm.DB, conditions []dirResultTextCondition) *gorm.DB {
	for _, cond := range conditions {
		clause, args := dirResultTextClause(cond)
		query = query.Where(clause, args...)
	}
	return query
}

func dirResultLengthClause(cond responseLengthCondition) (string, []interface{}) {
	switch cond.kind {
	case "gt":
		return "length > ?", []interface{}{cond.value}
	case "gte":
		return "length >= ?", []interface{}{cond.value}
	case "lt":
		return "length < ?", []interface{}{cond.value}
	case "lte":
		return "length <= ?", []interface{}{cond.value}
	case "range":
		return "length BETWEEN ? AND ?", []interface{}{cond.min, cond.max}
	default:
		return "length = ?", []interface{}{cond.value}
	}
}

func applyDirResultLengthInclude(query *gorm.DB, conditions []responseLengthCondition) *gorm.DB {
	if len(conditions) == 0 {
		return query
	}
	clauses := make([]string, 0, len(conditions))
	args := make([]interface{}, 0, len(conditions)*2)
	for _, cond := range conditions {
		clause, clauseArgs := dirResultLengthClause(cond)
		clauses = append(clauses, "("+clause+")")
		args = append(args, clauseArgs...)
	}
	return query.Where(strings.Join(clauses, " OR "), args...)
}

func applyDirResultLengthExclude(query *gorm.DB, conditions []responseLengthCondition) *gorm.DB {
	if len(conditions) == 0 {
		return query
	}
	for _, cond := range conditions {
		clause, args := dirResultLengthClause(cond)
		query = query.Not(clause, args...)
	}
	return query
}

func splitDirResultClausesByOr(raw string) []string {
	clauses := make([]string, 0, 4)
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

		if i+1 < len(runes) && (runes[i] == 'O' || runes[i] == 'o') && (runes[i+1] == 'R' || runes[i+1] == 'r') {
			prevOK := i == 0 || strings.TrimSpace(string(runes[i-1])) == ""
			nextOK := i+2 >= len(runes) || strings.TrimSpace(string(runes[i+2])) == ""
			if prevOK && nextOK {
				if clause := strings.TrimSpace(builder.String()); clause != "" {
					clauses = append(clauses, clause)
				}
				builder.Reset()
				i++
				continue
			}
		}

		builder.WriteRune(ch)
	}

	if clause := strings.TrimSpace(builder.String()); clause != "" {
		clauses = append(clauses, clause)
	}
	return clauses
}

func newDirResultFilterExpression() dirResultFilterExpression {
	return dirResultFilterExpression{
		Status:        make([]dirResultNumericCondition, 0, 2),
		Recursion:     make([]dirResultNumericCondition, 0, 2),
		LengthInclude: make([]responseLengthCondition, 0, 2),
		LengthExclude: make([]responseLengthCondition, 0, 2),
		Text:          make([]dirResultTextCondition, 0, 2),
		OrGroups:      make([]dirResultFilterExpression, 0, 2),
	}
}

func parseDirResultAndFilterExpression(raw string) (dirResultFilterExpression, error) {
	result := dirResultFilterExpression{
		Status:        make([]dirResultNumericCondition, 0, 2),
		Recursion:     make([]dirResultNumericCondition, 0, 2),
		LengthInclude: make([]responseLengthCondition, 0, 2),
		LengthExclude: make([]responseLengthCondition, 0, 2),
		Text:          make([]dirResultTextCondition, 0, 2),
	}
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
		case "dir.status", "status":
			cond, err := parseDirResultNumericCondition(op, valueRaw)
			if err != nil {
				return result, fmt.Errorf("字段 %s 解析失败: %w", field, err)
			}
			result.Status = append(result.Status, cond)
		case "dir.recursion", "recursion":
			cond, err := parseDirResultNumericCondition(op, valueRaw)
			if err != nil {
				return result, fmt.Errorf("字段 %s 解析失败: %w", field, err)
			}
			result.Recursion = append(result.Recursion, cond)
		case "response.type", "response.contenttype", "response.content_type", "contenttype":
			if op != "=" && op != ":" && op != "~" && op != "==" {
				return result, fmt.Errorf("字段 %s 仅支持 =、==、:、~", field)
			}
			result.Text = append(result.Text, dirResultTextCondition{column: "content_type", value: valueRaw, exact: op == "=="})
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
				result.LengthInclude = append(result.LengthInclude, condition)
			} else {
				result.LengthExclude = append(result.LengthExclude, condition)
			}
		case "dir.url", "url":
			if op != "=" && op != ":" && op != "~" && op != "==" {
				return result, fmt.Errorf("字段 %s 仅支持 =、==、:、~", field)
			}
			result.Text = append(result.Text, dirResultTextCondition{column: "url", value: valueRaw, exact: op == "=="})
		case "dir.title", "title":
			if op != "=" && op != ":" && op != "~" && op != "==" {
				return result, fmt.Errorf("字段 %s 仅支持 =、==、:、~", field)
			}
			result.Text = append(result.Text, dirResultTextCondition{column: "title", value: valueRaw, exact: op == "=="})
		case "keyword", "dir.keyword":
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

func parseDirResultFilterExpression(raw string) (dirResultFilterExpression, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return newDirResultFilterExpression(), nil
	}

	orClauses := splitDirResultClausesByOr(text)
	if len(orClauses) <= 1 {
		return parseDirResultAndFilterExpression(text)
	}

	result := newDirResultFilterExpression()
	for _, clause := range orClauses {
		group, err := parseDirResultAndFilterExpression(clause)
		if err != nil {
			return result, err
		}
		result.OrGroups = append(result.OrGroups, group)
	}
	return result, nil
}

func dirResultFilterSQL(expr dirResultFilterExpression) (string, []interface{}) {
	clauses := make([]string, 0, 8)
	args := make([]interface{}, 0, 12)

	if expr.Keyword != "" {
		like := "%" + strings.ToLower(expr.Keyword) + "%"
		clauses = append(clauses, "(LOWER(url) LIKE ? OR LOWER(title) LIKE ? OR LOWER(location) LIKE ? OR LOWER(content_type) LIKE ?)")
		args = append(args, like, like, like, like)
	}
	if expr.ResponseType != "" {
		like := "%" + strings.ToLower(expr.ResponseType) + "%"
		clauses = append(clauses, "LOWER(content_type) LIKE ?")
		args = append(args, like)
	}
	for _, cond := range expr.Text {
		clause, clauseArgs := dirResultTextClause(cond)
		clauses = append(clauses, clause)
		args = append(args, clauseArgs...)
	}
	for _, cond := range expr.Status {
		clause, clauseArgs := dirResultNumericClause("status", cond)
		clauses = append(clauses, clause)
		args = append(args, clauseArgs...)
	}
	for _, cond := range expr.Recursion {
		clause, clauseArgs := dirResultNumericClause("recursion", cond)
		clauses = append(clauses, clause)
		args = append(args, clauseArgs...)
	}
	if len(expr.LengthInclude) > 0 {
		includeClauses := make([]string, 0, len(expr.LengthInclude))
		for _, cond := range expr.LengthInclude {
			clause, clauseArgs := dirResultLengthClause(cond)
			includeClauses = append(includeClauses, "("+clause+")")
			args = append(args, clauseArgs...)
		}
		clauses = append(clauses, "("+strings.Join(includeClauses, " OR ")+")")
	}
	for _, cond := range expr.LengthExclude {
		clause, clauseArgs := dirResultLengthClause(cond)
		clauses = append(clauses, "NOT ("+clause+")")
		args = append(args, clauseArgs...)
	}

	return strings.Join(clauses, " AND "), args
}

func applyDirResultOrGroups(query *gorm.DB, groups []dirResultFilterExpression) *gorm.DB {
	if len(groups) == 0 {
		return query
	}

	groupClauses := make([]string, 0, len(groups))
	args := make([]interface{}, 0, len(groups)*4)
	for _, group := range groups {
		clause, clauseArgs := dirResultFilterSQL(group)
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

// GetDirScanResultsHandler 获取目录扫描结果（从 VulnScanTask）
func GetDirScanResultsHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取数据库连接失败: " + err.Error(),
		})
		return
	}

	taskID := c.Param("id")
	versionParam := c.Query("version")
	keyword := strings.TrimSpace(c.Query("keyword"))
	responseType := strings.TrimSpace(c.Query("responseType"))
	filterExprRaw := strings.TrimSpace(c.Query("filterExpr"))

	exprFilters := newDirResultFilterExpression()
	if filterExprRaw != "" {
		parsedExpr, parseErr := parseDirResultFilterExpression(filterExprRaw)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "筛选表达式错误: " + parseErr.Error(),
			})
			return
		}
		exprFilters = parsedExpr
	}

	// 检查任务是否存在
	var task database.VulnScanTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "任务不存在",
		})
		return
	}
	taskVersion := task.Version
	if versionParam != "" {
		if parsedVersion, err := strconv.Atoi(versionParam); err == nil && parsedVersion > 0 {
			taskVersion = parsedVersion
		}
	}

	// 解析分页参数
	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 1000 {
			limit = parsedLimit
		}
	}

	offset := 0
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	finalKeyword := keyword
	if finalKeyword == "" && len(exprFilters.OrGroups) == 0 {
		finalKeyword = exprFilters.Keyword
	}
	finalResponseType := responseType
	if finalResponseType == "" && len(exprFilters.OrGroups) == 0 {
		finalResponseType = exprFilters.ResponseType
	}

	// 构建过滤查询
	resultTaskIDs := []string{taskID}
	if task.ScanType == "combination" && strings.TrimSpace(task.ParentCombinationID) != "" {
		var idsErr error
		resultTaskIDs, idsErr = combinationResultTaskIDs(db, task, taskVersion)
		if idsErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "查询子任务失败: " + idsErr.Error(),
			})
			return
		}
	}
	query := db.Model(&database.VulnScanDirResult{}).Where("task_id IN ?", resultTaskIDs)
	if versionParam != "" {
		query = query.Where("version = ?", taskVersion)
	}
	if finalKeyword != "" {
		like := "%" + strings.ToLower(finalKeyword) + "%"
		query = query.Where(
			"LOWER(url) LIKE ? OR LOWER(title) LIKE ? OR LOWER(location) LIKE ? OR LOWER(content_type) LIKE ?",
			like, like, like, like,
		)
	}
	if finalResponseType != "" {
		like := "%" + strings.ToLower(finalResponseType) + "%"
		query = query.Where("LOWER(content_type) LIKE ?", like)
	}
	if len(exprFilters.OrGroups) > 0 {
		query = applyDirResultOrGroups(query, exprFilters.OrGroups)
	} else {
		query = applyDirResultNumericCondition(query, "status", exprFilters.Status)
		query = applyDirResultNumericCondition(query, "recursion", exprFilters.Recursion)
		query = applyDirResultTextConditions(query, exprFilters.Text)
		query = applyDirResultLengthInclude(query, exprFilters.LengthInclude)
		query = applyDirResultLengthExclude(query, exprFilters.LengthExclude)
	}

	// 获取排序参数
	sortBy := c.Query("sortBy")       // length, status
	sortOrder := c.Query("sortOrder") // asc, desc

	sortColumn := "created_at"
	switch sortBy {
	case "length":
		sortColumn = "length"
	case "status":
		sortColumn = "status"
	case "recursion":
		sortColumn = "recursion"
	}
	order := "asc"
	if strings.EqualFold(sortOrder, "desc") {
		order = "desc"
	}
	// 添加稳定次排序，避免跨页时同值记录顺序抖动导致“排序重置”的观感
	query = query.Order(fmt.Sprintf("%s %s", sortColumn, order)).Order("id ASC")

	// 获取总数
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取结果总数失败: " + err.Error(),
		})
		return
	}

	// 获取结果列表
	var results []database.VulnScanDirResult
	if err := query.Limit(limit).Offset(offset).Find(&results).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取扫描结果失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"results": results,
			"total":   total,
			"limit":   limit,
			"offset":  offset,
		},
	})
}
