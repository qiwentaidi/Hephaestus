package findings

import (
	"net"
	"strings"
	"time"

	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
)

type AssetRiskRow struct {
	AssetType         string
	AssetValue        string
	DirectCritical    int
	DirectHigh        int
	DirectMedium      int
	InheritedCritical int
	InheritedHigh     int
	InheritedMedium   int
	OpenFindings      int
}

// LoadRiskIndex reads the precomputed projection. The boolean reports whether
// the projection tables exist; an empty projection is meaningful and must not
// fall back to legacy scan counters.
func LoadRiskIndex(db *gorm.DB) ([]AssetRiskRow, bool, error) {
	if db == nil || !db.Migrator().HasTable(&database.AssetRisk{}) || !db.Migrator().HasTable(&database.UnifiedAsset{}) {
		return nil, false, nil
	}
	var rows []struct {
		AssetType         string
		AssetValue        string
		DirectCritical    int
		DirectHigh        int
		DirectMedium      int
		InheritedCritical int
		InheritedHigh     int
		InheritedMedium   int
		OpenFindings      int
	}
	err := db.Table("asset_risk AS r").
		Select("a.type AS asset_type, a.canonical_value AS asset_value, r.direct_critical, r.direct_high, r.direct_medium, r.inherited_critical, r.inherited_high, r.inherited_medium, r.open_findings").
		Joins("JOIN unified_assets a ON a.id = r.asset_id").
		Scan(&rows).Error
	if err != nil {
		return nil, true, err
	}
	result := make([]AssetRiskRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, AssetRiskRow{
			AssetType:         row.AssetType,
			AssetValue:        row.AssetValue,
			DirectCritical:    row.DirectCritical,
			DirectHigh:        row.DirectHigh,
			DirectMedium:      row.DirectMedium,
			InheritedCritical: row.InheritedCritical,
			InheritedHigh:     row.InheritedHigh,
			InheritedMedium:   row.InheritedMedium,
			OpenFindings:      row.OpenFindings,
		})
	}
	return result, true, nil
}

type RiskSummary struct {
	Total       int
	IP          int
	Domain      int
	URL         int
	Service     int
	Certificate int
	Critical    int
	High        int
	Medium      int
}

func SummarizeRisk(db *gorm.DB) (RiskSummary, error) {
	summary := RiskSummary{}
	if db == nil || !db.Migrator().HasTable(&database.SecurityFinding{}) {
		return summary, nil
	}
	var rows []struct {
		ID           string
		PrimaryType  string
		PrimaryValue string
		Severity     string
		Host         string
	}
	err := db.Table("security_findings AS f").
		Select("f.id, a.type AS primary_type, a.canonical_value AS primary_value, f.severity, f.host").
		Joins("JOIN unified_assets a ON a.id = f.primary_asset_id").
		Where("f.status IN ?", []string{StatusOpen, StatusConfirmed, StatusInProgress}).
		Scan(&rows).Error
	if err != nil {
		return summary, err
	}
	highest := map[string]int{}
	typeOf := map[string]string{}
	hostOf := map[string]string{}
	for _, row := range rows {
		key := row.PrimaryType + "\x00" + strings.ToLower(row.PrimaryValue)
		rank := severityRank(row.Severity)
		if rank > highest[key] {
			highest[key] = rank
			typeOf[key] = row.PrimaryType
			hostOf[key] = row.Host
		}
	}
	for key, rank := range highest {
		parts := strings.SplitN(key, "\x00", 2)
		if len(parts) != 2 {
			continue
		}
		host := hostOf[key]
		if host != "" && net.ParseIP(host) != nil {
			if ip := net.ParseIP(host); ip != nil && !isPublic(ip) {
				continue
			}
		}
		summary.Total++
		switch parts[0] {
		case "ip":
			summary.IP++
		case "domain":
			summary.Domain++
		case "url":
			summary.URL++
		case "service":
			summary.Service++
		case "certificate":
			summary.Certificate++
		}
		switch rank {
		case 3:
			summary.Critical++
		case 2:
			summary.High++
		case 1:
			summary.Medium++
		}
	}
	return summary, nil
}

