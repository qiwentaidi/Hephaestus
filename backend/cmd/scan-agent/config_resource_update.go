package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hephaestus-backend/pkg/agentruntime"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/logger"
	"hephaestus-backend/pkg/webscan"
)

const agentConfigVersionFileName = ".hephaestus-config-version"

type agentConfigVersionResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Version string `json:"version"`
	} `json:"data"`
	Error string `json:"error"`
}

func syncAgentConfigResources(ctx context.Context, session *agentruntime.NodeSession, serverAddress string) error {
	serverAddress = strings.TrimRight(strings.TrimSpace(serverAddress), "/")
	if serverAddress == "" || session == nil {
		return nil
	}
	remoteVersion, err := fetchAgentConfigResourceVersion(ctx, session, serverAddress)
	if err != nil {
		return err
	}
	localVersion := readAgentLocalConfigResourceVersion()
	if strings.TrimSpace(remoteVersion) == "" || strings.TrimSpace(remoteVersion) == strings.TrimSpace(localVersion) {
		return nil
	}

	zipPath, cleanup, err := downloadAgentConfigResourceZip(ctx, session, serverAddress)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := applyAgentConfigResourceZip(zipPath, remoteVersion); err != nil {
		return err
	}
	logger.Info("[agent] 配置资源已更新: %s -> %s", localVersion, remoteVersion)
	return nil
}

func fetchAgentConfigResourceVersion(ctx context.Context, session *agentruntime.NodeSession, serverAddress string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverAddress+"/api/v1/webscan/agent-config/version", nil)
	if err != nil {
		return "", err
	}
	if err := session.AuthorizePlatformRequest(req.Context(), session.ID, req); err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result agentConfigVersionResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&result); err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK || !result.Success {
		if strings.TrimSpace(result.Error) != "" {
			return "", fmt.Errorf("%s", result.Error)
		}
		return "", fmt.Errorf("agent config version request failed: %s", resp.Status)
	}
	return strings.TrimSpace(result.Data.Version), nil
}

func downloadAgentConfigResourceZip(ctx context.Context, session *agentruntime.NodeSession, serverAddress string) (string, func(), error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverAddress+"/api/v1/webscan/agent-config/download", nil)
	if err != nil {
		return "", nil, err
	}
	if err := session.AuthorizePlatformRequest(req.Context(), session.ID, req); err != nil {
		return "", nil, err
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		return "", nil, fmt.Errorf("agent config download failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}

	tempDir, err := os.MkdirTemp("", "hephaestus-agent-config-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }
	zipPath := filepath.Join(tempDir, "config.zip")
	out, err := os.Create(zipPath)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		cleanup()
		return "", nil, err
	}
	if err := out.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return zipPath, cleanup, nil
}

func readAgentLocalConfigResourceVersion() string {
	data, err := os.ReadFile(filepath.Join(config.GetConfigResourceRoot(), agentConfigVersionFileName))
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(data))
	return value
}

func applyAgentConfigResourceZip(zipPath, version string) error {
	tempDir, err := os.MkdirTemp("", "hephaestus-agent-config-apply-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	if err := unzipAgentConfigResource(zipPath, tempDir); err != nil {
		return err
	}
	payloadRoot := findAgentConfigPayloadRoot(tempDir)
	if payloadRoot == "" {
		return fmt.Errorf("未找到可应用的 Agent 配置资源")
	}

	targetRoot := config.GetConfigResourceRoot()
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return err
	}
	for _, name := range []string{"pocs", "crack", "dirsearch"} {
		src := filepath.Join(payloadRoot, name)
		dst := filepath.Join(targetRoot, name)
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
		if info, err := os.Stat(src); err != nil || !info.IsDir() {
			continue
		}
		if err := copyAgentConfigResourceDir(src, dst); err != nil {
			return err
		}
	}

	version = strings.TrimSpace(version)
	if version == "" {
		version = "v0.0.0"
	}
	if err := os.WriteFile(filepath.Join(targetRoot, agentConfigVersionFileName), []byte(version+"\n"), 0o644); err != nil {
		return err
	}
	if err := webscan.SignUpdatedCodeTemplatesForNuclei(filepath.Join(targetRoot, "pocs")); err != nil {
		return err
	}
	return webscan.RefreshPOCTemplateCache()
}

func unzipAgentConfigResource(zipPath, dest string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	for _, file := range reader.File {
		name := filepath.Clean(strings.ReplaceAll(file.Name, "\\", "/"))
		if name == "." || strings.HasPrefix(name, ".."+string(os.PathSeparator)) || filepath.IsAbs(name) {
			return fmt.Errorf("zip entry path is unsafe: %s", file.Name)
		}
		target := filepath.Join(destAbs, name)
		if target != destAbs && !strings.HasPrefix(target, destAbs+string(os.PathSeparator)) {
			return fmt.Errorf("zip entry path escapes destination: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			// Resource archives may omit Unix permissions or contain read-only
			// directories. The receiving Worker owns and must traverse its copy.
			if err := os.MkdirAll(target, file.Mode().Perm()|0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode().Perm()|0o600)
		if err != nil {
			_ = src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		_ = src.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func findAgentConfigPayloadRoot(root string) string {
	candidates := []string{root, filepath.Join(root, "config")}
	for _, candidate := range candidates {
		if hasAgentConfigResourcePayload(candidate) {
			return candidate
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		child := filepath.Join(root, entry.Name())
		if hasAgentConfigResourcePayload(child) {
			return child
		}
		nested := filepath.Join(child, "config")
		if hasAgentConfigResourcePayload(nested) {
			return nested
		}
	}
	return ""
}

func hasAgentConfigResourcePayload(root string) bool {
	for _, name := range []string{"pocs", "crack", "dirsearch"} {
		if info, err := os.Stat(filepath.Join(root, name)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func copyAgentConfigResourceDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	})
}
