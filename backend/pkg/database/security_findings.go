package database

import "time"

// SecurityFinding is the canonical vulnerability record. Unlike the legacy
// scanner tables it stores one normalized primary asset, one category and a
// deduplicated identity, and it is written once when the finding is observed.
type SecurityFinding struct {
	ID             string    `gorm:"primaryKey;column:id;type:varchar(80)" json:"id"`
	ScopeKey       string    `gorm:"column:scope_key;type:varchar(255);not null;default:'global';index" json:"scopeKey"`
	Source         string    `gorm:"column:source;type:varchar(60);not null;index" json:"source"`
	SourceID       string    `gorm:"column:source_id;type:varchar(255);not null;default:'';index" json:"sourceId"`
	TaskID         string    `gorm:"column:task_id;type:varchar(100);not null;default:'';index" json:"taskId"`
	TaskVersion    int       `gorm:"column:task_version;type:integer;not null;default:0" json:"taskVersion"`
	TemplateID     string    `gorm:"column:template_id;type:varchar(255);not null;default:''" json:"templateId"`
	Title          string    `gorm:"column:title;type:varchar(255);not null;default:''" json:"title"`
	Category       string    `gorm:"column:category;type:varchar(40);not null;default:'web';index" json:"category"`
	TargetType     string    `gorm:"column:target_type;type:varchar(30);not null;default:'url'" json:"targetType"`
	Target         string    `gorm:"column:target;type:text;not null" json:"target"`
	Host           string    `gorm:"column:host;type:varchar(255);not null;default:'';index" json:"host"`
	Port           int       `gorm:"column:port;type:integer;not null;default:0" json:"port"`
	Path           string    `gorm:"column:path;type:text" json:"path"`
	PrimaryAssetID string    `gorm:"column:primary_asset_id;type:varchar(80);not null;index" json:"primaryAssetId"`
	Severity       string    `gorm:"column:severity;type:varchar(20);not null;index" json:"severity"`
	Status         string    `gorm:"column:status;type:varchar(30);not null;default:'open';index" json:"status"`
	Confidence     string    `gorm:"column:confidence;type:varchar(30);not null;default:'medium'" json:"confidence"`
	Evidence       string    `gorm:"column:evidence;type:text" json:"evidence"`
	DedupKey       string    `gorm:"column:dedup_key;type:varchar(191);not null;uniqueIndex" json:"dedupKey"`
	CDNEdge        bool      `gorm:"column:cdn_edge;type:boolean;not null;default:false" json:"cdnEdge"`
	ObservedAt     time.Time `gorm:"column:observed_at;not null;index" json:"observedAt"`
	FirstSeenAt    time.Time `gorm:"column:first_seen_at;not null;index" json:"firstSeenAt"`
	LastSeenAt     time.Time `gorm:"column:last_seen_at;not null;index" json:"lastSeenAt"`
	CreatedAt      time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt      time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (SecurityFinding) TableName() string { return "security_findings" }

// FindingAssetLink is the impact surface of a finding. Exactly one link has
// role=primary; the rest are host/service/resolved_ip/certificate links.
type FindingAssetLink struct {
	ID         string     `gorm:"primaryKey;column:id;type:varchar(80)" json:"id"`
	FindingID  string     `gorm:"column:finding_id;type:varchar(80);not null;index;uniqueIndex:idx_finding_asset_role,priority:1" json:"findingId"`
	AssetID    string     `gorm:"column:asset_id;type:varchar(80);not null;index;uniqueIndex:idx_finding_asset_role,priority:2" json:"assetId"`
	Role       string     `gorm:"column:role;type:varchar(30);not null;uniqueIndex:idx_finding_asset_role,priority:3" json:"role"`
	Direct     bool       `gorm:"column:direct;type:boolean;not null;default:false;index" json:"direct"`
	Resolver   string     `gorm:"column:resolver;type:varchar(30);not null;default:''" json:"resolver"`
	ResolvedAt *time.Time `gorm:"column:resolved_at" json:"resolvedAt,omitempty"`
	Evidence   string     `gorm:"column:evidence;type:text" json:"evidence"`
	CreatedAt  time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"createdAt"`
}

func (FindingAssetLink) TableName() string { return "finding_asset_links" }

// AssetRisk is the read model used by the asset map and asset lists. It is
// recomputed whenever a finding or its links/status change, so queries never
// need to join vulnerability tables or resolve DNS.
type AssetRisk struct {
	AssetID           string    `gorm:"primaryKey;column:asset_id;type:varchar(80)" json:"assetId"`
	DirectCritical    int       `gorm:"column:direct_critical;type:integer;not null;default:0" json:"directCritical"`
	DirectHigh        int       `gorm:"column:direct_high;type:integer;not null;default:0" json:"directHigh"`
	DirectMedium      int       `gorm:"column:direct_medium;type:integer;not null;default:0" json:"directMedium"`
	InheritedCritical int       `gorm:"column:inherited_critical;type:integer;not null;default:0" json:"inheritedCritical"`
	InheritedHigh     int       `gorm:"column:inherited_high;type:integer;not null;default:0" json:"inheritedHigh"`
	InheritedMedium   int       `gorm:"column:inherited_medium;type:integer;not null;default:0" json:"inheritedMedium"`
	OpenFindings      int       `gorm:"column:open_findings;type:integer;not null;default:0" json:"openFindings"`
	UpdatedAt         time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (AssetRisk) TableName() string { return "asset_risk" }
