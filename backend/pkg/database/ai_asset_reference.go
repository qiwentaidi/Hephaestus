package database

import "time"

// AIAssetReference stores either a fixed selection snapshot or task/version
// references whose results are read on demand. Empty ReadMode is a legacy snapshot.
type AIAssetReference struct {
	ID             string                   `gorm:"primaryKey;type:varchar(255)" json:"id"`
	ConversationID string                   `gorm:"index;not null" json:"-"`
	TaskID         string                   `gorm:"index;not null" json:"taskId"`
	Version        int                      `json:"version"`
	Name           string                   `json:"name"`
	ScanType       string                   `json:"scanType"`
	Status         string                   `json:"status"`
	Selection      string                   `json:"selection"`
	Query          string                   `json:"query"`
	Kind           string                   `json:"kind"`
	Count          int                      `json:"count"`
	ReadMode       string                   `json:"readMode,omitempty"`
	Sources        []AIAssetReferenceSource `gorm:"serializer:json;type:text" json:"sources,omitempty"`
	AssetsJSON     string                   `gorm:"type:text;not null" json:"-"`
	CreatedAt      time.Time                `json:"capturedAt"`
}

type AIAssetReferenceSource struct {
	TaskID   string `json:"taskId"`
	Version  int    `json:"version"`
	ScanType string `json:"scanType"`
}

func (AIAssetReference) TableName() string { return "ai_asset_references" }
