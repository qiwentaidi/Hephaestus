package scan

import (
	"encoding/json"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/jsscan"
	"net/url"
	"sort"
	"strconv"
	"strings"

	sdk "github.com/qiwentaidi/trailblazer/pkg/sdk"
)

type jsScanUnauthorizedContext struct {
	MatchedURL string                      `json:"matchedUrl,omitempty"`
	Profiles   []sdk.StaticProtocolProfile `json:"profiles,omitempty"`
}

type jsScanVulnerabilityDetailResponse struct {
	database.VulnScanJSVulnerability
	UnauthorizedContext *jsScanUnauthorizedContext `json:"unauthorizedContext,omitempty"`
}

type siteTreeCountNode struct {
	Label    string              `json:"label"`
	Children []siteTreeCountNode `json:"children"`
}

func isSiteTreeRouteLabel(label string) bool {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return false
	}
	return strings.HasPrefix(trimmed, "/") || strings.HasPrefix(strings.ToLower(trimmed), "http://") || strings.HasPrefix(strings.ToLower(trimmed), "https://")
}

func countSiteTreeRoutes(nodes []siteTreeCountNode) int {
	total := 0
	for _, node := range nodes {
		if isSiteTreeRouteLabel(node.Label) {
			total++
		}
		if len(node.Children) > 0 {
			total += countSiteTreeRoutes(node.Children)
		}
	}
	return total
}

func extractSiteTreeRouteCount(raw string) int {
	if strings.TrimSpace(raw) == "" {
		return 0
	}

	var listNodes []siteTreeCountNode
	if err := json.Unmarshal([]byte(raw), &listNodes); err == nil {
		return countSiteTreeRoutes(listNodes)
	}

	var singleNode siteTreeCountNode
	if err := json.Unmarshal([]byte(raw), &singleNode); err == nil {
		return countSiteTreeRoutes([]siteTreeCountNode{singleNode})
	}

	return 0
}

func parseQueryInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func parseQueryBool(raw string) (bool, bool) {
	if raw == "" {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes":
		return true, true
	case "0", "false", "no":
		return false, true
	default:
		return false, false
	}
}

func normalizeSortOrder(raw string) string {
	if strings.EqualFold(raw, "asc") {
		return "ASC"
	}
	return "DESC"
}

func normalizeJSAssetType(raw string) string {
	switch strings.TrimSpace(raw) {
	case "email", "phone", "idCard", "sensitive":
		return strings.TrimSpace(raw)
	case "domain", "ip", "ipUrl":
		return "ipUrl"
	default:
		return strings.TrimSpace(raw)
	}
}

func normalizeProtocolContextPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if parsed, err := url.Parse(raw); err == nil && parsed != nil && parsed.Path != "" {
		raw = parsed.Path
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + strings.TrimLeft(raw, "/")
	}
	return strings.TrimRight(raw, "/")
}

func scoreUnauthorizedContextEndpoint(vuln database.VulnScanJSVulnerability, endpoint sdk.StaticProtocolEndpoint) int {
	score := 0

	if traceID := strings.TrimSpace(vuln.TraceID); traceID != "" && traceID == strings.TrimSpace(endpoint.TraceID) {
		score += 100
	}

	vulnURL := strings.TrimSpace(vuln.URL)
	endpointRequestURL := strings.TrimSpace(endpoint.RequestURL)
	if vulnURL != "" && endpointRequestURL != "" && strings.EqualFold(vulnURL, endpointRequestURL) {
		score += 60
	}

	vulnPath := normalizeProtocolContextPath(vulnURL)
	endpointPath := normalizeProtocolContextPath(endpoint.Path)
	if vulnPath != "" && endpointPath != "" {
		if vulnPath == endpointPath {
			score += 40
		} else if strings.HasSuffix(vulnPath, endpointPath) || strings.HasSuffix(endpointPath, vulnPath) {
			score += 20
		}
	}

	if method := strings.ToUpper(strings.TrimSpace(vuln.Method)); method != "" {
		if method == strings.ToUpper(strings.TrimSpace(endpoint.Method)) {
			score += 10
		}
	}

	return score
}

func buildUnauthorizedContextProfile(vuln database.VulnScanJSVulnerability, profile sdk.StaticProtocolProfile) *sdk.StaticProtocolProfile {
	if len(profile.Endpoints) == 0 {
		return nil
	}

	type scoredEndpoint struct {
		Endpoint sdk.StaticProtocolEndpoint
		Score    int
	}

	scored := make([]scoredEndpoint, 0, len(profile.Endpoints))
	for _, endpoint := range profile.Endpoints {
		if score := scoreUnauthorizedContextEndpoint(vuln, endpoint); score > 0 {
			scored = append(scored, scoredEndpoint{Endpoint: endpoint, Score: score})
		}
	}
	if len(scored) == 0 {
		return nil
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		if scored[i].Endpoint.Path != scored[j].Endpoint.Path {
			return scored[i].Endpoint.Path < scored[j].Endpoint.Path
		}
		return scored[i].Endpoint.Method < scored[j].Endpoint.Method
	})

	trimmedEndpoints := make([]sdk.StaticProtocolEndpoint, 0, min(len(scored), 6))
	for _, item := range scored[:min(len(scored), 6)] {
		trimmedEndpoints = append(trimmedEndpoints, item.Endpoint)
	}

	filtered := profile
	filtered.Endpoints = trimmedEndpoints
	filtered.PrimaryEndpoint = nil
	for _, endpoint := range trimmedEndpoints {
		if strings.TrimSpace(endpoint.TraceID) != "" && strings.TrimSpace(endpoint.TraceID) == strings.TrimSpace(vuln.TraceID) {
			selected := endpoint
			filtered.PrimaryEndpoint = &selected
			break
		}
	}
	if filtered.PrimaryEndpoint == nil && len(trimmedEndpoints) > 0 {
		selected := trimmedEndpoints[0]
		filtered.PrimaryEndpoint = &selected
	}
	return &filtered
}

func buildJSScanUnauthorizedContext(vuln database.VulnScanJSVulnerability) (*jsScanUnauthorizedContext, error) {
	if vuln.Type != "未授权访问" && vuln.Title != "未授权访问" {
		return nil, nil
	}
	if strings.TrimSpace(vuln.TaskID) == "" {
		return nil, nil
	}

	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}

	result, err := sdk.AnalyzeStoredJSProtocolsWithStore(vuln.TaskID, jsscan.NewLocalScanDataStore(db), vuln.Version)
	if err != nil {
		return nil, err
	}
	if result == nil || len(result.Profiles) == 0 {
		return nil, nil
	}

	matchedProfiles := make([]sdk.StaticProtocolProfile, 0, len(result.Profiles))
	for _, profile := range result.Profiles {
		if filtered := buildUnauthorizedContextProfile(vuln, profile); filtered != nil {
			matchedProfiles = append(matchedProfiles, *filtered)
		}
	}
	if len(matchedProfiles) == 0 {
		return nil, nil
	}

	return &jsScanUnauthorizedContext{
		MatchedURL: vuln.URL,
		Profiles:   matchedProfiles,
	}, nil
}

func expandJSAssetFilterTypes(raw string) []string {
	switch strings.TrimSpace(raw) {
	case "ipUrl":
		return []string{"ipUrl", "domain", "ip"}
	default:
		if strings.TrimSpace(raw) == "" {
			return nil
		}
		return []string{strings.TrimSpace(raw)}
	}
}
