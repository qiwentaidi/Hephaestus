package asset

import (
	"os"
	"reflect"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestFingerprintStatusFilterPostgres(t *testing.T) {
	dsn := os.Getenv("HEPHAESTUS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set HEPHAESTUS_TEST_POSTGRES_DSN to run PostgreSQL filter regression tests")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })
	// A connection-local fixture never modifies application tables or data.
	if err := tx.Exec("CREATE TEMP TABLE fingerprint_status_fixture (task_id TEXT, version INT, status INT) ON COMMIT DROP").Error; err != nil {
		t.Fatal(err)
	}
	statuses := []int{0, 199, 200, 201, 204, 299, 300, 301, 302, 399, 400, 401, 403, 404, 429, 499, 500, 502, 503, 599, 600}
	for _, status := range statuses {
		if err := tx.Exec("INSERT INTO fingerprint_status_fixture VALUES (?, ?, ?)", "target", 2, status).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Exec("INSERT INTO fingerprint_status_fixture VALUES ('other', 2, 403), ('target', 1, 404)").Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		filter string
		want   []int
	}{
		{"2xx", []int{200, 201, 204, 299}},
		{"3xx", []int{300, 301, 302, 399}},
		{"4xx", []int{400, 401, 403, 404, 429, 499}},
		{"5xx", []int{500, 502, 503, 599}},
		{"400", []int{400}},
		{"404", []int{404}},
		{"", statuses},
		{"all", statuses},
	} {
		t.Run(tc.filter, func(t *testing.T) {
			query := ApplyFingerprintStatusFilter(tx.Table("fingerprint_status_fixture").Where("task_id = ? AND version = ?", "target", 2), tc.filter)
			var total int64
			if err := query.Count(&total).Error; err != nil {
				t.Fatal(err)
			}
			if total != int64(len(tc.want)) {
				t.Fatalf("total = %d, want %d", total, len(tc.want))
			}
			var got []int
			for offset := 0; offset < int(total); offset += 2 {
				var page []int
				if err := query.Session(&gorm.Session{}).Order("status ASC").Offset(offset).Limit(2).Pluck("status", &page).Error; err != nil {
					t.Fatal(err)
				}
				got = append(got, page...)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("paginated statuses = %v, want %v", got, tc.want)
			}
		})
	}
}
