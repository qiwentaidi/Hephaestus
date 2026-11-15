package asset

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// assetScopeArchiveIndex is the view-level suppression index for aggregate
// ledger rows. Root values are canonical root domains; port values are
// "host:port/transport" identities.
type assetScopeArchiveIndex struct {
	roots map[string]struct{}
	ports map[string]struct{}
}

func loadAssetScopeArchiveIndex(db *gorm.DB) (assetScopeArchiveIndex, error) {
	index := assetScopeArchiveIndex{roots: map[string]struct{}{}, ports: map[string]struct{}{}}
	if db == nil || !db.Migrator().HasTable(&database.AssetScopeArchive{}) {
		return index, nil
	}
	var rows []database.AssetScopeArchive
	if err := db.Where("scope_key = ?", "global").Find(&rows).Error; err != nil {
		return index, err
	}
	for _, row := range rows {
		switch strings.ToLower(strings.TrimSpace(row.Category)) {
		case database.AssetScopeCategoryRoot:
			if root := assetTopologyRootDomain(row.Value); root != "" {
				index.roots[root] = struct{}{}
			}
		case database.AssetScopeCategoryPort:
			if value := strings.TrimSpace(row.Value); value != "" {
				index.ports[value] = struct{}{}
			}
		}
	}
	return index, nil
}

func (index assetScopeArchiveIndex) hidesRoot(root string) bool {
	root = strings.TrimSpace(strings.ToLower(root))
	if root == "" {
		return false
	}
	_, archived := index.roots[root]
	return archived
}

// hidesHost reports whether a host belongs to an archived root domain. IP
// assets are intentionally never hidden: one IP can back several root domains.
func (index assetScopeArchiveIndex) hidesHost(host string) bool {
	host = normalizeAssetHost(host)
	if host == "" || net.ParseIP(host) != nil {
		return false
	}
	root := assetTopologyRootDomain(host)
	return root != "" && index.hidesRoot(root)
}

func (index assetScopeArchiveIndex) hidesPort(host string, port int, transport string) bool {
	value := assetScopePortValue(host, port, transport)
	if value == "" {
		return false
	}
	_, archived := index.ports[value]
	return archived
}

// assetScopePortValue is the stable port-scope identity used by both the API
// and the ledger projection.
func assetScopePortValue(host string, port int, transport string) string {
	host = normalizeAssetHost(host)
	transport = strings.ToLower(strings.TrimSpace(transport))
	if transport == "" {
		transport = "tcp"
	}
	if host == "" || port <= 0 || port > 65535 {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(port)) + "/" + transport
}

func parseAssetScopePortValue(value string) (host string, port int, transport string, ok bool) {
	value = strings.TrimSpace(value)
	slash := strings.LastIndex(value, "/")
	if slash <= 0 || slash == len(value)-1 {
		return "", 0, "", false
	}
	transport = strings.ToLower(strings.TrimSpace(value[slash+1:]))
	host, port, parsed := parseAssetScopeHostPort(value[:slash])
	if !parsed || transport == "" {
		return "", 0, "", false
	}
	return host, port, transport, true
}

func parseAssetScopeHostPort(raw string) (string, int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0, false
	}
	host, portText, err := net.SplitHostPort(raw)
	if err != nil {
		separator := strings.LastIndex(raw, ":")
		if separator <= 0 {
			return "", 0, false
		}
		host, portText = raw[:separator], raw[separator+1:]
	}
	port, err := strconv.Atoi(strings.TrimSpace(portText))
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, false
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return "", 0, false
	}
	return host, port, true
}

// canonicalAssetScopeArchive validates a ledger selection and returns the
// canonical category/value pair stored by the API.
func canonicalAssetScopeArchive(item ledgerEntitySelection) (category, value string, err error) {
	category = strings.ToLower(strings.TrimSpace(item.Category))
	switch category {
	case database.AssetScopeCategoryRoot:
		raw := strings.TrimSpace(item.Value)
		if raw == "" {
			raw = strings.TrimPrefix(strings.TrimSpace(item.ID), "root:")
		}
		root := assetTopologyRootDomain(raw)
		if root == "" {
			return "", "", errors.New("无效的根域名")
		}
		return category, root, nil
	case database.AssetScopeCategoryPort:
		host := strings.TrimSpace(item.Host)
		port := item.Port
		transport := strings.ToLower(strings.TrimSpace(item.Transport))
		if host == "" || port <= 0 {
			if parsedHost, parsedPort, ok := parseAssetScopeHostPort(item.Value); ok {
				host, port = parsedHost, parsedPort
			}
		}
		if host == "" || port <= 0 || port > 65535 {
			return "", "", errors.New("无效的端口")
		}
		value := assetScopePortValue(host, port, transport)
		if value == "" {
			return "", "", errors.New("无效的端口")
		}
		return category, value, nil
	default:
		return "", "", errors.New("不支持的作用域类型")
	}
}

