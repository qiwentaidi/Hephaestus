package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/logger"
)

// applyEmbeddedDirsearchDicts is kept as a no-op hook for CLI commands. The
// acceptable CLI path uses normal disk resources only; embedded resources are
// intentionally not wired through pkg/config.
func applyEmbeddedDirsearchDicts(_ map[string]interface{}) {}

// cliRemoteDB CLI 远程数据库参数（结果汇总到服务端/桌面端）
type cliRemoteDB struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func (r cliRemoteDB) enabled() bool {
	return strings.TrimSpace(r.Host) != ""
}

// cliGlobalFlags 所有 CLI 子命令共享的通用参数
type cliGlobalFlags struct {
	remote    cliRemoteDB
	serverURL string
	keepDB    bool
}

// cliExecutableDir 返回当前二进制所在目录（解析符号链接）
func cliExecutableDir() string {
	execPath, err := os.Executable()
	if err != nil || execPath == "" {
		return ""
	}
	if resolved, resolveErr := filepath.EvalSymlinks(execPath); resolveErr == nil {
		execPath = resolved
	}
	return filepath.Dir(execPath)
}

// initCLIRuntime 初始化 CLI 扫描运行时：
//   - 使用 desktop 运行模式（同时允许 SQLite 与 PostgreSQL）
//   - 默认在临时目录创建独立 SQLite 库，扫描结束清理
//   - 指定 --db-host 等参数时连接远程 PostgreSQL，结果直接写入服务端库
//   - 资源目录优先使用二进制同级 config/，其次复用用户目录资源
func initCLIRuntime(global cliGlobalFlags) (func(), error) {
	config.SetRuntimeMode(config.RuntimeModeDesktop)

	tmpDir, err := os.MkdirTemp("", "hephaestus-cli-")
	if err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}
	if err := os.Setenv("HEPHAESTUS_CONFIG_DIR", tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("设置配置目录失败: %w", err)
	}
	config.ForceSetConfigBaseDir(tmpDir)

	resourceDir := cliResolveDiskResourceDir()
	if resourceDir != "" {
		_ = os.Setenv("HEPHAESTUS_RESOURCE_DIR", resourceDir)
		logger.Info("[cli] 使用资源目录: %s", resourceDir)
	} else {
		logger.Warning("[cli] 未找到可用资源（二进制同级 config/、~/.config/hephaestus/config 均不可用），PoC 模板/指纹库可能不可用（可先执行 scan-agent deploy）")
	}

	var dbYAML string
	if global.remote.enabled() {
		sslmode := strings.TrimSpace(global.remote.SSLMode)
		if sslmode == "" {
			sslmode = "disable"
		}
		dbYAML = fmt.Sprintf(
			"database:\n  type: postgres\n  host: %s\n  port: %s\n  user: %s\n  password: %s\n  dbname: %s\n  sslmode: %s\n",
			strings.TrimSpace(global.remote.Host),
			strings.TrimSpace(global.remote.Port),
			strings.TrimSpace(global.remote.User),
			global.remote.Password,
			strings.TrimSpace(global.remote.DBName),
			sslmode,
		)
		logger.Info("[cli] 远程库模式: %s:%s/%s（结果将汇总到服务端，Web 端可见）",
			strings.TrimSpace(global.remote.Host), strings.TrimSpace(global.remote.Port), strings.TrimSpace(global.remote.DBName))
	} else {
		dbPath := filepath.ToSlash(filepath.Join(tmpDir, "cli.db"))
		dbYAML = fmt.Sprintf("database:\n  type: sqlite\n  path: %s\n", dbPath)
		logger.Info("[cli] 本地临时库模式: %s", dbPath)
	}

	configPath := filepath.Join(tmpDir, "config-desktop.yaml")
	if err := os.WriteFile(configPath, []byte(dbYAML), 0600); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("写入临时配置失败: %w", err)
	}

	if _, err := database.GetDB(); err != nil {
		os.RemoveAll(tmpDir)
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			return nil, fmt.Errorf("本二进制为纯 Go 编译，不支持本地 SQLite 模式。请通过 -db-host/-db-port/-db-user/-db-pass/-db-name 指定远程 PostgreSQL（结果将同步到服务端 Web 端）")
		}
		return nil, fmt.Errorf("数据库初始化失败: %w", err)
	}

	cleanup := func() {
		if global.keepDB {
			logger.Info("[cli] 保留临时目录: %s", tmpDir)
			return
		}
		if sqlDB, dbErr := database.GetSQLDB(); dbErr == nil && sqlDB != nil {
			_ = sqlDB.Close()
		}
		_ = os.RemoveAll(tmpDir)
	}
	return cleanup, nil
}

func cliResolveDiskResourceDir() string {
	if execDir := cliExecutableDir(); execDir != "" {
		sibling := filepath.Join(execDir, "config")
		if hasCLIResourceSubdir(sibling) {
			return sibling
		}
	}
	if homeDir, err := os.UserHomeDir(); err == nil && strings.TrimSpace(homeDir) != "" {
		userResourceDir := filepath.Join(homeDir, ".config", "hephaestus", "config")
		if hasCLIResourceSubdir(userResourceDir) {
			return userResourceDir
		}
	}
	return ""
}

func hasCLIResourceSubdir(root string) bool {
	for _, sub := range []string{"pocs", "crack", "dirsearch"} {
		if info, err := os.Stat(filepath.Join(root, sub)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// readLinesFile 按行读取目标文件，跳过空行与注释行
func readLinesFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败 %s: %w", path, err)
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("文件 %s 中没有有效目标", path)
	}
	return lines, nil
}

// splitComma 逗号分隔转切片（跳过空项）
func splitComma(raw string) []string {
	var out []string
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
