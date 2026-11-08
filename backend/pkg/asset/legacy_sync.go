package asset

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
)

type LegacySyncOptions struct {
	ScopeKey  string
	TaskID    string
	Version   int
	Since     *time.Time
	BatchSize int
}

type LegacySyncSummary struct {
	Assets         int            `json:"assets"`
	Observations   int            `json:"observations"`
	BySource       map[string]int `json:"bySource"`
	SkippedInvalid int            `json:"skippedInvalid"`
}

// BackfillMissingSubdomainObservations repairs catalogs created before
// subdomain tasks were part of the unified sync. It finds subdomain result
// tasks that have no subdomain observation yet and replays only those tasks.
// Replaying is idempotent, so a crash or a concurrent completion is harmless.
func BackfillMissingSubdomainObservations(ctx context.Context, db *gorm.DB) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("asset store database is nil")
	}
	if !db.Migrator().HasTable(&database.SubdomainScanResult{}) || !db.Migrator().HasTable(&database.UnifiedAssetObservation{}) {
		return 0, nil
	}
	var resultTaskIDs []string
	if err := db.WithContext(ctx).Model(&database.SubdomainScanResult{}).Where("task_id <> ''").Distinct().Pluck("task_id", &resultTaskIDs).Error; err != nil {
		return 0, err
	}
	if len(resultTaskIDs) == 0 {
		return 0, nil
	}
	var observedTaskIDs []string
	if err := db.WithContext(ctx).Model(&database.UnifiedAssetObservation{}).Where("source = ? AND task_id <> ''", "subdomain").Distinct().Pluck("task_id", &observedTaskIDs).Error; err != nil {
		return 0, err
	}
	seen := make(map[string]struct{}, len(observedTaskIDs))
	for _, taskID := range observedTaskIDs {
		seen[taskID] = struct{}{}
	}
	store := &Store{DB: db}
	synced := 0
	for _, taskID := range resultTaskIDs {
		if _, exists := seen[taskID]; exists {
			continue
		}
		if _, err := store.SyncLegacy(ctx, LegacySyncOptions{TaskID: taskID}); err != nil {
			return synced, err
		}
		synced++
	}
	return synced, nil
}

// SyncCompletedTask refreshes the unified catalog for one finished scan
// version. It is intentionally idempotent so every execution backend can call
// it after marking a task complete.
func SyncCompletedTask(ctx context.Context, db *gorm.DB, taskID string, version int) (LegacySyncSummary, error) {
	return (&Store{DB: db}).SyncLegacy(ctx, LegacySyncOptions{TaskID: taskID, Version: version})
}

