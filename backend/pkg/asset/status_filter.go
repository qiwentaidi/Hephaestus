package asset

import (
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// ApplyFingerprintStatusFilter 按 HTTP 状态码过滤指纹查询。
// 支持状态族（"2xx".."5xx"）和精确状态码；多个条件之间为 OR，无法解析的条件会被忽略。
func ApplyFingerprintStatusFilter(query *gorm.DB, filters []string) *gorm.DB {
	clauses := make([]string, 0, len(filters))
	args := make([]interface{}, 0, len(filters)*2)
	for _, filter := range filters {
		filter = strings.TrimSpace(filter)
		if len(filter) == 3 && filter[0] >= '2' && filter[0] <= '5' && filter[1:] == "xx" {
			lowerBound := int(filter[0]-'0') * 100
			clauses = append(clauses, "(status >= ? AND status < ?)")
			args = append(args, lowerBound, lowerBound+100)
			continue
		}
		if status, err := strconv.Atoi(filter); err == nil {
			clauses = append(clauses, "status = ?")
			args = append(args, status)
		}
	}
	if len(clauses) == 0 {
		return query
	}
	return query.Where("("+strings.Join(clauses, " OR ")+")", args...)
}
