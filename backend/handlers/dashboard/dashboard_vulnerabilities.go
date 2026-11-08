package dashboard

import (
	"context"
	"time"

	"hephaestus-backend/pkg/vulnerabilitycenter"

	"gorm.io/gorm"
)

type DashboardVulnerabilityStats struct {
	Severity map[string]int64
	Disposal VulnDisposalStatsResponse
}

func LoadDashboardVulnerabilityStats(ctx context.Context, db *gorm.DB, startDate *time.Time) (DashboardVulnerabilityStats, error) {
	stats := DashboardVulnerabilityStats{Severity: map[string]int64{
		"critical": 0, "high": 0, "medium": 0, "low": 0, "info": 0,
	}}
	rows, err := vulnerabilitycenter.Breakdowns(ctx, db, startDate)
	if err != nil {
		return stats, err
	}
	for _, row := range rows {
		stats.Severity[row.Severity] += row.Count

		switch row.Status {
		case "in_progress":
			stats.Disposal.InProgress += row.Count
		case "fixed":
			stats.Disposal.Fixed += row.Count
		case "false_positive":
			stats.Disposal.FalsePositive += row.Count
		case "ignored":
			stats.Disposal.Ignored += row.Count
		default:
			// Empty or unrecognized legacy states remain pending disposition.
			stats.Disposal.Open += row.Count
		}
	}
	return stats, nil
}