func setAssetScopeArchive(ctx context.Context, db *gorm.DB, category, value string, archived bool) error {
	if db == nil {
		return errors.New("asset scope archive database is nil")
	}
	category = strings.ToLower(strings.TrimSpace(category))
	value = strings.TrimSpace(value)
	if category != database.AssetScopeCategoryRoot && category != database.AssetScopeCategoryPort {
		return fmt.Errorf("unsupported asset scope category %q", category)
	}
	if value == "" {
		return errors.New("asset scope value is required")
	}
	now := time.Now().UTC()
	if archived {
		row := database.AssetScopeArchive{
			ID:         assetstore.AssetID("global", "scope_"+category, value),
			ScopeKey:   "global",
			Category:   category,
			Value:      value,
			ArchivedAt: now,
		}
		return db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "scope_key"}, {Name: "category"}, {Name: "value"}},
			DoUpdates: clause.Assignments(map[string]any{"archived_at": now, "updated_at": now}),
		}).Create(&row).Error
	}
	return db.WithContext(ctx).
		Where("scope_key = ? AND category = ? AND value = ?", "global", category, value).
		Delete(&database.AssetScopeArchive{}).Error
}

// applyAssetScopeArchivesToLedger removes scope-hidden rows from the built set
// and, in the archived view, appends one synthetic row per scope so it can be
// restored even when the underlying assets have disappeared.
func applyAssetScopeArchivesToLedger(set ledgerEntitySet, scopes assetScopeArchiveIndex, includeArchived bool) ledgerEntitySet {
	if len(scopes.roots) == 0 && len(scopes.ports) == 0 {
		return set
	}
	before := set
	filtered := ledgerEntitySet{
		roots:      make([]LedgerEntity, 0, len(set.roots)),
		ips:        make([]LedgerEntity, 0, len(set.ips)),
		subdomains: make([]LedgerEntity, 0, len(set.subdomains)),
		ports:      make([]LedgerEntity, 0, len(set.ports)),
		websites:   make([]LedgerEntity, 0, len(set.websites)),
	}
	for _, entity := range before.roots {
		if scopes.hidesRoot(entity.Value) {
			continue
		}
		filtered.roots = append(filtered.roots, entity)
	}
	for _, entity := range before.ips {
		// IP assets stay visible; they may be shared by non-archived roots.
		filtered.ips = append(filtered.ips, entity)
	}
	for _, entity := range before.subdomains {
		if scopes.hidesHost(entity.Value) {
			continue
		}
		filtered.subdomains = append(filtered.subdomains, entity)
	}
	for _, entity := range before.ports {
		if scopes.hidesPort(entity.Host, entity.Port, entity.Transport) || scopes.hidesHost(entity.Host) {
			continue
		}
		filtered.ports = append(filtered.ports, entity)
	}
	for _, entity := range before.websites {
		if scopes.hidesHost(entity.Host) {
			continue
		}
		filtered.websites = append(filtered.websites, entity)
	}
	if includeArchived {
		for root := range scopes.roots {
			filtered.roots = append(filtered.roots, archivedRootLedgerEntity(before, root))
		}
		for value := range scopes.ports {
			if host, _, _, ok := parseAssetScopePortValue(value); ok && scopes.hidesHost(host) {
				// The parent root scope already hides this port; keep only the
				// root restore entry until the root is restored.
				continue
			}
			filtered.ports = append(filtered.ports, archivedPortLedgerEntity(before, value))
		}
	}
	sortLedgerEntitySet(&filtered)
	return filtered
}

