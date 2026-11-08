package database

import "time"

// UnifiedAsset is the canonical identity of an observed attack-surface asset.
// The scope key keeps the global desktop deployment compatible while allowing
// projects or tenants to use isolated asset namespaces later.
type UnifiedAsset struct {
	ID             string         `gorm:"primaryKey;column:id;type:varchar(80)" json:"id"`
	ScopeKey       string         `gorm:"column:scope_key;type:varchar(255);not null;default:'global';uniqueIndex:idx_unified_asset_identity,priority:1" json:"scopeKey"`
	Type           string         `gorm:"column:type;type:varchar(40);not null;uniqueIndex:idx_unified_asset_identity,priority:2;index" json:"type"`
	CanonicalValue string         `gorm:"column:canonical_value;type:text;not null;uniqueIndex:idx_unified_asset_identity,priority:3" json:"canonicalValue"`
	DisplayValue   string         `gorm:"column:display_value;type:text;not null" json:"displayValue"`
	Name           string         `gorm:"column:name;type:varchar(255);not null;default:''" json:"name"`
	Notes          string         `gorm:"column:notes;type:text" json:"notes"`
	Tags           []string       `gorm:"column:tags;type:text;serializer:json" json:"tags"`
	GroupKey       string         `gorm:"column:group_key;type:varchar(255);not null;default:'';index" json:"groupKey"`
	Verification   string         `gorm:"column:verification;type:varchar(30);not null;default:'';index" json:"verification"`
	Status         string         `gorm:"column:status;type:varchar(30);not null;default:'active';index" json:"status"`
	Confidence     string         `gorm:"column:confidence;type:varchar(30);not null;default:'medium';index" json:"confidence"`
	ArchivedAt     *time.Time     `gorm:"column:archived_at;index" json:"archivedAt,omitempty"`
	Metadata       map[string]any `gorm:"column:metadata;type:text;serializer:json" json:"metadata"`
	FirstSeenAt    time.Time      `gorm:"column:first_seen_at;not null;index" json:"firstSeenAt"`
	LastSeenAt     time.Time      `gorm:"column:last_seen_at;not null;index" json:"lastSeenAt"`
	CreatedAt      time.Time      `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt      time.Time      `gorm:"column:updated_at;default:CURRENT_TIMESTAMP;index" json:"updatedAt"`
}

func (UnifiedAsset) TableName() string { return "unified_assets" }

// UnifiedAssetObservation preserves provenance for an asset identity. The
// deterministic observation key makes retries safe without losing source
// history or evidence references.
type UnifiedAssetObservation struct {
	ID             string         `gorm:"primaryKey;column:id;type:varchar(80)" json:"id"`
	AssetID        string         `gorm:"column:asset_id;type:varchar(80);not null;index" json:"assetId"`
	ScopeKey       string         `gorm:"column:scope_key;type:varchar(255);not null;default:'global';index" json:"scopeKey"`
	Type           string         `gorm:"column:type;type:varchar(40);not null;index" json:"type"`
	CanonicalValue string         `gorm:"column:canonical_value;type:text;not null;index" json:"canonicalValue"`
	Source         string         `gorm:"column:source;type:varchar(100);not null;index" json:"source"`
	SourceID       string         `gorm:"column:source_id;type:text;default:'';index" json:"sourceId"`
	SourceVersion  int            `gorm:"column:source_version;default:0;index" json:"sourceVersion"`
	TaskID         string         `gorm:"column:task_id;type:varchar(100);not null;default:'';index" json:"taskId"`
	TaskVersion    int            `gorm:"column:task_version;type:integer;not null;default:0" json:"taskVersion"`
	ImportBatchID  string         `gorm:"column:import_batch_id;type:varchar(80);not null;default:'';index" json:"importBatchId"`
	Origin         string         `gorm:"column:origin;type:varchar(30);not null;default:'';index" json:"origin"`
	ObservationKey string         `gorm:"column:observation_key;type:varchar(64);not null;uniqueIndex" json:"observationKey"`
	Confidence     string         `gorm:"column:confidence;type:varchar(30);not null;default:'medium';index" json:"confidence"`
	ObservedAt     time.Time      `gorm:"column:observed_at;not null;index" json:"observedAt"`
	ExpiresAt      *time.Time     `gorm:"column:expires_at;index" json:"expiresAt,omitempty"`
	EvidenceRef    string         `gorm:"column:evidence_ref;type:text" json:"evidenceRef"`
	Attributes     map[string]any `gorm:"column:attributes;type:text;serializer:json" json:"attributes"`
	CreatedAt      time.Time      `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
}

