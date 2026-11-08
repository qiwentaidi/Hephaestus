package asset

import (
	"encoding/json"
	"fmt"
	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/ipmetadata"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// AssetCenterItem is a task-independent projection of observed attack-surface
// assets. It intentionally records observations and source tasks rather than
// making any ownership or company attribution claim.
type AssetCenterItem struct {
	AssetID          string    `json:"assetId,omitempty"`
	Type             string    `json:"type"`
	Value            string    `json:"value"`
	ObservationCount int       `json:"observationCount"`
	TaskCount        int       `json:"taskCount"`
	SourceTaskIDs    []string  `json:"sourceTaskIds"`
	SourceTypes      []string  `json:"sourceTypes"`
	VersionCount     int       `json:"versionCount"`
	ServiceCount     int       `json:"serviceCount"`
	ServiceLabels    []string  `json:"serviceLabels"`
	IPs              []string  `json:"ips"`
	Operator         string    `json:"operator"`
	Location         string    `json:"location"`
	ASN              string    `json:"asn"`
	EndpointHost     string    `json:"endpointHost"`
	Port             int       `json:"port"`
	Protocol         string    `json:"protocol"`
	Service          string    `json:"service"`
	CriticalCount    int       `json:"criticalCount"`
	HighCount        int       `json:"highCount"`
	FirstSeenAt      time.Time `json:"firstSeenAt"`
	LastSeenAt       time.Time `json:"-"`
	UpdatedAt        time.Time `json:"updatedAt"`
	Status           string    `json:"status"`

	// riskFindings holds the distinct severity findings observed on the asset.
	// It is not serialized: counts are derived from it so repeated observations
	// of the same finding (for example across task versions) never inflate the
	// risk badge.
	riskFindings map[string]struct{}
}

// AssetTopologyNode describes one displayable entity in a local asset
// relationship graph. Topology entry points are intentionally limited to
// domain and IP assets; endpoints and technologies are derived children.
type AssetTopologyNode struct {
	ID      string           `json:"id"`
	Type    string           `json:"type"`
	Label   string           `json:"label"`
	Asset   *AssetCenterItem `json:"asset,omitempty"`
	Service string           `json:"service,omitempty"`
}

type AssetTopologyEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
}

type assetTopologyRootAggregate struct {
	item  AssetCenterItem
	tasks map[string]struct{}
	ips   map[string]struct{}
}

type AssetTopologyData struct {
	Root  AssetCenterItem     `json:"root"`
	Nodes []AssetTopologyNode `json:"nodes"`
	Edges []AssetTopologyEdge `json:"edges"`
}

type AssetMapDimensionStat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type AssetMapAssetItem struct {
	AssetID       string `json:"assetId,omitempty"`
	Value         string `json:"value"`
	Location      string `json:"location"`
	Province      string `json:"province,omitempty"`
	ProvinceLabel string `json:"provinceLabel,omitempty"`
	City          string `json:"city,omitempty"`
	Country       string `json:"country,omitempty"`
	Operator      string `json:"operator"`
	ASN           string `json:"asn"`
	TaskCount     int    `json:"taskCount"`
	CriticalCount int    `json:"criticalCount"`
	HighCount     int    `json:"highCount"`
	MediumCount   int    `json:"mediumCount"`
	// Direct risk was observed on the IP itself; inherited risk reaches the
	// IP through a resolved domain or certificate. The two are intentionally
	// reported separately so the UI never presents an inherited domain risk
	// as a vulnerability of the IP.
	DirectCritical    int      `json:"directCritical"`
	DirectHigh        int      `json:"directHigh"`
	DirectMedium      int      `json:"directMedium"`
	InheritedCritical int      `json:"inheritedCritical"`
	InheritedHigh     int      `json:"inheritedHigh"`
	InheritedMedium   int      `json:"inheritedMedium"`
	Ports             []string `json:"ports,omitempty"`
	Services          []string `json:"services,omitempty"`
	LastSeenAt        string   `json:"lastSeenAt,omitempty"`
	UpdatedAt         string   `json:"updatedAt"`
}

