package asset

import (
	"fmt"
	"net"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type LedgerEntityCategory string

const (
	LedgerEntityRoot      LedgerEntityCategory = "root"
	LedgerEntityIP        LedgerEntityCategory = "ip"
	LedgerEntitySubdomain LedgerEntityCategory = "subdomain"
	LedgerEntityPort      LedgerEntityCategory = "port"
	LedgerEntityWebsite   LedgerEntityCategory = "website"
)

type LedgerEntity struct {
	ID                   string    `json:"id"`
	Category             string    `json:"category"`
	Value                string    `json:"value"`
	DisplayValue         string    `json:"displayValue"`
	Name                 string    `json:"name,omitempty"`
	RootDomain           string    `json:"rootDomain,omitempty"`
	Host                 string    `json:"host,omitempty"`
	IPs                  []string  `json:"ips,omitempty"`
	Domains              []string  `json:"domains,omitempty"`
	Port                 int       `json:"port,omitempty"`
	Protocol             string    `json:"protocol,omitempty"`
	Transport            string    `json:"transport,omitempty"`
	Service              string    `json:"service,omitempty"`
	Title                string    `json:"title,omitempty"`
	HTTPStatus           int       `json:"httpStatus,omitempty"`
	Fingerprints         []string  `json:"fingerprints,omitempty"`
	HighRiskFingerprints []string  `json:"highRiskFingerprints,omitempty"`
	SubdomainCount       int       `json:"subdomainCount,omitempty"`
	IPCount              int       `json:"ipCount,omitempty"`
	PortCount            int       `json:"portCount,omitempty"`
	WebsiteCount         int       `json:"websiteCount,omitempty"`
	AssetID              string    `json:"assetId,omitempty"`
	Sources              []string  `json:"sources"`
	SourceTaskIDs        []string  `json:"sourceTaskIds"`
	LastTaskID           string    `json:"lastTaskId,omitempty"`
	LastTaskName         string    `json:"lastTaskName,omitempty"`
	FirstSeenAt          time.Time `json:"firstSeenAt"`
	LastSeenAt           time.Time `json:"lastSeenAt"`
	Status               string    `json:"status"`
	Verification         string    `json:"verification"`
	Archived             bool      `json:"archived"`
}

type LedgerEntityCounts struct {
	Root      int `json:"root"`
	IP        int `json:"ip"`
	Subdomain int `json:"subdomain"`
	Port      int `json:"port"`
	Website   int `json:"website"`
}

type ledgerEntitySet struct {
	roots      []LedgerEntity
	ips        []LedgerEntity
	subdomains []LedgerEntity
	ports      []LedgerEntity
	websites   []LedgerEntity
}

type ledgerAssetEntry struct {
	asset        database.UnifiedAsset
	observations []database.UnifiedAssetObservation
	sources      []string
	taskIDs      []string
	first        time.Time
	last         time.Time
	verification string
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func mergeLabels(current string, additions ...string) string {
	labels := uniqueStrings(append(strings.Split(current, " / "), additions...))
	return strings.Join(labels, " / ")
}

func mergeUnique(current []string, additions ...string) []string {
	return uniqueStrings(append(append([]string{}, current...), additions...))
}

func verificationRankLocal(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "verified":
		return 3
	case "observed":
		return 2
	case "declared":
		return 1
	default:
		return 0
	}
}

func bestVerification(current, incoming string) string {
	if verificationRankLocal(incoming) > verificationRankLocal(current) {
		return strings.ToLower(strings.TrimSpace(incoming))
	}
	return strings.ToLower(strings.TrimSpace(current))
}

func containsAny(values []string, candidates ...string) bool {
	wanted := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		wanted[strings.ToLower(candidate)] = struct{}{}
	}
	for _, value := range values {
		if _, ok := wanted[strings.ToLower(value)]; ok {
			return true
		}
	}
	return false
}

