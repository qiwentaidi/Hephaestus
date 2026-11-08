package database

import "time"

// AgentPythonConfig separates applied settings from a pending node-side probe.
// Local nodes use a stable host key, independent of their per-boot worker ID.
type AgentPythonConfig struct {
	NodeKey         string     `gorm:"primaryKey;size:255" json:"-"`
	Mode            string     `json:"mode"`
	Path            string     `json:"path"`
	Version         string     `json:"version"`
	RequestID       string     `json:"requestId"`
	RequestMode     string     `json:"requestMode"`
	RequestPath     string     `json:"requestPath"`
	Save            bool       `json:"-"`
	Status          string     `json:"status"`
	DetectedPath    string     `json:"detectedPath"`
	DetectedVersion string     `json:"detectedVersion"`
	Error           string     `json:"error"`
	CheckedAt       *time.Time `json:"checkedAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

func (AgentPythonConfig) TableName() string { return "agent_python_config" }
