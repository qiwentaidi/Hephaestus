package webscan

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/logger"

	"gopkg.in/yaml.v2"
)

const (
	// pocTemplateIndexVersion 标识索引文件结构。修改索引字段时递增，旧索引会被忽略并重建。
	pocTemplateIndexVersion = 1
	// pocTemplateIndexFileName 是模板索引的落盘文件名，位于配置资源目录下。
	pocTemplateIndexFileName = "poc-template-index.json.gz"
)

// pocCacheMu 串行化索引加载与模板重建，避免启动预热和首个请求同时全量扫描目录。
var pocCacheMu sync.Mutex

// pocCacheStatusTracker 记录模板缓存预热状态，供桌面端启动遮罩轮询。
var pocCacheStatusTracker = struct {
	mu      sync.Mutex
	state   string
	errMsg  string
	started time.Time
}{state: "idle"}

func setPOCTemplateCacheStatus(state, errMsg string) {
	pocCacheStatusTracker.mu.Lock()
	defer pocCacheStatusTracker.mu.Unlock()
	pocCacheStatusTracker.state = state
	pocCacheStatusTracker.errMsg = errMsg
	if state == "building" {
		pocCacheStatusTracker.started = time.Now()
	}
}

// POCTemplateCacheStatus 返回当前缓存状态（idle/building/ready/failed）、错误信息和已用时。
func POCTemplateCacheStatus() (state string, errMsg string, elapsedSeconds int64) {
	pocCacheStatusTracker.mu.Lock()
	defer pocCacheStatusTracker.mu.Unlock()
	elapsed := int64(0)
	if !pocCacheStatusTracker.started.IsZero() {
		elapsed = int64(time.Since(pocCacheStatusTracker.started).Seconds())
	}
	return pocCacheStatusTracker.state, pocCacheStatusTracker.errMsg, elapsed
}

// RetryPOCTemplateCacheWarmup 在构建失败或处于 idle 时重新开始预热；构建中则忽略。
func RetryPOCTemplateCacheWarmup() {
	pocCacheStatusTracker.mu.Lock()
	state := pocCacheStatusTracker.state
	pocCacheStatusTracker.mu.Unlock()
	if state == "building" {
		return
	}
	// 先同步置为 building，避免前端重试后立刻轮询时仍读到旧的 failed 状态。
	setPOCTemplateCacheStatus("building", "")
	StartPOCTemplateCacheWarmup()
}

type pocTemplateIndexEntry struct {
	Key       string   `json:"key"`
	FilePath  string   `json:"filePath"`
	SourceDir string   `json:"sourceDir"`
	ModTimeNs int64    `json:"modTimeNs"`
	Size      int64    `json:"size"`
	Template  Template `json:"template"`
}

type pocTemplateIndex struct {
	Version int                     `json:"version"`
	BuiltAt string                  `json:"builtAt"`
	Folders []string                `json:"folders"`
	Entries []pocTemplateIndexEntry `json:"entries"`
}

func defaultPOCTemplateIndexPath() string {
	return filepath.Join(config.GetConfigResourceRoot(), pocTemplateIndexFileName)
}

// buildPOCTemplateCache 读取并解析所有模板文件，返回内存缓存和可落盘的索引条目。
func buildPOCTemplateCache(templateFolders []string) (map[string]Template, []pocTemplateIndexEntry, error) {
	templateFiles, err := collectTemplateFiles(templateFolders)
	if err != nil {
		return nil, nil, err
	}

	nextDB := make(map[string]Template, len(templateFiles))
	entries := make([]pocTemplateIndexEntry, 0, len(templateFiles))
	for _, item := range templateFiles {
		content, err := os.ReadFile(item.path)
		if err != nil {
			continue
		}
		var template Template
		if err := yaml.Unmarshal(content, &template); err != nil {
			continue
		}

		// yaml.v2 会把嵌套 map 解析成 map[interface{}]interface{}，JSON 无法编码；
		// 统一转换成字符串键，保证索引和内存缓存结构一致。
		template.Info.Metadata = normalizeTemplateMetadata(template.Info.Metadata)
		if !item.modTime.IsZero() {
			template.ModifiedTime = item.modTime.Format("2006-01-02")
		}
		template.FilePath = item.path
		template.RawContent = string(content)

		nextDB[item.key] = template
		entries = append(entries, pocTemplateIndexEntry{
			Key:       item.key,
			FilePath:  item.path,
			SourceDir: item.sourceDir,
			ModTimeNs: item.modTime.UnixNano(),
			Size:      item.size,
			Template:  template,
		})
	}

	return nextDB, entries, nil
}

func setPOCTemplateCache(next map[string]Template) {
	mutx.Lock()
	WorkFlowDB = next
	mutx.Unlock()
}