func severityRank(value string) int {
	switch NormalizeSeverity(value) {
	case "critical":
		return 3
	case "high":
		return 2
	case "medium":
		return 1
	default:
		return 0
	}
}

func isPublic(ip net.IP) bool {
	return ip != nil && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsUnspecified() &&
		!ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsMulticast()
}

type LinkView struct {
	Type   string `json:"type"`
	Value  string `json:"value"`
	Role   string `json:"role"`
	Direct bool   `json:"direct"`
}

type View struct {
	ID           string     `json:"id"`
	Source       string     `json:"source"`
	Title        string     `json:"title"`
	Severity     string     `json:"severity"`
	Category     string     `json:"category"`
	TargetType   string     `json:"targetType"`
	Target       string     `json:"target"`
	PrimaryType  string     `json:"primaryType"`
	PrimaryValue string     `json:"primaryValue"`
	Direct       bool       `json:"direct"`
	CDNEdge      bool       `json:"cdnEdge"`
	Status       string     `json:"status"`
	ObservedAt   string     `json:"observedAt,omitempty"`
	Links        []LinkView `json:"links"`
}

func FindingsForAsset(db *gorm.DB, assetType, value string) ([]View, error) {
	if db == nil || !db.Migrator().HasTable(&database.SecurityFinding{}) {
		return []View{}, nil
	}
	normalized, err := assetstore.Normalize(DefaultScope, assetType, value)
	if err != nil {
		return nil, err
	}
	assetID := assetstore.AssetID(DefaultScope, normalized.Type, normalized.CanonicalValue)
	var findings []database.SecurityFinding
	query := db.Where("primary_asset_id = ?", assetID)
	if db.Migrator().HasTable(&database.FindingAssetLink{}) {
		query = query.Or("id IN (?)", db.Model(&database.FindingAssetLink{}).Select("finding_id").Where("asset_id = ?", assetID))
	}
	if err := query.Order("last_seen_at DESC").Limit(200).Find(&findings).Error; err != nil {
		return nil, err
	}
	views := make([]View, 0, len(findings))
	for _, finding := range findings {
		links := make([]LinkView, 0)
		direct := finding.PrimaryAssetID == assetID
		if db.Migrator().HasTable(&database.FindingAssetLink{}) {
			var rows []struct {
				AssetType  string
				AssetValue string
				Role       string
				Direct     bool
			}
			_ = db.Table("finding_asset_links AS l").
				Select("a.type AS asset_type, a.canonical_value AS asset_value, l.role, l.direct").
				Joins("JOIN unified_assets a ON a.id = l.asset_id").
				Where("l.finding_id = ?", finding.ID).
				Scan(&rows).Error
			for _, row := range rows {
				links = append(links, LinkView{Type: row.AssetType, Value: row.AssetValue, Role: row.Role, Direct: row.Direct})
				if row.Direct && row.AssetType == normalized.Type && row.AssetValue == normalized.CanonicalValue {
					direct = true
				}
			}
		}
		var primary struct {
			Type           string
			CanonicalValue string
		}
		_ = db.Table("unified_assets").
			Select("type, canonical_value").
			Where("id = ?", finding.PrimaryAssetID).
			Scan(&primary).Error
		view := View{
			ID:           finding.ID,
			Source:       finding.Source,
			Title:        finding.Title,
			Severity:     finding.Severity,
			Category:     finding.Category,
			TargetType:   finding.TargetType,
			Target:       finding.Target,
			PrimaryType:  primary.Type,
			PrimaryValue: primary.CanonicalValue,
			Direct:       direct,
			CDNEdge:      finding.CDNEdge,
			Status:       finding.Status,
			Links:        links,
		}
		if !finding.ObservedAt.IsZero() {
			view.ObservedAt = finding.ObservedAt.UTC().Format(time.RFC3339)
		}
		views = append(views, view)
	}
	return views, nil
}
