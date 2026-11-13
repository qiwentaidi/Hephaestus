package webscan

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"hephaestus-backend/pkg/database"

	"gopkg.in/yaml.v2"
)

// poc文件名与poc内容的关联
var WorkFlowDB map[string]Template

type TemplateInfo struct {
	Name           string         `yaml:"name"`
	Author         string         `yaml:"author"`
	Severity       string         `yaml:"severity"`
	Description    string         `yaml:"description"`
	Classification Classification `yaml:"classification"`
	Metadata       map[string]any `yaml:"metadata"`
	Tags           string         `yaml:"tags"`
}

type Classification struct {
	CvssMetrics string `yaml:"cvss-metrics"`
	CvssScore   string `yaml:"cvss-score"`
	CweID       string `yaml:"cwe-id"`
}

type Template struct {
	Id           string       `yaml:"id"`
	Info         TemplateInfo `yaml:"info"`
	ModifiedTime string       `yaml:"modified_time"`
	FilePath     string       `yaml:"-"` // 使用 `yaml:"-"` 避免 YAML 解析时影响
	RawContent   string       `yaml:"-"` // 缓存原始 YAML 内容，供全文检索使用
}

type pocTemplateFile struct {
	baseKey    string
	key        string
	path       string
	sourceDir  string
	sourceRank int
	modTime    time.Time
	size       int64
}

func normalizeTemplateTimeValue(raw any) string {
	switch value := raw.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "<nil>" {
			return ""
		}
		return text
	}
}

func templateMetadataTime(metadata map[string]any, keys ...string) string {
	if metadata == nil {
		return ""
	}
	for _, key := range keys {
		if value := normalizeTemplateTimeValue(metadata[key]); value != "" {
			return value
		}
	}
	return ""
}

func (template Template) CreatedTime() string {
	return templateMetadataTime(template.Info.Metadata, "create-at", "created-at", "create_at", "created_at")
}

func (template Template) UpdatedTime() string {
	return templateMetadataTime(template.Info.Metadata, "update-at", "updated-at", "update_at", "updated_at")
}

var mutx sync.RWMutex

// RefreshPOCTemplateCache 重新加载当前 PoC 目录下的模板缓存，并把解析结果写入持久化索引。
func RefreshPOCTemplateCache() error {
	pocCacheMu.Lock()
	defer pocCacheMu.Unlock()
	return refreshPOCTemplateCacheLocked()
}

// GetTagsList 使用指定目录列表重建模板缓存。任务级的追加目录通过这里进入缓存，
// 因此这里不写持久化索引（索引只对应标准目录集合）。
func GetTagsList(templateFolders []string) error {
	pocCacheMu.Lock()
	defer pocCacheMu.Unlock()

	nextDB, _, err := buildPOCTemplateCache(templateFolders)
	if err != nil {
		return err
	}
	setPOCTemplateCache(nextDB)
	return nil
}

func collectTemplateFiles(templateFolders []string) ([]pocTemplateFile, error) {
	folders := normalizePOCTemplateFolderInputs(templateFolders)
	files := make([]pocTemplateFile, 0)
	for sourceRank, folder := range folders {
		if _, err := os.Stat(folder); os.IsNotExist(err) {
			continue
		}
		sourceDir := folder
		if abs, err := filepath.Abs(folder); err == nil {
			sourceDir = filepath.Clean(abs)
		}
		if err := filepath.WalkDir(folder, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !isSupportedPOCTemplateFile(d.Name()) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}

			cleanPath := filepath.Clean(path)
			if abs, err := filepath.Abs(cleanPath); err == nil {
				cleanPath = filepath.Clean(abs)
			}
			baseKey := strings.TrimSuffix(strings.TrimSuffix(d.Name(), ".yaml"), ".yml")
			files = append(files, pocTemplateFile{
				baseKey:    baseKey,
				path:       cleanPath,
				sourceDir:  sourceDir,
				sourceRank: sourceRank,
				modTime:    info.ModTime(),
				size:       info.Size(),
			})
			return nil
		}); err != nil {
			return nil, err
		}
	}

	assignTemplateKeys(files)
	sort.Slice(files, func(i, j int) bool {
		if files[i].sourceRank == files[j].sourceRank {
			return files[i].path < files[j].path
		}
		return files[i].sourceRank < files[j].sourceRank
	})
	return files, nil
}

func isSupportedPOCTemplateFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}

func assignTemplateKeys(files []pocTemplateFile) {
	used := make(map[string]int, len(files))
	for i := range files {
		key := files[i].baseKey
		if used[key] > 0 {
			key = uniqueTemplateKey(files[i])
		}
		if count := used[key]; count > 0 {
			key = fmt.Sprintf("%s-%d", key, count+1)
		}
		files[i].key = key
		used[key]++
	}
}

func uniqueTemplateKey(item pocTemplateFile) string {
	sourceName := filepath.Base(item.sourceDir)
	if sourceName == "." || sourceName == string(os.PathSeparator) || sourceName == "" {
		sourceName = "custom"
	}
	rel, err := filepath.Rel(item.sourceDir, item.path)
	if err != nil {
		rel = filepath.Base(item.path)
	}
	rel = strings.TrimSuffix(strings.TrimSuffix(rel, ".yaml"), ".yml")
	parts := []string{sourceName}
	for _, part := range strings.FieldsFunc(rel, func(r rune) bool {
		return r == '/' || r == '\\' || r == os.PathSeparator
	}) {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return sanitizeTemplateKey(strings.Join(parts, "__"))
}

func sanitizeTemplateKey(raw string) string {
	key := strings.TrimSpace(raw)
	key = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, key)
	key = strings.Trim(key, "-_.")
	if key == "" {
		return "template"
	}
	return key
}

