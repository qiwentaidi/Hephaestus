package asset

import (
	"encoding/json"
	"hephaestus-backend/pkg/database"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

func collectAssetCenterAggregates() ([]AssetCenterItem, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	return collectAssetCenterAggregatesWithDB(db)
}

func collectAssetCenterAggregatesWithDB(db *gorm.DB) ([]AssetCenterItem, error) {
	if items, used, err := collectUnifiedAssetCenterAggregatesWithDB(db); err != nil {
		return nil, err
	} else if used {
		return items, nil
	}
	var scanTasks []database.VulnScanTask
	if err := db.Find(&scanTasks).Error; err != nil {
		return nil, err
	}
	metadata := assetCenterTaskMetadata(scanTasks)
	aggregates := make(map[string]*assetCenterAggregate)

	var portResults []database.PortScanResult
	if err := db.Find(&portResults).Error; err != nil {
		return nil, err
	}
	portResultsByID := make(map[uint]database.PortScanResult, len(portResults))
	for _, result := range portResults {
		portResultsByID[result.ID] = result
		taskName, updatedAt := assetCenterTaskInfo(metadata, result.TaskID, result.Version, result.CreatedAt)
		protocol, service := portScanEndpointInfo(result)
		addAssetCenterHostAndEndpoint(aggregates, result.Host, "", result.TaskID, taskName, "portscan", result.Version, result.CreatedAt, updatedAt, true, protocol, result.Port, "", service)
	}

	var portAssets []database.PortScanResultAsset
	if err := db.Find(&portAssets).Error; err != nil {
		return nil, err
	}
	for _, asset := range portAssets {
		observedAt := asset.CreatedAt
		taskName, updatedAt := assetCenterTaskInfo(metadata, asset.TaskID, asset.Version, observedAt)
		protocol, service := strings.ToLower(asset.Transport), ""
		if result, ok := portResultsByID[asset.ResultID]; ok {
			protocol, service = portScanEndpointInfo(result)
		}
		addAssetCenterHostAndEndpoint(aggregates, asset.Subdomain, "", asset.TaskID, taskName, "portscan", asset.Version, observedAt, updatedAt, true, protocol, asset.Port, "", service)
		addAssetCenterHostAndEndpoint(aggregates, asset.IP, "", asset.TaskID, taskName, "portscan", asset.Version, observedAt, updatedAt, true, protocol, asset.Port, "", service)
	}

	var fingerprints []database.FingerprintInfo
	if err := db.Find(&fingerprints).Error; err != nil {
		return nil, err
	}
	for _, fingerprint := range fingerprints {
		taskName, updatedAt := assetCenterTaskInfo(metadata, fingerprint.TaskID, fingerprint.Version, fingerprint.CreatedAt)
		endpoint, _ := normalizeAssetEndpoint(fingerprint.URL, fingerprint.Host, fingerprint.Scheme, fingerprint.Port)
		addAssetCenterHostAndEndpoint(aggregates, fingerprint.Host, fingerprint.URL, fingerprint.TaskID, taskName, "webscan", fingerprint.Version, fingerprint.CreatedAt, updatedAt, true, fingerprint.Scheme, fingerprint.Port, "", "")
		addAssetCenterEndpointServices(aggregates, endpoint, assetFingerprintServices(fingerprint))
	}

	var directoryResults []database.VulnScanDirResult
	if err := db.Find(&directoryResults).Error; err != nil {
		return nil, err
	}
	for _, result := range directoryResults {
		taskName, updatedAt := assetCenterTaskInfo(metadata, result.TaskID, result.Version, result.CreatedAt)
		_, host := normalizeAssetEndpoint(result.URL, "", "", 0)
		addAssetCenterHostAndEndpoint(aggregates, host, result.URL, result.TaskID, taskName, "dirscan", result.Version, result.CreatedAt, updatedAt, false, "", 0, "", "")
	}

	var vulnerabilities []database.VulnerabilityInfo
	if err := db.Find(&vulnerabilities).Error; err != nil {
		return nil, err
	}
	for _, vulnerability := range vulnerabilities {
		meta, ok := metadata[assetCenterTaskKey(vulnerability.TaskID, vulnerability.Version)]
		observedAt := time.Time{}
		if ok {
			observedAt = meta.UpdatedAt
		}
		taskName, updatedAt := assetCenterTaskInfo(metadata, vulnerability.TaskID, vulnerability.Version, observedAt)
		_, host := normalizeAssetEndpoint(vulnerability.VulnURL, "", "", 0)
		finding := strings.TrimSpace(vulnerability.TemplateID)
		if finding == "" {
			finding = strings.TrimSpace(vulnerability.VulnName)
		}
		addAssetCenterHostAndEndpoint(aggregates, host, vulnerability.VulnURL, vulnerability.TaskID, taskName, "webscan-vulnerability", vulnerability.Version, observedAt, updatedAt, false, "", 0, vulnerability.Severity, "", finding+"|"+strings.TrimSpace(vulnerability.VulnURL))
	}

	var jsAssets []database.VulnScanJSAsset
	if err := db.Find(&jsAssets).Error; err != nil {
		return nil, err
	}
	for _, asset := range jsAssets {
		taskName, updatedAt := assetCenterTaskInfo(metadata, asset.TaskID, asset.Version, asset.CreatedAt)
		_, host := normalizeAssetEndpoint(asset.Value, "", "", 0)
		if host == "" {
			continue
		}
		addAssetCenterHostAndEndpoint(aggregates, host, asset.Value, asset.TaskID, taskName, "jsscan", asset.Version, asset.CreatedAt, updatedAt, false, "", 0, "", "")
	}

	var jsVulnerabilities []database.VulnScanJSVulnerability
	if err := db.Find(&jsVulnerabilities).Error; err != nil {
		return nil, err
	}
	for _, vulnerability := range jsVulnerabilities {
		taskName, updatedAt := assetCenterTaskInfo(metadata, vulnerability.TaskID, vulnerability.Version, vulnerability.CreatedAt)
		_, host := normalizeAssetEndpoint(vulnerability.URL, "", "", 0)
		addAssetCenterHostAndEndpoint(aggregates, host, vulnerability.URL, vulnerability.TaskID, taskName, "jsscan-vulnerability", vulnerability.Version, vulnerability.CreatedAt, updatedAt, false, "", 0, vulnerability.Level, "", vulnerability.Type+"|"+vulnerability.Title+"|"+strings.TrimSpace(vulnerability.URL))
	}

	latestSubdomainVersions := make(map[string]int)
	for _, task := range scanTasks {
		if task.ScanType == "subdomain" && task.Version > latestSubdomainVersions[task.ID] {
			latestSubdomainVersions[task.ID] = task.Version
		}
	}
	var subdomains []database.SubdomainScanResult
	if err := db.Find(&subdomains).Error; err != nil {
		return nil, err
	}
	for _, result := range subdomains {
		version := latestSubdomainVersions[result.TaskID]
		if version <= 0 {
			version = 1
		}
		taskName, updatedAt := assetCenterTaskInfo(metadata, result.TaskID, version, result.CreatedAt)
		addAssetCenterHostAndEndpoint(aggregates, result.Subdomain, "", result.TaskID, taskName, "subdomain", version, result.CreatedAt, updatedAt, false, "", 0, "", "")
		var ips []string
		if json.Unmarshal([]byte(result.IPs), &ips) == nil {
			addAssetCenterDomainIPs(aggregates, result.Subdomain, ips)
			for _, ip := range ips {
				addAssetCenterHostAndEndpoint(aggregates, ip, "", result.TaskID, taskName, "subdomain", version, result.CreatedAt, updatedAt, false, "", 0, "", "")
			}
		}
	}

	items := make([]AssetCenterItem, 0, len(aggregates))
	for _, aggregate := range aggregates {
		aggregate.TaskCount = len(aggregate.tasks)
		aggregate.SourceTaskIDs = make([]string, 0, len(aggregate.tasks))
		for taskID := range aggregate.tasks {
			aggregate.SourceTaskIDs = append(aggregate.SourceTaskIDs, taskID)
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
	return items, nil
}

func assetCenterMatchesKeyword(item AssetCenterItem, keyword string) bool {
	if keyword == "" {
		return true
	}
	parts := []string{item.Value, item.Operator, item.Location, item.ASN, item.EndpointHost, item.Protocol, item.Service}
	parts = append(parts, item.IPs...)
	return strings.Contains(strings.ToLower(strings.Join(parts, " ")), keyword)
}
