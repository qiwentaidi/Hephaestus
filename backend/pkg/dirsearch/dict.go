package dirsearch

import (
	"bufio"
	_ "embed"
	"os"
	"path/filepath"
	"strings"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/logger"

	arrayutil "github.com/qiwentaidi/utils/array"
)

// DefaultDirsearchDictName 平台内置默认目录字典文件名。
// 该字典约 850 行，覆盖现代 Web 栈（API/SPA/微服务/云原生）的高价值路径。
const DefaultDirsearchDictName = "common-modern.txt"

//go:embed assets/common-modern.txt
var embeddedDefaultDict string

// DirsearchDictDir 返回托管目录扫描字典目录（config/dirsearch）。
func DirsearchDictDir() string {
	return filepath.Join(config.GetConfigResourceRoot(), "dirsearch")
}

// LoadDefaultDirsearchDict 加载默认目录字典并展开 %EXT% 占位符。
// 优先使用托管目录中的 common-modern.txt（用户可在字典管理中编辑），
// 文件缺失时回退到编译进二进制的内置版本，保证默认字典永远可用。
func LoadDefaultDirsearchDict(newExts []string) []string {
	if file, err := config.ResolveManagedDictFile("dirsearch", DefaultDirsearchDictName); err == nil {
		if dict := LoadDirDict(file.Path, "%EXT%", newExts); len(dict) > 0 {
			return arrayutil.RemoveDuplicates(dict)
		}
		logger.Warning("默认目录字典 %s 加载为空，回退到内置字典", file.Path)
	}
	return parseDictContent(embeddedDefaultDict, "%EXT%", newExts)
}

// ResolveDirsearchDictPaths 将托管字典名解析为字典文件绝对路径。
// 名称可带或不带 .txt 后缀；仅允许文件名，拒绝路径分隔符与 .. 以防止目录穿越。
// 返回解析成功的路径列表与未命中的名称列表。
func ResolveDirsearchDictPaths(names []string) (paths []string, missing []string) {
	for _, name := range names {
		cleaned := strings.TrimSpace(name)
		if cleaned == "" {
			continue
		}
		base := filepath.Base(cleaned)
		if base != cleaned || base == "." || base == ".." || strings.Contains(base, "..") {
			missing = append(missing, cleaned)
			continue
		}
		if !strings.HasSuffix(strings.ToLower(base), ".txt") {
			base += ".txt"
		}
		if file, err := config.ResolveManagedDictFile("dirsearch", base); err == nil {
			paths = append(paths, file.Path)
		} else {
			missing = append(missing, cleaned)
		}
	}
	return paths, missing
}

// parseDictContent 从字符串内容解析字典条目，与 LoadDirDict 的占位符规则一致。
func parseDictContent(content, old string, new []string) (dict []string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(new) > 0 {
			if strings.Contains(line, old) {
				for _, n := range new {
					dict = append(dict, strings.ReplaceAll(line, old, n))
				}
			} else {
				dict = append(dict, line)
			}
		} else if !strings.Contains(line, old) {
			dict = append(dict, line)
		}
	}
	return arrayutil.RemoveDuplicates(dict)
}

// LoadDirsearchDict 加载目录字典
// dictPath: 字典文件路径列表
// newExts: 扩展名列表，用于替换 %EXT% 占位符
func LoadDirsearchDict(dictPath, newExts []string) []string {
	var dicts []string

	for _, dict := range dictPath {
		dicts = append(dicts, LoadDirDict(dict, "%EXT%", newExts)...)
	}

	return arrayutil.RemoveDuplicates(dicts)
}

// LoadDirDict 从文件加载字典
// filepath: 文件路径
// old: 要替换的占位符（如 %EXT%）
// new: 替换后的扩展名列表
func LoadDirDict(filepath, old string, new []string) (dict []string) {
	file, err := os.Open(filepath)
	if err != nil {
		return dict
	}
	defer file.Close()

	s := bufio.NewScanner(file)

	for s.Scan() {
		line := s.Text()
		if line != "" { // 去除空行
			if len(new) > 0 {
				if strings.Contains(line, old) { // 如果新数组不为空,将old字段替换成new数组
					for _, n := range new {
						dict = append(dict, strings.ReplaceAll(line, old, n))
					}
				} else {
					dict = append(dict, line)
				}
			} else {
				if !strings.Contains(line, old) {
					dict = append(dict, line)
				}
			}
		}
	}

	return dict
}
