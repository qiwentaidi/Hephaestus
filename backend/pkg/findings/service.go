// Package findings owns the canonical vulnerability-to-asset attribution
// model. Findings are written when a scanner observes them, together with the
// resolved asset links and a DNS snapshot, so risk queries never have to infer
// ownership from legacy scan tables at read time.
package findings

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
)

const (
	StatusOpen          = "open"
	StatusConfirmed     = "confirmed"
	StatusInProgress    = "in_progress"
	StatusFixed         = "fixed"
	StatusFalsePositive = "false_positive"
	StatusIgnored       = "ignored"
	StatusAcceptedRisk  = "accepted_risk"

	DefaultScope = "global"
)

type Input struct {
	ScopeKey          string
	Source            string
	SourceID          string
	TaskID            string
	TaskVersion       int
	TemplateID        string
	Title             string
	VulnerabilityType string
	Protocol          string
	Category          string
	TargetType        string
	Target            string
	Severity          string
	Status            string
	Evidence          string
	Confidence        string
	ObservedAt        time.Time
	ResolvedIPs       []string
	CDNEdge           bool
	Identity          string
}

type findingResult struct {
	Finding database.SecurityFinding
	Links   []database.FindingAssetLink
}

var ingestMu sync.Mutex

func NormalizeSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical", "critical risk", "severe", "fatal", "严重", "紧急":
		return "critical"
	case "high", "high risk", "高危":
		return "high"
	case "medium", "medium risk", "moderate", "中危":
		return "medium"
	case "low", "低危":
		return "low"
	case "info", "informational", "信息":
		return "info"
	default:
		return ""
	}
}

func ActiveStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "open", "new", "in_progress", "confirmed", "处理中":
		return true
	default:
		return false
	}
}

func NormalizeStatus(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "open", "new":
		return StatusOpen
	case "confirmed":
		return StatusConfirmed
	case "in_progress", "处理中":
		return StatusInProgress
	case "fixed", "已修复":
		return StatusFixed
	case "false_positive", "false-positive", "误报":
		return StatusFalsePositive
	case "ignored", "已忽略":
		return StatusIgnored
	case "accepted_risk", "accepted", "已接受":
		return StatusAcceptedRisk
	default:
		return value
	}
}

func stableID(prefix, value string) string {
	sum := sha256.Sum256([]byte(value))
	return prefix + "_" + hex.EncodeToString(sum[:])[:48]
}

