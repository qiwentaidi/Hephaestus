package portpresets

import (
	"regexp"
	"strings"

	portutil "github.com/qiwentaidi/utils/port"
)

const (
	WorkbenchPortPresetAll      = "all"
	WorkbenchPortPresetTop1000  = "top1000"
	WorkbenchPortPresetDatabase = "database"

	WorkbenchPortRangeFull = "1-65535"

	// WorkbenchPortRangeTop1000 is the platform's common service preset. It
	// intentionally includes the first 1024 well-known ports plus common high
	// application and database ports that users expect from a "top 1000" scan.
	WorkbenchPortRangeTop1000  = "1-1024,1080,1099,1433,1521,2049,2375,2376,3000,3001,3306,3389,5000,5001,5432,5601,5672,5900,5901,6000,6001,6379,7001,7002,8000,8008,8009,8080,8081,8088,8090,8443,8888,9000,9001,9042,9090,9200,9300,11211,15672,27017,27018,27019,50000"
	WorkbenchPortRangeDatabase = "1433,U:1434,1521,2483,2484,2881,2883,3050,3306,4000,5000,50000,5236,5432,5433,54321,6379,7474,8086,8123,9000,9004,9042,9088,9092,9200,9300,11211,U:11211,27017,27018,27019"
)

// ResolveWorkbenchPortRange converts a chat-specified preset or custom port
// expression into the concrete portRange accepted by the port scanner.
func ResolveWorkbenchPortRange(preset string, custom string) (string, string, bool) {
	if normalized := NormalizeWorkbenchPortRange(custom); normalized != "" {
		return normalized, "自定义端口范围", true
	}

	switch NormalizeWorkbenchPortPreset(preset) {
	case WorkbenchPortPresetAll:
		return WorkbenchPortRangeFull, "全端口", true
	case WorkbenchPortPresetDatabase:
		return WorkbenchPortRangeDatabase, "常见数据库端口", true
	case WorkbenchPortPresetTop1000:
		return WorkbenchPortRangeTop1000, "Top 1000 常见端口", true
	default:
		return "", "", false
	}
}

func NormalizeWorkbenchPortRange(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	value = strings.NewReplacer("，", ",", "、", ",", "；", ",", ";", ",", " ", "").Replace(value)
	ports, err := portutil.ParsePortDefinitions(value)
	if err != nil || len(ports) == 0 {
		return ""
	}
	return value
}

func DetectWorkbenchPortRange(text string) string {
	value := strings.TrimSpace(text)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	if !strings.Contains(value, "端口") && !strings.Contains(lower, "port") {
		return ""
	}

	normalized := strings.NewReplacer("，", ",", "、", ",", "；", ",", ";", ",").Replace(value)
	portExpr := `((?:[ut]:)?\d{1,5}(?:-\d{1,5})?(?:\s*,\s*(?:[ut]:)?\d{1,5}(?:-\d{1,5})?)*)`
	for _, pattern := range []string{
		`(?i)(?:端口|ports?)\D{0,16}` + portExpr,
		`(?i)` + portExpr + `\D{0,16}(?:端口|ports?)`,
	} {
		expr := regexp.MustCompile(pattern)
		for _, matches := range expr.FindAllStringSubmatchIndex(normalized, -1) {
			if len(matches) < 4 || isWorkbenchPortRangeAddressFragment(normalized, matches[2], matches[3]) {
				continue
			}
			if normalized := NormalizeWorkbenchPortRange(normalized[matches[2]:matches[3]]); normalized != "" {
				return normalized
			}
		}
	}
	return ""
}

func isWorkbenchPortRangeAddressFragment(value string, start int, end int) bool {
	if start > 0 {
		switch value[start-1] {
		case '.', '/':
			return true
		}
	}
	if end < len(value) {
		switch value[end] {
		case '.', '/':
			return true
		}
	}
	return false
}

func NormalizeWorkbenchPortPreset(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.NewReplacer("_", "", "-", "", " ", "").Replace(value)
	switch value {
	case "all", "full", "fullport", "fullports", "allports", "165535", "1to65535":
		return WorkbenchPortPresetAll
	case "top1000", "topports", "common", "commonports":
		return WorkbenchPortPresetTop1000
	case "db", "database", "databases", "databaseports":
		return WorkbenchPortPresetDatabase
	default:
		return DetectWorkbenchPortPreset(raw)
	}
}

func DetectWorkbenchPortPreset(text string) string {
	value := strings.ToLower(strings.TrimSpace(text))
	compact := strings.NewReplacer(" ", "", "-", "", "_", "", "，", ",", "、", ",").Replace(value)
	switch {
	case strings.Contains(value, "全端口") ||
		strings.Contains(value, "全部端口") ||
		strings.Contains(compact, "1-65535") ||
		strings.Contains(compact, "165535") ||
		strings.Contains(compact, "allports") ||
		strings.Contains(compact, "fullports"):
		return WorkbenchPortPresetAll
	case strings.Contains(value, "数据库端口") ||
		strings.Contains(value, "数据库服务") ||
		strings.Contains(value, "数据库") ||
		strings.Contains(compact, "dbports") ||
		strings.Contains(compact, "databaseports"):
		return WorkbenchPortPresetDatabase
	case strings.Contains(compact, "top1000") ||
		strings.Contains(value, "top 1000") ||
		strings.Contains(value, "常见端口") ||
		strings.Contains(value, "常用端口") ||
		strings.Contains(value, "高频端口"):
		return WorkbenchPortPresetTop1000
	default:
		return ""
	}
}
