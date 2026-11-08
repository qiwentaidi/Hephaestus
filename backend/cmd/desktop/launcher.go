package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"hephaestus-backend/pkg/config"
)

type LauncherGroup struct {
	Name     string         `json:"name"`
	Children []LauncherItem `json:"children"`
}

type LauncherItem struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Path    string `json:"path"`
	Target  string `json:"target"`
	Favicon string `json:"favicon"`
	Jdk     string `json:"jdk"`
}

type LauncherJDK struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

const launcherMigrationSourceSlack = "slack"

func inferLauncherItemType(path string) string {
	lowerPath := strings.ToLower(strings.TrimSpace(path))
	switch {
	case strings.HasPrefix(lowerPath, "http://"), strings.HasPrefix(lowerPath, "https://"):
		return "Link"
	case strings.HasSuffix(lowerPath, ".jar"):
		return "JAR"
	case strings.HasSuffix(lowerPath, ".exe"), strings.HasSuffix(lowerPath, ".lnk"), strings.HasSuffix(lowerPath, ".bat"), strings.HasSuffix(lowerPath, ".app"):
		return "APP"
	default:
		return "CMD"
	}
}

func addDroppedFilesToGroup(groupName string, files []string) error {
	groups, err := readLauncherGroups()
	if err != nil {
		return err
	}

	groupIndex := -1
	for index, group := range groups {
		if group.Name == groupName {
			groupIndex = index
			break
		}
	}
	if groupIndex < 0 {
		return fmt.Errorf("launcher group not found: %s", groupName)
	}

	children := groups[groupIndex].Children
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		item := LauncherItem{
			Name:    filepath.Base(file),
			Type:    inferLauncherItemType(file),
			Path:    file,
			Target:  "",
			Favicon: generateLauncherIconFromFile(file),
			Jdk:     "",
		}
		children = append(children, item)
	}
	groups[groupIndex].Children = children
	return writeLauncherGroups(groups)
}

func launcherDir() string {
	dir := filepath.Join(config.GetConfigBaseDir(), "launcher")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func launcherNavigationFile() string {
	return filepath.Join(launcherDir(), "navigation.json")
}

func launcherJDKFile() string {
	return filepath.Join(launcherDir(), "jdk.json")
}

func launcherMigrationMarkerFile() string {
	return filepath.Join(launcherDir(), ".migration.json")
}

func legacySlackLauncherNavigationFile() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, "slack", "navogation.json")
}

func legacySlackLauncherJDKFile() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, "slack", "jdkConfig.json")
}

type launcherMigrationMarker struct {
	ImportedFrom string `json:"importedFrom"`
	ImportedAt   string `json:"importedAt"`
}

func hasLauncherMigrationMarker() bool {
	_, err := os.Stat(launcherMigrationMarkerFile())
	return err == nil
}

func writeLauncherMigrationMarker(source string) {
	_ = writeLauncherJSON(launcherMigrationMarkerFile(), launcherMigrationMarker{
		ImportedFrom: source,
		ImportedAt:   time.Now().Format(time.RFC3339),
	})
}

func ensureLegacySlackLauncherMigrated() error {
	if hasLauncherMigrationMarker() {
		return nil
	}

	groups, err := readLauncherGroupsRaw()
	if err != nil {
		return err
	}
	jdks, err := readLauncherJDKsRaw()
	if err != nil {
		return err
	}

	if len(groups) > 0 || len(jdks) > 0 {
		writeLauncherMigrationMarker("existing-hephaestus")
		return nil
	}

	legacyGroups, legacyGroupErr := readLegacySlackLauncherGroups()
	legacyJDKs, legacyJDKErr := readLegacySlackLauncherJDKs()
	if legacyGroupErr != nil && isLauncherJSONDecodeError(legacyGroupErr) {
		log.Printf("skip invalid legacy launcher groups: %v", legacyGroupErr)
		legacyGroups = nil
	} else if legacyGroupErr != nil && !os.IsNotExist(legacyGroupErr) {
		return legacyGroupErr
	}
	if legacyJDKErr != nil && isLauncherJSONDecodeError(legacyJDKErr) {
		log.Printf("skip invalid legacy launcher jdks: %v", legacyJDKErr)
		legacyJDKs = nil
	} else if legacyJDKErr != nil && !os.IsNotExist(legacyJDKErr) {
		return legacyJDKErr
	}

	if len(legacyGroups) == 0 && len(legacyJDKs) == 0 {
		return nil
	}

	if len(legacyGroups) > 0 {
		if err := writeLauncherGroups(legacyGroups); err != nil {
			return err
		}
	}
	if len(legacyJDKs) > 0 {
		if err := writeLauncherJDKs(legacyJDKs); err != nil {
			return err
		}
	}
	writeLauncherMigrationMarker(launcherMigrationSourceSlack)
	return nil
}