type AssetMapAssetListData struct {
	Items    []AssetMapAssetItem `json:"items"`
	Total    int                 `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"pageSize"`
}

type AssetMapRegionStat struct {
	Province          string                  `json:"province"`
	ProvinceLabel     string                  `json:"provinceLabel"`
	AssetCount        int                     `json:"assetCount"`
	TaskCount         int                     `json:"taskCount"`
	CriticalCount     int                     `json:"criticalCount"`
	HighCount         int                     `json:"highCount"`
	MediumCount       int                     `json:"mediumCount"`
	DirectCritical    int                     `json:"directCritical"`
	DirectHigh        int                     `json:"directHigh"`
	DirectMedium      int                     `json:"directMedium"`
	InheritedCritical int                     `json:"inheritedCritical"`
	InheritedHigh     int                     `json:"inheritedHigh"`
	InheritedMedium   int                     `json:"inheritedMedium"`
	Cities            []AssetMapDimensionStat `json:"cities"`
	Operators         []AssetMapDimensionStat `json:"operators"`
	ASNs              []AssetMapDimensionStat `json:"asns"`
}

type AssetMapCountryStat struct {
	Name              string                  `json:"name"`
	Count             int                     `json:"count"`
	Country           string                  `json:"country"`
	CountryLabel      string                  `json:"countryLabel"`
	AssetCount        int                     `json:"assetCount"`
	TaskCount         int                     `json:"taskCount"`
	CriticalCount     int                     `json:"criticalCount"`
	HighCount         int                     `json:"highCount"`
	MediumCount       int                     `json:"mediumCount"`
	DirectCritical    int                     `json:"directCritical"`
	DirectHigh        int                     `json:"directHigh"`
	DirectMedium      int                     `json:"directMedium"`
	InheritedCritical int                     `json:"inheritedCritical"`
	InheritedHigh     int                     `json:"inheritedHigh"`
	InheritedMedium   int                     `json:"inheritedMedium"`
	Cities            []AssetMapDimensionStat `json:"cities"`
	Operators         []AssetMapDimensionStat `json:"operators"`
	ASNs              []AssetMapDimensionStat `json:"asns"`
}

type AssetMapRiskAssetSummary struct {
	Total       int `json:"total"`
	IP          int `json:"ip"`
	Domain      int `json:"domain"`
	URL         int `json:"url"`
	Service     int `json:"service"`
	Certificate int `json:"certificate"`
	Critical    int `json:"critical"`
	High        int `json:"high"`
	Medium      int `json:"medium"`
}

type AssetMapOverviewData struct {
	TotalIPAssets      int                      `json:"totalIpAssets"`
	LocatedIPAssets    int                      `json:"locatedIpAssets"`
	UnknownIPAssets    int                      `json:"unknownIpAssets"`
	OverseasIPAssets   int                      `json:"overseasIpAssets"`
	NonPublicIPAssets  int                      `json:"nonPublicIpAssets"`
	UnresolvedIPAssets int                      `json:"unresolvedIpAssets"`
	TotalCritical      int                      `json:"totalCritical"`
	TotalHigh          int                      `json:"totalHigh"`
	TotalMedium        int                      `json:"totalMedium"`
	UpdatedAt          string                   `json:"updatedAt,omitempty"`
	Regions            []AssetMapRegionStat     `json:"regions"`
	Operators          []AssetMapDimensionStat  `json:"operators"`
	ASNs               []AssetMapDimensionStat  `json:"asns"`
	Countries          []AssetMapCountryStat    `json:"countries"`
	RiskAssets         AssetMapRiskAssetSummary `json:"riskAssets"`
}

type assetCenterTaskMeta struct {
	Name      string
	UpdatedAt time.Time
}

type assetCenterAggregate struct {
	AssetCenterItem
	tasks        map[string]struct{}
	versions     map[string]struct{}
	sources      map[string]struct{}
	services     map[string]struct{}
	portServices map[string]struct{}
	ips          map[string]struct{}
	riskFindings map[string]struct{}
}