func (UnifiedAssetObservation) TableName() string { return "unified_asset_observations" }

// UnifiedAssetRelation stores a directed, provenance-backed relationship
// between two canonical assets, such as domain -> IP or host -> service.
type UnifiedAssetRelation struct {
	ID            string         `gorm:"primaryKey;column:id;type:varchar(80)" json:"id"`
	ScopeKey      string         `gorm:"column:scope_key;type:varchar(255);not null;default:'global';uniqueIndex:idx_unified_asset_relation_identity,priority:1;index" json:"scopeKey"`
	FromAssetID   string         `gorm:"column:from_asset_id;type:varchar(80);not null;uniqueIndex:idx_unified_asset_relation_identity,priority:2;index" json:"fromAssetId"`
	ToAssetID     string         `gorm:"column:to_asset_id;type:varchar(80);not null;uniqueIndex:idx_unified_asset_relation_identity,priority:3;index" json:"toAssetId"`
	RelationType  string         `gorm:"column:relation_type;type:varchar(60);not null;uniqueIndex:idx_unified_asset_relation_identity,priority:4;index" json:"relationType"`
	Source        string         `gorm:"column:source;type:varchar(100);not null;index" json:"source"`
	SourceID      string         `gorm:"column:source_id;type:text;default:'';index" json:"sourceId"`
	SourceVersion int            `gorm:"column:source_version;default:0;index" json:"sourceVersion"`
	Active        bool           `gorm:"column:active;not null;default:true;index" json:"active"`
	FirstSeenAt   time.Time      `gorm:"column:first_seen_at;not null;index" json:"firstSeenAt"`
	LastSeenAt    time.Time      `gorm:"column:last_seen_at;not null;index" json:"lastSeenAt"`
	Metadata      map[string]any `gorm:"column:metadata;type:text;serializer:json" json:"metadata"`
	CreatedAt     time.Time      `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
	UpdatedAt     time.Time      `gorm:"column:updated_at;default:CURRENT_TIMESTAMP;index" json:"updatedAt"`
}

func (UnifiedAssetRelation) TableName() string { return "unified_asset_relations" }

// UnifiedAssetChange is an append-only lifecycle/change timeline entry. The
// change key makes retries of the same observation safe.
type UnifiedAssetChange struct {
	ID             string         `gorm:"primaryKey;column:id;type:varchar(80)" json:"id"`
	ChangeKey      string         `gorm:"column:change_key;type:varchar(64);not null;uniqueIndex" json:"changeKey"`
	ScopeKey       string         `gorm:"column:scope_key;type:varchar(255);not null;index" json:"scopeKey"`
	AssetID        string         `gorm:"column:asset_id;type:varchar(80);not null;index" json:"assetId"`
	AssetType      string         `gorm:"column:asset_type;type:varchar(40);not null;index" json:"assetType"`
	CanonicalValue string         `gorm:"column:canonical_value;type:text;not null;index" json:"canonicalValue"`
	ChangeType     string         `gorm:"column:change_type;type:varchar(40);not null;index" json:"changeType"`
	Source         string         `gorm:"column:source;type:varchar(100);default:'';index" json:"source"`
	SourceID       string         `gorm:"column:source_id;type:text;default:'';index" json:"sourceId"`
	SourceVersion  int            `gorm:"column:source_version;default:0;index" json:"sourceVersion"`
	BeforeJSON     map[string]any `gorm:"column:before_json;type:text;serializer:json" json:"before,omitempty"`
	AfterJSON      map[string]any `gorm:"column:after_json;type:text;serializer:json" json:"after,omitempty"`
	DetectedAt     time.Time      `gorm:"column:detected_at;not null;index" json:"detectedAt"`
	CreatedAt      time.Time      `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index" json:"createdAt"`
}

func (UnifiedAssetChange) TableName() string { return "unified_asset_changes" }