func readLegacySlackLauncherGroups() ([]LauncherGroup, error) {
	path := legacySlackLauncherNavigationFile()
	if path == "" {
		return nil, os.ErrNotExist
	}
	var groups []LauncherGroup
	if err := readLauncherJSONFromPath(path, &groups); err != nil {
		return nil, err
	}
	return normalizeLauncherGroups(groups), nil
}

func readLegacySlackLauncherJDKs() ([]LauncherJDK, error) {
	path := legacySlackLauncherJDKFile()
	if path == "" {
		return nil, os.ErrNotExist
	}
	var jdks []LauncherJDK
	if err := readLauncherJSONFromPath(path, &jdks); err != nil {
		return nil, err
	}
	return jdks, nil
}

func readLauncherGroups() ([]LauncherGroup, error) {
	if err := ensureLegacySlackLauncherMigrated(); err != nil {
		return nil, err
	}
	return readLauncherGroupsRaw()
}

func readLauncherGroupsRaw() ([]LauncherGroup, error) {
	var groups []LauncherGroup
	if err := readLauncherJSON(launcherNavigationFile(), &groups); err != nil {
		return nil, err
	}
	return normalizeLauncherGroups(groups), nil
}

func writeLauncherGroups(groups []LauncherGroup) error {
	return writeLauncherJSON(launcherNavigationFile(), normalizeLauncherGroups(groups))
}

func readLauncherJDKs() ([]LauncherJDK, error) {
	if err := ensureLegacySlackLauncherMigrated(); err != nil {
		return nil, err
	}
	return readLauncherJDKsRaw()
}

func readLauncherJDKsRaw() ([]LauncherJDK, error) {
	var jdks []LauncherJDK
	if err := readLauncherJSON(launcherJDKFile(), &jdks); err != nil {
		return nil, err
	}
	return normalizeLauncherJDKs(jdks), nil
}

func writeLauncherJDKs(jdks []LauncherJDK) error {
	return writeLauncherJSON(launcherJDKFile(), normalizeLauncherJDKs(jdks))
}

func readLauncherJSON(path string, target any) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := writeDefaultLauncherJSON(path, target); err != nil {
			return err
		}
	}

	if err := readLauncherJSONFromPath(path, target); err != nil {
		if !isLauncherJSONDecodeError(err) {
			return err
		}
		if backupErr := backupInvalidLauncherJSON(path); backupErr != nil {
			return errors.Join(err, backupErr)
		}
		if defaultErr := writeDefaultLauncherJSON(path, target); defaultErr != nil {
			return errors.Join(err, defaultErr)
		}
		return readLauncherJSONFromPath(path, target)
	}
	return nil
}

func readLauncherJSONFromPath(path string, target any) error {

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		data = []byte("[]")
	}
	return json.Unmarshal(data, target)
}

func isLauncherJSONDecodeError(err error) bool {
	var syntaxError *json.SyntaxError
	if errors.As(err, &syntaxError) {
		return true
	}
	var typeError *json.UnmarshalTypeError
	return errors.As(err, &typeError)
}

func backupInvalidLauncherJSON(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	backupPath := fmt.Sprintf("%s.invalid-%s", path, time.Now().Format("20060102150405"))
	return os.WriteFile(backupPath, data, 0644)
}

func writeDefaultLauncherJSON(path string, target any) error {
	switch target.(type) {
	case *[]LauncherGroup, *[]LauncherJDK:
		return writeLauncherJSON(path, []any{})
	default:
		return writeLauncherJSON(path, target)
	}
}

func writeLauncherJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func normalizeLauncherGroups(groups []LauncherGroup) []LauncherGroup {
	if groups == nil {
		return []LauncherGroup{}
	}
	for index := range groups {
		groups[index].Name = strings.TrimSpace(groups[index].Name)
		if groups[index].Children == nil {
			groups[index].Children = []LauncherItem{}
		}
		for childIndex := range groups[index].Children {
			groups[index].Children[childIndex].Name = strings.TrimSpace(groups[index].Children[childIndex].Name)
			groups[index].Children[childIndex].Type = strings.TrimSpace(groups[index].Children[childIndex].Type)
			groups[index].Children[childIndex].Path = strings.TrimSpace(groups[index].Children[childIndex].Path)
			groups[index].Children[childIndex].Target = strings.TrimSpace(groups[index].Children[childIndex].Target)
			groups[index].Children[childIndex].Favicon = strings.TrimSpace(groups[index].Children[childIndex].Favicon)
			groups[index].Children[childIndex].Jdk = strings.TrimSpace(groups[index].Children[childIndex].Jdk)
			if groups[index].Children[childIndex].Type == "" {
				groups[index].Children[childIndex].Type = inferLauncherItemType(groups[index].Children[childIndex].Path)
			}
			if groups[index].Children[childIndex].Favicon == "" {
				switch groups[index].Children[childIndex].Type {
				case "Link":
					groups[index].Children[childIndex].Favicon = generateLauncherIconFromURL(groups[index].Children[childIndex].Path)
				default:
					groups[index].Children[childIndex].Favicon = generateLauncherIconFromFile(groups[index].Children[childIndex].Path)
				}
			}
		}
	}
	return groups
}