func findingDedupKey(input Input, category, primaryType, primaryValue string) string {
	identity := strings.TrimSpace(input.Identity)
	if identity == "" {
		identity = strings.TrimSpace(input.TemplateID)
	}
	if identity == "" {
		identity = strings.ToLower(strings.TrimSpace(input.Title))
	}
	if identity == "" {
		identity = strings.TrimSpace(input.SourceID)
	}
	scope := strings.TrimSpace(input.ScopeKey)
	if scope == "" {
		scope = DefaultScope
	}
	raw := strings.Join([]string{
		scope,
		strings.ToLower(category),
		primaryType,
		strings.ToLower(strings.TrimSpace(primaryValue)),
		identity,
	}, "\x00")
	// Keep the stored key bounded; URLs and titles can be much longer than a
	// safe varchar index entry.
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func Ingest(db *gorm.DB, input Input) (database.SecurityFinding, error) {
	if db == nil {
		return database.SecurityFinding{}, errors.New("finding ingestion requires a database")
	}
	if !db.Migrator().HasTable(&database.SecurityFinding{}) {
		if err := db.Migrator().AutoMigrate(&database.SecurityFinding{}, &database.FindingAssetLink{}, &database.AssetRisk{}); err != nil {
			return database.SecurityFinding{}, err
		}
	}
	ingestMu.Lock()
	defer ingestMu.Unlock()

	if input.ObservedAt.IsZero() {
		input.ObservedAt = time.Now().UTC()
	}
	target := ParseTarget(input.Target)
	if target.Type == "unknown" {
		return database.SecurityFinding{}, fmt.Errorf("unparseable finding target %q", input.Target)
	}
	if strings.TrimSpace(input.TargetType) != "" && strings.TrimSpace(input.TargetType) != target.Type {
		// An explicit scanner target type wins when the parser represented
		// the same target in a different shape (for example host:port).
		switch strings.TrimSpace(input.TargetType) {
		case "ip", "domain", "url", "service", "certificate":
			target.Type = strings.TrimSpace(input.TargetType)
		}
	}
	if !validTargetHost(target) {
		return database.SecurityFinding{}, fmt.Errorf("finding target %q is not a valid network asset", input.Target)
	}
	if LooksLikeCredential(input) {
		if target.Type != "url" {
			return database.SecurityFinding{}, fmt.Errorf("credential finding target %q is not a network asset", input.Target)
		}
		if (target.Path == "" || target.Path == "/") && isCloudProviderHost(target.Host) {
			return database.SecurityFinding{}, fmt.Errorf("credential finding target %q points at a cloud provider, not the affected asset", input.Target)
		}
	}
	category := strings.TrimSpace(input.Category)
	if category == "" {
		category = Classify(input.Title, input.VulnerabilityType, input.Protocol, target.Type, target.Port)
	}
	if !input.CDNEdge && target.Host != "" {
		input.CDNEdge = DetectCDNEdge(target.Host)
	}
	if category == CategoryCertificate && target.Certificate == "" {
		if fingerprint := certificateFingerprintPattern.FindString(strings.Join([]string{input.Target, input.Evidence}, " ")); fingerprint != "" {
			target.Certificate = fingerprint
		}
	}
	primaryType, primaryValue := PrimaryAsset(category, target)
	if primaryType == "" || primaryValue == "" {
		return database.SecurityFinding{}, fmt.Errorf("cannot resolve primary asset for target %q", input.Target)
	}
	severity := NormalizeSeverity(input.Severity)
	if severity == "" {
		severity = "info"
	}
	status := NormalizeStatus(input.Status)
	if status == "" {
		status = StatusOpen
	}
	confidence := strings.TrimSpace(input.Confidence)
	if confidence == "" {
		confidence = "medium"
	}
	dedupKey := findingDedupKey(input, category, primaryType, primaryValue)

	var result findingResult
	err := db.Transaction(func(tx *gorm.DB) error {
		primaryAsset, _, err := ensureAsset(tx, input, primaryType, primaryValue, "primary", input.ObservedAt)
		if err != nil {
			return err
		}
		links, err := buildLinks(tx, input, category, primaryType, target, primaryAsset.ID)
		if err != nil {
			return err
		}

		var existing database.SecurityFinding
		existingErr := tx.Where("dedup_key = ?", dedupKey).First(&existing).Error
		oldPrimary := existing.PrimaryAssetID
		oldLinks := make([]database.FindingAssetLink, 0)
		if existingErr == nil {
			if err := tx.Where("finding_id = ?", existing.ID).Find(&oldLinks).Error; err != nil {
				return err
			}
		} else if errors.Is(existingErr, gorm.ErrRecordNotFound) {
			existing = database.SecurityFinding{
				ID:          stableID("finding", dedupKey),
				ScopeKey:    input.ScopeKey,
				DedupKey:    dedupKey,
				FirstSeenAt: input.ObservedAt,
				CreatedAt:   time.Now().UTC(),
			}
			if existing.ScopeKey == "" {
				existing.ScopeKey = DefaultScope
			}
		} else {
			return existingErr
		}

		existing.Source = strings.TrimSpace(input.Source)
		existing.SourceID = strings.TrimSpace(input.SourceID)
		existing.TaskID = strings.TrimSpace(input.TaskID)
		existing.TaskVersion = input.TaskVersion
		existing.TemplateID = strings.TrimSpace(input.TemplateID)
		existing.Title = strings.TrimSpace(input.Title)
		existing.Category = category
		existing.TargetType = target.Type
		existing.Target = targetValue(target)
		existing.Host = target.Host
		existing.Port = target.Port
		existing.Path = target.Path
		existing.PrimaryAssetID = primaryAsset.ID
		existing.Severity = severity
		if existing.ID != "" && !ActiveStatus(existing.Status) && ActiveStatus(status) {
			// Re-observing a closed finding updates last_seen/evidence but does
			// not silently reopen it; explicit status updates own that decision.
			status = existing.Status
		}
		existing.Status = status
		existing.Confidence = confidence
		existing.Evidence = input.Evidence
		existing.CDNEdge = input.CDNEdge
		if existing.ObservedAt.IsZero() || input.ObservedAt.After(existing.ObservedAt) {
			existing.ObservedAt = input.ObservedAt
		}
		if existing.LastSeenAt.IsZero() || input.ObservedAt.After(existing.LastSeenAt) {
			existing.LastSeenAt = input.ObservedAt
		}
		if existing.FirstSeenAt.IsZero() || input.ObservedAt.Before(existing.FirstSeenAt) {
			existing.FirstSeenAt = input.ObservedAt
		}
		existing.UpdatedAt = time.Now().UTC()
		if err := tx.Save(&existing).Error; err != nil {
			return err
		}

		if err := tx.Where("finding_id = ?", existing.ID).Delete(&database.FindingAssetLink{}).Error; err != nil {
			return err
		}
		for index := range links {
			links[index].ID = stableID("finding_link", existing.ID+"\x00"+links[index].AssetID+"\x00"+links[index].Role)
			links[index].FindingID = existing.ID
			links[index].CreatedAt = time.Now().UTC()
		}
		if len(links) > 0 {
			if err := tx.Create(&links).Error; err != nil {
				return err
			}
		}

		affected := map[string]struct{}{}
		if oldPrimary != "" {
			affected[oldPrimary] = struct{}{}
		}
		affected[primaryAsset.ID] = struct{}{}
		for _, link := range oldLinks {
			affected[link.AssetID] = struct{}{}
		}
		for _, link := range links {
			affected[link.AssetID] = struct{}{}
		}
		for assetID := range affected {
			if err := recomputeAssetRisk(tx, assetID); err != nil {
				return err
			}
		}
		result = findingResult{Finding: existing, Links: links}
		return nil
	})
	if err != nil {
		return database.SecurityFinding{}, err
	}
	return result.Finding, nil
}

func targetValue(target Target) string {
	if target.URL != "" {
		return target.URL
	}
	if target.Host != "" && target.Port > 0 {
		return ServiceValue(target.Host, target.Port)
	}
	if target.Host != "" {
		return target.Host
	}
	if target.Certificate != "" {
		return target.Certificate
	}
	return ""
}

func ensureAsset(tx *gorm.DB, input Input, assetType, value, role string, observedAt time.Time) (database.UnifiedAsset, database.UnifiedAssetObservation, error) {
	scope := strings.TrimSpace(input.ScopeKey)
	if scope == "" {
		scope = DefaultScope
	}
	store := assetstore.Store{DB: tx}
	asset, observation, err := store.Upsert(context.Background(), assetstore.Observation{
		ScopeKey:     scope,
		Type:         assetType,
		Value:        value,
		DisplayValue: value,
		Source:       "findings",
		SourceID:     strings.Join([]string{input.Source, input.SourceID, input.TaskID, fmt.Sprint(input.TaskVersion), role}, ":"),
		TaskID:       input.TaskID,
		TaskVersion:  input.TaskVersion,
		ObservedAt:   observedAt,
		Confidence:   "high",
	})
	return asset, observation, err
}

func buildLinks(tx *gorm.DB, input Input, category, primaryType string, target Target, primaryAssetID string) ([]database.FindingAssetLink, error) {
	links := make([]database.FindingAssetLink, 0, 5)
	links = append(links, database.FindingAssetLink{
		ID:       stableID("finding_link", primaryAssetID+"\x00primary"),
		AssetID:  primaryAssetID,
		Role:     "primary",
		Direct:   true,
		Resolver: "primary",
	})
	seen := map[string]struct{}{primaryAssetID: {}}
	for _, link := range ResolveLinks(category, primaryType, target, input.ResolvedIPs) {
		if input.CDNEdge && link.AssetType == "ip" && !link.Direct {
			link.Role = "cdn_edge_ip"
		}
		asset, _, err := ensureAsset(tx, input, link.AssetType, link.Value, link.Role, input.ObservedAt)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[asset.ID]; exists {
			continue
		}
		seen[asset.ID] = struct{}{}
		links = append(links, database.FindingAssetLink{
			ID:         stableID("finding_link", asset.ID+"\x00"+link.Role),
			AssetID:    asset.ID,
			Role:       link.Role,
			Direct:     link.Direct,
			Resolver:   link.Resolver,
			ResolvedAt: link.ResolvedAt,
			Evidence:   "",
		})
	}
	sort.SliceStable(links, func(i, j int) bool {
		if links[i].Direct != links[j].Direct {
			return links[i].Direct
		}
		return links[i].AssetID < links[j].AssetID
	})
	return links, nil
}

func recomputeAssetRisk(tx *gorm.DB, assetID string) error {
	if strings.TrimSpace(assetID) == "" {
		return nil
	}
	active := []string{StatusOpen, StatusConfirmed, StatusInProgress}
	type severityCount struct {
		Severity string
		Count    int
	}
	var direct []severityCount
	if err := tx.Raw(`
		SELECT severity, count(*) AS count
		FROM security_findings
		WHERE status IN (?) AND id IN (
			SELECT id FROM security_findings WHERE primary_asset_id = ?
			UNION
			SELECT finding_id FROM finding_asset_links WHERE asset_id = ? AND direct = ?
		)
		GROUP BY severity`, active, assetID, assetID, true).Scan(&direct).Error; err != nil {
		return err
	}
	var inherited []severityCount
	if err := tx.Raw(`
		SELECT f.severity, count(*) AS count
		FROM finding_asset_links l
		JOIN security_findings f ON f.id = l.finding_id
		WHERE l.asset_id = ? AND l.direct = ? AND l.role NOT IN (?, ?) AND f.status IN (?)
		GROUP BY f.severity`, assetID, false, "related_ip", "cdn_edge_ip", active).Scan(&inherited).Error; err != nil {
		return err
	}
	counts := map[string]int{}
	for _, row := range direct {
		switch NormalizeSeverity(row.Severity) {
		case "critical":
			counts["direct_critical"] += row.Count
		case "high":
			counts["direct_high"] += row.Count
		case "medium":
			counts["direct_medium"] += row.Count
		}
	}
	for _, row := range inherited {
		switch NormalizeSeverity(row.Severity) {
		case "critical":
			counts["inherited_critical"] += row.Count
		case "high":
			counts["inherited_high"] += row.Count
		case "medium":
			counts["inherited_medium"] += row.Count
		}
	}
	openFindings := counts["direct_critical"] + counts["direct_high"] + counts["direct_medium"] +
		counts["inherited_critical"] + counts["inherited_high"] + counts["inherited_medium"]
	if openFindings == 0 {
		return tx.Where("asset_id = ?", assetID).Delete(&database.AssetRisk{}).Error
	}
	risk := database.AssetRisk{
		AssetID:           assetID,
		DirectCritical:    counts["direct_critical"],
		DirectHigh:        counts["direct_high"],
		DirectMedium:      counts["direct_medium"],
		InheritedCritical: counts["inherited_critical"],
		InheritedHigh:     counts["inherited_high"],
		InheritedMedium:   counts["inherited_medium"],
		OpenFindings:      openFindings,
		UpdatedAt:         time.Now().UTC(),
	}
	return tx.Save(&risk).Error
}

func UpdateStatusBySourceID(db *gorm.DB, source, sourceID, status string) error {
	if db == nil {
		return errors.New("finding status update requires a database")
	}
	var finding database.SecurityFinding
	if err := db.Where("source = ? AND source_id = ?", source, sourceID).First(&finding).Error; err != nil {
		return err
	}
	return applyStatus(db, finding, status)
}

func UpdateStatusByMatch(db *gorm.DB, source, taskID, templateID, target, status string) error {
	if db == nil {
		return errors.New("finding status update requires a database")
	}
	query := db.Where("source = ? AND task_id = ?", source, taskID)
	if strings.TrimSpace(templateID) != "" {
		query = query.Where("template_id = ?", templateID)
	}
	if strings.TrimSpace(target) != "" {
		query = query.Where("target = ?", strings.TrimSpace(target))
	}
	var finding database.SecurityFinding
	if err := query.Order("last_seen_at DESC").First(&finding).Error; err != nil {
		return err
	}
	return applyStatus(db, finding, status)
}

func applyStatus(db *gorm.DB, finding database.SecurityFinding, status string) error {
	normalized := NormalizeStatus(status)
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&database.SecurityFinding{}).Where("id = ?", finding.ID).Updates(map[string]any{
			"status":     normalized,
			"updated_at": time.Now().UTC(),
		}).Error; err != nil {
			return err
		}
		affected := map[string]struct{}{finding.PrimaryAssetID: {}}
		var links []database.FindingAssetLink
		if err := tx.Where("finding_id = ?", finding.ID).Find(&links).Error; err != nil {
			return err
		}
		for _, link := range links {
			affected[link.AssetID] = struct{}{}
		}
		for assetID := range affected {
			if err := recomputeAssetRisk(tx, assetID); err != nil {
				return err
			}
		}
		return nil
	})
}