type assetMapLocation struct {
	Country       string
	Province      string
	ProvinceLabel string
	City          string
}

type assetMapRegionAggregate struct {
	AssetMapRegionStat
	tasks     map[string]struct{}
	cities    map[string]int
	operators map[string]int
	asns      map[string]int
}

type assetMapCountryAggregate struct {
	AssetMapCountryStat
	tasks     map[string]struct{}
	cities    map[string]int
	operators map[string]int
	asns      map[string]int
}

type assetIPMetadata struct {
	Operator string
	Location string
	ASN      string
}

var assetIPMetadataResolver = ipmetadata.NewResolver(config.GetDataDir())

func lookupAssetIPMetadata(value string) assetIPMetadata {
	metadata := assetIPMetadataResolver.Lookup(value)
	return assetIPMetadata{Operator: metadata.Operator, Location: metadata.Location, ASN: metadata.ASN}
}

func normalizeAssetHost(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if ip, ok := assetstore.ParseIPLike(value); ok {
		return ip
	}
	if assetstore.LooksLikeIPv6(value) {
		return strings.TrimSuffix(value, ":")
	}
	value = strings.TrimPrefix(value, "*.")
	value = strings.Trim(value, "[]")
	value = strings.TrimSuffix(value, ".")
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Hostname() != "" {
		return strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	}
	if parsed, err := url.Parse("//" + value); err == nil && parsed.Hostname() != "" {
		return strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	}
	return value
}

func assetHostType(host string) string {
	return assetstore.ClassifyHost(host)
}

func normalizeAssetEndpoint(rawURL, fallbackHost, fallbackScheme string, fallbackPort int) (string, string) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err == nil && parsed.Hostname() != "" {
		host := normalizeAssetHost(parsed.Hostname())
		scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
		if scheme == "" {
			scheme = fallbackScheme
		}
		port, _ := strconv.Atoi(parsed.Port())
		if port == 0 {
			port = fallbackPort
		}
		if port == 0 {
			if scheme == "https" {
				port = 443
			} else if scheme == "http" {
				port = 80
			}
		}
		if host != "" && scheme != "" && port > 0 {
			return fmt.Sprintf("%s://%s:%d", scheme, host, port), host
		}
	}

	host := normalizeAssetHost(fallbackHost)
	scheme := strings.ToLower(strings.TrimSpace(fallbackScheme))
	if host == "" || scheme == "" || fallbackPort <= 0 {
		return "", host
	}
	return fmt.Sprintf("%s://%s:%d", scheme, host, fallbackPort), host
}

func assetCenterTaskKey(taskID string, version int) string {
	return taskID + "\x00" + strconv.Itoa(version)
}

func assetCenterFallbackTaskName(taskID, scanType string) string {
	if strings.TrimSpace(taskID) == "" {
		return ""
	}
	if scanType == "subdomain" {
		return "子域名扫描任务"
	}
	return "扫描任务 " + taskID
}

