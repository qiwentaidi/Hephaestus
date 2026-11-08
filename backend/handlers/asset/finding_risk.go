package asset

import (
	"strings"
	"sync"
	"time"

	"hephaestus-backend/pkg/findings"

	"gorm.io/gorm"
)

// findingRiskIndex is the map-facing adapter over the asset_risk projection.
// The projection is written by the findings ingestion path; this file only
// reads it and never derives risk from legacy scan tables.
type findingAssetCounts struct {
	DirectCritical    int
	DirectHigh        int
	DirectMedium      int
	InheritedCritical int
	InheritedHigh     int
	InheritedMedium   int
}

func (counts findingAssetCounts) totalCritical() int {
	return counts.DirectCritical + counts.InheritedCritical
}

func (counts findingAssetCounts) totalHigh() int {
	return counts.DirectHigh + counts.InheritedHigh
}

func (counts findingAssetCounts) totalMedium() int {
	return counts.DirectMedium + counts.InheritedMedium
}

type findingRiskIndex struct {
	active bool
	assets map[string]findingAssetCounts
}

func newFindingRiskIndex() *findingRiskIndex {
	return &findingRiskIndex{assets: make(map[string]findingAssetCounts)}
}

func findingAssetKey(assetType, value string) string {
	return strings.ToLower(strings.TrimSpace(assetType)) + "\x00" + strings.ToLower(strings.TrimSpace(value))
}

func (index *findingRiskIndex) hasFindings() bool {
	return index != nil && index.active
}

func (index *findingRiskIndex) countsFor(assetType, value string) (findingAssetCounts, bool) {
	if index == nil {
		return findingAssetCounts{}, false
	}
	counts, ok := index.assets[findingAssetKey(assetType, value)]
	return counts, ok
}

var projectionRiskCache struct {
	mu        sync.Mutex
	fetchedAt time.Time
	index     *findingRiskIndex
}

const projectionRiskCacheTTL = 5 * time.Second

func loadFindingRiskIndexCached(db *gorm.DB) *findingRiskIndex {
	if db == nil {
		return nil
	}
	projectionRiskCache.mu.Lock()
	if projectionRiskCache.index != nil && time.Since(projectionRiskCache.fetchedAt) < projectionRiskCacheTTL {
		index := projectionRiskCache.index
		projectionRiskCache.mu.Unlock()
		return index
	}
	projectionRiskCache.mu.Unlock()

	rows, active, err := findings.LoadRiskIndex(db)
	if err != nil {
		return nil
	}
	index := newFindingRiskIndex()
	index.active = active
	for _, row := range rows {
		index.assets[findingAssetKey(row.AssetType, row.AssetValue)] = findingAssetCounts{
			DirectCritical:    row.DirectCritical,
			DirectHigh:        row.DirectHigh,
			DirectMedium:      row.DirectMedium,
			InheritedCritical: row.InheritedCritical,
			InheritedHigh:     row.InheritedHigh,
			InheritedMedium:   row.InheritedMedium,
		}
	}

	projectionRiskCache.mu.Lock()
	projectionRiskCache.index = index
	projectionRiskCache.fetchedAt = time.Now()
	projectionRiskCache.mu.Unlock()
	return index
}

func mapRiskAssetSummary(summary findings.RiskSummary) AssetMapRiskAssetSummary {
	return AssetMapRiskAssetSummary{
		Total:       summary.Total,
		IP:          summary.IP,
		Domain:      summary.Domain,
		URL:         summary.URL,
		Service:     summary.Service,
		Certificate: summary.Certificate,
		Critical:    summary.Critical,
		High:        summary.High,
		Medium:      summary.Medium,
	}
}
