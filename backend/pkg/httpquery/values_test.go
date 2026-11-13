package httpquery

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name  string
		query string
		key   string
		want  []string
	}{
		{name: "empty", query: "", key: "status", want: []string{}},
		{name: "all", query: "status=all", key: "status", want: []string{}},
		{name: "single", query: "status=success", key: "status", want: []string{"success"}},
		{name: "csv", query: "status=open%2Cfailure", key: "status", want: []string{"open", "failure"}},
		{name: "repeated", query: "status=open&status=failure", key: "status", want: []string{"open", "failure"}},
		{name: "mixed", query: "status=open%2Cfailure&status=blocked%2Call", key: "status", want: []string{"open", "failure", "blocked"}},
		{name: "dedupe", query: "status=open&status=open", key: "status", want: []string{"open"}},
		{name: "other key", query: "kind=all", key: "status", want: []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/?"+tc.query, nil)
			got := Values(c, tc.key)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Values(%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}