func normalizeLauncherJDKs(jdks []LauncherJDK) []LauncherJDK {
	if jdks == nil {
		return []LauncherJDK{}
	}
	for index := range jdks {
		jdks[index].Name = strings.TrimSpace(jdks[index].Name)
		jdks[index].Path = strings.TrimSpace(jdks[index].Path)
	}
	return jdks
}

func resetLauncherMigrationState() error {
	var errs []error
	for _, path := range []string{
		launcherNavigationFile(),
		launcherJDKFile(),
		launcherMigrationMarkerFile(),
	} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func runLauncherItem(item LauncherItem) error {
	itemType := strings.ToUpper(strings.TrimSpace(item.Type))
	switch itemType {
	case "LINK":
		return openURL(item.Path)
	case "JAR":
		return runJAR(item)
	case "APP":
		return runApp(item.Path)
	default:
		return runCommandItem(item)
	}
}

func runJAR(item LauncherItem) error {
	javaBin := strings.TrimSpace(item.Jdk)
	if javaBin == "" {
		javaBin = "java"
	}
	cmd := exec.Command(javaBin, "-jar", filepath.Base(item.Path))
	cmd.Dir = filepath.Dir(item.Path)
	return cmd.Start()
}

func runApp(path string) error {
	switch runtime.GOOS {
	case "darwin":
		if strings.HasSuffix(strings.ToLower(path), ".app") {
			return exec.Command("open", path).Start()
		}
		return exec.Command("open", path).Start()
	case "windows":
		return exec.Command(path).Start()
	default:
		return exec.Command(path).Start()
	}
}

func runCommandItem(item LauncherItem) error {
	target := strings.TrimSpace(item.Target)
	if target == "" {
		return openTerminalForPath(item.Path)
	}

	resolvedTarget := strings.ReplaceAll(target, "%path%", filepath.Base(item.Path))
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/C", resolvedTarget)
		cmd.Dir = filepath.Dir(item.Path)
		return cmd.Start()
	}
	cmd := exec.Command("sh", "-c", resolvedTarget)
	cmd.Dir = filepath.Dir(item.Path)
	return cmd.Start()
}

func openURL(raw string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", raw).Start()
	case "windows":
		return exec.Command("cmd", "/C", "start", raw).Start()
	default:
		return exec.Command("xdg-open", raw).Start()
	}
}

func openFolder(path string) error {
	dir := path
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		dir = filepath.Dir(path)
	}

	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", dir).Start()
	case "windows":
		return exec.Command("explorer", dir).Start()
	default:
		return exec.Command("xdg-open", dir).Start()
	}
}

func openTerminalForPath(path string) error {
	dir := path
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		dir = filepath.Dir(path)
	}

	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-a", "Terminal", dir).Start()
	case "windows":
		return exec.Command("cmd", "/C", "start", "cmd", "/K", "cd /d", dir).Start()
	default:
		return exec.Command("xdg-open", dir).Start()
	}
}

func generateLauncherIconFromFile(path string) string {
	if !isLauncherIconFile(path) {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return compressLauncherImage(path, data)
}

func isLauncherIconFile(path string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".png", ".jpg", ".jpeg", ".svg", ".ico", ".icns", ".webp":
		return true
	default:
		return false
	}
}

func generateLauncherIconFromURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}

	candidates := []string{
		fmt.Sprintf("%s://%s/favicon.ico", parsed.Scheme, parsed.Host),
	}

	client := &http.Client{}
	for _, target := range candidates {
		resp, err := client.Get(target)
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil || resp.StatusCode != http.StatusOK {
			continue
		}
		if icon := compressLauncherImage(target, body); icon != "" {
			return icon
		}
	}

	return ""
}

func compressLauncherImage(path string, data []byte) string {
	lowerPath := strings.ToLower(path)
	mime := "image/png"
	switch {
	case strings.HasSuffix(lowerPath, ".svg"):
		mime = "image/svg+xml"
	case strings.HasSuffix(lowerPath, ".ico"):
		mime = "image/x-icon"
	case strings.HasSuffix(lowerPath, ".jpg"), strings.HasSuffix(lowerPath, ".jpeg"):
		mime = "image/jpeg"
	case strings.HasSuffix(lowerPath, ".webp"):
		mime = "image/webp"
	case strings.HasSuffix(lowerPath, ".icns"):
		mime = "image/icns"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}
