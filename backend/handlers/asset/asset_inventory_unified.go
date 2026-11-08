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

func collectUnifiedAssetInventoryWithDB(db *gorm.DB, detailHost string) ([]AssetInventoryItem, bool, error) {
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
	items := make(map[string]*AssetInventoryItem)
	parents := make(map[string][]string)

	for _, assetRow := range assets {
		if assetRow.Type == assetstore.TypeService {
			continue
		}
		assetObservations, known := byAsset[assetRow.ID]
		if known && len(assetObservations) == 0 {
			continue
		}
		host := inventoryHost(assetRow.CanonicalValue)
		if assetRow.Type == assetstore.TypeURL {
			parsed, err := url.Parse(assetRow.CanonicalValue)
			if err != nil || parsed.Hostname() == "" {
				continue
			}
			host = inventoryHost(parsed.Hostname())
		}
		if host == "" {
			continue
		}
		item := items[host]
		if item == nil {
			itemType := assetRow.Type
			if assetRow.Type == assetstore.TypeURL {
				itemType = assetHostType(host)
			}
			item = &AssetInventoryItem{Type: itemType, Value: host, IPs: []string{}, Ports: []AssetInventoryPort{}, Fingerprints: []string{}, Sources: []AssetInventorySource{}, Observations: []AssetInventoryObservation{}}
			items[host] = item
		}
		for _, observation := range assetObservations {
			attrs := observation.Attributes
			taskID, version := unifiedObservationTask(observation)
			meta := metadata[assetCenterTaskKey(taskID, version)]
			taskName := meta.Name
			if taskName == "" {
				taskName = assetCenterFallbackTaskName(taskID, "")
			}
			source := AssetInventorySource{TaskID: taskID, TaskName: taskName, ScanType: metaScanType(tasks, taskID, version), Version: version, Source: observation.Source}
			if source.TaskID != "" {
				found := false
				for _, existing := range item.Sources {
					if existing.TaskID == source.TaskID && existing.Version == source.Version && existing.Source == source.Source {
						found = true
						break
					}
				}
				if !found {
					item.Sources = append(item.Sources, source)
				}
			}
			item.ObservationCount++
			if item.FirstSeenAt.IsZero() || observation.ObservedAt.Before(item.FirstSeenAt) {
				item.FirstSeenAt = observation.ObservedAt
			}
			if observation.ObservedAt.After(item.LastSeenAt) {
				item.LastSeenAt = observation.ObservedAt
			}
			if detailHost != "" && host == detailHost {
				item.Observations = append(item.Observations, AssetInventoryObservation{AssetInventorySource: source, Port: unifiedAttrInt(attrs, "port"), Transport: unifiedAttrString(attrs, "transport"), Protocol: unifiedAttrString(attrs, "protocol"), Service: strings.TrimSpace(strings.Join([]string{unifiedAttrString(attrs, "product"), unifiedAttrString(attrs, "serviceVersion")}, " ")), URL: assetRow.CanonicalValue, Fingerprints: unifiedAttrStrings(attrs, "fingerprints"), HighRiskFingerprints: unifiedAttrStrings(attrs, "highRiskFingerprints"), DiscoveryStatus: unifiedAttrString(attrs, "discoveryStatus"), ObservedAt: observation.ObservedAt})
			}
			item.Fingerprints = inventoryLabels(item.Fingerprints, unifiedAttrStrings(attrs, "fingerprints", "highRiskFingerprints")...)
			port := unifiedAttrInt(attrs, "port")
			if assetRow.Type == assetstore.TypeURL {
				parsed, _ := url.Parse(assetRow.CanonicalValue)
				port, _ = strconv.Atoi(parsed.Port())
				if port == 0 && parsed.Scheme == "https" {
					port = 443
				}
				if port == 0 && parsed.Scheme == "http" {
					port = 80
				}
			}
			if port > 0 {
				transport := unifiedAttrString(attrs, "transport")
				if transport == "" {
					transport = "tcp"
				}
				index := -1
				for i := range item.Ports {
					if item.Ports[i].Port == port && item.Ports[i].Transport == transport {
						index = i
						break
					}
				}
				if index < 0 {
					item.Ports = append(item.Ports, AssetInventoryPort{Port: port, Transport: transport, Protocols: []string{}, Services: []string{}, Fingerprints: []string{}, HighRiskFingerprints: []string{}, URLs: []string{}, DiscoveryStatus: "pending"})
					index = len(item.Ports) - 1
				}
				entry := &item.Ports[index]
				entry.Protocols = inventoryLabels(entry.Protocols, unifiedAttrString(attrs, "protocol"))
				entry.Services = inventoryLabels(entry.Services, strings.TrimSpace(strings.Join([]string{unifiedAttrString(attrs, "product"), unifiedAttrString(attrs, "serviceVersion")}, " ")))
				entry.Fingerprints = inventoryLabels(entry.Fingerprints, unifiedAttrStrings(attrs, "fingerprints")...)
				entry.HighRiskFingerprints = inventoryLabels(entry.HighRiskFingerprints, unifiedAttrStrings(attrs, "highRiskFingerprints")...)
				if assetRow.Type == assetstore.TypeURL {
					entry.URLs = inventoryLabels(entry.URLs, assetRow.CanonicalValue)
				}
				if unifiedAttrString(attrs, "discoveryStatus") == "verified" || observation.Source == "fingerprint" {
					entry.DiscoveryStatus = "verified"
				}
				if observation.ObservedAt.After(entry.LastSeenAt) {
					entry.LastSeenAt = observation.ObservedAt
				}
			}
			if observation.Source == "subdomain" && assetRow.Type == assetstore.TypeIP {
				if parent := unifiedAttrString(attrs, "subdomain"); parent != "" {
					parents[parent] = append(parents[parent], host)
				}
			}
		}
	}
	for parent, ips := range parents {
		if item := items[inventoryHost(parent)]; item != nil {
			item.IPs = inventoryLabels(item.IPs, ips...)
		}
	}
	resolvedDomainIPs, err := loadUnifiedResolvedDomainIPs(db)
	if err != nil {
		return nil, false, err
	}
	for domain, ips := range resolvedDomainIPs {
		if item := items[inventoryHost(domain)]; item != nil {
			item.IPs = inventoryLabels(item.IPs, ips...)
		}
	}
	result := make([]AssetInventoryItem, 0, len(items))
	for _, item := range items {
		taskIDs := map[string]struct{}{}
		for _, source := range item.Sources {
			if source.TaskID != "" {
				taskIDs[source.TaskID] = struct{}{}
			}
		}
		item.TaskCount = len(taskIDs)
		item.OpenPortCount = 0
		for _, port := range item.Ports {
			if port.DiscoveryStatus == "verified" {
				item.OpenPortCount++
			}
		}
		item.Observations = append([]AssetInventoryObservation(nil), item.Observations...)
		sort.Slice(item.Sources, func(i, j int) bool { return item.Sources[i].TaskID < item.Sources[j].TaskID })
		sort.Slice(item.Ports, func(i, j int) bool { return item.Ports[i].Port < item.Ports[j].Port })
		sort.Slice(item.Observations, func(i, j int) bool { return item.Observations[i].ObservedAt.After(item.Observations[j].ObservedAt) })
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LastSeenAt.After(result[j].LastSeenAt) })
	return result, true, nil
}

func metaScanType(tasks []database.VulnScanTask, taskID string, version int) string {
	for _, task := range tasks {
		if task.ID == taskID && task.Version == version {
			return task.ScanType
		}
	}
	return ""
}
