package asset

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Inventory is a live projection of persisted scan observations. Historical
// versions and newly saved local/agent results use the same path, without a
// second copy of the scan data or a backfill job that can become stale.
type AssetInventorySource struct {
	TaskID   string `json:"taskId"`
	TaskName string `json:"taskName"`
	ScanType string `json:"scanType"`
	Version  int    `json:"version"`
	Source   string `json:"source"`
}
type AssetInventoryObservation struct {
	AssetInventorySource
	Port                 int       `json:"port"`
	Transport            string    `json:"transport"`
	Protocol             string    `json:"protocol"`
	Service              string    `json:"service"`
	URL                  string    `json:"url"`
	Title                string    `json:"title"`
	Status               int       `json:"status"`
	Fingerprints         []string  `json:"fingerprints"`
	HighRiskFingerprints []string  `json:"highRiskFingerprints"`
	DiscoveryStatus      string    `json:"discoveryStatus"`
	ObservedAt           time.Time `json:"observedAt"`
}
type AssetInventoryPort struct {
	Port                 int       `json:"port"`
	Transport            string    `json:"transport"`
	Protocols            []string  `json:"protocols"`
	Services             []string  `json:"services"`
	Fingerprints         []string  `json:"fingerprints"`
	HighRiskFingerprints []string  `json:"highRiskFingerprints"`
	URLs                 []string  `json:"urls"`
	DiscoveryStatus      string    `json:"discoveryStatus"`
	LastSeenAt           time.Time `json:"lastSeenAt"`
}
type AssetInventoryItem struct {
	Type             string                      `json:"type"`
	Value            string                      `json:"value"`
	IPs              []string                    `json:"ips"`
	Ports            []AssetInventoryPort        `json:"ports"`
	Fingerprints     []string                    `json:"fingerprints"`
	Sources          []AssetInventorySource      `json:"sources"`
	TaskCount        int                         `json:"taskCount"`
	ObservationCount int                         `json:"observationCount"`
	OpenPortCount    int                         `json:"openPortCount"`
	FirstSeenAt      time.Time                   `json:"firstSeenAt"`
	LastSeenAt       time.Time                   `json:"lastSeenAt"`
	Observations     []AssetInventoryObservation `json:"-"`
}

func inventoryLabels(values []string, additions ...string) []string {
	for _, addition := range additions {
		addition = strings.TrimSpace(addition)
		if addition == "" {
			continue
		}
		exists := false
		for _, value := range values {
			if strings.EqualFold(value, addition) {
				exists = true
				break
			}
		}
		if !exists {
			values = append(values, addition)
		}
	}
	sort.Strings(values)
	return values
}

func inventoryHost(value string) string {
	if ip, ok := assetstore.ParseIPLike(value); ok {
		return ip
	}
	if assetstore.LooksLikeIPv6(value) {
		return strings.TrimSuffix(value, ":")
	}
	host := normalizeAssetHost(value)
	if ip, ok := assetstore.ParseIPLike(host); ok {
		return ip
	}
	return host
}

