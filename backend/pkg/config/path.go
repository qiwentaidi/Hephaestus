package config

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	configBaseDir  string
	configBaseOnce sync.Once
	logsDirOnce    sync.Once
	dataDirOnce    sync.Once
)

func resolveExecutableDir() string {
	execPath, err := os.Executable()
	if err != nil || execPath == "" {
		return ""
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return filepath.Dir(execPath)
	}
	return filepath.Dir(execPath)
}

// copyFile 复制单个文件
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, srcInfo.Mode())
}

// GetConfigBaseDir 获取配置文件所在目录。
// 对独立部署的 Agent，config.yaml 位于二进制同级目录，
// 而静态资源（pocs/crack/dirsearch）位于同级 config/ 目录。
func GetConfigBaseDir() string {
	configBaseOnce.Do(func() {
		if envDir := filepath.Clean(os.Getenv("HEPHAESTUS_CONFIG_DIR")); envDir != "" && envDir != "." {
			configBaseDir = envDir
			if err := os.MkdirAll(configBaseDir, 0755); err == nil {
				return
			}
		}

		if isAgentProcess() {
			if execDir := resolveExecutableDir(); execDir != "" {
				siblingConfigDir := filepath.Join(execDir, "config")
				if _, err := os.Stat(siblingConfigDir); err == nil {
					configBaseDir = execDir
					return
				}
				if _, err := os.Stat(filepath.Join(execDir, "config.yaml")); err == nil {
					configBaseDir = execDir
					return
				}
				if _, err := os.Stat(filepath.Join(execDir, "config.example.yaml")); err == nil {
					configBaseDir = execDir
					return
				}
			}
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			// 如果获取用户主目录失败，使用当前工作目录下的 .config/hephaestus
			wd, _ := os.Getwd()
			configBaseDir = filepath.Join(wd, ".config", "hephaestus")
		} else {
			configBaseDir = filepath.Join(homeDir, ".config", "hephaestus")
		}

		// 确保目录存在
		if err := os.MkdirAll(configBaseDir, 0755); err != nil {
			// 如果创建失败，使用当前工作目录
			wd, _ := os.Getwd()
			configBaseDir = filepath.Join(wd, ".config", "hephaestus")
			os.MkdirAll(configBaseDir, 0755)
		}

	})

	return configBaseDir
}

// ForceSetConfigBaseDir overrides the cached config base directory.
// CLI mode creates an isolated temporary config directory after package init,
// so it must be able to reset the path chosen by earlier package-level setup.
func ForceSetConfigBaseDir(dir string) {
	cleanDir := filepath.Clean(strings.TrimSpace(dir))
	if cleanDir == "" || cleanDir == "." {
		return
	}
	if err := os.MkdirAll(cleanDir, 0755); err != nil {
		return
	}
	configBaseDir = cleanDir
	configBaseOnce = sync.Once{}
	configBaseOnce.Do(func() {
		configBaseDir = cleanDir
	})
	logsDirOnce = sync.Once{}
	dataDirOnce = sync.Once{}
}

// GetConfigResourceRoot returns the unified resource directory used by
// pocs/crack/dirsearch and downloadable config bundles.
func GetConfigResourceRoot() string {
	if envDir := resolveResourceRootFromEnv(); envDir != "" {
		return envDir
	}

	resourceRoot := filepath.Join(GetConfigBaseDir(), "config")
	if err := os.MkdirAll(resourceRoot, 0755); err == nil {
		return resourceRoot
	}

	wd, _ := os.Getwd()
	return filepath.Join(wd, ".config", "hephaestus", "config")
}

// GetLogsDir returns the unified runtime log directory under the user's
// Hephaestus config base directory.
func GetLogsDir() string {
	logsDir := filepath.Join(GetConfigBaseDir(), "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		wd, _ := os.Getwd()
		logsDir = filepath.Join(wd, ".config", "hephaestus", "logs")
		_ = os.MkdirAll(logsDir, 0755)
	}

	logsDirOnce.Do(func() {
		migrateLegacyLogs(logsDir)
	})

	return logsDir
}

