package httpquery

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// Values returns the normalized values of a query parameter. Repeated
// parameters and comma-separated values are both accepted. Empty entries and
// the "all" sentinel are ignored, and duplicates are removed.
func Values(c *gin.Context, key string) []string {
	out := make([]string, 0)
	seen := make(map[string]struct{})
	for _, raw := range c.QueryArray(key) {
		for _, part := range strings.Split(raw, ",") {
			value := strings.TrimSpace(part)
			if value == "" || strings.EqualFold(value, "all") {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	return out
}