func addInventoryObservation(items map[string]*AssetInventoryItem, host, detailHost string, observation AssetInventoryObservation) {
	host = inventoryHost(host)
	if host == "" || observation.TaskID == "" {
		return
	}
	item := items[host]
	if item == nil {
		item = &AssetInventoryItem{Type: assetHostType(host), Value: host, IPs: []string{}, Ports: []AssetInventoryPort{}, Fingerprints: []string{}, Sources: []AssetInventorySource{}}
		items[host] = item
	}
	item.ObservationCount++
	if !observation.ObservedAt.IsZero() && (item.FirstSeenAt.IsZero() || observation.ObservedAt.Before(item.FirstSeenAt)) {
		item.FirstSeenAt = observation.ObservedAt
	}
	if observation.ObservedAt.After(item.LastSeenAt) {
		item.LastSeenAt = observation.ObservedAt
	}
	sourceFound := false
	for _, source := range item.Sources {
		if source.TaskID == observation.TaskID && source.Version == observation.Version && source.Source == observation.Source {
			sourceFound = true
			break
		}
	}
	if !sourceFound {
		item.Sources = append(item.Sources, observation.AssetInventorySource)
	}
	item.Fingerprints = inventoryLabels(item.Fingerprints, observation.Fingerprints...)
	item.Fingerprints = inventoryLabels(item.Fingerprints, observation.HighRiskFingerprints...)
	if host == detailHost {
		item.Observations = append(item.Observations, observation)
	}
	if observation.Port < 1 || observation.Port > 65535 {
		return
	}
	index := -1
	for i := range item.Ports {
		if item.Ports[i].Port == observation.Port && item.Ports[i].Transport == observation.Transport {
			index = i
			break
		}
	}
	if index == -1 {
		item.Ports = append(item.Ports, AssetInventoryPort{Port: observation.Port, Transport: observation.Transport, Protocols: []string{}, Services: []string{}, Fingerprints: []string{}, HighRiskFingerprints: []string{}, URLs: []string{}, DiscoveryStatus: "pending"})
		index = len(item.Ports) - 1
	}
	port := &item.Ports[index]
	port.Protocols = inventoryLabels(port.Protocols, observation.Protocol)
	port.Services = inventoryLabels(port.Services, observation.Service)
	port.Fingerprints = inventoryLabels(port.Fingerprints, observation.Fingerprints...)
	port.HighRiskFingerprints = inventoryLabels(port.HighRiskFingerprints, observation.HighRiskFingerprints...)
	port.URLs = inventoryLabels(port.URLs, observation.URL)
	if observation.DiscoveryStatus == "verified" {
		port.DiscoveryStatus = "verified"
	}
	if observation.ObservedAt.After(port.LastSeenAt) {
		port.LastSeenAt = observation.ObservedAt
	}
}

