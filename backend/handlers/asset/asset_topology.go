package asset

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	assetstore "hephaestus-backend/pkg/asset"

	"golang.org/x/net/publicsuffix"
)

func assetTopologyNodeID(assetType, value string) string {
	return assetType + ":" + value
}

func assetTopologyEndpointLabel(item AssetCenterItem) string {
	if item.Port > 0 && item.Protocol != "" {
		return fmt.Sprintf("%d / %s", item.Port, strings.ToUpper(item.Protocol))
	}
	if item.Port > 0 {
		return strconv.Itoa(item.Port)
	}
	return item.Value
}

func assetTopologyRootDomain(value string) string {
	host := normalizeAssetHost(value)
	if !strings.Contains(host, ".") || !assetstore.IsDomainHost(host) {
		return ""
	}
	root, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return host
	}
	return root
}

func buildAssetTopologyEntries(items []AssetCenterItem) []AssetCenterItem {
	entries := make([]AssetCenterItem, 0, len(items))
	rootAggregates := make(map[string]*assetTopologyRootAggregate)

	for _, item := range items {
		if item.Type != "domain" && item.Type != "ip" {
			continue
		}
		entries = append(entries, item)
		if item.Type != "domain" {
			continue
		}

		rootDomain := assetTopologyRootDomain(item.Value)
		if rootDomain == "" {
			continue
		}
		aggregate := rootAggregates[rootDomain]
		if aggregate == nil {
			aggregate = &assetTopologyRootAggregate{
				item:  AssetCenterItem{Type: "root", Value: rootDomain, FirstSeenAt: item.FirstSeenAt, UpdatedAt: item.UpdatedAt},
				tasks: make(map[string]struct{}),
				ips:   make(map[string]struct{}),
			}
			rootAggregates[rootDomain] = aggregate
		}
		aggregate.item.ObservationCount += item.ObservationCount
		aggregate.item.CriticalCount += item.CriticalCount
		aggregate.item.HighCount += item.HighCount
		if !item.FirstSeenAt.IsZero() && (aggregate.item.FirstSeenAt.IsZero() || item.FirstSeenAt.Before(aggregate.item.FirstSeenAt)) {
			aggregate.item.FirstSeenAt = item.FirstSeenAt
		}
		if item.UpdatedAt.After(aggregate.item.UpdatedAt) {
			aggregate.item.UpdatedAt = item.UpdatedAt
		}
		for _, taskID := range item.SourceTaskIDs {
			aggregate.tasks[taskID] = struct{}{}
		}
		for _, ip := range item.IPs {
			if normalized := net.ParseIP(strings.TrimSpace(ip)); normalized != nil {
				aggregate.ips[normalized.String()] = struct{}{}
			}
		}
	}

	for _, aggregate := range rootAggregates {
		aggregate.item.TaskCount = len(aggregate.tasks)
		aggregate.item.SourceTaskIDs = make([]string, 0, len(aggregate.tasks))
		for taskID := range aggregate.tasks {
			aggregate.item.SourceTaskIDs = append(aggregate.item.SourceTaskIDs, taskID)
		}
		sort.Strings(aggregate.item.SourceTaskIDs)
		aggregate.item.IPs = make([]string, 0, len(aggregate.ips))
		for ip := range aggregate.ips {
			aggregate.item.IPs = append(aggregate.item.IPs, ip)
		}
		sort.Strings(aggregate.item.IPs)
		entries = append(entries, aggregate.item)
	}

	typeOrder := map[string]int{"root": 0, "domain": 1, "ip": 2}
	sort.Slice(entries, func(i, j int) bool {
		if typeOrder[entries[i].Type] != typeOrder[entries[j].Type] {
			return typeOrder[entries[i].Type] < typeOrder[entries[j].Type]
		}
		return entries[i].Value < entries[j].Value
	})
	return entries
}

