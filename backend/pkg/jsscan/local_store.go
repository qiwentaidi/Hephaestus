package jsscan

import (
	tbdb "github.com/qiwentaidi/trailblazer/pkg/core/database"
	sdk "github.com/qiwentaidi/trailblazer/pkg/sdk"
	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
)

type localScanDataStore struct {
	db *gorm.DB
}

func NewLocalScanDataStore(db *gorm.DB) *localScanDataStore {
	return &localScanDataStore{db: db}
}

func (s *localScanDataStore) ListJSResources(taskID string, versions ...int) ([]sdk.JSResource, error) {
	query := s.db.Model(&database.VulnScanJSResource{}).Where("task_id = ?", taskID)
	if len(versions) > 0 && versions[0] > 0 {
		query = query.Where("version = ?", versions[0])
	}

	var rows []database.VulnScanJSResource
	if err := query.Order("fetched_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}

	result := make([]sdk.JSResource, 0, len(rows))
	for _, row := range rows {
		result = append(result, tbdb.JSResource{
			TaskID:       row.TaskID,
			Version:      row.Version,
			URL:          row.URL,
			Content:      row.Content,
			ResponseCode: row.ResponseCode,
			Headers:      row.Headers,
			Size:         row.Size,
			FetchedAt:    row.FetchedAt,
		})
	}
	return result, nil
}

func (s *localScanDataStore) ListAPIResources(taskID string, versions ...int) ([]sdk.APIResource, error) {
	query := s.db.Model(&database.VulnScanJSAPIResource{}).Where("task_id = ?", taskID)
	if len(versions) > 0 && versions[0] > 0 {
		query = query.Where("version = ?", versions[0])
	}

	var rows []database.VulnScanJSAPIResource
	if err := query.Order("fetched_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}

	result := make([]sdk.APIResource, 0, len(rows))
	for _, row := range rows {
		result = append(result, tbdb.APIResource{
			TaskID:           row.TaskID,
			Version:          row.Version,
			URL:              row.URL,
			Method:           row.Method,
			TraceID:          row.TraceID,
			HasProtocolTrace: row.HasProtocolTrace,
			RequestHeaders:   row.RequestHeaders,
			RequestBody:      row.RequestBody,
			ResponseHeaders:  row.ResponseHeaders,
			ResponseBody:     row.ResponseBody,
			ResponseCode:     row.ResponseCode,
			Headers:          row.Headers,
			FetchedAt:        row.FetchedAt,
		})
	}
	return result, nil
}

func (s *localScanDataStore) ListProtocolTraces(taskID string, versions ...int) ([]sdk.ProtocolTraceRecord, error) {
	return []sdk.ProtocolTraceRecord{}, nil
}