func collectAssetInventory(db *gorm.DB, detailHost string) ([]AssetInventoryItem, error) {
	if items, used, err := collectUnifiedAssetInventoryWithDB(db, detailHost); err != nil {
		return nil, err
	} else if used {
		return items, nil
	}
	var tasks []database.VulnScanTask
	if err := db.Select("id", "version", "task_name", "scan_type").Find(&tasks).Error; err != nil {
		return nil, err
	}
	metadata := make(map[string]database.VulnScanTask, len(tasks))
	for _, task := range tasks {
		metadata[assetCenterTaskKey(task.ID, task.Version)] = task
	}
	source := func(taskID string, version int, kind string) AssetInventorySource {
		task := metadata[assetCenterTaskKey(taskID, version)]
		name := strings.TrimSpace(task.TaskName)
		if name == "" {
			name = assetCenterFallbackTaskName(taskID, task.ScanType)
		}
		return AssetInventorySource{TaskID: taskID, TaskName: name, ScanType: task.ScanType, Version: version, Source: kind}
	}
	items := make(map[string]*AssetInventoryItem)
	var ports []database.PortScanResult
	if err := db.Select("id", "task_id", "version", "host", "port", "transport", "protocol", "product", "service_version", "discovery_status", "created_at").Find(&ports).Error; err != nil {
		return nil, err
	}
	byID := make(map[uint]database.PortScanResult, len(ports))
	portObservation := func(result database.PortScanResult) AssetInventoryObservation {
		transport := strings.ToLower(strings.TrimSpace(result.Transport))
		if transport == "" {
			transport = "tcp"
		}
		status := strings.ToLower(strings.TrimSpace(result.DiscoveryStatus))
		// Legacy rows predate discovery_status and were saved only after verification.
		if status == "" {
			status = "verified"
		}
		return AssetInventoryObservation{AssetInventorySource: source(result.TaskID, result.Version, "portscan"), Port: result.Port, Transport: transport, Protocol: result.Protocol, Service: strings.TrimSpace(result.Product + " " + result.ServiceVersion), DiscoveryStatus: status, ObservedAt: result.CreatedAt, Fingerprints: []string{}, HighRiskFingerprints: []string{}}
	}
	for _, result := range ports {
		byID[result.ID] = result
		addInventoryObservation(items, result.Host, detailHost, portObservation(result))
	}
	var aliases []database.PortScanResultAsset
	if err := db.Find(&aliases).Error; err != nil {
		return nil, err
	}
	aliasKeys := map[string]bool{}
	for _, alias := range aliases {
		host := inventoryHost(alias.Subdomain)
		if host == "" {
			continue
		}
		key := fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%d\x00%s", host, alias.TaskID, alias.Version, alias.ResultID, alias.Port, alias.Transport)
		if aliasKeys[key] {
			continue
		}
		aliasKeys[key] = true
		result, exists := byID[alias.ResultID]
		if exists && (result.TaskID != alias.TaskID || result.Version != alias.Version) {
			exists = false
		}
		if !exists {
			result = database.PortScanResult{TaskID: alias.TaskID, Version: alias.Version, Host: alias.IP, Port: alias.Port, Transport: alias.Transport, DiscoveryStatus: "pending", CreatedAt: alias.CreatedAt}
		}
		// An IP-only alias repeats the original observation rather than adding history.
		if host != inventoryHost(result.Host) {
			addInventoryObservation(items, host, detailHost, portObservation(result))
		}
		if item := items[host]; item != nil && item.Type == "domain" {
			if ip := net.ParseIP(strings.TrimSpace(alias.IP)); ip != nil {
				item.IPs = inventoryLabels(item.IPs, ip.String())
			}
		}
	}
	var fingerprints []database.FingerprintInfo
	if err := db.Select("task_id", "version", "url", "host", "scheme", "port", "title", "status", "fingerprints", "high_risk_fingerprints", "created_at").Find(&fingerprints).Error; err != nil {
		return nil, err
	}
	for _, result := range fingerprints {
		_, endpointHost := normalizeAssetEndpoint(result.URL, result.Host, result.Scheme, result.Port)
		host := endpointHost
		if host == "" {
			host = result.Host
		}
		port := result.Port
		protocol := strings.ToLower(result.Scheme)
		// Recover missing fields from historical URLs, retaining the stored port
		// when the URL omits it. An explicit URL port always takes precedence.
		if parsed, err := url.Parse(strings.TrimSpace(result.URL)); err == nil && parsed.Hostname() != "" {
			host = parsed.Hostname()
			if parsed.Scheme != "" {
				protocol = strings.ToLower(parsed.Scheme)
			}
			if parsed.Port() != "" {
				port, _ = strconv.Atoi(parsed.Port())
			}
		}
		if port == 0 {
			if protocol == "https" {
				port = 443
			} else if protocol == "http" {
				port = 80
			}
		}
		status := "pending"
		if result.Status > 0 {
			status = "verified"
		}
		addInventoryObservation(items, host, detailHost, AssetInventoryObservation{AssetInventorySource: source(result.TaskID, result.Version, "fingerprint"), Port: port, Transport: "tcp", Protocol: protocol, URL: result.URL, Title: result.Title, Status: result.Status, Fingerprints: parseAssetFingerprintLabels(result.Fingerprints), HighRiskFingerprints: parseAssetFingerprintLabels(result.HighRiskFingerprints), DiscoveryStatus: status, ObservedAt: result.CreatedAt})
	}
	result := make([]AssetInventoryItem, 0, len(items))
	for _, item := range items {
		taskIDs := map[string]bool{}
		for _, source := range item.Sources {
			taskIDs[source.TaskID] = true
		}
		item.TaskCount = len(taskIDs)
		for _, port := range item.Ports {
			if port.DiscoveryStatus == "verified" {
				item.OpenPortCount++
			}
		}
		sort.Slice(item.Ports, func(i, j int) bool {
			if item.Ports[i].Port == item.Ports[j].Port {
				return item.Ports[i].Transport < item.Ports[j].Transport
			}
			return item.Ports[i].Port < item.Ports[j].Port
		})
		sort.Slice(item.Sources, func(i, j int) bool {
			a, b := item.Sources[i], item.Sources[j]
			if a.TaskID != b.TaskID {
				return a.TaskID < b.TaskID
			}
			if a.Version != b.Version {
				return a.Version > b.Version
			}
			return a.Source < b.Source
		})
		sort.SliceStable(item.Observations, func(i, j int) bool { return item.Observations[i].ObservedAt.After(item.Observations[j].ObservedAt) })
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].LastSeenAt.Equal(result[j].LastSeenAt) {
			return result[i].Value < result[j].Value
		}
		return result[i].LastSeenAt.After(result[j].LastSeenAt)
	})
	return result, nil
}

type assetInventoryFilter struct {
	Type, Keyword, Service, Fingerprint, TaskID string
	Port                                        int
}

