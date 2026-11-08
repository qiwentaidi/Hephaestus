package asset

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
)

// collectUnifiedAssetCenterAggregatesWithDB projects the canonical asset
// catalog into the existing asset-center response shape. The UI can therefore
// switch sources without changing its table/topology contract.
func collectUnifiedAssetCenterAggregatesWithDB(db *gorm.DB) ([]AssetCenterItem, bool, error) {
	if db == nil || !db.Migrator().HasTable(&database.UnifiedAsset{}) || !db.Migrator().HasTable(&database.UnifiedAssetObservation{}) {
		return nil, false, nil
	}
	var count int64
	if err := db.Model(&database.UnifiedAsset{}).Count(&count).Error; err != nil {
		return nil, false, err
	}
	if count == 0 {
		if _, err := (&assetstore.Store{DB: db}).SyncLegacy(context.Background(), assetstore.LegacySyncOptions{}); err != nil {
			return nil, false, fmt.Errorf("初始化统一资产目录失败: %w", err)
		}
		if err := db.Model(&database.UnifiedAsset{}).Count(&count).Error; err != nil {
			return nil, false, err
		}
	}
	if count == 0 {
		return nil, false, nil
	}

	var tasks []database.VulnScanTask
	if err := db.Find(&tasks).Error; err != nil {
		return nil, false, err
	}
	metadata := assetCenterTaskMetadata(tasks)
	var assets []database.UnifiedAsset
	if err := db.Where("archived_at IS NULL").Order("last_seen_at DESC, canonical_value ASC").Find(&assets).Error; err != nil {
		return nil, false, err
	}
	var observations []database.UnifiedAssetObservation
	if err := db.Order("observed_at DESC, created_at DESC").Find(&observations).Error; err != nil {
		return nil, false, err
	}
	byAsset := indexAssetCenterObservations(observations)
	aggregates := make(map[string]*assetCenterAggregate)
	domainIPs := make(map[string][]string)
	resolvedDomainIPs, err := loadUnifiedResolvedDomainIPs(db)
	if err != nil {
		return nil, false, err
	}
	for domain, ips := range resolvedDomainIPs {
		domainIPs[domain] = append(domainIPs[domain], ips...)
	}

	for _, assetRow := range assets {
		if assetRow.Type != assetstore.TypeDomain && assetRow.Type != assetstore.TypeIP && assetRow.Type != assetstore.TypeURL {
			continue
		}
		for _, observation := range byAsset[assetRow.ID] {
			taskID, version := unifiedObservationTask(observation)
			taskName, updatedAt := assetCenterTaskInfo(metadata, taskID, version, observation.ObservedAt)
			attrs := observation.Attributes
			severity := unifiedAttrString(attrs, "severity")
			service := strings.TrimSpace(unifiedAttrString(attrs, "product"))
			if versionedService := strings.TrimSpace(unifiedAttrString(attrs, "serviceVersion")); versionedService != "" {
				service = strings.TrimSpace(strings.Join([]string{service, versionedService}, " "))
			}
			findingKey := unifiedSecurityFindingKey(attrs, assetRow.CanonicalValue)
			if assetRow.Type == assetstore.TypeURL {
				parsed, err := url.Parse(assetRow.CanonicalValue)
				if err != nil || parsed.Hostname() == "" {
					continue
				}
				port, _ := strconv.Atoi(parsed.Port())
				if port == 0 {
					if parsed.Scheme == "https" {
						port = 443
					} else if parsed.Scheme == "http" {
						port = 80
					}
				}
				addAssetCenterHostAndEndpoint(aggregates, parsed.Hostname(), assetRow.CanonicalValue, taskID, taskName, observation.Source, version, observation.ObservedAt, updatedAt, true, parsed.Scheme, port, severity, "", findingKey)
				setAssetCenterStatus(aggregates, assetRow, parsed.Hostname(), assetRow.Status)
				endpoint := assetCenterEndpointKey(assetRow.CanonicalValue)
				addAssetCenterEndpointServices(aggregates, endpoint, unifiedAttrStrings(attrs, "fingerprints", "highRiskFingerprints"))
				continue
			}

			port := unifiedAttrInt(attrs, "port")
			protocol := unifiedAttrString(attrs, "protocol")
			if protocol == "" {
				protocol = unifiedAttrString(attrs, "transport")
			}
			includeEndpoint := observation.Source == "portscan" && port > 0 && protocol != ""
			addAssetCenterHostAndEndpointTyped(aggregates, assetRow.Type, assetRow.CanonicalValue, "", taskID, taskName, observation.Source, version, observation.ObservedAt, updatedAt, includeEndpoint, protocol, port, severity, service, findingKey)
			setAssetCenterStatus(aggregates, assetRow, assetRow.CanonicalValue, assetRow.Status)
			if observation.Source == "subdomain" && assetRow.Type == assetstore.TypeIP {
				parent := unifiedAttrString(attrs, "subdomain")
				if parent != "" {
					domainIPs[parent] = append(domainIPs[parent], assetRow.CanonicalValue)
				}
			}
		}
	}
	for domain, ips := range domainIPs {
		addAssetCenterDomainIPs(aggregates, domain, ips)
	}

	return finalizeAssetCenterAggregates(aggregates), true, nil
}