// GetDataDir returns the unified runtime data directory under the user's
// Hephaestus config base directory.
func GetDataDir() string {
	dataDir := filepath.Join(GetConfigBaseDir(), "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		wd, _ := os.Getwd()
		dataDir = filepath.Join(wd, ".config", "hephaestus", "data")
		_ = os.MkdirAll(dataDir, 0755)
	}

	dataDirOnce.Do(func() {
		migrateLegacyDataDir(dataDir)
	})

	return dataDir
}

func GetScanAssetDir(assetType string) string {
	cleanType := strings.ToLower(strings.TrimSpace(assetType))
	switch cleanType {
	case "screenshot", "favicon":
	default:
		return GetDataDir()
	}

	assetDir := filepath.Join(GetDataDir(), cleanType)
	_ = os.MkdirAll(assetDir, 0755)
	return assetDir
}

func migrateLegacyLogs(newLogsDir string) {
	for _, oldLogsDir := range listLegacyLogsDirs() {
		if samePath(oldLogsDir, newLogsDir) {
			continue
		}

		movedCount, err := migrateLogFiles(oldLogsDir, newLogsDir)
		if err != nil {
			log.Printf("[日志迁移] 从 %s 迁移到 %s 失败: %v", oldLogsDir, newLogsDir, err)
			continue
		}
		if movedCount == 0 {
			continue
		}

		log.Printf("[日志迁移] 已将 %d 个日志文件从 %s 迁移到 %s", movedCount, oldLogsDir, newLogsDir)
		_ = removeDirIfEmpty(oldLogsDir)
		_ = removeDirIfEmpty(filepath.Dir(oldLogsDir))
	}
}

func listLegacyLogsDirs() []string {
	candidates := make([]string, 0, 2)

	if wd, err := os.Getwd(); err == nil && wd != "" {
		candidates = append(candidates, filepath.Join(wd, "data", "logs"))
	}

	if execDir := resolveExecutableDir(); execDir != "" {
		candidates = append(candidates, filepath.Join(execDir, "data", "logs"))
	}

	seen := make(map[string]struct{}, len(candidates))
	dirs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		cleanPath := filepath.Clean(candidate)
		if _, err := os.Stat(cleanPath); err != nil {
			continue
		}
		if _, ok := seen[cleanPath]; ok {
			continue
		}
		seen[cleanPath] = struct{}{}
		dirs = append(dirs, cleanPath)
	}

	return dirs
}

func migrateLegacyDataDir(newDataDir string) {
	for _, oldDataDir := range listLegacyDataDirs() {
		if samePath(oldDataDir, newDataDir) {
			continue
		}

		movedCount, err := migrateDirContentsWithConflictRename(oldDataDir, newDataDir, "migrated", map[string]struct{}{
			"logs": {},
		})
		if err != nil {
			log.Printf("[数据迁移] 从 %s 迁移到 %s 失败: %v", oldDataDir, newDataDir, err)
			continue
		}
		if movedCount == 0 {
			continue
		}

		log.Printf("[数据迁移] 已将 %d 个数据项从 %s 迁移到 %s", movedCount, oldDataDir, newDataDir)
		_ = removeDirIfEmpty(oldDataDir)
	}
}

func listLegacyDataDirs() []string {
	candidates := make([]string, 0, 2)

	if wd, err := os.Getwd(); err == nil && wd != "" {
		candidates = append(candidates, filepath.Join(wd, "data"))
	}

	if execDir := resolveExecutableDir(); execDir != "" {
		candidates = append(candidates, filepath.Join(execDir, "data"))
	}

	seen := make(map[string]struct{}, len(candidates))
	dirs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		cleanPath := filepath.Clean(candidate)
		if _, err := os.Stat(cleanPath); err != nil {
			continue
		}
		if _, ok := seen[cleanPath]; ok {
			continue
		}
		seen[cleanPath] = struct{}{}
		dirs = append(dirs, cleanPath)
	}

	return dirs
}

