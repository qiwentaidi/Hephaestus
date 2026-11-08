package crack

// VulnerabilityInfo is the crack package's portable finding model.
// It intentionally does not depend on project-local packages.
type VulnerabilityInfo struct {
	ID                 string
	Name               string
	Description        string
	Reference          string
	Type               string
	Severity           string
	URL                string
	Request            string
	Response           string
	ResponseTime       string
	Extract            string
	PrincipleDetection bool
}