func buildLedgerEntry(assetRow database.UnifiedAsset, observations []database.UnifiedAssetObservation) *ledgerAssetEntry {
	entry := &ledgerAssetEntry{asset: assetRow, observations: observations, first: assetRow.FirstSeenAt, last: assetRow.LastSeenAt, verification: assetRow.Verification}
	sourceSet := map[string]struct{}{}
	taskSet := map[string]struct{}{}
	for _, observation := range observations {
		if source := strings.TrimSpace(observation.Source); source != "" {
			sourceSet[source] = struct{}{}
		}
		if taskID := assetstore.ObservationTaskID(observation); taskID != "" {
			taskSet[taskID] = struct{}{}
		}
		if !observation.ObservedAt.IsZero() {
			if entry.first.IsZero() || observation.ObservedAt.Before(entry.first) {
				entry.first = observation.ObservedAt
			}
			if observation.ObservedAt.After(entry.last) {
				entry.last = observation.ObservedAt
			}
		}
		origin := assetstore.ObservationOrigin(observation)
		if origin == "manual" || origin == "import" {
			if entry.verification == "" {
				entry.verification = "declared"
			}
		} else {
			entry.verification = bestVerification(entry.verification, "observed")
		}
	}
	entry.sources = make([]string, 0, len(sourceSet))
	for source := range sourceSet {
		entry.sources = append(entry.sources, source)
	}
	sort.Strings(entry.sources)
	entry.taskIDs = make([]string, 0, len(taskSet))
	for taskID := range taskSet {
		entry.taskIDs = append(entry.taskIDs, taskID)
	}
	sort.Strings(entry.taskIDs)
	if strings.TrimSpace(entry.verification) == "" {
		entry.verification = "declared"
	}
	return entry
}

func rootForHost(host string, ipRoots map[string]map[string]struct{}) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if normalized, ok := assetstore.ParseIPLike(host); ok {
		roots := ipRoots[normalized]
		if len(roots) == 0 {
			return ""
		}
		values := make([]string, 0, len(roots))
		for root := range roots {
			values = append(values, root)
		}
		sort.Strings(values)
		return values[0]
	}
	return assetTopologyRootDomain(host)
}

func domainIPsForHost(host string, resolved map[string][]string) []string {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	if normalized, ok := assetstore.ParseIPLike(host); ok {
		return []string{normalized}
	}
	return uniqueStrings(resolved[host])
}

func isWebDetailSource(source string) bool {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "fingerprint", "dirscan", "fofa", "hunter", "quake", "uncover":
		return true
	}
	return false
}

func websiteDetails(observations []database.UnifiedAssetObservation) (string, int, []string, []string) {
	title := ""
	status := 0
	fingerprints := []string{}
	highRisk := []string{}
	for _, observation := range observations {
		if isWebDetailSource(observation.Source) {
			if title == "" {
				title = unifiedAttrString(observation.Attributes, "pageTitle")
				if title == "" {
					title = unifiedAttrString(observation.Attributes, "title")
				}
			}
			if status == 0 {
				status = unifiedAttrInt(observation.Attributes, "status")
				if status == 0 {
					status = unifiedAttrInt(observation.Attributes, "statusCode")
				}
			}
		}
		fingerprints = mergeUnique(fingerprints, unifiedAttrStrings(observation.Attributes, "fingerprints")...)
		highRisk = mergeUnique(highRisk, unifiedAttrStrings(observation.Attributes, "highRiskFingerprints")...)
	}
	return title, status, fingerprints, highRisk
}

func normalizePortProtocol(protocol, transport string) string {
	transport = strings.ToLower(strings.TrimSpace(transport))
	labels := strings.Split(protocol, " / ")
	result := make([]string, 0, len(labels))
	seen := map[string]struct{}{}
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" || strings.EqualFold(label, transport) {
			continue
		}
		key := strings.ToLower(label)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, label)
	}
	return strings.Join(result, " / ")
}