func (s *Store) SyncLegacy(ctx context.Context, options LegacySyncOptions) (LegacySyncSummary, error) {
	if s == nil || s.DB == nil {
		return LegacySyncSummary{}, fmt.Errorf("asset store database is nil")
	}
	batchSize := options.BatchSize
	if batchSize <= 0 || batchSize > 500 {
		batchSize = 200
	}
	inputs := make([]Observation, 0, batchSize)
	pendingRelations := make([]RelationInput, 0, batchSize)
	summary := LegacySyncSummary{BySource: map[string]int{}}
	assetIDs := map[string]struct{}{}
	flush := func() error {
		if len(inputs) == 0 {
			return nil
		}
		assets, observations, err := s.UpsertMany(ctx, inputs)
		if err != nil {
			return err
		}
		for _, assetRow := range assets {
			assetIDs[assetRow.ID] = struct{}{}
		}
		summary.Assets = len(assetIDs)
		summary.Observations += len(observations)
		for _, observation := range observations {
			summary.BySource[observation.Source]++
		}
		for _, relation := range pendingRelations {
			if _, err := s.UpsertRelation(ctx, relation); err != nil {
				return err
			}
		}
		inputs = inputs[:0]
		pendingRelations = pendingRelations[:0]
		return nil
	}
	addRelation := func(fromType, fromValue, toType, toValue, relationType, source, sourceID string, version int, observedAt time.Time) {
		from, err := Normalize(options.ScopeKey, fromType, fromValue)
		if err != nil {
			return
		}
		to, err := Normalize(options.ScopeKey, toType, toValue)
		if err != nil {
			return
		}
		pendingRelations = append(pendingRelations, RelationInput{ScopeKey: from.ScopeKey, FromAssetID: AssetID(from.ScopeKey, from.Type, from.CanonicalValue), ToAssetID: AssetID(to.ScopeKey, to.Type, to.CanonicalValue), RelationType: relationType, Source: source, SourceID: sourceID, SourceVersion: version, ObservedAt: observedAt})
	}
	appendInput := func(input Observation) error {
		if _, err := Normalize(input.ScopeKey, input.Type, input.Value); err != nil {
			summary.SkippedInvalid++
			return nil
		}
		inputs = append(inputs, input)
		if len(inputs) >= batchSize {
			return flush()
		}
		return nil
	}
	appendURL := func(rawURL, source, sourceID, taskID string, version int, observedAt time.Time, confidence string, attrs map[string]any) error {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			return nil
		}
		normalized, err := Normalize(options.ScopeKey, TypeURL, rawURL)
		if err != nil {
			summary.SkippedInvalid++
			return nil
		}
		if err := appendInput(Observation{ScopeKey: options.ScopeKey, Type: TypeURL, Value: normalized.CanonicalValue, DisplayValue: rawURL, Source: source, SourceID: sourceID, SourceVersion: version, TaskID: taskID, TaskVersion: version, Origin: "scan", ObservedAt: observedAt, Confidence: confidence, Attributes: attrs}); err != nil {
			return err
		}
		if parsed, err := url.Parse(normalized.CanonicalValue); err == nil && parsed.Hostname() != "" {
			host := parsed.Hostname()
			if err := appendInput(Observation{ScopeKey: options.ScopeKey, Value: host, DisplayValue: host, Source: source, SourceID: sourceID + ":host", SourceVersion: version, TaskID: taskID, TaskVersion: version, Origin: "scan", ObservedAt: observedAt, Confidence: confidence, Attributes: attrs}); err != nil {
				return err
			}
			addRelation(TypeURL, normalized.CanonicalValue, "", host, "endpoint_of", source, sourceID, version, observedAt)
		}
		return nil
	}
	appendAsset := func(value, source, sourceID, taskID string, version int, observedAt time.Time, confidence string, attrs map[string]any) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		normalized, err := Normalize(options.ScopeKey, "", value)
		if err != nil {
			summary.SkippedInvalid++
			return nil
		}
		if err := appendInput(Observation{ScopeKey: options.ScopeKey, Type: normalized.Type, Value: normalized.CanonicalValue, DisplayValue: value, Source: source, SourceID: sourceID, SourceVersion: version, TaskID: taskID, TaskVersion: version, Origin: "scan", ObservedAt: observedAt, Confidence: confidence, Attributes: attrs}); err != nil {
			return err
		}
		if normalized.Type == TypeURL {
			if parsed, err := url.Parse(normalized.CanonicalValue); err == nil && parsed.Hostname() != "" {
				host := parsed.Hostname()
				if err := appendInput(Observation{ScopeKey: options.ScopeKey, Value: host, DisplayValue: host, Source: source, SourceID: sourceID + ":host", SourceVersion: version, TaskID: taskID, TaskVersion: version, Origin: "scan", ObservedAt: observedAt, Confidence: confidence, Attributes: attrs}); err != nil {
					return err
				}
				addRelation(TypeURL, normalized.CanonicalValue, "", host, "endpoint_of", source, sourceID, version, observedAt)
			}
		}
		return nil
	}

	taskVersions := map[string]int{}
	taskUpdatedAt := map[string]time.Time{}
	var tasks []database.VulnScanTask
	taskQuery := s.DB.WithContext(ctx).Select("id", "version", "scan_type", "updated_at")
	if options.TaskID != "" {
		taskQuery = taskQuery.Where("id = ?", options.TaskID)
		if options.Version > 0 {
			taskQuery = taskQuery.Where("version = ?", options.Version)
		}
	}
	if err := taskQuery.Find(&tasks).Error; err != nil {
		return summary, err
	}
	for _, task := range tasks {
		if task.Version > taskVersions[task.ID] {
			taskVersions[task.ID] = task.Version
		}
		taskUpdatedAt[fmt.Sprintf("%s:%d", task.ID, task.Version)] = task.UpdatedAt
	}
	taskTime := func(taskID string, version int, fallback time.Time) time.Time {
		if value, ok := taskUpdatedAt[fmt.Sprintf("%s:%d", taskID, version)]; ok && !value.IsZero() {
			return value
		}
		return fallback
	}

	var ports []database.PortScanResult
	portQuery := s.DB.WithContext(ctx)
	if options.TaskID != "" {
		portQuery = portQuery.Where("task_id = ?", options.TaskID)
	}
	if options.Version > 0 {
		portQuery = portQuery.Where("version = ?", options.Version)
	}
	if options.Since != nil {
		portQuery = portQuery.Where("created_at >= ?", *options.Since)
	}
	if err := portQuery.Find(&ports).Error; err != nil {
		return summary, err
	}
	for _, result := range ports {
		observedAt := result.CreatedAt
		if observedAt.IsZero() {
			observedAt = time.Now().UTC()
		}
		attrs := map[string]any{"taskId": result.TaskID, "version": result.Version, "resultId": result.ID, "port": result.Port, "transport": result.Transport, "protocol": result.Protocol, "product": result.Product, "serviceVersion": result.ServiceVersion, "discoveryStatus": result.DiscoveryStatus}
		if err := appendInput(Observation{ScopeKey: options.ScopeKey, Value: result.Host, Source: "portscan", SourceID: fmt.Sprintf("%d", result.ID), SourceVersion: result.Version, TaskID: result.TaskID, TaskVersion: result.Version, Origin: "scan", ObservedAt: observedAt, Confidence: confidenceForDiscovery(result.DiscoveryStatus), Attributes: attrs}); err != nil {
			return summary, err
		}
		service := strings.TrimSpace(strings.Join([]string{result.Product, result.ServiceVersion}, " "))
		if service != "" {
			if err := appendInput(Observation{ScopeKey: options.ScopeKey, Type: TypeService, Value: service, Source: "portscan", SourceID: fmt.Sprintf("%d", result.ID), SourceVersion: result.Version, TaskID: result.TaskID, TaskVersion: result.Version, Origin: "scan", ObservedAt: observedAt, Confidence: confidenceForDiscovery(result.DiscoveryStatus), Attributes: attrs}); err != nil {
				return summary, err
			}
			addRelation("", result.Host, TypeService, service, "exposes_service", "portscan", fmt.Sprintf("%d", result.ID), result.Version, observedAt)
		}
	}

	var fingerprints []database.FingerprintInfo
	fingerprintQuery := s.DB.WithContext(ctx)
	if options.TaskID != "" {
		fingerprintQuery = fingerprintQuery.Where("task_id = ?", options.TaskID)
	}
	if options.Version > 0 {
		fingerprintQuery = fingerprintQuery.Where("version = ?", options.Version)
	}
	if options.Since != nil {
		fingerprintQuery = fingerprintQuery.Where("created_at >= ?", *options.Since)
	}
	if err := fingerprintQuery.Find(&fingerprints).Error; err != nil {
		return summary, err
	}
	for _, result := range fingerprints {
		if strings.TrimSpace(result.URL) == "" {
			continue
		}
		observedAt := result.CreatedAt
		if observedAt.IsZero() {
			observedAt = time.Now().UTC()
		}
		fingerprintSourceID := fmt.Sprintf("%s:%d:%s", result.TaskID, result.Version, result.URL)
		fingerprintAttrs := map[string]any{"taskId": result.TaskID, "version": result.Version, "status": result.Status, "title": result.Title, "fingerprints": parseJSONList(result.Fingerprints), "highRiskFingerprints": parseJSONList(result.HighRiskFingerprints)}
		if err := appendInput(Observation{ScopeKey: options.ScopeKey, Type: TypeURL, Value: result.URL, Source: "fingerprint", SourceID: fingerprintSourceID, SourceVersion: result.Version, TaskID: result.TaskID, TaskVersion: result.Version, Origin: "scan", ObservedAt: observedAt, Confidence: confidenceForHTTPStatus(result.Status), Attributes: fingerprintAttrs}); err != nil {
			return summary, err
		}
		if parsed, err := url.Parse(result.URL); err == nil && parsed.Hostname() != "" {
			host := parsed.Hostname()
			if err := appendInput(Observation{ScopeKey: options.ScopeKey, Value: host, Source: "fingerprint", SourceID: fingerprintSourceID + ":host", SourceVersion: result.Version, TaskID: result.TaskID, TaskVersion: result.Version, Origin: "scan", ObservedAt: observedAt, Confidence: confidenceForHTTPStatus(result.Status), Attributes: fingerprintAttrs}); err != nil {
				return summary, err
			}
			addRelation(TypeURL, result.URL, "", host, "endpoint_of", "fingerprint", fingerprintSourceID, result.Version, observedAt)
		}
	}

	var subdomains []database.SubdomainScanResult
	subdomainQuery := s.DB.WithContext(ctx)
	if options.TaskID != "" {
		subdomainQuery = subdomainQuery.Where("task_id = ?", options.TaskID)
	}
	if options.Since != nil {
		subdomainQuery = subdomainQuery.Where("created_at >= ?", *options.Since)
	}
	if err := subdomainQuery.Find(&subdomains).Error; err != nil {
		return summary, err
	}
	for _, result := range subdomains {
		if strings.TrimSpace(result.Subdomain) == "" {
			continue
		}
		observedAt := result.CreatedAt
		if observedAt.IsZero() {
			observedAt = time.Now().UTC()
		}
		version := taskVersions[result.TaskID]
		attrs := map[string]any{"taskId": result.TaskID, "version": version, "resultId": result.ID, "domain": result.Domain, "subdomain": result.Subdomain, "source": result.Source, "sources": parseJSONList(result.Sources), "assetType": result.AssetType, "assetName": result.AssetName, "isWildcard": result.IsWildcard}
		if ip, ok := ParseIPLike(result.Subdomain); ok {
			if err := appendInput(Observation{ScopeKey: options.ScopeKey, Type: TypeIP, Value: ip, Source: "subdomain", SourceID: result.ID + ":ip", SourceVersion: version, TaskID: result.TaskID, TaskVersion: version, Origin: "scan", ObservedAt: observedAt, Confidence: "medium", Attributes: attrs}); err != nil {
				return summary, err
			}
			continue
		}
		if strings.Contains(result.Subdomain, ":") {
			summary.SkippedInvalid++
			continue
		}
		if err := appendInput(Observation{ScopeKey: options.ScopeKey, Type: TypeDomain, Value: result.Subdomain, Source: "subdomain", SourceID: result.ID, SourceVersion: version, TaskID: result.TaskID, TaskVersion: version, Origin: "scan", ObservedAt: observedAt, Confidence: "medium", Attributes: attrs}); err != nil {
			return summary, err
		}
		for _, ip := range parseJSONList(result.IPs) {
			normalized, ok := ParseIPLike(ip)
			if !ok {
				summary.SkippedInvalid++
				continue
			}
			if err := appendInput(Observation{ScopeKey: options.ScopeKey, Type: TypeIP, Value: normalized, Source: "subdomain", SourceID: result.ID + ":ip:" + normalized, SourceVersion: version, TaskID: result.TaskID, TaskVersion: version, Origin: "scan", ObservedAt: observedAt, Confidence: "medium", Attributes: attrs}); err != nil {
				return summary, err
			}
			addRelation(TypeDomain, result.Subdomain, TypeIP, normalized, "resolves_to", "subdomain", result.ID, version, observedAt)
		}
	}
	if s.DB.Migrator().HasTable(&database.VulnScanDirResult{}) {
		var directoryResults []database.VulnScanDirResult
		directoryQuery := s.DB.WithContext(ctx)
		if options.TaskID != "" {
			directoryQuery = directoryQuery.Where("task_id = ?", options.TaskID)
		}
		if options.Version > 0 {
			directoryQuery = directoryQuery.Where("version = ?", options.Version)
		}
		if options.Since != nil {
			directoryQuery = directoryQuery.Where("created_at >= ?", *options.Since)
		}
		if err := directoryQuery.Find(&directoryResults).Error; err != nil {
			return summary, err
		}
		for _, result := range directoryResults {
			observedAt := result.CreatedAt
			if observedAt.IsZero() {
				observedAt = time.Now().UTC()
			}
			attrs := map[string]any{"taskId": result.TaskID, "version": result.Version, "resultId": result.ID, "status": result.Status, "title": result.Title, "contentType": result.ContentType, "length": result.Length}
			sourceID := fmt.Sprintf("dirscan:%s:%d:%s", result.TaskID, result.Version, result.ID)
			if err := appendURL(result.URL, "dirscan", sourceID, result.TaskID, result.Version, observedAt, confidenceForHTTPStatus(result.Status), attrs); err != nil {
				return summary, err
			}
		}
	}

	if s.DB.Migrator().HasTable(&database.VulnScanJSAsset{}) {
		var jsAssets []database.VulnScanJSAsset
		jsQuery := s.DB.WithContext(ctx)
		if options.TaskID != "" {
			jsQuery = jsQuery.Where("task_id = ?", options.TaskID)
		}
		if options.Version > 0 {
			jsQuery = jsQuery.Where("version = ?", options.Version)
		}
		if options.Since != nil {
			jsQuery = jsQuery.Where("created_at >= ?", *options.Since)
		}
		if err := jsQuery.Find(&jsAssets).Error; err != nil {
			return summary, err
		}
		for _, result := range jsAssets {
			if !isJSScanNetworkAssetType(result.Type) || !isJSScanAssetCandidate(result.Value) {
				continue
			}
			observedAt := result.CreatedAt
			if observedAt.IsZero() {
				observedAt = time.Now().UTC()
			}
			attrs := map[string]any{"taskId": result.TaskID, "version": result.Version, "resultId": result.ID, "assetType": result.Type, "source": result.Source}
			sourceID := fmt.Sprintf("jsscan:%s:%d:%s", result.TaskID, result.Version, result.ID)
			if err := appendAsset(result.Value, "jsscan", sourceID, result.TaskID, result.Version, observedAt, "medium", attrs); err != nil {
				return summary, err
			}
		}
	}

	if s.DB.Migrator().HasTable(&database.VulnerabilityInfo{}) {
		var vulnerabilities []database.VulnerabilityInfo
		vulnerabilityQuery := s.DB.WithContext(ctx)
		if options.TaskID != "" {
			vulnerabilityQuery = vulnerabilityQuery.Where("task_id = ?", options.TaskID)
		}
		if options.Version > 0 {
			vulnerabilityQuery = vulnerabilityQuery.Where("version = ?", options.Version)
		}
		if err := vulnerabilityQuery.Find(&vulnerabilities).Error; err != nil {
			return summary, err
		}
		for _, result := range vulnerabilities {
			if strings.TrimSpace(result.VulnURL) == "" {
				continue
			}
			observedAt := taskTime(result.TaskID, result.Version, time.Now().UTC())
			attrs := map[string]any{"taskId": result.TaskID, "version": result.Version, "severity": result.Severity, "title": result.VulnName, "templateId": result.TemplateID, "disposalStatus": result.DisposalStatus}
			findingKey := strings.TrimSpace(result.TemplateID)
			if findingKey == "" {
				findingKey = strings.TrimSpace(result.VulnName)
			}
			sourceID := fmt.Sprintf("vuln:%s:%d:%s:%s", result.TaskID, result.Version, findingKey, result.VulnURL)
			if err := appendURL(result.VulnURL, "webscan-vulnerability", sourceID, result.TaskID, result.Version, observedAt, "medium", attrs); err != nil {
				return summary, err
			}
		}
	}

	if s.DB.Migrator().HasTable(&database.PortScanResultAsset{}) {
		var portAliases []database.PortScanResultAsset
		aliasQuery := s.DB.WithContext(ctx)
		if options.TaskID != "" {
			aliasQuery = aliasQuery.Where("task_id = ?", options.TaskID)
		}
		if options.Version > 0 {
			aliasQuery = aliasQuery.Where("version = ?", options.Version)
		}
		if options.Since != nil {
			aliasQuery = aliasQuery.Where("created_at >= ?", *options.Since)
		}
		if err := aliasQuery.Find(&portAliases).Error; err != nil {
			return summary, err
		}
		for _, result := range portAliases {
			observedAt := result.CreatedAt
			if observedAt.IsZero() {
				observedAt = time.Now().UTC()
			}
			attrs := map[string]any{"taskId": result.TaskID, "version": result.Version, "resultId": result.ResultID, "port": result.Port, "transport": result.Transport, "domain": result.Domain, "assetType": result.AssetType, "assetName": result.AssetName, "sourceModule": result.SourceModule}
			sourceID := fmt.Sprintf("portalias:%d", result.ID)
			aliasHost := strings.TrimSpace(result.Subdomain)
			if aliasHost == "" {
				aliasHost = strings.TrimSpace(result.Domain)
			}
			if aliasHost != "" {
				if err := appendAsset(aliasHost, "portscan", sourceID+":host", result.TaskID, result.Version, observedAt, "medium", attrs); err != nil {
					return summary, err
				}
			}
			if strings.TrimSpace(result.IP) != "" {
				if err := appendAsset(result.IP, "portscan", sourceID+":ip", result.TaskID, result.Version, observedAt, "medium", attrs); err != nil {
					return summary, err
				}
			}
			if aliasHost != "" && strings.TrimSpace(result.IP) != "" {
				addRelation(TypeDomain, aliasHost, TypeIP, result.IP, "resolves_to", "portscan", sourceID, result.Version, observedAt)
			}
		}
	}

	if err := flush(); err != nil {
		return summary, err
	}
	return summary, nil
}