func addAssetCenterObservation(
	aggregates map[string]*assetCenterAggregate,
	assetType, value, taskID, taskName, source string,
	version int,
	observedAt, updatedAt time.Time,
	severity, service string,
	findingKey ...string,
) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	key := assetType + "\x00" + value
	aggregate := aggregates[key]
	if aggregate == nil {
		aggregate = &assetCenterAggregate{
			AssetCenterItem: AssetCenterItem{Type: assetType, Value: value, Status: "active", FirstSeenAt: observedAt, LastSeenAt: observedAt, UpdatedAt: updatedAt},
			tasks:           make(map[string]struct{}),
			versions:        make(map[string]struct{}),
			services:        make(map[string]struct{}),
			portServices:    make(map[string]struct{}),
			sources:         make(map[string]struct{}),
			ips:             make(map[string]struct{}),
			riskFindings:    make(map[string]struct{}),
		}
		aggregates[key] = aggregate
	}

	aggregate.ObservationCount++
	if !observedAt.IsZero() && (aggregate.FirstSeenAt.IsZero() || observedAt.Before(aggregate.FirstSeenAt)) {
		aggregate.FirstSeenAt = observedAt
	}
	if observedAt.After(aggregate.LastSeenAt) {
		aggregate.LastSeenAt = observedAt
	}
	if updatedAt.After(aggregate.UpdatedAt) {
		aggregate.UpdatedAt = updatedAt
	}
	if source = strings.TrimSpace(source); source != "" {
		aggregate.sources[source] = struct{}{}
	}
	if taskID != "" {
		aggregate.tasks[taskID] = struct{}{}
		aggregate.versions[assetCenterTaskKey(taskID, version)] = struct{}{}
	}
	if service = strings.TrimSpace(service); service != "" {
		aggregate.services[service] = struct{}{}
		aggregate.portServices[service] = struct{}{}
	}
	if riskSeverity := normalizeAssetRiskSeverity(severity); riskSeverity != "" {
		riskKey := ""
		if len(findingKey) > 0 {
			riskKey = strings.TrimSpace(findingKey[0])
		}
		if riskKey == "" {
			// Legacy collectors that cannot provide a stable finding identity
			// still de-duplicate within a single task version.
			riskKey = taskID + "|" + strconv.Itoa(version) + "|" + value
		}
		if aggregate.riskFindings == nil {
			aggregate.riskFindings = make(map[string]struct{})
		}
		aggregate.riskFindings[riskSeverity+"\x00"+riskKey] = struct{}{}
	}
	_ = taskName
}

// normalizeAssetRiskSeverity maps the severity spellings used by the scan
// modules onto the two levels that drive risk badges.
func normalizeAssetRiskSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical", "critical risk", "severe", "fatal", "严重", "紧急":
		return "critical"
	case "high", "high risk", "高危":
		return "high"
	default:
		return ""
	}
}

func countAssetRiskFindings(findings map[string]struct{}) (critical, high int) {
	for key := range findings {
		switch {
		case strings.HasPrefix(key, "critical\x00"):
			critical++
		case strings.HasPrefix(key, "high\x00"):
			high++
		}
	}
	return critical, high
}

func addAssetCenterDomainIPs(aggregates map[string]*assetCenterAggregate, domain string, ips []string) {
	domain = normalizeAssetHost(domain)
	if domain == "" {
		return
	}
	aggregate := aggregates["domain\x00"+domain]
	if aggregate == nil {
		return
	}
	for _, ip := range ips {
		if normalized := net.ParseIP(strings.TrimSpace(ip)); normalized != nil {
			aggregate.ips[normalized.String()] = struct{}{}
		}
	}
}

func addAssetCenterHostAndEndpoint(
	aggregates map[string]*assetCenterAggregate,
	host, rawURL, taskID, taskName, source string,
	version int,
	observedAt, updatedAt time.Time,
	includeEndpoint bool,
	scheme string, port int, severity, service string,
	findingKey ...string,
) {
	addAssetCenterHostAndEndpointTyped(aggregates, "", host, rawURL, taskID, taskName, source, version, observedAt, updatedAt, includeEndpoint, scheme, port, severity, service, findingKey...)
}

func addAssetCenterHostAndEndpointTyped(
	aggregates map[string]*assetCenterAggregate,
	hostType, host, rawURL, taskID, taskName, source string,
	version int,
	observedAt, updatedAt time.Time,
	includeEndpoint bool,
	scheme string, port int, severity, service string,
	findingKey ...string,
) {
	host = normalizeAssetHost(host)
	if host == "" {
		return
	}
	if hostType == "" {
		hostType = assetHostType(host)
	}
	addAssetCenterObservation(aggregates, hostType, host, taskID, taskName, source, version, observedAt, updatedAt, severity, "", findingKey...)
	endpoint, endpointHost := normalizeAssetEndpoint(rawURL, host, scheme, port)
	if endpointHost != "" && endpointHost != host {
		addAssetCenterObservation(aggregates, assetHostType(endpointHost), endpointHost, taskID, taskName, source, version, observedAt, updatedAt, severity, "", findingKey...)
	}
	if includeEndpoint && endpoint != "" {
		addAssetCenterObservation(aggregates, "endpoint", endpoint, taskID, taskName, source, version, observedAt, updatedAt, severity, service, findingKey...)
	}
}

