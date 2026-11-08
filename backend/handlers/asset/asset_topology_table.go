package asset

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	assetstore "hephaestus-backend/pkg/asset"
	"hephaestus-backend/pkg/database"
)

// A row is one observed path in the same projection used by the topology.
// Domain-owned endpoints bypass the IP column, just as they bypass the IP
// layer in the graph. DNS relationships alone do not establish which IP
// served a particular virtual host.
type AssetTopologyTableRow struct {
	ID            string    `json:"id"`
	AssetID       string    `json:"assetId,omitempty"`
	Type          string    `json:"type"`
	RootDomain    string    `json:"rootDomain"`
	Domain        string    `json:"domain"`
	IP            string    `json:"ip"`
	Endpoint      string    `json:"endpoint"`
	EndpointHost  string    `json:"endpointHost"`
	Port          int       `json:"port"`
	Protocol      string    `json:"protocol"`
	Service       string    `json:"service"`
	Technologies  []string  `json:"technologies"`
	SourceTaskIDs []string  `json:"sourceTaskIds"`
	SourceTypes   []string  `json:"sourceTypes"`
	LastSeenAt    time.Time `json:"lastSeenAt"`
	Status        string    `json:"status"`
}

type AssetTopologyTableStats struct {
	Roots     int `json:"roots"`
	Domains   int `json:"domains"`
	IPs       int `json:"ips"`
	Endpoints int `json:"endpoints"`
}