// ComputeDedupKey exposes the canonical finding identity so status handlers
// can update a finding without knowing its generated ID.
func ComputeDedupKey(input Input) (string, error) {
	target := ParseTarget(input.Target)
	if target.Type == "unknown" {
		return "", fmt.Errorf("unparseable finding target %q", input.Target)
	}
	category := strings.TrimSpace(input.Category)
	if category == "" {
		category = Classify(input.Title, input.VulnerabilityType, input.Protocol, target.Type, target.Port)
	}
	if !input.CDNEdge && target.Host != "" {
		input.CDNEdge = DetectCDNEdge(target.Host)
	}
	if category == CategoryCertificate && target.Certificate == "" {
		if fingerprint := certificateFingerprintPattern.FindString(strings.Join([]string{input.Target, input.Evidence}, " ")); fingerprint != "" {
			target.Certificate = fingerprint
		}
	}
	primaryType, primaryValue := PrimaryAsset(category, target)
	if primaryType == "" || primaryValue == "" {
		return "", fmt.Errorf("cannot resolve primary asset for target %q", input.Target)
	}
	return findingDedupKey(input, category, primaryType, primaryValue), nil
}

func UpdateStatusByDedupKey(db *gorm.DB, dedupKey, status string) error {
	if db == nil || strings.TrimSpace(dedupKey) == "" {
		return errors.New("finding status update requires a dedup key")
	}
	var finding database.SecurityFinding
	if err := db.Where("dedup_key = ?", dedupKey).First(&finding).Error; err != nil {
		return err
	}
	return applyStatus(db, finding, status)
}
