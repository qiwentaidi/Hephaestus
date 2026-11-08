package asset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
)

// AssetUpdate contains the user-managed fields of a ledger asset. Observation
// ingestion never touches these fields, so names, notes, tags, groups and the
// archive state survive later scans.
type AssetUpdate struct {
	Name     *string
	Notes    *string
	Tags     *[]string
	GroupKey *string
	Archived *bool
}

func normalizeTags(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	tags := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len([]rune(value)) > 64 {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		tags = append(tags, value)
		if len(tags) >= 50 {
			break
		}
	}
	return tags
}

func (s Store) UpdateAsset(ctx context.Context, id string, input AssetUpdate) (database.UnifiedAsset, error) {
	if s.DB == nil {
		return database.UnifiedAsset{}, errors.New("asset store database is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return database.UnifiedAsset{}, errors.New("asset id is required")
	}
	var assetRow database.UnifiedAsset
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).First(&assetRow).Error; err != nil {
			return err
		}
		before := assetState(assetRow)
		now := time.Now().UTC()
		updates := map[string]any{"updated_at": now}
		changeType := "updated"
		if input.Name != nil {
			name := strings.TrimSpace(*input.Name)
			if len([]rune(name)) > 255 {
				return errors.New("asset name must be at most 255 characters")
			}
			updates["name"] = name
			assetRow.Name = name
		}
		if input.Notes != nil {
			notes := strings.TrimSpace(*input.Notes)
			if len([]rune(notes)) > 10000 {
				return errors.New("asset notes must be at most 10000 characters")
			}
			updates["notes"] = notes
			assetRow.Notes = notes
		}
		if input.GroupKey != nil {
			groupKey := strings.TrimSpace(*input.GroupKey)
			if len([]rune(groupKey)) > 255 {
				return errors.New("asset group key must be at most 255 characters")
			}
			updates["group_key"] = groupKey
			assetRow.GroupKey = groupKey
		}
		if input.Tags != nil {
			tags := normalizeTags(*input.Tags)
			encoded, err := json.Marshal(tags)
			if err != nil {
				return err
			}
			updates["tags"] = string(encoded)
			assetRow.Tags = tags
		}
		if input.Archived != nil {
			if *input.Archived && assetRow.ArchivedAt == nil {
				now := time.Now().UTC()
				updates["archived_at"] = now
				assetRow.ArchivedAt = &now
				changeType = "archived"
			} else if !*input.Archived && assetRow.ArchivedAt != nil {
				updates["archived_at"] = nil
				assetRow.ArchivedAt = nil
				changeType = "unarchived"
			}
		}
		if len(updates) == 1 {
			return nil
		}
		assetRow.UpdatedAt = now
		if err := tx.Model(&database.UnifiedAsset{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		if err := recordAssetChange(tx, assetRow, changeType, "manual", "", 0, now, before, assetState(assetRow)); err != nil {
			return err
		}
		return tx.Where("id = ?", id).First(&assetRow).Error
	})
	return assetRow, err
}

type LedgerFilter struct {
	Type            string
	Status          string
	Verification    string
	Source          string
	Keyword         string
	GroupKey        string
	IncludeArchived bool
	Limit           int
	Offset          int
}

type LedgerItem struct {
	database.UnifiedAsset
	Sources          []string  `json:"sources"`
	ObservationCount int       `json:"observationCount"`
	TaskCount        int       `json:"taskCount"`
	LastTaskID       string    `json:"lastTaskId,omitempty"`
	LastTaskName     string    `json:"lastTaskName,omitempty"`
	LastObservedAt   time.Time `json:"lastObservedAt,omitempty"`
	Archived         bool      `json:"archived"`
}

type LedgerSummary struct {
	Total    int64 `json:"total"`
	Domains  int64 `json:"domains"`
	IPs      int64 `json:"ips"`
	URLs     int64 `json:"urls"`
	Declared int64 `json:"declared"`
	Observed int64 `json:"observed"`
	Verified int64 `json:"verified"`
	Manual   int64 `json:"manual"`
	Imported int64 `json:"imported"`
	Scanned  int64 `json:"scanned"`
	Archived int64 `json:"archived"`
}