func inventoryContains(values []string, query string) bool {
	return strings.Contains(strings.ToLower(strings.Join(values, " ")), strings.ToLower(query))
}
func matchesAssetInventory(item AssetInventoryItem, filter assetInventoryFilter) bool {
	if filter.Type != "all" && item.Type != filter.Type {
		return false
	}
	if !inventoryContains(item.Fingerprints, filter.Fingerprint) {
		return false
	}
	labels := append([]string{item.Value}, item.IPs...)
	labels = append(labels, item.Fingerprints...)
	sourceFound := filter.TaskID == ""
	for _, source := range item.Sources {
		labels = append(labels, source.TaskID, source.TaskName)
		if source.TaskID == filter.TaskID {
			sourceFound = true
		}
	}
	if !sourceFound {
		return false
	}
	portFound := filter.Port == 0 && filter.Service == ""
	for _, port := range item.Ports {
		labels = append(labels, strconv.Itoa(port.Port), port.Transport)
		labels = append(labels, port.Protocols...)
		labels = append(labels, port.Services...)
		if (filter.Port == 0 || filter.Port == port.Port) && inventoryContains(append(append([]string{}, port.Protocols...), port.Services...), filter.Service) {
			portFound = true
		}
	}
	return portFound && inventoryContains(labels, filter.Keyword)
}
func inventoryPagination(c *gin.Context, total int) (int, int, int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	pages := (total + size - 1) / size
	if pages < 1 {
		pages = 1
	}
	if page > pages {
		page = pages
	}
	start := (page - 1) * size
	end := start + size
	if end > total {
		end = total
	}
	return page, size, start, end
}
func ListAssetInventoryHandler(c *gin.Context) {
	filter := assetInventoryFilter{Type: c.DefaultQuery("type", "all"), Keyword: strings.TrimSpace(c.Query("keyword")), Service: strings.TrimSpace(c.Query("service")), Fingerprint: strings.TrimSpace(c.Query("fingerprint")), TaskID: c.Query("taskId")}
	if filter.Type != "all" && filter.Type != "domain" && filter.Type != "ip" {
		c.JSON(400, gin.H{"success": false, "error": "type must be all, domain or ip"})
		return
	}
	if raw := c.Query("port"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			c.JSON(400, gin.H{"success": false, "error": "port must be between 1 and 65535"})
			return
		}
		filter.Port = port
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	items, err := collectAssetInventory(db.WithContext(c.Request.Context()), "")
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	filtered := []AssetInventoryItem{}
	domains, ips, openPorts := 0, 0, 0
	fingerprints := []string{}
	tasks := map[string]AssetInventorySource{}
	for _, item := range items {
		if item.Type == "domain" {
			domains++
		} else {
			ips++
		}
		openPorts += item.OpenPortCount
		fingerprints = inventoryLabels(fingerprints, item.Fingerprints...)
		for _, source := range item.Sources {
			previous, ok := tasks[source.TaskID]
			if !ok || source.Version > previous.Version {
				tasks[source.TaskID] = source
			}
		}
		if matchesAssetInventory(item, filter) {
			filtered = append(filtered, item)
		}
	}
	sources := []AssetInventorySource{}
	for _, source := range tasks {
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].TaskID < sources[j].TaskID })
	page, size, start, end := inventoryPagination(c, len(filtered))
	c.JSON(200, gin.H{"success": true, "data": gin.H{"items": filtered[start:end], "total": len(filtered), "page": page, "pageSize": size, "tasks": sources, "stats": gin.H{"assets": len(items), "domains": domains, "ips": ips, "openPorts": openPorts, "fingerprints": len(fingerprints)}}})
}
func GetAssetInventoryDetailHandler(c *gin.Context) {
	host := inventoryHost(c.Query("value"))
	if host == "" {
		c.JSON(400, gin.H{"success": false, "error": "value is required"})
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	items, err := collectAssetInventory(db.WithContext(c.Request.Context()), host)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	for _, item := range items {
		if item.Value == host {
			page, size, start, end := inventoryPagination(c, len(item.Observations))
			c.JSON(200, gin.H{"success": true, "data": gin.H{"asset": item, "observations": item.Observations[start:end], "total": len(item.Observations), "page": page, "pageSize": size}})
			return
		}
	}
	c.JSON(404, gin.H{"success": false, "error": "asset not found"})
}