func buildLedgerEntitySet(db *gorm.DB, includeArchived bool) (ledgerEntitySet, error) {
	set := ledgerEntitySet{roots: []LedgerEntity{}, ips: []LedgerEntity{}, subdomains: []LedgerEntity{}, ports: []LedgerEntity{}, websites: []LedgerEntity{}}
	var assets []database.UnifiedAsset
	assetQuery := db.Order("last_seen_at DESC, id ASC")
	if !includeArchived {
		assetQuery = assetQuery.Where("archived_at IS NULL")
	}
	if err := assetQuery.Find(&assets).Error; err != nil {
		return set, err
	}
	var observations []database.UnifiedAssetObservation
	if err := db.Order("observed_at DESC, created_at DESC").Find(&observations).Error; err != nil {
		return set, err
	}
	byAsset := indexAssetCenterObservations(observations)
	// Keep historical evidence in storage while excluding invalid hosts and
	// assets backed solely by non-network JS scan results from the ledger.
	assets = slices.DeleteFunc(assets, func(row database.UnifiedAsset) bool {
		assetObservations, known := byAsset[row.ID]
		return (known && len(assetObservations) == 0) ||
			(row.Type == assetstore.TypeDomain && !assetstore.IsDomainHost(row.CanonicalValue))
	})
	archivedRoots := map[string]struct{}{}
	if !includeArchived {
		var archivedAssets []database.UnifiedAsset
		if err := db.Where("archived_at IS NOT NULL AND type = ?", assetstore.TypeDomain).Select("id", "canonical_value").Find(&archivedAssets).Error; err != nil {
			return set, err
		}
		for _, assetRow := range archivedAssets {
			if assetObservations, known := byAsset[assetRow.ID]; known && len(assetObservations) == 0 {
				continue
			}
			if assetTopologyRootDomain(assetRow.CanonicalValue) == assetRow.CanonicalValue {
				archivedRoots[assetRow.CanonicalValue] = struct{}{}
			}
		}
	}
	byID := make(map[string]database.UnifiedAsset, len(assets))
	entries := make(map[string]*ledgerAssetEntry, len(assets))
	for _, assetRow := range assets {
		byID[assetRow.ID] = assetRow
		entries[assetRow.ID] = buildLedgerEntry(assetRow, byAsset[assetRow.ID])
	}
	resolved, err := loadUnifiedResolvedDomainIPs(db)
	if err != nil {
		return set, err
	}
	for id, assetObservations := range byAsset {
		assetRow, ok := byID[id]
		if !ok || assetRow.Type != assetstore.TypeIP {
			continue
		}
		for _, observation := range assetObservations {
			parent := unifiedAttrString(observation.Attributes, "subdomain")
			if parent != "" {
				resolved[parent] = append(resolved[parent], assetRow.CanonicalValue)
			}
		}
	}
	for domain, ips := range resolved {
		resolved[domain] = uniqueStrings(ips)
	}

	type domainRecord struct {
		entry *ledgerAssetEntry
		value string
		root  string
		ips   []string
	}
	domains := []domainRecord{}
	ipRoots := map[string]map[string]struct{}{}
	for _, assetRow := range assets {
		if assetRow.Type != assetstore.TypeDomain {
			continue
		}
		entry := entries[assetRow.ID]
		value := assetRow.CanonicalValue
		root := assetTopologyRootDomain(value)
		ips := uniqueStrings(resolved[value])
		domains = append(domains, domainRecord{entry: entry, value: value, root: root, ips: ips})
		if root != "" {
			for _, ip := range ips {
				if ipRoots[ip] == nil {
					ipRoots[ip] = map[string]struct{}{}
				}
				ipRoots[ip][root] = struct{}{}
			}
		}
	}

	ipDomains := map[string][]string{}
	for _, domain := range domains {
		for _, ip := range domain.ips {
			ipDomains[ip] = append(ipDomains[ip], domain.value)
		}
	}
	for id, assetObservations := range byAsset {
		assetRow, ok := byID[id]
		if !ok || assetRow.Type != assetstore.TypeIP {
			continue
		}
		for _, observation := range assetObservations {
			parent := unifiedAttrString(observation.Attributes, "subdomain")
			if parent == "" {
				parent = unifiedAttrString(observation.Attributes, "domain")
			}
			if assetstore.IsDomainHost(strings.TrimSuffix(parent, ".")) {
				ipDomains[assetRow.CanonicalValue] = append(ipDomains[assetRow.CanonicalValue], parent)
			}
		}
	}
	for ip, values := range ipDomains {
		ipDomains[ip] = uniqueStrings(values)
	}

	// Group HTTP(S) URL assets by protocol, canonical host and effective port.
	websiteIndex := map[string]int{}
	for _, assetRow := range assets {
		if assetRow.Type != assetstore.TypeURL {
			continue
		}
		origin, err := assetstore.WebsiteOrigin(assetRow.CanonicalValue)
		if err != nil {
			continue
		}
		entry := entries[assetRow.ID]
		parsed, _ := url.Parse(origin)
		host, protocol := parsed.Hostname(), parsed.Scheme
		port := 80
		if protocol == "https" {
			port = 443
		}
		if parsed.Port() != "" {
			port, _ = strconv.Atoi(parsed.Port())
		}
		title, status, fingerprints, highRisk := websiteDetails(entry.observations)
		// Every stored HTTP(S) URL is a website candidate, independent of source.
		key := assetRow.ScopeKey + "\x00" + origin
		index, exists := websiteIndex[key]
		if !exists {
			index = len(set.websites)
			websiteIndex[key] = index
			set.websites = append(set.websites, LedgerEntity{
				ID: "website:" + assetstore.AssetID(assetRow.ScopeKey, assetstore.TypeURL, origin), Category: string(LedgerEntityWebsite),
				Value: origin, DisplayValue: origin, Name: assetRow.Name,
				RootDomain: rootForHost(host, ipRoots), Host: host, IPs: domainIPsForHost(host, resolved),
				Port: port, Protocol: protocol, Transport: "tcp",
				AssetID: assetRow.ID, FirstSeenAt: entry.first, LastSeenAt: entry.last,
				Status: assetRow.Status, Verification: entry.verification, Archived: true,
			})
		}
		website := &set.websites[index]
		website.Sources = mergeUnique(website.Sources, entry.sources...)
		website.SourceTaskIDs = mergeUnique(website.SourceTaskIDs, entry.taskIDs...)
		website.Fingerprints = mergeUnique(website.Fingerprints, fingerprints...)
		website.HighRiskFingerprints = mergeUnique(website.HighRiskFingerprints, highRisk...)
		if website.Title == "" {
			website.Title = title
		}
		if website.HTTPStatus == 0 {
			website.HTTPStatus = status
		}
		if website.Name == "" {
			website.Name = assetRow.Name
		}
		if website.FirstSeenAt.IsZero() || (!entry.first.IsZero() && entry.first.Before(website.FirstSeenAt)) {
			website.FirstSeenAt = entry.first
		}
		if entry.last.After(website.LastSeenAt) {
			website.LastSeenAt = entry.last
		}
		website.Verification = bestVerification(website.Verification, entry.verification)
		if assetRow.ArchivedAt == nil {
			if website.Archived {
				website.AssetID = assetRow.ID
				website.Status = assetRow.Status
			}
			website.Archived = false
			if assetRow.Status == "active" {
				website.Status = "active"
			}
		}
	}

	portMap := map[string]*LedgerEntity{}
	portKeys := []string{}
	addPort := func(host string, port int, transport, protocol, service, assetID string, entry *ledgerAssetEntry) *LedgerEntity {
		transport = strings.ToLower(strings.TrimSpace(transport))
		if transport == "" {
			transport = "tcp"
		}
		protocol = normalizePortProtocol(protocol, transport)
		host = strings.TrimSpace(host)
		if host == "" || port <= 0 {
			return nil
		}
		key := fmt.Sprintf("%s\x00%d\x00%s", host, port, transport)
		entity := portMap[key]
		if entity == nil {
			entity = &LedgerEntity{
				ID: fmt.Sprintf("port:%s:%d:%s", host, port, transport), Category: string(LedgerEntityPort),
				Value: net.JoinHostPort(host, strconv.Itoa(port)), DisplayValue: net.JoinHostPort(host, strconv.Itoa(port)),
				Host: host, Port: port, Transport: transport, Protocol: protocol,
				RootDomain: rootForHost(host, ipRoots), IPs: domainIPsForHost(host, resolved),
				AssetID: assetID, Sources: []string{}, SourceTaskIDs: []string{},
			}
			portMap[key] = entity
			portKeys = append(portKeys, key)
		}
		if protocol != "" {
			entity.Protocol = mergeLabels(entity.Protocol, protocol)
		}
		if service != "" {
			entity.Service = mergeLabels(entity.Service, service)
		}
		if entry != nil {
			entity.Sources = mergeUnique(entity.Sources, entry.sources...)
			entity.SourceTaskIDs = mergeUnique(entity.SourceTaskIDs, entry.taskIDs...)
			entity.Verification = bestVerification(entity.Verification, entry.verification)
			if entity.AssetID == "" {
				entity.AssetID = entry.asset.ID
			}
			if entry.asset.ArchivedAt != nil {
				entity.Archived = true
			}
			if entity.Name == "" {
				entity.Name = entry.asset.Name
			}
			if entity.FirstSeenAt.IsZero() || (!entry.first.IsZero() && entry.first.Before(entity.FirstSeenAt)) {
				entity.FirstSeenAt = entry.first
			}
			if entry.last.After(entity.LastSeenAt) {
				entity.LastSeenAt = entry.last
			}
			if entity.Status == "" || entry.asset.Status == "active" {
				entity.Status = entry.asset.Status
			}
		}
		return entity
	}

	for _, assetRow := range assets {
		if assetRow.Type != assetstore.TypeIP && assetRow.Type != assetstore.TypeDomain {
			continue
		}
		entry := entries[assetRow.ID]
		host := assetRow.CanonicalValue
		for _, observation := range entry.observations {
			port := unifiedAttrInt(observation.Attributes, "port")
			if port <= 0 {
				continue
			}
			transport := unifiedAttrString(observation.Attributes, "transport")
			protocol := unifiedAttrString(observation.Attributes, "protocol")
			if protocol == "" {
				protocol = transport
			}
			service := strings.TrimSpace(strings.Join([]string{unifiedAttrString(observation.Attributes, "product"), unifiedAttrString(observation.Attributes, "serviceVersion")}, " "))
			entity := addPort(host, port, transport, protocol, service, assetRow.ID, entry)
			if entity != nil {
				entity.Fingerprints = mergeUnique(entity.Fingerprints, unifiedAttrStrings(observation.Attributes, "fingerprints")...)
				entity.HighRiskFingerprints = mergeUnique(entity.HighRiskFingerprints, unifiedAttrStrings(observation.Attributes, "highRiskFingerprints")...)
			}
		}
	}
	for _, website := range set.websites {
		entity := addPort(website.Host, website.Port, website.Transport, website.Protocol, "", website.AssetID, nil)
		if entity == nil {
			continue
		}
		entity.WebsiteCount++
		entity.Fingerprints = mergeUnique(entity.Fingerprints, website.Fingerprints...)
		entity.HighRiskFingerprints = mergeUnique(entity.HighRiskFingerprints, website.HighRiskFingerprints...)
		entity.Sources = mergeUnique(entity.Sources, website.Sources...)
		entity.SourceTaskIDs = mergeUnique(entity.SourceTaskIDs, website.SourceTaskIDs...)
		entity.Verification = bestVerification(entity.Verification, website.Verification)
		if entity.Title == "" {
			entity.Title = website.Title
		}
		if entity.HTTPStatus == 0 {
			entity.HTTPStatus = website.HTTPStatus
		}
		if entity.FirstSeenAt.IsZero() || (!website.FirstSeenAt.IsZero() && website.FirstSeenAt.Before(entity.FirstSeenAt)) {
			entity.FirstSeenAt = website.FirstSeenAt
		}
		if website.LastSeenAt.After(entity.LastSeenAt) {
			entity.LastSeenAt = website.LastSeenAt
		}
		if entity.Status == "" || website.Status == "active" {
			entity.Status = website.Status
		}
		if website.Archived {
			entity.Archived = true
		}
	}
	for _, key := range portKeys {
		set.ports = append(set.ports, *portMap[key])
	}

	portCountByHost := map[string]int{}
	for _, entity := range set.ports {
		portCountByHost[entity.Host]++
	}
	websiteCountByHost := map[string]int{}
	for _, entity := range set.websites {
		websiteCountByHost[entity.Host]++
	}

	for _, assetRow := range assets {
		if assetRow.Type != assetstore.TypeIP {
			continue
		}
		entry := entries[assetRow.ID]
		value := assetRow.CanonicalValue
		root := rootForHost(value, ipRoots)
		set.ips = append(set.ips, LedgerEntity{
			ID: "ip:" + value, Category: string(LedgerEntityIP), Value: value, DisplayValue: assetRow.DisplayValue, Name: assetRow.Name,
			RootDomain: root, Host: value, IPs: []string{value}, Domains: ipDomains[value],
			PortCount: portCountByHost[value], WebsiteCount: websiteCountByHost[value],
			AssetID: assetRow.ID, Sources: entry.sources, SourceTaskIDs: entry.taskIDs,
			FirstSeenAt: entry.first, LastSeenAt: entry.last, Status: assetRow.Status, Verification: entry.verification,
			Archived: assetRow.ArchivedAt != nil,
		})
	}
	for _, domain := range domains {
		if domain.root == "" || domain.value == domain.root {
			continue
		}
		set.subdomains = append(set.subdomains, LedgerEntity{
			ID: "subdomain:" + domain.value, Category: string(LedgerEntitySubdomain), Value: domain.value, DisplayValue: domain.entry.asset.DisplayValue, Name: domain.entry.asset.Name,
			RootDomain: domain.root, Host: domain.value, IPs: domain.ips, IPCount: len(domain.ips),
			PortCount: portCountByHost[domain.value], WebsiteCount: websiteCountByHost[domain.value],
			AssetID: domain.entry.asset.ID, Sources: domain.entry.sources, SourceTaskIDs: domain.entry.taskIDs,
			FirstSeenAt: domain.entry.first, LastSeenAt: domain.entry.last, Status: domain.entry.asset.Status, Verification: domain.entry.verification,
			Archived: domain.entry.asset.ArchivedAt != nil,
		})
	}

	rootMap := map[string]*LedgerEntity{}
	rootSubdomainHosts := map[string]map[string]struct{}{}
	for _, domain := range domains {
		if domain.root == "" {
			continue
		}
		if _, archived := archivedRoots[domain.root]; archived {
			continue
		}
		root := rootMap[domain.root]
		if root == nil {
			root = &LedgerEntity{
				ID: "root:" + domain.root, Category: string(LedgerEntityRoot), Value: domain.root, DisplayValue: domain.root,
				RootDomain: domain.root, Host: domain.root, Sources: []string{}, SourceTaskIDs: []string{}, Status: "active",
			}
			rootMap[domain.root] = root
		}
		root.IPs = mergeUnique(root.IPs, domain.ips...)
		root.Sources = mergeUnique(root.Sources, domain.entry.sources...)
		root.SourceTaskIDs = mergeUnique(root.SourceTaskIDs, domain.entry.taskIDs...)
		root.Verification = bestVerification(root.Verification, domain.entry.verification)
		if root.FirstSeenAt.IsZero() || (!domain.entry.first.IsZero() && domain.entry.first.Before(root.FirstSeenAt)) {
			root.FirstSeenAt = domain.entry.first
		}
		if domain.entry.last.After(root.LastSeenAt) {
			root.LastSeenAt = domain.entry.last
		}
		if domain.value == domain.root {
			root.AssetID = domain.entry.asset.ID
			root.Name = domain.entry.asset.Name
			root.Status = domain.entry.asset.Status
			root.Archived = domain.entry.asset.ArchivedAt != nil
		} else {
			if rootSubdomainHosts[domain.root] == nil {
				rootSubdomainHosts[domain.root] = map[string]struct{}{}
			}
			rootSubdomainHosts[domain.root][domain.value] = struct{}{}
		}
	}
	ensureRoot := func(value string) *LedgerEntity {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		if _, archived := archivedRoots[value]; archived {
			return nil
		}
		root := rootMap[value]
		if root == nil {
			root = &LedgerEntity{
				ID: "root:" + value, Category: string(LedgerEntityRoot), Value: value, DisplayValue: value,
				RootDomain: value, Host: value, Sources: []string{}, SourceTaskIDs: []string{}, Status: "active",
			}
			rootMap[value] = root
		}
		return root
	}
	for _, entity := range set.ports {
		root := ensureRoot(entity.RootDomain)
		if root == nil {
			continue
		}
		root.PortCount++
		root.Sources = mergeUnique(root.Sources, entity.Sources...)
		root.SourceTaskIDs = mergeUnique(root.SourceTaskIDs, entity.SourceTaskIDs...)
		root.Verification = bestVerification(root.Verification, entity.Verification)
		if root.FirstSeenAt.IsZero() || (!entity.FirstSeenAt.IsZero() && entity.FirstSeenAt.Before(root.FirstSeenAt)) {
			root.FirstSeenAt = entity.FirstSeenAt
		}
		if entity.LastSeenAt.After(root.LastSeenAt) {
			root.LastSeenAt = entity.LastSeenAt
		}
	}
	for _, entity := range set.websites {
		root := ensureRoot(entity.RootDomain)
		if root == nil {
			continue
		}
		root.WebsiteCount++
		root.Sources = mergeUnique(root.Sources, entity.Sources...)
		root.SourceTaskIDs = mergeUnique(root.SourceTaskIDs, entity.SourceTaskIDs...)
		root.Verification = bestVerification(root.Verification, entity.Verification)
		if net.ParseIP(entity.Host) != nil {
			root.IPs = mergeUnique(root.IPs, entity.Host)
		} else if entity.Host != "" && entity.Host != root.Value {
			if rootSubdomainHosts[root.Value] == nil {
				rootSubdomainHosts[root.Value] = map[string]struct{}{}
			}
			rootSubdomainHosts[root.Value][entity.Host] = struct{}{}
		}
		if root.FirstSeenAt.IsZero() || (!entity.FirstSeenAt.IsZero() && entity.FirstSeenAt.Before(root.FirstSeenAt)) {
			root.FirstSeenAt = entity.FirstSeenAt
		}
		if entity.LastSeenAt.After(root.LastSeenAt) {
			root.LastSeenAt = entity.LastSeenAt
		}
	}
	for value, root := range rootMap {
		root.IPs = uniqueStrings(root.IPs)
		root.IPCount = len(root.IPs)
		root.SubdomainCount = len(rootSubdomainHosts[value])
		if root.Status == "" {
			root.Status = "active"
		}
		if root.FirstSeenAt.IsZero() {
			root.FirstSeenAt = root.LastSeenAt
		}
		set.roots = append(set.roots, *root)
	}

	sort.Slice(set.roots, func(i, j int) bool { return set.roots[i].Value < set.roots[j].Value })
	sort.Slice(set.subdomains, func(i, j int) bool { return set.subdomains[i].Value < set.subdomains[j].Value })
	sort.Slice(set.ips, func(i, j int) bool { return set.ips[i].Value < set.ips[j].Value })
	sort.Slice(set.ports, func(i, j int) bool {
		if set.ports[i].Port == set.ports[j].Port {
			return set.ports[i].Host < set.ports[j].Host
		}
		return set.ports[i].Port < set.ports[j].Port
	})
	sort.Slice(set.websites, func(i, j int) bool {
		if set.websites[i].LastSeenAt.Equal(set.websites[j].LastSeenAt) {
			return set.websites[i].Value < set.websites[j].Value
		}
		return set.websites[i].LastSeenAt.After(set.websites[j].LastSeenAt)
	})
	return set, nil
}