func ObservationTaskID(observation database.UnifiedAssetObservation) string {
	if taskID := strings.TrimSpace(observation.TaskID); taskID != "" {
		return taskID
	}
	if observation.Attributes != nil {
		if value, ok := observation.Attributes["taskId"]; ok {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func ObservationTaskVersion(observation database.UnifiedAssetObservation) int {
	if observation.TaskVersion > 0 {
		return observation.TaskVersion
	}
	if observation.Attributes != nil {
		if value, ok := observation.Attributes["version"]; ok {
			switch typed := value.(type) {
			case int:
				return typed
			case int64:
				return int(typed)
			case float64:
				return int(typed)
			default:
				var parsed int
				if _, err := fmt.Sscanf(strings.TrimSpace(fmt.Sprint(value)), "%d", &parsed); err == nil {
					return parsed
				}
			}
		}
	}
	return observation.SourceVersion
}

func ObservationOrigin(observation database.UnifiedAssetObservation) string {
	if origin := strings.ToLower(strings.TrimSpace(observation.Origin)); origin != "" {
		return origin
	}
	switch strings.ToLower(strings.TrimSpace(observation.Source)) {
	case "manual":
		return "manual"
	case "import", "csv", "excel":
		return "import"
	default:
		return "scan"
	}
}

func (s Store) ListLedger(ctx context.Context, filter LedgerFilter) ([]LedgerItem, int64, error) {
	if s.DB == nil {
		return nil, 0, errors.New("asset store database is nil")
	}
	query := s.DB.WithContext(ctx).Model(&database.UnifiedAsset{})
	if !filter.IncludeArchived {
		query = query.Where("archived_at IS NULL")
	}
	if value := strings.ToLower(strings.TrimSpace(filter.Type)); value != "" && value != "all" {
		query = query.Where("type = ?", value)
	}
	if value := strings.ToLower(strings.TrimSpace(filter.Status)); value != "" && value != "all" {
		query = query.Where("status = ?", value)
	}
	if value := strings.ToLower(strings.TrimSpace(filter.Verification)); value != "" && value != "all" {
		query = query.Where("verification = ?", value)
	}
	if value := strings.TrimSpace(filter.GroupKey); value != "" {
		query = query.Where("group_key = ?", value)
	}
	if value := strings.TrimSpace(filter.Keyword); value != "" {
		like := "%" + value + "%"
		query = query.Where("canonical_value LIKE ? OR display_value LIKE ? OR name LIKE ?", like, like, like)
	}
	if value := strings.ToLower(strings.TrimSpace(filter.Source)); value != "" && value != "all" {
		subquery := s.DB.WithContext(ctx).Model(&database.UnifiedAssetObservation{}).Select("asset_id")
		switch value {
		case "manual", "import":
			subquery = subquery.Where("origin = ?", value)
		case "scan":
			subquery = subquery.Where("source NOT IN ?", []string{"manual", "import", "csv", "excel"})
		default:
			subquery = subquery.Where("source = ?", value)
		}
		query = query.Where("id IN (?)", subquery)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	var assets []database.UnifiedAsset
	if err := query.Order("last_seen_at DESC, canonical_value ASC").Limit(limit).Offset(offset).Find(&assets).Error; err != nil {
		return nil, 0, err
	}
	items := make([]LedgerItem, 0, len(assets))
	if len(assets) == 0 {
		return items, total, nil
	}
	ids := make([]string, 0, len(assets))
	for _, assetRow := range assets {
		ids = append(ids, assetRow.ID)
	}
	var observations []database.UnifiedAssetObservation
	if err := s.DB.WithContext(ctx).Where("asset_id IN ?", ids).Order("observed_at DESC, created_at DESC").Find(&observations).Error; err != nil {
		return nil, 0, err
	}
	byAsset := make(map[string][]database.UnifiedAssetObservation, len(assets))
	taskIDs := make([]string, 0, len(assets))
	taskSeen := map[string]struct{}{}
	for _, observation := range observations {
		byAsset[observation.AssetID] = append(byAsset[observation.AssetID], observation)
		if taskID := ObservationTaskID(observation); taskID != "" {
			if _, exists := taskSeen[taskID]; !exists {
				taskSeen[taskID] = struct{}{}
				taskIDs = append(taskIDs, taskID)
			}
		}
	}
	taskNames := map[string]string{}
	const taskChunkSize = 500
	for start := 0; start < len(taskIDs); start += taskChunkSize {
		end := start + taskChunkSize
		if end > len(taskIDs) {
			end = len(taskIDs)
		}
		var tasks []database.VulnScanTask
		if err := s.DB.WithContext(ctx).Select("id", "version", "task_name").Where("id IN ?", taskIDs[start:end]).Order("version DESC").Find(&tasks).Error; err != nil {
			return nil, 0, err
		}
		for _, task := range tasks {
			if _, exists := taskNames[task.ID]; !exists {
				taskNames[task.ID] = strings.TrimSpace(task.TaskName)
			}
		}
	}
	for _, assetRow := range assets {
		item := LedgerItem{UnifiedAsset: assetRow, Sources: []string{}, Archived: assetRow.ArchivedAt != nil}
		sourceSeen := map[string]struct{}{}
		tasksForAsset := map[string]struct{}{}
		for _, observation := range byAsset[assetRow.ID] {
			item.ObservationCount++
			if observation.ObservedAt.After(item.LastObservedAt) {
				item.LastObservedAt = observation.ObservedAt
			}
			source := strings.TrimSpace(observation.Source)
			if source == "" {
				source = ObservationOrigin(observation)
			}
			if _, exists := sourceSeen[source]; !exists {
				sourceSeen[source] = struct{}{}
				item.Sources = append(item.Sources, source)
			}
			if taskID := ObservationTaskID(observation); taskID != "" {
				tasksForAsset[taskID] = struct{}{}
				if item.LastTaskID == "" {
					item.LastTaskID = taskID
					item.LastTaskName = taskNames[taskID]
				}
			}
		}
		sort.Strings(item.Sources)
		item.TaskCount = len(tasksForAsset)
		if item.Verification == "" {
			item.Verification = "declared"
			for _, source := range item.Sources {
				if source != "manual" && source != "import" {
					item.Verification = "observed"
					break
				}
			}
		}
		if item.LastObservedAt.IsZero() {
			item.LastObservedAt = item.LastSeenAt
		}
		items = append(items, item)
	}
	return items, total, nil
}

func (s Store) LedgerSummary(ctx context.Context, includeArchived bool) (LedgerSummary, error) {
	if s.DB == nil {
		return LedgerSummary{}, errors.New("asset store database is nil")
	}
	base := func() *gorm.DB {
		query := s.DB.WithContext(ctx).Model(&database.UnifiedAsset{})
		if !includeArchived {
			query = query.Where("archived_at IS NULL")
		}
		return query
	}
	summary := LedgerSummary{}
	if err := base().Count(&summary.Total).Error; err != nil {
		return summary, err
	}
	if err := s.DB.WithContext(ctx).Model(&database.UnifiedAsset{}).Where("archived_at IS NOT NULL").Count(&summary.Archived).Error; err != nil {
		return summary, err
	}
	countBy := func(column, value string) (int64, error) {
		var count int64
		err := base().Where(column+" = ?", value).Count(&count).Error
		return count, err
	}
	var err error
	if summary.Domains, err = countBy("type", TypeDomain); err != nil {
		return summary, err
	}
	if summary.IPs, err = countBy("type", TypeIP); err != nil {
		return summary, err
	}
	if summary.URLs, err = countBy("type", TypeURL); err != nil {
		return summary, err
	}
	if summary.Declared, err = countBy("verification", "declared"); err != nil {
		return summary, err
	}
	if summary.Observed, err = countBy("verification", "observed"); err != nil {
		return summary, err
	}
	if summary.Verified, err = countBy("verification", "verified"); err != nil {
		return summary, err
	}
	countSource := func(sources []string, negate bool) (int64, error) {
		subquery := s.DB.WithContext(ctx).Model(&database.UnifiedAssetObservation{}).Select("asset_id")
		if negate {
			subquery = subquery.Where("source NOT IN ?", sources)
		} else {
			subquery = subquery.Where("source IN ?", sources)
		}
		var count int64
		err := base().Where("id IN (?)", subquery).Count(&count).Error
		return count, err
	}
	if summary.Manual, err = countSource([]string{"manual"}, false); err != nil {
		return summary, err
	}
	if summary.Imported, err = countSource([]string{"import", "csv", "excel"}, false); err != nil {
		return summary, err
	}
	if summary.Scanned, err = countSource([]string{"manual", "import", "csv", "excel"}, true); err != nil {
		return summary, err
	}
	return summary, nil
}