// EnsurePOCTemplateCache 保证模板缓存可用：优先加载持久化索引，索引缺失或过期时回退到一次完整构建。
func EnsurePOCTemplateCache() error {
	pocCacheMu.Lock()
	defer pocCacheMu.Unlock()

	if WorkFlowDB != nil {
		setPOCTemplateCacheStatus("ready", "")
		return nil
	}

	folders := normalizePOCTemplateFolderInputs(GetPOCTemplateFolders())
	indexPath := defaultPOCTemplateIndexPath()
	if loadPOCTemplateIndexFrom(indexPath, folders) {
		setPOCTemplateCacheStatus("ready", "")
		return nil
	}
	if _, err := os.Stat(indexPath); err == nil {
		logger.Info("PoC 模板索引已失效，正在重建")
	}

	return refreshPOCTemplateCacheLocked()
}

// StartPOCTemplateCacheWarmup 在后台预热模板缓存，避免阻塞窗口创建。
func StartPOCTemplateCacheWarmup() {
	go func() {
		started := time.Now()
		if err := EnsurePOCTemplateCache(); err != nil {
			logger.Warning("预热 PoC 模板缓存失败: %v", err)
			return
		}
		logger.Info("PoC 模板缓存已就绪，用时 %s", time.Since(started).Round(time.Millisecond))
	}()
}

// refreshPOCTemplateCacheLocked 重建缓存并写入索引，调用方必须持有 pocCacheMu。
func refreshPOCTemplateCacheLocked() error {
	folders := normalizePOCTemplateFolderInputs(GetPOCTemplateFolders())
	setPOCTemplateCacheStatus("building", "")
	nextDB, entries, err := buildPOCTemplateCache(folders)
	if err != nil {
		setPOCTemplateCacheStatus("failed", err.Error())
		return err
	}
	setPOCTemplateCache(nextDB)
	setPOCTemplateCacheStatus("ready", "")

	if err := writePOCTemplateIndexTo(defaultPOCTemplateIndexPath(), folders, entries); err != nil {
		logger.Warning("写入 PoC 模板索引失败: %v", err)
	}
	return nil
}

func loadPOCTemplateIndexFrom(path string, folders []string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	reader, err := gzip.NewReader(file)
	if err != nil {
		return false
	}
	defer reader.Close()

	var payload pocTemplateIndex
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return false
	}
	if payload.Version != pocTemplateIndexVersion {
		return false
	}
	if !sameTemplateFolders(payload.Folders, folders) {
		return false
	}

	nextDB := make(map[string]Template, len(payload.Entries))
	for _, entry := range payload.Entries {
		info, err := os.Stat(entry.FilePath)
		if err != nil || info.IsDir() {
			return false
		}
		if info.Size() != entry.Size || info.ModTime().UnixNano() != entry.ModTimeNs {
			return false
		}
		nextDB[entry.Key] = entry.Template
	}

	setPOCTemplateCache(nextDB)
	return true
}

func writePOCTemplateIndexTo(path string, folders []string, entries []pocTemplateIndexEntry) error {
	payload := pocTemplateIndex{
		Version: pocTemplateIndexVersion,
		BuiltAt: time.Now().UTC().Format(time.RFC3339),
		Folders: append([]string(nil), folders...),
		Entries: entries,
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	temp, err := os.CreateTemp(dir, ".poc-index-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := func() {
		_ = os.Remove(tempPath)
	}

	writer := gzip.NewWriter(temp)
	if err := json.NewEncoder(writer).Encode(&payload); err != nil {
		_ = writer.Close()
		_ = temp.Close()
		cleanup()
		return err
	}
	if err := writer.Close(); err != nil {
		_ = temp.Close()
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

func sameTemplateFolders(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	left := append([]string(nil), a...)
	right := append([]string(nil), b...)
	sort.Strings(left)
	sort.Strings(right)
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func normalizeTemplateMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	normalized := make(map[string]any, len(metadata))
	for key, value := range metadata {
		normalized[key] = normalizeTemplateMetadataValue(value)
	}
	return normalized
}

func normalizeTemplateMetadataValue(value any) any {
	switch typed := value.(type) {
	case map[any]any:
		normalized := make(map[string]any, len(typed))
		for key, item := range typed {
			normalized[fmt.Sprint(key)] = normalizeTemplateMetadataValue(item)
		}
		return normalized
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, item := range typed {
			normalized[key] = normalizeTemplateMetadataValue(item)
		}
		return normalized
	case []any:
		normalized := make([]any, len(typed))
		for i, item := range typed {
			normalized[i] = normalizeTemplateMetadataValue(item)
		}
		return normalized
	default:
		return value
	}
}