func buildAssetTopology(items []AssetCenterItem, rootType, rootValue string) (AssetTopologyData, bool) {
	entryItems := buildAssetTopologyEntries(items)
	assetsByKey := make(map[string]AssetCenterItem, len(items))
	for _, item := range items {
		assetsByKey[item.Type+"\x00"+item.Value] = item
	}
	for _, item := range entryItems {
		assetsByKey[item.Type+"\x00"+item.Value] = item
	}

	root, ok := assetsByKey[rootType+"\x00"+rootValue]
	if !ok {
		return AssetTopologyData{}, false
	}

	nodes := make([]AssetTopologyNode, 0, 24)
	edges := make([]AssetTopologyEdge, 0, 32)
	nodeIDs := make(map[string]struct{})
	edgeIDs := make(map[string]struct{})

	addAssetNode := func(item AssetCenterItem) string {
		id := assetTopologyNodeID(item.Type, item.Value)
		if _, exists := nodeIDs[id]; exists {
			return id
		}
		label := item.Value
		if item.Type == "endpoint" {
			label = assetTopologyEndpointLabel(item)
		}
		copy := item
		nodes = append(nodes, AssetTopologyNode{ID: id, Type: item.Type, Label: label, Asset: &copy})
		nodeIDs[id] = struct{}{}
		return id
	}

	addTechnologyNode := func(endpointID, service string) string {
		service = strings.TrimSpace(service)
		if service == "" {
			return ""
		}
		id := "technology:" + endpointID + ":" + service
		if _, exists := nodeIDs[id]; !exists {
			nodes = append(nodes, AssetTopologyNode{ID: id, Type: "technology", Label: service, Service: service})
			nodeIDs[id] = struct{}{}
		}
		return id
	}

	addEdge := func(source, target, kind string) {
		if source == "" || target == "" || source == target {
			return
		}
		key := source + "\x00" + target + "\x00" + kind
		if _, exists := edgeIDs[key]; exists {
			return
		}
		edges = append(edges, AssetTopologyEdge{Source: source, Target: target, Kind: kind})
		edgeIDs[key] = struct{}{}
	}

	rootID := addAssetNode(root)
	endpointLimit := 18
	endpointCount := 0
	addEndpointsForHost := func(host, sourceID string) {
		host = normalizeAssetHost(host)
		for _, item := range items {
			if endpointCount >= endpointLimit || item.Type != "endpoint" || normalizeAssetHost(item.EndpointHost) != host {
				continue
			}
			endpointID := addAssetNode(item)
			addEdge(sourceID, endpointID, "serves")
			endpointCount++
			for _, service := range item.ServiceLabels {
				technologyID := addTechnologyNode(endpointID, service)
				addEdge(endpointID, technologyID, "identifies")
			}
		}
	}

	addDomainRelationships := func(domain AssetCenterItem, domainID string) {
		for _, ip := range domain.IPs {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			ipAsset, exists := assetsByKey["ip\x00"+ip]
			if !exists {
				ipAsset = AssetCenterItem{Type: "ip", Value: ip}
			}
			ipID := addAssetNode(ipAsset)
			addEdge(domainID, ipID, "resolves")
			addEndpointsForHost(ip, ipID)
		}
		addEndpointsForHost(domain.Value, domainID)
	}

	if rootType == "root" {
		for _, item := range items {
			if item.Type != "domain" || assetTopologyRootDomain(item.Value) != root.Value {
				continue
			}
			if item.Value == root.Value {
				addDomainRelationships(item, rootID)
				continue
			}
			domainID := addAssetNode(item)
			addEdge(rootID, domainID, "contains")
			addDomainRelationships(item, domainID)
		}
	} else if rootType == "domain" {
		rootDomain := assetTopologyRootDomain(root.Value)
		if rootDomain != "" && rootDomain != root.Value {
			if rootAsset, exists := assetsByKey["root\x00"+rootDomain]; exists {
				parentRootID := addAssetNode(rootAsset)
				addEdge(parentRootID, rootID, "contains")
			}
		}
		addDomainRelationships(root, rootID)
	} else {
		for _, item := range items {
			if item.Type != "domain" {
				continue
			}
			for _, ip := range item.IPs {
				if strings.TrimSpace(ip) != root.Value {
					continue
				}
				domainID := addAssetNode(item)
				rootDomain := assetTopologyRootDomain(item.Value)
				if rootDomain != "" && rootDomain != item.Value {
					if rootAsset, exists := assetsByKey["root\x00"+rootDomain]; exists {
						parentRootID := addAssetNode(rootAsset)
						addEdge(parentRootID, domainID, "contains")
					}
				} else if rootDomain == item.Value {
					if rootAsset, exists := assetsByKey["root\x00"+rootDomain]; exists {
						parentRootID := addAssetNode(rootAsset)
						addEdge(parentRootID, rootID, "resolves")
					}
				}
				addEdge(domainID, rootID, "resolves")
				break
			}
		}
		addEndpointsForHost(root.Value, rootID)
	}

	return AssetTopologyData{Root: root, Nodes: nodes, Edges: edges}, true
}
