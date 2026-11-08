package theme

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupThemeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "themes.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.CustomTheme{}); err != nil {
		t.Fatal(err)
	}

	previous := openThemeDB
	openThemeDB = func() (*gorm.DB, error) { return db, nil }
	t.Cleanup(func() { openThemeDB = previous })
	return db
}

func runThemeHandler(t *testing.T, handler gin.HandlerFunc, method, target, body string, params gin.Params) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	context.Request = request
	context.Params = params
	handler(context)
	return recorder
}

func marshalThemeBody(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}

func createTheme(t *testing.T, name string) themePayload {
	t.Helper()
	recorder := runThemeHandler(t, CreateCustomThemeHandler, http.MethodPost, "/api/v1/themes",
		marshalThemeBody(t, writeRequest{Name: name, Definition: validDefinition()}), nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("create theme returned %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Success bool         `json:"success"`
		Data    themePayload `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response.Data
}

func TestCreateThemeRejectsDuplicateNames(t *testing.T) {
	setupThemeTestDB(t)
	createTheme(t, "Ocean Blue")

	recorder := runThemeHandler(t, CreateCustomThemeHandler, http.MethodPost, "/api/v1/themes",
		marshalThemeBody(t, writeRequest{Name: "  ocean   blue  ", Definition: validDefinition()}), nil)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Code      string   `json:"code"`
		Conflicts []string `json:"conflicts"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != codeDuplicateName || len(response.Conflicts) != 1 || response.Conflicts[0] != "ocean blue" {
		t.Fatalf("unexpected duplicate response: %+v", response)
	}
}

func TestUpdateAndDeleteTheme(t *testing.T) {
	db := setupThemeTestDB(t)
	created := createTheme(t, "Ocean")

	updatedDefinition := validDefinition()
	updatedDefinition.Colors.Primary = "#33aa88"
	target := "/api/v1/themes/1"
	recorder := runThemeHandler(t, UpdateCustomThemeHandler, http.MethodPut, target,
		marshalThemeBody(t, writeRequest{Name: "Ocean Night", Definition: updatedDefinition}),
		gin.Params{{Key: "id", Value: "1"}})
	if recorder.Code != http.StatusOK {
		t.Fatalf("update returned %d: %s", recorder.Code, recorder.Body.String())
	}

	var record database.CustomTheme
	if err := db.First(&record, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if record.Name != "Ocean Night" || !strings.Contains(record.Definition, "#33aa88") {
		t.Fatalf("update not persisted: %+v", record)
	}

	recorder = runThemeHandler(t, DeleteCustomThemeHandler, http.MethodDelete, target, "",
		gin.Params{{Key: "id", Value: "1"}})
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete returned %d: %s", recorder.Code, recorder.Body.String())
	}
	recorder = runThemeHandler(t, DeleteCustomThemeHandler, http.MethodDelete, target, "",
		gin.Params{{Key: "id", Value: "1"}})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("second delete returned %d, expected 404", recorder.Code)
	}
}

func TestUpdateThemeRejectsDuplicateName(t *testing.T) {
	setupThemeTestDB(t)
	createTheme(t, "Ocean")
	createTheme(t, "Forest")

	recorder := runThemeHandler(t, UpdateCustomThemeHandler, http.MethodPut, "/api/v1/themes/2",
		marshalThemeBody(t, writeRequest{Name: " ocean ", Definition: validDefinition()}),
		gin.Params{{Key: "id", Value: "2"}})
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestImportRejectsConflictsAtomically(t *testing.T) {
	setupThemeTestDB(t)
	createTheme(t, "Ocean")

	importBody := marshalThemeBody(t, importRequest{Themes: []writeRequest{
		{Name: "Forest", Definition: validDefinition()},
		{Name: "ocean", Definition: validDefinition()},
	}})
	recorder := runThemeHandler(t, ImportCustomThemesHandler, http.MethodPost, "/api/v1/themes/import", importBody, nil)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = runThemeHandler(t, ListCustomThemesHandler, http.MethodGet, "/api/v1/themes", "", nil)
	var response struct {
		Data []themePayload `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data) != 1 || response.Data[0].Name != "Ocean" {
		t.Fatalf("conflicting import should not persist any theme: %+v", response.Data)
	}
}

func TestImportRejectsDuplicatesInsideBundle(t *testing.T) {
	setupThemeTestDB(t)

	importBody := marshalThemeBody(t, importRequest{Themes: []writeRequest{
		{Name: "Twin", Definition: validDefinition()},
		{Name: " twin ", Definition: validDefinition()},
	}})
	recorder := runThemeHandler(t, ImportCustomThemesHandler, http.MethodPost, "/api/v1/themes/import", importBody, nil)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestImportCreatesThemes(t *testing.T) {
	db := setupThemeTestDB(t)

	importBody := marshalThemeBody(t, importRequest{Themes: []writeRequest{
		{Name: "Forest", Definition: validDefinition()},
		{Name: "Sand", Definition: validDefinition()},
	}})
	recorder := runThemeHandler(t, ImportCustomThemesHandler, http.MethodPost, "/api/v1/themes/import", importBody, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("import returned %d: %s", recorder.Code, recorder.Body.String())
	}

	var count int64
	if err := db.Model(&database.CustomTheme{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 imported themes, got %d", count)
	}
}