func isJSScanNetworkAssetType(assetType string) bool {
	switch strings.ToLower(strings.TrimSpace(assetType)) {
	case "apiroot", "apiroute", "ipurl", "ip_url", "domain", "ip":
		return true
	default:
		return false
	}
}

// IsJSScanNonNetworkObservation identifies historical scan observations whose
// recorded JS result type is evidence such as a secret or personal information.
func IsJSScanNonNetworkObservation(observation database.UnifiedAssetObservation) bool {
	if observation.Source != "jsscan" {
		return false
	}
	assetType, _ := observation.Attributes["assetType"].(string)
	return strings.TrimSpace(assetType) != "" && !isJSScanNetworkAssetType(assetType)
}

func isJSScanAssetCandidate(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		_, err := WebsiteOrigin(value)
		return err == nil
	}
	if net.ParseIP(strings.Trim(value, "[]")) != nil {
		return true
	}
	host := strings.TrimSuffix(lower, ".")
	return strings.Contains(host, ".") && IsDomainHost(strings.TrimPrefix(host, "*."))
}

func confidenceForDiscovery(status string) string {
	if strings.EqualFold(strings.TrimSpace(status), "verified") {
		return "high"
	}
	return "medium"
}

func confidenceForHTTPStatus(status int) string {
	if status >= 200 && status < 500 {
		return "high"
	}
	return "medium"
}

func parseJSONList(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return values
}
