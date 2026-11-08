package platform

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return c, recorder
}

func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是合法 JSON: %v，原始内容: %s", err, recorder.Body.String())
	}
	return body
}

func TestUsername(t *testing.T) {
	c, _ := newTestContext(t)
	if got := Username(c); got != "" {
		t.Fatalf("未注入用户名时应返回空字符串，实际 %q", got)
	}

	c.Set("username", "alice")
	if got := Username(c); got != "alice" {
		t.Fatalf("期望 alice，实际 %q", got)
	}
}

func TestWriteAnswers(t *testing.T) {
	tests := []struct {
		name   string
		write  func(c *gin.Context)
		status int
		want   map[string]any
	}{
		{
			name:   "success",
			write:  WriteSuccess,
			status: http.StatusOK,
			want:   map[string]any{"success": true},
		},
		{
			name:   "data",
			write:  func(c *gin.Context) { WriteData(c, gin.H{"id": "t1"}) },
			status: http.StatusOK,
			want:   map[string]any{"success": true, "data": map[string]any{"id": "t1"}},
		},
		{
			name:   "message",
			write:  func(c *gin.Context) { WriteMessage(c, "已保存") },
			status: http.StatusOK,
			want:   map[string]any{"success": true, "message": "已保存"},
		},
		{
			name:   "error",
			write:  func(c *gin.Context) { WriteError(c, http.StatusBadRequest, "参数错误") },
			status: http.StatusBadRequest,
			want:   map[string]any{"success": false, "error": "参数错误"},
		},
		{
			name:   "unauthorized",
			write:  WriteUnauthorized,
			status: http.StatusUnauthorized,
			want:   map[string]any{"success": false, "error": "未登录"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newTestContext(t)
			tt.write(c)
			if recorder.Code != tt.status {
				t.Fatalf("状态码期望 %d，实际 %d", tt.status, recorder.Code)
			}
			got := decodeBody(t, recorder)
			if len(got) != len(tt.want) {
				t.Fatalf("字段数期望 %d，实际 %d：%v", len(tt.want), len(got), got)
			}
			for key, want := range tt.want {
				gotValue, ok := got[key]
				if !ok {
					t.Fatalf("缺少字段 %q：%v", key, got)
				}
				wantJSON, _ := json.Marshal(want)
				gotJSON, _ := json.Marshal(gotValue)
				if string(wantJSON) != string(gotJSON) {
					t.Fatalf("字段 %q 期望 %s，实际 %s", key, wantJSON, gotJSON)
				}
			}
		})
	}
}