func addAssetCenterEndpointServices(aggregates map[string]*assetCenterAggregate, endpoint string, services []string) {
	aggregate := aggregates["endpoint\x00"+strings.TrimSpace(endpoint)]
	if aggregate == nil {
		return
	}
	for _, service := range services {
		if service = strings.TrimSpace(service); service != "" {
			aggregate.services[service] = struct{}{}
		}
	}
}

func assetCenterPrimaryPortService(services map[string]struct{}) string {
	labels := make([]string, 0, len(services))
	for service := range services {
		labels = append(labels, service)
	}
	if len(labels) == 0 {
		return ""
	}
	sort.Strings(labels)
	return labels[0]
}

func parseAssetFingerprintLabels(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "null") || raw == "[]" {
		return nil
	}

	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		values = strings.Split(raw, ",")
	}

	seen := make(map[string]struct{}, len(values))
	labels := make([]string, 0, len(values))
	for _, value := range values {
		label := strings.Trim(strings.TrimSpace(value), "\"'")
		key := strings.ToLower(label)
		if label == "" || key == "null" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		labels = append(labels, label)
	}
	return labels
}

func assetFingerprintServices(fingerprint database.FingerprintInfo) []string {
	services := make([]string, 0)
	seen := make(map[string]struct{})
	for _, labels := range [][]string{
		parseAssetFingerprintLabels(fingerprint.HighRiskFingerprints),
		parseAssetFingerprintLabels(fingerprint.Fingerprints),
	} {
		for _, label := range labels {
			key := strings.ToLower(label)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			services = append(services, label)
		}
	}
	return services
}

func portScanEndpointInfo(result database.PortScanResult) (string, string) {
	protocol := strings.ToLower(strings.TrimSpace(result.Protocol))
	if protocol == "" {
		protocol = strings.ToLower(strings.TrimSpace(result.Transport))
	}
	service := strings.Trim(strings.Join([]string{result.Product, result.ServiceVersion}, " "), " ")
	return protocol, service
}

func populateAssetCenterDisplayFields(item *AssetCenterItem) {
	switch item.Type {
	case "ip":
		metadata := lookupAssetIPMetadata(item.Value)
		item.Operator = metadata.Operator
		item.Location = metadata.Location
		item.ASN = metadata.ASN
	case "endpoint":
		parsed, err := url.Parse(item.Value)
		if err != nil {
			return
		}
		item.EndpointHost = normalizeAssetHost(parsed.Hostname())
		item.Protocol = strings.ToLower(strings.TrimSpace(parsed.Scheme))
		item.Port, _ = strconv.Atoi(parsed.Port())
	}
}

func assetCenterTaskMetadata(dbTasks []database.VulnScanTask) map[string]assetCenterTaskMeta {
	metadata := make(map[string]assetCenterTaskMeta, len(dbTasks))
	for _, task := range dbTasks {
		name := strings.TrimSpace(task.TaskName)
		if name == "" {
			name = assetCenterFallbackTaskName(task.ID, task.ScanType)
		}
		metadata[assetCenterTaskKey(task.ID, task.Version)] = assetCenterTaskMeta{Name: name, UpdatedAt: task.UpdatedAt}
	}
	return metadata
}

func assetCenterTaskInfo(metadata map[string]assetCenterTaskMeta, taskID string, version int, observedAt time.Time) (string, time.Time) {
	if strings.TrimSpace(taskID) == "" {
		return "", observedAt
	}
	if meta, ok := metadata[assetCenterTaskKey(taskID, version)]; ok {
		return meta.Name, meta.UpdatedAt
	}
	return assetCenterFallbackTaskName(taskID, ""), observedAt
}
