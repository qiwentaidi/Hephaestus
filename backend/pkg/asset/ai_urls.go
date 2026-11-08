package asset

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
	"hephaestus-backend/pkg/database"
)

// AIURL keeps provenance separate from scanner task IDs. It never executes a
// request or infers a vulnerability: it only records an observed HTTP(S) URL.
type AIURL struct {
	URL, ProjectID, ConversationID, FindingID, ExecutionID, ToolName string
	ObservedAt                                                       time.Time
}

func (s Store) RecordAIURL(ctx context.Context, input AIURL) (bool, error) {
	origin, err := WebsiteOrigin(input.URL)
	if err != nil {
		return false, nil
	} // Credentials and non-web targets aren't websites.
	parsed, err := url.Parse(strings.TrimSpace(input.URL))
	if err != nil {
		return false, nil
	}
	// Never copy HTTP basic-auth credentials into asset names.
	parsed.User = nil
	normalized, err := Normalize("", TypeURL, parsed.String())
	if err != nil {
		return false, nil
	}
	sourceID, evidenceRef := "", ""
	switch {
	case input.FindingID != "":
		sourceID, evidenceRef = "finding:"+input.FindingID, "finding:"+input.FindingID
	case input.ExecutionID != "":
		sourceID, evidenceRef = "execution:"+input.ExecutionID, "tool_call:"+input.ExecutionID
	default:
		return false, fmt.Errorf("AI URL requires finding or execution provenance")
	}
	if input.ObservedAt.IsZero() {
		return false, fmt.Errorf("AI URL requires observation time")
	}
	obs := Observation{Type: TypeURL, Value: normalized.CanonicalValue, Source: "ai", SourceID: sourceID,
		Origin: "scan", Confidence: "medium", ObservedAt: input.ObservedAt, EvidenceRef: evidenceRef,
		Attributes: map[string]any{"projectId": input.ProjectID, "conversationId": input.ConversationID,
			"findingId": input.FindingID, "executionId": input.ExecutionID, "tool": input.ToolName, "websiteOrigin": origin},
	}
	inserted := false
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&database.UnifiedAssetObservation{}).Where("observation_key = ?", observationKey(obs, normalized)).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		} // Replays must not reactivate stale/retired assets.
		store := Store{DB: tx}
		assetRow, _, err := store.Upsert(ctx, obs)
		if err != nil {
			return err
		}
		u, _ := url.Parse(origin)
		hostInput := obs
		hostInput.Type, hostInput.Value, hostInput.SourceID = ClassifyHost(u.Hostname()), u.Hostname(), sourceID+":host"
		host, _, err := store.Upsert(ctx, hostInput)
		if err != nil {
			return err
		}
		_, err = store.UpsertRelation(ctx, RelationInput{ScopeKey: DefaultScope, FromAssetID: assetRow.ID, ToAssetID: host.ID,
			RelationType: "endpoint_of", Source: "ai", SourceID: sourceID, ObservedAt: input.ObservedAt})
		if err != nil {
			return err
		}
		inserted = true
		return nil
	})
	return inserted, err
}

type AIURLBackfillSummary struct {
	Examined int `json:"examined"`
	Eligible int `json:"eligible"`
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

// BackfillAIFindingURLs is resumable: stable finding provenance prevents
// duplicate observations across startup, manual repair, and live ingestion.
func (s Store) BackfillAIFindingURLs(ctx context.Context, apply bool) (AIURLBackfillSummary, error) {
	var summary AIURLBackfillSummary
	if !s.DB.Migrator().HasTable("pentest_findings") {
		return summary, nil
	}
	hasSuperseded := s.DB.Migrator().HasColumn("pentest_findings", "superseded_at")
	cursor := ""
	for {
		var batch []struct {
			ID             string
			ProjectID      string
			ConversationID string
			Target         string
			CreatedAt      time.Time
		}
		q := s.DB.WithContext(ctx).Table("pentest_findings").Select("id, project_id, conversation_id, target, created_at").Where("id > ?", cursor).Order("id ASC").Limit(200)
		if hasSuperseded {
			q = q.Where("superseded_at IS NULL")
		}
		if err := q.Find(&batch).Error; err != nil {
			return summary, err
		}
		if len(batch) == 0 {
			break
		}
		for _, row := range batch {
			summary.Examined++
			urls := AIFindingURLs(row.Target)
			if len(urls) == 0 {
				summary.Skipped++
				continue
			}
			summary.Eligible++
			if apply {
				imported := false
				for _, rawURL := range urls {
					added, err := s.RecordAIURL(ctx, AIURL{URL: rawURL, ProjectID: row.ProjectID, ConversationID: row.ConversationID, FindingID: row.ID, ObservedAt: row.CreatedAt})
					if err != nil {
						return summary, fmt.Errorf("backfill finding %s: %w", row.ID, err)
					}
					imported = imported || added
				}
				if imported {
					summary.Imported++
				}
			}
		}
		cursor = batch[len(batch)-1].ID
	}
	return summary, nil
}