func (set ledgerEntitySet) category(category LedgerEntityCategory) []LedgerEntity {
	switch category {
	case LedgerEntityRoot:
		return set.roots
	case LedgerEntityIP:
		return set.ips
	case LedgerEntitySubdomain:
		return set.subdomains
	case LedgerEntityPort:
		return set.ports
	case LedgerEntityWebsite:
		return set.websites
	default:
		return nil
	}
}

func (set ledgerEntitySet) counts() LedgerEntityCounts {
	return LedgerEntityCounts{Root: len(set.roots), IP: len(set.ips), Subdomain: len(set.subdomains), Port: len(set.ports), Website: len(set.websites)}
}

// filteredCounts reports how many rows each category would keep under the
// active filters. The asset center uses it to size scope-aware exports.
func (set ledgerEntitySet) filteredCounts(filter assetstore.LedgerFilter) LedgerEntityCounts {
	return LedgerEntityCounts{
		Root:      len(filterLedgerEntities(set.roots, filter)),
		IP:        len(filterLedgerEntities(set.ips, filter)),
		Subdomain: len(filterLedgerEntities(set.subdomains, filter)),
		Port:      len(filterLedgerEntities(set.ports, filter)),
		Website:   len(filterLedgerEntities(set.websites, filter)),
	}
}