// An empty entry marks an asset supported only by excluded observations.
// Assets without observations have no entry.
func indexAssetCenterObservations(observations []database.UnifiedAssetObservation) map[string][]database.UnifiedAssetObservation {
	byAsset := make(map[string][]database.UnifiedAssetObservation)
	for _, observation := range observations {
		if !assetstore.IsJSScanNonNetworkObservation(observation) {
			byAsset[observation.AssetID] = append(byAsset[observation.AssetID], observation)
		} else if _, exists := byAsset[observation.AssetID]; !exists {
			byAsset[observation.AssetID] = nil
		}
	}
	return byAsset
}

func setAssetCenterStatus(aggregates map[string]*assetCenterAggregate, assetRow database.UnifiedAsset, value, status string) {
	key := assetRow.Type + "\x00" + strings.TrimSpace(value)
	if aggregate := aggregates[key]; aggregate != nil {
		aggregate.AssetID = assetRow.ID
		aggregate.Status = status
	}
}

func unifiedObservationTask(observation database.UnifiedAssetObservation) (string, int) {
	return assetstore.ObservationTaskID(observation), assetstore.ObservationTaskVersion(observation)
}

func unifiedAttrString(attributes map[string]any, key string) string {
	if attributes == nil {
		return ""
	}
	value, ok := attributes[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// unifiedSecurityFindingKey identifies a finding independently of the task
// version so repeated scans do not multiply the risk counters.
func unifiedSecurityFindingKey(attributes map[string]any, assetValue string) string {
	finding := unifiedAttrString(attributes, "templateId")
	if finding == "" {
		finding = unifiedAttrString(attributes, "title")
	}
	if finding == "" {
		finding = unifiedAttrString(attributes, "name")
	}
	return finding + "|" + strings.TrimSpace(assetValue)
}

func unifiedAttrInt(attributes map[string]any, key string) int {
	value := unifiedAttrString(attributes, key)
	parsed, _ := strconv.Atoi(value)
	if parsed == 0 {
		if number, ok := attributes[key].(float64); ok {
			return int(number)
		}
		if number, ok := attributes[key].(int); ok {
			return number
		}
	}
	return parsed
}

func unifiedAttrStrings(attributes map[string]any, keys ...string) []string {
	seen := map[string]struct{}{}
	values := []string{}
	for _, key := range keys {
		value, ok := attributes[key]
		if !ok {
			continue
		}
		switch list := value.(type) {
		case []any:
			for _, entry := range list {
				label := strings.TrimSpace(fmt.Sprint(entry))
				if label != "" {
					if _, exists := seen[strings.ToLower(label)]; !exists {
						seen[strings.ToLower(label)] = struct{}{}
						values = append(values, label)
					}
				}
			}
		case []string:
			for _, entry := range list {
				label := strings.TrimSpace(entry)
				if label != "" {
					if _, exists := seen[strings.ToLower(label)]; !exists {
						seen[strings.ToLower(label)] = struct{}{}
						values = append(values, label)
					}
				}
			}
		}
	}
	return values
}

func loadUnifiedResolvedDomainIPs(db *gorm.DB) (map[string][]string, error) {
	resolved := map[string][]string{}
	if db == nil || !db.Migrator().HasTable(&database.UnifiedAssetRelation{}) || !db.Migrator().HasTable(&database.UnifiedAsset{}) {
		return resolved, nil
	}
	var relations []database.UnifiedAssetRelation
	if err := db.Where("relation_type = ? AND active = ?", "resolves_to", true).Find(&relations).Error; err != nil {
		return nil, err
	}
	if len(relations) == 0 {
		return resolved, nil
	}
	ids := make([]string, 0, len(relations)*2)
	seen := map[string]struct{}{}
	for _, relation := range relations {
		for _, id := range []string{relation.FromAssetID, relation.ToAssetID} {
			if _, exists := seen[id]; !exists {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	byID := make(map[string]database.UnifiedAsset, len(ids))
	const chunkSize = 500
	for start := 0; start < len(ids); start += chunkSize {
		end := start + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		var batch []database.UnifiedAsset
		if err := db.Select("id", "type", "canonical_value").Where("id IN ?", ids[start:end]).Find(&batch).Error; err != nil {
			return nil, err
		}
		for _, assetRow := range batch {
			byID[assetRow.ID] = assetRow
		}
	}
	for _, relation := range relations {
		from, fromOK := byID[relation.FromAssetID]
		to, toOK := byID[relation.ToAssetID]
		if !fromOK || !toOK || from.Type != assetstore.TypeDomain || to.Type != assetstore.TypeIP {
			continue
		}
		resolved[from.CanonicalValue] = append(resolved[from.CanonicalValue], to.CanonicalValue)
	}
	return resolved, nil
}

func assetCenterEndpointKey(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" {
		return value
	}
	port, _ := strconv.Atoi(parsed.Port())
	if port == 0 {
		if parsed.Scheme == "https" {
			port = 443
		} else if parsed.Scheme == "http" {
			port = 80
		}
	}
	return fmt.Sprintf("%s://%s:%d", strings.ToLower(parsed.Scheme), normalizeAssetHost(parsed.Hostname()), port)
}

func finalizeAssetCenterAggregates(aggregates map[string]*assetCenterAggregate) []AssetCenterItem {
	items := make([]AssetCenterItem, 0, len(aggregates))
	for _, aggregate := range aggregates {
		aggregate.TaskCount = len(aggregate.tasks)
		aggregate.SourceTaskIDs = make([]string, 0, len(aggregate.tasks))
		for taskID := range aggregate.tasks {
			if taskID != "" {
				aggregate.SourceTaskIDs = append(aggregate.SourceTaskIDs, taskID)
			}
		}
		sort.Strings(aggregate.SourceTaskIDs)
		aggregate.SourceTypes = make([]string, 0, len(aggregate.sources))
		for source := range aggregate.sources {
			if source != "" {
				aggregate.SourceTypes = append(aggregate.SourceTypes, source)
			}
		}
		sort.Strings(aggregate.SourceTypes)
		aggregate.VersionCount = len(aggregate.versions)
		aggregate.ServiceCount = len(aggregate.services)
		aggregate.ServiceLabels = make([]string, 0, len(aggregate.services))
		for service := range aggregate.services {
			aggregate.ServiceLabels = append(aggregate.ServiceLabels, service)
		}
		sort.Strings(aggregate.ServiceLabels)
		aggregate.Service = assetCenterPrimaryPortService(aggregate.portServices)
		aggregate.IPs = make([]string, 0, len(aggregate.ips))
		for ip := range aggregate.ips {
			aggregate.IPs = append(aggregate.IPs, ip)
		}
		sort.Strings(aggregate.IPs)
		aggregate.CriticalCount, aggregate.HighCount = countAssetRiskFindings(aggregate.riskFindings)
		aggregate.AssetCenterItem.riskFindings = aggregate.riskFindings
		populateAssetCenterDisplayFields(&aggregate.AssetCenterItem)
		items = append(items, aggregate.AssetCenterItem)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].LastSeenAt.Equal(items[j].LastSeenAt) {
			return items[i].Value < items[j].Value
		}
		return items[i].LastSeenAt.After(items[j].LastSeenAt)
	})
	return items
}