func migrateLogFiles(oldLogsDir, newLogsDir string) (int, error) {
	entries, err := os.ReadDir(oldLogsDir)
	if err != nil {
		return 0, err
	}

	movedCount := 0
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}

		srcPath := filepath.Join(oldLogsDir, entry.Name())
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}

		dstPath, err := resolveMigratedLogPath(newLogsDir, entry.Name())
		if err != nil {
			return movedCount, err
		}

		if err := moveFile(srcPath, dstPath); err != nil {
			return movedCount, err
		}
		movedCount++
	}

	return movedCount, nil
}

func resolveMigratedLogPath(logsDir, fileName string) (string, error) {
	return resolveMigratedFilePath(logsDir, fileName, "migrated")
}

func resolveMigratedFilePath(dir, fileName, suffix string) (string, error) {
	ext := filepath.Ext(fileName)
	base := strings.TrimSuffix(fileName, ext)

	candidate := filepath.Join(dir, fileName)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate, nil
	}

	for i := 1; ; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s.%s.%d%s", base, suffix, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
	}
}

func migrateFilesWithConflictRename(oldDir, newDir, suffix string) (int, error) {
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return 0, err
	}

	movedCount := 0
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}

		srcPath := filepath.Join(oldDir, entry.Name())
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}

		dstPath, err := resolveMigratedFilePath(newDir, entry.Name(), suffix)
		if err != nil {
			return movedCount, err
		}
		if err := moveFile(srcPath, dstPath); err != nil {
			return movedCount, err
		}
		movedCount++
	}

	return movedCount, nil
}

func migrateDirContentsWithConflictRename(oldDir, newDir, suffix string, skipNames map[string]struct{}) (int, error) {
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return 0, err
	}

	movedCount := 0
	for _, entry := range entries {
		name := entry.Name()
		if _, skip := skipNames[name]; skip {
			continue
		}

		srcPath := filepath.Join(oldDir, name)
		dstPath := filepath.Join(newDir, name)

		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}

		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return movedCount, err
			}
			childMoved, err := migrateDirContentsWithConflictRename(srcPath, dstPath, suffix, nil)
			if err != nil {
				return movedCount, err
			}
			movedCount += childMoved
			_ = removeDirIfEmpty(srcPath)
			continue
		}

		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}

		finalDst := dstPath
		if _, err := os.Stat(finalDst); err == nil {
			finalDst, err = resolveMigratedFilePath(newDir, name, suffix)
			if err != nil {
				return movedCount, err
			}
		}
		if err := moveFile(srcPath, finalDst); err != nil {
			return movedCount, err
		}
		movedCount++
	}

	return movedCount, nil
}

func moveFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func removeDirIfEmpty(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return nil
	}
	return os.Remove(path)
}

func resolveResourceRootFromEnv() string {
	if envDir := strings.TrimSpace(os.Getenv("HEPHAESTUS_RESOURCE_DIR")); envDir != "" {
		cleanDir := filepath.Clean(envDir)
		if cleanDir != "." {
			return cleanDir
		}
	}

	if envDir := strings.TrimSpace(os.Getenv("HEPHAESTUS_CONFIG_DIR")); envDir != "" {
		cleanDir := filepath.Clean(envDir)
		if cleanDir == "." {
			return ""
		}

		// Allow callers to point either at the resource root itself or at the
		// main config directory whose sibling/child `config` folder stores assets.
		if dirExists(filepath.Join(cleanDir, "pocs")) || dirExists(filepath.Join(cleanDir, "crack")) || dirExists(filepath.Join(cleanDir, "dirsearch")) {
			return cleanDir
		}
		if dirExists(filepath.Join(cleanDir, "config")) {
			return filepath.Join(cleanDir, "config")
		}
	}

	return ""
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