func normalizePOCTemplateFolderInputs(paths []string) []string {
	result := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, raw := range paths {
		normalized := normalizePOCTemplateFolderPath(raw)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func normalizePOCTemplateFolderPath(raw string) string {
	path := strings.TrimSpace(raw)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			if path == "~" {
				path = home
			} else if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
				path = filepath.Join(home, path[2:])
			}
		}
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func GetPOCTemplateFolders() []string {
	folders := []string{GetPoCDir()}
	additional, err := GetAdditionalPOCTemplateFolders()
	if err != nil {
		return folders
	}
	folders = append(folders, additional...)
	return normalizePOCTemplateFolderInputs(folders)
}

func GetAdditionalPOCTemplateFolders() ([]string, error) {
	raw, err := database.GetSetting(database.SettingKeyPOCTemplateFolders)
	if err != nil || raw == nil {
		return nil, err
	}

	switch value := raw.(type) {
	case []interface{}:
		folders := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				folders = append(folders, text)
			}
		}
		return normalizePOCTemplateFolderInputs(folders), nil
	case []string:
		return normalizePOCTemplateFolderInputs(value), nil
	case map[string]interface{}:
		if foldersRaw, ok := value["folders"].([]interface{}); ok {
			folders := make([]string, 0, len(foldersRaw))
			for _, item := range foldersRaw {
				if text, ok := item.(string); ok {
					folders = append(folders, text)
				}
			}
			return normalizePOCTemplateFolderInputs(folders), nil
		}
	}
	return nil, nil
}

func SetAdditionalPOCTemplateFolders(paths []string) ([]string, error) {
	folders := normalizePOCTemplateFolderInputs(paths)
	baseDir := normalizePOCTemplateFolderPath(GetPoCDir())
	result := make([]string, 0, len(folders))
	for _, folder := range folders {
		if folder == baseDir {
			continue
		}
		info, err := os.Stat(folder)
		if err != nil {
			return nil, fmt.Errorf("模板目录不可访问: %s", folder)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("模板路径不是目录: %s", folder)
		}
		result = append(result, folder)
	}
	if err := database.SetSetting(database.SettingKeyPOCTemplateFolders, result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetTemplateList 获取所有模板 ID 列表（线程安全）
func GetTemplateList() []string {
	_ = EnsurePOCTemplateCache()
	mutx.RLock()
	defer mutx.RUnlock()

	ids := make([]string, 0, len(WorkFlowDB))
	for id := range WorkFlowDB {
		ids = append(ids, id)
	}
	return ids
}

// GetTemplateMapCopy 获取模板映射副本（线程安全）
func GetTemplateMapCopy() map[string]Template {
	_ = EnsurePOCTemplateCache()
	mutx.RLock()
	defer mutx.RUnlock()

	result := make(map[string]Template, len(WorkFlowDB))
	maps.Copy(result, WorkFlowDB)
	return result
}

func normalizePOCKey(key string) (string, error) {
	normalized := strings.TrimSpace(key)
	normalized = strings.TrimSuffix(strings.TrimSuffix(normalized, ".yaml"), ".yml")
	if normalized == "" {
		return "", errors.New("模板 ID 不能为空")
	}
	if strings.Contains(normalized, "/") || strings.Contains(normalized, `\`) || normalized == "." || normalized == ".." {
		return "", errors.New("模板 ID 非法")
	}
	return normalized, nil
}

func resolvePOCFilePath(key string) (string, error) {
	normalized, err := normalizePOCKey(key)
	if err != nil {
		return "", err
	}

	if template, ok := getTemplateByKey(normalized); ok && strings.TrimSpace(template.FilePath) != "" {
		return filepath.Clean(template.FilePath), nil
	}

	baseAbs, err := filepath.Abs(GetPoCDir())
	if err != nil {
		return "", err
	}

	targetAbs, err := filepath.Abs(filepath.Join(baseAbs, normalized+".yaml"))
	if err != nil {
		return "", err
	}

	basePrefix := baseAbs + string(os.PathSeparator)
	if targetAbs != baseAbs && !strings.HasPrefix(targetAbs, basePrefix) {
		return "", errors.New("模板路径非法")
	}
	return targetAbs, nil
}

func getTemplateByKey(key string) (Template, bool) {
	_ = EnsurePOCTemplateCache()
	mutx.RLock()
	defer mutx.RUnlock()
	template, ok := WorkFlowDB[key]
	return template, ok
}

func ResolveTemplateSelection(selection string) string {
	key, err := normalizePOCKey(selection)
	if err == nil {
		if template, ok := getTemplateByKey(key); ok && strings.TrimSpace(template.FilePath) != "" {
			return template.FilePath
		}
	}
	return selection
}

// ReadPOCContent 读取 PoC 文件内容
func ReadPOCContent(key string) (string, error) {
	filePath, err := resolvePOCFilePath(key)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// SavePOCContent 保存 PoC 文件内容，并刷新 WorkFlowDB
func SavePOCContent(key, content string) error {
	filePath, err := resolvePOCFilePath(key)
	if err != nil {
		return err
	}

	if strings.TrimSpace(content) == "" {
		return errors.New("模板内容不能为空")
	}

	var template Template
	if err := yaml.Unmarshal([]byte(content), &template); err != nil {
		return err
	}
	if strings.TrimSpace(template.Id) == "" {
		return errors.New("模板内容缺少 id 字段")
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return err
	}
	if err := SignUpdatedCodeTemplatesForNuclei(filePath); err != nil {
		return err
	}

	return RefreshPOCTemplateCache()
}

// DeletePOC 删除 PoC 文件，并刷新 WorkFlowDB
func DeletePOC(key string) error {
	filePath, err := resolvePOCFilePath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(filePath); err != nil {
		return err
	}
	return RefreshPOCTemplateCache()
}
