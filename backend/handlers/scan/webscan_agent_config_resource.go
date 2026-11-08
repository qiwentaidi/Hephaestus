package scan

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	cfg "hephaestus-backend/pkg/config"

	"github.com/gin-gonic/gin"
)

func AgentConfigResourceVersionHandler(c *gin.Context) {
	if err := verifyAgentResourceAuth(c, false); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": err.Error()})
		return
	}

	version := cfg.ReadConfigUpdateVersion()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"version": version,
		},
	})
}

func AgentConfigResourceDownloadHandler(c *gin.Context) {
	if err := verifyAgentResourceAuth(c, false); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": err.Error()})
		return
	}

	version := cfg.ReadConfigUpdateVersion()
	data, err := buildAgentConfigResourceZip(version)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	sum := sha256.Sum256(data)
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"hephaestus-config-%s.zip\"", sanitizeConfigResourceVersion(version)))
	c.Header("X-Hephaestus-Config-Version", version)
	c.Header("X-Hephaestus-Config-SHA256", hex.EncodeToString(sum[:]))
	c.Data(http.StatusOK, "application/zip", data)
}

func buildAgentConfigResourceZip(version string) ([]byte, error) {
	root := cfg.GetConfigResourceRoot()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"pocs", "crack", "dirsearch"} {
		src := filepath.Join(root, name)
		if _, err := os.Stat(src); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			_ = zw.Close()
			return nil, err
		}
		if err := addPathToZip(zw, src, name); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	w, err := zw.Create(cfg.ConfigUpdateVersionFileName)
	if err != nil {
		_ = zw.Close()
		return nil, err
	}
	if _, err := w.Write([]byte(strings.TrimSpace(version) + "\n")); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func addPathToZip(zw *zip.Writer, srcRoot, zipRoot string) error {
	return filepath.Walk(srcRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(zipRoot, rel))
		if rel == "." {
			name = zipRoot
		}
		if info.IsDir() {
			if name == "" {
				return nil
			}
			_, err := zw.Create(name + "/")
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = name
		header.Method = zip.Deflate
		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
}

func sanitizeConfigResourceVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return cfg.DefaultConfigVersion
	}
	var b strings.Builder
	for _, r := range version {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return cfg.DefaultConfigVersion
	}
	return b.String()
}
