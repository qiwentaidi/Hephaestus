package findings

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCreateFindingRequiresTitleAndTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{
		`{"source":"ai","target":"https://example.test"}`,
		`{"source":"ai","title":"SQL injection"}`,
		`{"source":"ai","title":"   ","target":"https://example.test"}`,
	} {
		writer := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(writer)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/findings", strings.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		CreateFindingHandler(ctx)
		if writer.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400", body, writer.Code)
		}
	}
}
