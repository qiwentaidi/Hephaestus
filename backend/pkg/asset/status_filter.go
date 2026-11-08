package asset

import (
	"strconv"

	"gorm.io/gorm"
)

// ApplyFingerprintStatusFilter 按 HTTP 状态码过滤指纹查询。
// 支持状态族（"2xx".."5xx"）和精确状态码；无法解析的过滤串不改变查询。
func ApplyFingerprintStatusFilter(query *gorm.DB, filter string) *gorm.DB {
	if len(filter) == 3 && filter[0] >= '2' && filter[0] <= '5' && filter[1:] == "xx" {
		lowerBound := int(filter[0]-'0') * 100
		return query.Where("status >= ? AND status < ?", lowerBound, lowerBound+100)
	}
	if status, err := strconv.Atoi(filter); err == nil {
		return query.Where("status = ?", status)
	}
	return query
}
