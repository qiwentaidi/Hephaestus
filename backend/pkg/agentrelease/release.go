// Package agentrelease describes the binaries compatible with this server.
// Installers and release assets share the same version contract.
package agentrelease

import (
	"embed"
	"encoding/base64"
	"fmt"
	"hephaestus-backend/pkg/version"
	"strings"
)

const Repository = "qiwentaidi/Hephaestus-Agent"

//go:embed installers/*
var Installers embed.FS

// Installer returns a self-contained BAT by embedding its PowerShell companion.
func Installer(name string) ([]byte, error) {
	data, err := Installers.ReadFile("installers/" + name)
	if err != nil || name != "install.bat" {
		return data, err
	}
	ps, err := Installers.ReadFile("installers/install.ps1")
	if err != nil {
		return nil, err
	}
	bat := strings.ReplaceAll(string(data), "__HEPHAESTUS_POWERSHELL_BASE64__", base64.StdEncoding.EncodeToString(ps))
	return []byte(strings.ReplaceAll(bat, "\n", "\r\n")), nil
}

type Package struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Release struct {
	ServerVersion string    `json:"serverVersion"`
	AgentVersion  string    `json:"agentVersion"`
	Compatibility string    `json:"compatibility"`
	ReleaseURL    string    `json:"releaseUrl"`
	ChecksumURL   string    `json:"checksumUrl"`
	Packages      []Package `json:"packages"`
}

// Current uses exact matching while nodes share the server's database schema.
// Future compatible releases can be selected here without changing installers.
func Current() Release {
	base := "https://github.com/" + Repository + "/releases"
	r := Release{ServerVersion: version.Version, AgentVersion: version.Version,
		Compatibility: "exact", ReleaseURL: base + "/tag/" + version.Version,
		ChecksumURL: base + "/download/" + version.Version + "/SHA256SUMS.txt"}
	for _, platform := range [][2]string{{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"}} {
		name := fmt.Sprintf("hephaestus-agent-%s-%s.zip", platform[0], platform[1])
		r.Packages = append(r.Packages, Package{OS: platform[0], Arch: platform[1], Name: name,
			URL: base + "/download/" + r.AgentVersion + "/" + name})
	}
	return r
}