// LoadLedgerEntityCounts returns the same deduplicated, non-archived counts
// used by the asset center ledger endpoint.
func LoadLedgerEntityCounts(db *gorm.DB, includeArchived bool) (LedgerEntityCounts, error) {
	set, err := buildLedgerEntitySet(db, includeArchived)
	if err != nil {
		return LedgerEntityCounts{}, err
	}
	return set.counts(), nil
}

func normalizeLedgerEntityCategory(value string) (LedgerEntityCategory, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "root", "root_domain", "domain":
		return LedgerEntityRoot, true
	case "ip":
		return LedgerEntityIP, true
	case "subdomain", "sub":
		return LedgerEntitySubdomain, true
	case "port":
		return LedgerEntityPort, true
	case "website", "web", "url":
		return LedgerEntityWebsite, true
	default:
		return "", false
	}
}

func ledgerEntityMatches(entity LedgerEntity, filter assetstore.LedgerFilter) bool {
	if status := strings.ToLower(strings.TrimSpace(filter.Status)); status != "" && status != "all" && strings.ToLower(entity.Status) != status {
		return false
	}
	if verification := strings.ToLower(strings.TrimSpace(filter.Verification)); verification != "" && verification != "all" && strings.ToLower(entity.Verification) != verification {
		return false
	}
	if source := strings.ToLower(strings.TrimSpace(filter.Source)); source != "" && source != "all" {
		switch source {
		case "manual", "import":
			if !containsAny(entity.Sources, source) {
				return false
			}
		case "scan":
			if !containsAny(entity.Sources, "portscan", "fingerprint", "webscan", "subdomain", "dirscan", "jsscan", "webscan-vulnerability") {
				return false
			}
		default:
			if !containsAny(entity.Sources, source) {
				return false
			}
		}
	}
	keyword := strings.ToLower(strings.TrimSpace(filter.Keyword))
	if keyword == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join(append([]string{
		entity.Value, entity.DisplayValue, entity.Name, entity.RootDomain, entity.Host, entity.Service, entity.Protocol, entity.Title,
		strings.Join(entity.IPs, " "), strings.Join(entity.Fingerprints, " "), strings.Join(entity.SourceTaskIDs, " "),
	}, strconv.Itoa(entity.Port)), " "))
	return strings.Contains(haystack, keyword)
}