func archivedRootLedgerEntity(before ledgerEntitySet, root string) LedgerEntity {
	entity := LedgerEntity{
		ID: "root:" + root, Category: string(LedgerEntityRoot), Value: root, DisplayValue: root,
		RootDomain: root, Host: root, Sources: []string{}, SourceTaskIDs: []string{},
		Status: "active", Archived: true, ScopeArchived: true,
	}
	for _, candidate := range before.roots {
		if candidate.Value == root {
			entity = candidate
			break
		}
	}
	hiddenHosts := map[string]struct{}{}
	hiddenPorts := 0
	hiddenWebsites := 0
	for _, subdomain := range before.subdomains {
		if subdomain.RootDomain != root {
			continue
		}
		if subdomain.Value != root {
			hiddenHosts[subdomain.Value] = struct{}{}
		}
		entity.IPs = mergeUnique(entity.IPs, subdomain.IPs...)
		entity.Sources = mergeUnique(entity.Sources, subdomain.Sources...)
		entity.SourceTaskIDs = mergeUnique(entity.SourceTaskIDs, subdomain.SourceTaskIDs...)
		entity.Verification = bestVerification(entity.Verification, subdomain.Verification)
		entity.FirstSeenAt = earlierTime(entity.FirstSeenAt, subdomain.FirstSeenAt)
		entity.LastSeenAt = laterTime(entity.LastSeenAt, subdomain.LastSeenAt)
	}
	for _, port := range before.ports {
		if port.RootDomain != root || !isDomainScopeHost(port.Host) {
			continue
		}
		hiddenPorts++
		entity.Sources = mergeUnique(entity.Sources, port.Sources...)
		entity.SourceTaskIDs = mergeUnique(entity.SourceTaskIDs, port.SourceTaskIDs...)
		entity.Verification = bestVerification(entity.Verification, port.Verification)
		entity.FirstSeenAt = earlierTime(entity.FirstSeenAt, port.FirstSeenAt)
		entity.LastSeenAt = laterTime(entity.LastSeenAt, port.LastSeenAt)
	}
	for _, website := range before.websites {
		if website.RootDomain != root || !isDomainScopeHost(website.Host) {
			continue
		}
		hiddenWebsites++
		if website.Host != root {
			hiddenHosts[website.Host] = struct{}{}
		}
		entity.Sources = mergeUnique(entity.Sources, website.Sources...)
		entity.SourceTaskIDs = mergeUnique(entity.SourceTaskIDs, website.SourceTaskIDs...)
		entity.Verification = bestVerification(entity.Verification, website.Verification)
		entity.FirstSeenAt = earlierTime(entity.FirstSeenAt, website.FirstSeenAt)
		entity.LastSeenAt = laterTime(entity.LastSeenAt, website.LastSeenAt)
	}
	entity.IPs = uniqueStrings(entity.IPs)
	entity.IPCount = len(entity.IPs)
	entity.SubdomainCount = len(hiddenHosts)
	entity.PortCount = hiddenPorts
	entity.WebsiteCount = hiddenWebsites
	entity.ArchiveSubdomainCount = len(hiddenHosts)
	entity.ArchivePortCount = hiddenPorts
	entity.ArchiveWebsiteCount = hiddenWebsites
	entity.AssetID = ""
	entity.Archived = true
	entity.ScopeArchived = true
	return entity
}

func isDomainScopeHost(host string) bool {
	host = normalizeAssetHost(host)
	return host != "" && net.ParseIP(host) == nil
}

func archivedPortLedgerEntity(before ledgerEntitySet, value string) LedgerEntity {
	for _, entity := range before.ports {
		if assetScopePortValue(entity.Host, entity.Port, entity.Transport) != value {
			continue
		}
		entity.AssetID = ""
		entity.Archived = true
		entity.ScopeArchived = true
		return entity
	}
	host, port, transport, ok := parseAssetScopePortValue(value)
	if !ok {
		host, port, transport = strings.TrimSpace(value), 0, "tcp"
	}
	display := host
	if port > 0 {
		display = net.JoinHostPort(host, strconv.Itoa(port))
	}
	return LedgerEntity{
		ID: fmt.Sprintf("port:%s:%d:%s", host, port, transport), Category: string(LedgerEntityPort),
		Value: display, DisplayValue: display, Host: host, Port: port, Transport: transport,
		Protocol: transport, Sources: []string{}, SourceTaskIDs: []string{},
		Status: "active", Archived: true, ScopeArchived: true,
	}
}

func earlierTime(current, candidate time.Time) time.Time {
	if candidate.IsZero() {
		return current
	}
	if current.IsZero() || candidate.Before(current) {
		return candidate
	}
	return current
}

func laterTime(current, candidate time.Time) time.Time {
	if candidate.After(current) {
		return candidate
	}
	return current
}

// filterAssetCenterItemsByArchivedRoots keeps the asset-center projection
// (inventory, topology, map, dashboard) consistent with the ledger. Only root
// scopes apply here; port scopes are ledger/export-only in this phase.
func filterAssetCenterItemsByArchivedRoots(items []AssetCenterItem, scopes assetScopeArchiveIndex) []AssetCenterItem {
	if len(scopes.roots) == 0 {
		return items
	}
	filtered := make([]AssetCenterItem, 0, len(items))
	for _, item := range items {
		switch item.Type {
		case "endpoint":
			parsed, err := url.Parse(item.Value)
			if err == nil && scopes.hidesHost(parsed.Hostname()) {
				continue
			}
		case "domain", "url":
			if scopes.hidesHost(item.Value) {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// sortLedgerEntitySet keeps pagination stable after scope rows are appended.
func sortLedgerEntitySet(set *ledgerEntitySet) {
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
}

// filterAssetInventoryByArchivedRoots keeps the inventory projection consistent
// with the ledger. Port scopes remain ledger/export-only in this phase.
func filterAssetInventoryByArchivedRoots(items []AssetInventoryItem, scopes assetScopeArchiveIndex) []AssetInventoryItem {
	if len(scopes.roots) == 0 {
		return items
	}
	filtered := make([]AssetInventoryItem, 0, len(items))
	for _, item := range items {
		if item.Type == "domain" && scopes.hidesHost(item.Value) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}