type AssetTopologyTableTask struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func buildAssetTopologyTable(items []AssetCenterItem) ([]AssetTopologyTableRow, AssetTopologyTableStats) {
	domains := map[string]AssetCenterItem{}
	ips := map[string]AssetCenterItem{}
	endpoints := map[string][]AssetCenterItem{}
	endpointIDs := map[string]bool{}
	roots := map[string]bool{}
	for _, item := range items {
		switch item.Type {
		case "domain":
			domains[inventoryHost(item.Value)] = item
			if root := assetTopologyRootDomain(item.Value); root != "" {
				roots[root] = true
			}
		case "ip":
			ips[inventoryHost(item.Value)] = item
		case "endpoint":
			if endpointIDs[item.Value] {
				continue
			}
			if item.EndpointHost == "" {
				populateAssetCenterDisplayFields(&item)
			}
			host := inventoryHost(item.EndpointHost)
			endpoints[host] = append(endpoints[host], item)
			endpointIDs[item.Value] = true
		}
	}
	rows := []AssetTopologyTableRow{}
	seen := map[string]bool{}
	addRow := func(domain, ip, endpoint *AssetCenterItem) {
		row := AssetTopologyTableRow{Type: "ip", Technologies: []string{}, SourceTaskIDs: []string{}, SourceTypes: []string{}}
		var leaf *AssetCenterItem
		if domain != nil {
			row.Type = "domain"
			row.RootDomain = assetTopologyRootDomain(domain.Value)
			row.Domain = inventoryHost(domain.Value)
			if row.Domain == row.RootDomain {
				row.Domain = ""
			}
			if row.RootDomain != "" {
				roots[row.RootDomain] = true
			}
			leaf = domain
		}
		if ip != nil {
			row.IP = inventoryHost(ip.Value)
			// A synthetic IP node is grounded only by its domain's DNS evidence.
			if len(ip.SourceTaskIDs) > 0 || leaf == nil {
				leaf = ip
			}
		}
		if endpoint != nil {
			row.Endpoint = endpoint.Value
			row.EndpointHost = endpoint.EndpointHost
			row.Port = endpoint.Port
			row.Protocol = endpoint.Protocol
			row.Service = endpoint.Service
			row.Technologies = append(row.Technologies, endpoint.ServiceLabels...)
			leaf = endpoint
		}
		if leaf != nil {
			row.AssetID = leaf.AssetID
			row.Status = leaf.Status
			if row.Status == "" {
				row.Status = "active"
			}
			row.SourceTaskIDs = append(row.SourceTaskIDs, leaf.SourceTaskIDs...)
			for _, source := range leaf.SourceTypes {
				exists := false
				for _, existing := range row.SourceTypes {
					if existing == source {
						exists = true
						break
					}
				}
				if !exists {
					row.SourceTypes = append(row.SourceTypes, source)
				}
			}
			row.LastSeenAt = leaf.LastSeenAt
			if row.LastSeenAt.IsZero() {
				row.LastSeenAt = leaf.UpdatedAt
			}
		}
		row.ID = strings.Join([]string{row.RootDomain, row.Domain, row.IP, row.Endpoint}, "\x00")
		if !seen[row.ID] {
			seen[row.ID] = true
			rows = append(rows, row)
		}
	}
	referencedIPs := map[string]bool{}
	usedEndpoints := map[string]bool{}
	for host, domain := range domains {
		paths := 0
		for _, endpoint := range endpoints[host] {
			addRow(&domain, nil, &endpoint)
			usedEndpoints[endpoint.Value] = true
			paths++
		}
		for _, address := range domain.IPs {
			normalized, ok := assetstore.ParseIPLike(address)
			if !ok {
				continue
			}
			address = normalized
			referencedIPs[address] = true
			ip, exists := ips[address]
			if !exists {
				ip = AssetCenterItem{Type: "ip", Value: address}
				ips[address] = ip
			}
			if len(endpoints[address]) == 0 {
				addRow(&domain, &ip, nil)
				paths++
			}
			for _, endpoint := range endpoints[address] {
				addRow(&domain, &ip, &endpoint)
				usedEndpoints[endpoint.Value] = true
				paths++
			}
		}
		if paths == 0 {
			addRow(&domain, nil, nil)
		}
	}
	for address, ip := range ips {
		if referencedIPs[address] {
			continue
		}
		if len(endpoints[address]) == 0 {
			addRow(nil, &ip, nil)
		}
		for _, endpoint := range endpoints[address] {
			addRow(nil, &ip, &endpoint)
			usedEndpoints[endpoint.Value] = true
		}
	}
	// Keep any endpoint whose host node is missing, rather than silently losing
	// incomplete historical data. No parent DNS relationship is inferred.
	for host, values := range endpoints {
		for _, endpoint := range values {
			if usedEndpoints[endpoint.Value] {
				continue
			}
			if assetstore.ClassifyHost(host) == "ip" {
				ip := AssetCenterItem{Type: "ip", Value: host}
				ips[host] = ip
				addRow(nil, &ip, &endpoint)
			} else {
				domain := AssetCenterItem{Type: "domain", Value: host}
				domains[host] = domain
				addRow(&domain, nil, &endpoint)
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.RootDomain != b.RootDomain {
			if a.RootDomain == "" {
				return false
			}
			if b.RootDomain == "" {
				return true
			}
			return a.RootDomain < b.RootDomain
		}
		if a.Domain != b.Domain {
			return a.Domain < b.Domain
		}
		if a.IP != b.IP {
			return a.IP < b.IP
		}
		if a.Port != b.Port {
			return a.Port < b.Port
		}
		return a.Endpoint < b.Endpoint
	})
	return rows, AssetTopologyTableStats{Roots: len(roots), Domains: len(domains), IPs: len(ips), Endpoints: len(endpointIDs)}
}

type assetTopologyTableFilter struct {
	Type, Status, RootDomain, Keyword, Service, Technology, TaskID string
	Port                                                           int
}

func matchesAssetTopologyTableRow(row AssetTopologyTableRow, filter assetTopologyTableFilter) bool {
	if filter.Type != "all" && filter.Type != row.Type {
		return false
	}
	status := row.Status
	if status == "" {
		status = "active"
	}
	if filter.Status != "" && filter.Status != "all" && filter.Status != status {
		return false
	}
	if filter.RootDomain != "" && filter.RootDomain != row.RootDomain {
		return false
	}
	if filter.Port != 0 && filter.Port != row.Port {
		return false
	}
	if !inventoryContains(append([]string{row.Protocol, row.Service}, row.Technologies...), filter.Service) || !inventoryContains(row.Technologies, filter.Technology) {
		return false
	}
	labels := []string{row.RootDomain, row.Domain, row.IP, row.Endpoint, strconv.Itoa(row.Port), row.Protocol, row.Service}
	labels = append(labels, row.Technologies...)
	sourceFound := filter.TaskID == ""
	for _, taskID := range row.SourceTaskIDs {
		if taskID == filter.TaskID {
			sourceFound = true
		}
	}
	return sourceFound && inventoryContains(labels, filter.Keyword)
}

func ListAssetTopologyTableHandler(c *gin.Context) {
	filter := assetTopologyTableFilter{Type: c.DefaultQuery("type", "all"), Status: c.DefaultQuery("status", "all"), RootDomain: strings.ToLower(strings.TrimSpace(c.Query("rootDomain"))), Keyword: strings.TrimSpace(c.Query("keyword")), Service: strings.TrimSpace(c.Query("service")), Technology: strings.TrimSpace(c.Query("technology")), TaskID: c.Query("taskId")}
	if filter.Type != "all" && filter.Type != "domain" && filter.Type != "ip" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "type must be all, domain or ip"})
		return
	}
	if filter.Status != "all" && filter.Status != "active" && filter.Status != "stale" && filter.Status != "retired" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "status must be all, active, stale or retired"})
		return
	}
	if raw := c.Query("port"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "port must be between 1 and 65535"})
			return
		}
		filter.Port = port
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	db = db.WithContext(c.Request.Context())
	items, err := collectAssetCenterAggregatesWithDB(db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	rows, stats := buildAssetTopologyTable(items)
	var scanTasks []database.VulnScanTask
	if err := db.WithContext(c.Request.Context()).Select("id", "task_name", "version").Order("version DESC").Find(&scanTasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	tasks := map[string]AssetTopologyTableTask{}
	for _, task := range scanTasks {
		if _, exists := tasks[task.ID]; !exists {
			tasks[task.ID] = AssetTopologyTableTask{ID: task.ID, Name: task.TaskName}
		}
	}
	sourceIDs := map[string]bool{}
	rootNames := map[string]bool{}
	filtered := []AssetTopologyTableRow{}
	for _, row := range rows {
		if row.RootDomain != "" {
			rootNames[row.RootDomain] = true
		}
		for _, taskID := range row.SourceTaskIDs {
			sourceIDs[taskID] = true
		}
		if matchesAssetTopologyTableRow(row, filter) {
			filtered = append(filtered, row)
		}
	}
	roots := []string{}
	for root := range rootNames {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	sources := []AssetTopologyTableTask{}
	for taskID := range sourceIDs {
		task := tasks[taskID]
		task.ID = taskID
		sources = append(sources, task)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].ID < sources[j].ID })
	page, size, start, end := inventoryPagination(c, len(filtered))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": filtered[start:end], "total": len(filtered), "page": page, "pageSize": size, "tasks": sources, "rootDomains": roots, "stats": stats}})
}
