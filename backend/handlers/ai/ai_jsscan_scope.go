package ai

func isPentestJSScanReadTool(name string) bool {
	switch name {
	case "attack_surface_get_jsscan_overview", "attack_surface_search_jsscan_operations",
		jsScanVulnerabilitySearchTool,
		"attack_surface_search_jsscan_resources", "attack_surface_get_jsscan_evidence",
		"attack_surface_export_jsscan_openapi":
		return true
	default:
		return false
	}
}
