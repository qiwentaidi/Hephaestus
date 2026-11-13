package toolbox

import (
	"fmt"
	"testing"
	"time"

	"hephaestus-backend/pkg/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestAPIDocDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.APIDocTestTask{}); err != nil {
		t.Fatalf("migrate api_doc_test_task: %v", err)
	}
	return db
}

func TestUpdateAPIDocTestTaskPersistsHeaders(t *testing.T) {
	db := newTestAPIDocDB(t)

	task := database.APIDocTestTask{
		ID:           "api_doc_test_1",
		TaskName:     "api.example.com",
		Status:       "completed",
		SourceURL:    "https://api.example.com/swagger.json",
		BaseURL:      "https://api.example.com",
		DocumentType: "swagger",
		Headers:      map[string]string{"Authorization": "Bearer old"},
		MaxEndpoints: 120,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("seed task: %v", err)
	}

	req := apiDocTestTaskCreateRequest{
		SourceURL:      "https://api.example.com/openapi.json",
		BaseURL:        "https://api.example.com/v1",
		DocumentType:   "openapi",
		Headers:        map[string]string{"Authorization": "Bearer new", "Cookie": "session=abc"},
		MaxEndpoints:   42,
		TimeoutSeconds: 15,
	}
	now := time.Now()
	if err := updateAPIDocTestTask(db, task.ID, req, now); err != nil {
		t.Fatalf("update task: %v", err)
	}

	var stored database.APIDocTestTask
	if err := db.Where("id = ?", task.ID).First(&stored).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if stored.SourceURL != req.SourceURL {
		t.Fatalf("source_url = %q, want %q", stored.SourceURL, req.SourceURL)
	}
	if stored.BaseURL != req.BaseURL {
		t.Fatalf("base_url = %q, want %q", stored.BaseURL, req.BaseURL)
	}
	if stored.DocumentType != req.DocumentType {
		t.Fatalf("document_type = %q, want %q", stored.DocumentType, req.DocumentType)
	}
	if stored.MaxEndpoints != req.MaxEndpoints {
		t.Fatalf("max_endpoints = %d, want %d", stored.MaxEndpoints, req.MaxEndpoints)
	}
	if got := stored.Headers["Authorization"]; got != "Bearer new" {
		t.Fatalf("headers[Authorization] = %q, want %q", got, "Bearer new")
	}
	if got := stored.Headers["Cookie"]; got != "session=abc" {
		t.Fatalf("headers[Cookie] = %q, want %q", got, "session=abc")
	}
	if len(stored.Headers) != 2 {
		t.Fatalf("headers = %v, want exactly 2 entries", stored.Headers)
	}
}

func TestUpdateAPIDocTestTaskClearsZeroValues(t *testing.T) {
	db := newTestAPIDocDB(t)

	task := database.APIDocTestTask{
		ID:           "api_doc_test_2",
		TaskName:     "api.example.com",
		Status:       "failed",
		SourceURL:    "https://api.example.com/swagger.json",
		BaseURL:      "https://api.example.com",
		DocumentType: "swagger",
		Headers:      map[string]string{"Authorization": "Bearer old"},
		MaxEndpoints: 120,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("seed task: %v", err)
	}

	req := apiDocTestTaskCreateRequest{
		SourceURL:    "https://api.example.com/swagger.json",
		BaseURL:      "  ",
		DocumentType: "auto",
		Headers:      nil,
		MaxEndpoints: 10,
	}
	if err := updateAPIDocTestTask(db, task.ID, req, time.Now()); err != nil {
		t.Fatalf("update task: %v", err)
	}

	var stored database.APIDocTestTask
	if err := db.Where("id = ?", task.ID).First(&stored).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if stored.BaseURL != "" {
		t.Fatalf("base_url = %q, want empty", stored.BaseURL)
	}
	if len(stored.Headers) != 0 {
		t.Fatalf("headers = %v, want empty", stored.Headers)
	}
	if stored.MaxEndpoints != 10 {
		t.Fatalf("max_endpoints = %d, want 10", stored.MaxEndpoints)
	}
}