func attachLedgerTaskNames(db *gorm.DB, entities []LedgerEntity) error {
	taskSet := map[string]struct{}{}
	for _, entity := range entities {
		for _, taskID := range entity.SourceTaskIDs {
			taskSet[taskID] = struct{}{}
		}
	}
	if len(taskSet) == 0 {
		return nil
	}
	taskIDs := make([]string, 0, len(taskSet))
	for taskID := range taskSet {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)
	meta := map[string]database.VulnScanTask{}
	const chunkSize = 500
	for start := 0; start < len(taskIDs); start += chunkSize {
		end := start + chunkSize
		if end > len(taskIDs) {
			end = len(taskIDs)
		}
		var tasks []database.VulnScanTask
		if err := db.Select("id", "version", "task_name", "updated_at").Where("id IN ?", taskIDs[start:end]).Order("version DESC").Find(&tasks).Error; err != nil {
			return err
		}
		for _, task := range tasks {
			if _, exists := meta[task.ID]; !exists {
				meta[task.ID] = task
			}
		}
	}
	for index := range entities {
		var latest time.Time
		var taskID, taskName string
		for _, candidate := range entities[index].SourceTaskIDs {
			task, ok := meta[candidate]
			if !ok {
				continue
			}
			if taskID == "" || task.UpdatedAt.After(latest) {
				latest = task.UpdatedAt
				taskID = candidate
				taskName = strings.TrimSpace(task.TaskName)
			}
		}
		if taskID == "" && len(entities[index].SourceTaskIDs) > 0 {
			taskID = entities[index].SourceTaskIDs[0]
		}
		if taskName == "" {
			taskName = taskID
		}
		entities[index].LastTaskID = taskID
		entities[index].LastTaskName = taskName
	}
	return nil
}

func ListAssetLedgerEntitiesHandler(c *gin.Context) {
	category, ok := normalizeLedgerEntityCategory(c.DefaultQuery("category", "root"))
	if !ok {
		c.JSON(400, gin.H{"success": false, "error": "category must be root, ip, subdomain, port or website"})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	db = db.WithContext(c.Request.Context())
	includeArchived := c.Query("includeArchived") == "true" || c.Query("includeArchived") == "1"
	set, err := buildLedgerEntitySet(db, includeArchived)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	filter := assetstore.LedgerFilter{
		Status: c.Query("status"), Verification: c.Query("verification"), Source: c.Query("source"), Keyword: c.Query("keyword"),
	}
	filtered := filterLedgerEntities(set.category(category), filter)
	total := len(filtered)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	items := filtered[start:end]
	if err := attachLedgerTaskNames(db, items); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{
		"items": items, "total": total, "page": page, "pageSize": pageSize, "counts": set.counts(),
		"filteredCounts": set.filteredCounts(filter),
	}})
}
