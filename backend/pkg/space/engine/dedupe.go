package engine

import (
	"fmt"
	"strings"
)

// resultDomainKey keeps the domain component meaningful. Some engines put a
// service name (for example "ssh") or the IP itself in the domain field; those
// values must not split otherwise identical IP:port records.
func resultDomainKey(result UnifiedResult) string {
	domain := strings.ToLower(strings.TrimSpace(result.Domain))
	domain = strings.TrimSuffix(strings.TrimPrefix(domain, "*."), ".")
	if domain == "" || !strings.Contains(domain, ".") {
		return ""
	}
	if ip := strings.ToLower(strings.TrimSpace(result.IP)); ip != "" && domain == ip {
		return ""
	}
	return domain
}

// ResultIdentity returns the dedupe identity of a single result. Asset
// mapping treats the same IP + port + domain as one record even when engines
// report different URL schemes, titles or protocols for it.
func ResultIdentity(result UnifiedResult) string {
	ip := strings.ToLower(strings.TrimSpace(result.IP))
	domain := resultDomainKey(result)
	if ip == "" && domain == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d:%s", ip, result.Port, domain)
}

// mergeICPFacts keeps ICP facts that only one engine reported when several
// engines return the same IP:port:domain record. The first occurrence still
// provides the displayed URL, title and source provenance.
func mergeICPFacts(target *UnifiedResult, source UnifiedResult) {
	if target.ICP == "" && source.ICP != "" {
		target.ICP = source.ICP
	}
	if target.ICPName == "" && source.ICPName != "" {
		target.ICPName = source.ICPName
	}
}

// DeduplicateResults removes results that share the same IP:port:domain while
// keeping the first occurrence (and therefore its source provenance).
func DeduplicateResults(results []UnifiedResult) []UnifiedResult {
	if len(results) == 0 {
		return results
	}
	seen := make(map[string]int, len(results))
	deduplicated := make([]UnifiedResult, 0, len(results))
	for _, result := range results {
		key := ResultIdentity(result)
		if key != "" {
			if index, exists := seen[key]; exists {
				mergeICPFacts(&deduplicated[index], result)
				continue
			}
			seen[key] = len(deduplicated)
		}
		deduplicated = append(deduplicated, result)
	}
	return deduplicated
}

// DeduplicateAgainst removes incoming results that duplicate either the
// already stored results or an earlier incoming result. ICP facts from a
// dropped duplicate are merged into the retained record so paging never loses
// ICP information that a later engine reported.
func DeduplicateAgainst(existing, incoming []UnifiedResult) []UnifiedResult {
	if len(incoming) == 0 {
		return incoming
	}
	existingIndex := make(map[string]int, len(existing))
	for index := range existing {
		if key := ResultIdentity(existing[index]); key != "" {
			existingIndex[key] = index
		}
	}
	addedIndex := make(map[string]int, len(incoming))
	added := make([]UnifiedResult, 0, len(incoming))
	for _, result := range incoming {
		key := ResultIdentity(result)
		if key != "" {
			if index, exists := existingIndex[key]; exists {
				mergeICPFacts(&existing[index], result)
				continue
			}
			if index, exists := addedIndex[key]; exists {
				mergeICPFacts(&added[index], result)
				continue
			}
			addedIndex[key] = len(added)
		}
		added = append(added, result)
	}
	return added
}
