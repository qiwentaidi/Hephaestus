package webscan

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
// 解析是 CPU/IO 混合型工作，按 CPU 数并发读取，缩短首次冷启动构建时间。
func buildPOCTemplateCache(templateFolders []string) (map[string]Template, []pocTemplateIndexEntry, error) {
	templateFiles, err := collectTemplateFiles(templateFolders)
	if err != nil {
		return nil, nil, err
	}
	if len(templateFiles) == 0 {
		return map[string]Template{}, []pocTemplateIndexEntry{}, nil
	}

	type buildResult struct {
		template Template
		entry    pocTemplateIndexEntry
		ok       bool
	}
	results := make([]buildResult, len(templateFiles))

	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(templateFiles) {
		workers = len(templateFiles)
	}

	logger.Info("开始构建 PoC 模板缓存: %d 个模板, 并发 %d", len(templateFiles), workers)

	jobIndexes := make(chan int)
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for index := range jobIndexes {
				item := templateFiles[index]
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

				results[index] = buildResult{
					template: template,
					entry: pocTemplateIndexEntry{
						Key:       item.key,
						FilePath:  item.path,
						SourceDir: item.sourceDir,
						ModTimeNs: item.modTime.UnixNano(),
						Size:      item.size,
						Template:  template,
					},
					ok: true,
				}
			}
		}()
	}
	for index := range templateFiles {
		jobIndexes <- index
	}
	close(jobIndexes)
	wg.Wait()

	nextDB := make(map[string]Template, len(templateFiles))
	entries := make([]pocTemplateIndexEntry, 0, len(templateFiles))
	for index, item := range templateFiles {
		result := results[index]
		if !result.ok {
			continue
		}
		nextDB[item.key] = result.template
		entries = append(entries, result.entry)
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
	loadStarted := time.Now()
	if loadPOCTemplateIndexFrom(indexPath, folders) {
		mutx.RLock()
		loaded := len(WorkFlowDB)
		mutx.RUnlock()
		setPOCTemplateCacheStatus("ready", "")
		logger.Info("PoC 模板索引加载完成: %d 个模板, 用时 %s", loaded, time.Since(loadStarted).Round(time.Millisecond))
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
		if err := warmPOCTemplateCache(); err != nil {
			logger.Warning("预热 PoC 模板缓存失败: %v", err)
			return
		}
		logger.Info("PoC 模板缓存已就绪，用时 %s", time.Since(started).Round(time.Millisecond))
	}()
}

// warmPOCTemplateCache 是启动预热的严格版本：即使 WorkFlowDB 已被扫描路径
// 提前填充，也会确认索引存在；索引缺失/过期时用标准目录重建并落盘。
// 否则首次启动可能只内存预热成功、索引没写，导致每次启动都重新全量构建。
func warmPOCTemplateCache() error {
	pocCacheMu.Lock()
	defer pocCacheMu.Unlock()

	folders := normalizePOCTemplateFolderInputs(GetPOCTemplateFolders())
	indexPath := defaultPOCTemplateIndexPath()
	loadStarted := time.Now()
	if loadPOCTemplateIndexFrom(indexPath, folders) {
		mutx.RLock()
		loaded := len(WorkFlowDB)
		mutx.RUnlock()
		setPOCTemplateCacheStatus("ready", "")
		logger.Info("PoC 模板索引加载完成: %d 个模板, 用时 %s", loaded, time.Since(loadStarted).Round(time.Millisecond))
		return nil
	}
	if _, err := os.Stat(indexPath); err == nil {
		logger.Info("PoC 模板索引已失效，正在重建")
	}
	return refreshPOCTemplateCacheLocked()
}

// refreshPOCTemplateCacheLocked 重建缓存并写入索引，调用方必须持有 pocCacheMu。
func refreshPOCTemplateCacheLocked() error {
	folders := normalizePOCTemplateFolderInputs(GetPOCTemplateFolders())
	started := time.Now()
	setPOCTemplateCacheStatus("building", "")
	nextDB, entries, err := buildPOCTemplateCache(folders)
	if err != nil {
		setPOCTemplateCacheStatus("failed", err.Error())
		return err
	}
	setPOCTemplateCache(nextDB)

	// 先把索引写到磁盘，再对外宣布 ready：避免遮罩刚消失、用户立即退出时
	// 索引还没落盘，导致下次启动又从头构建一遍。
	if err := writePOCTemplateIndexTo(defaultPOCTemplateIndexPath(), folders, entries); err != nil {
		logger.Warning("写入 PoC 模板索引失败: %v", err)
	}
	setPOCTemplateCacheStatus("ready", "")
	logger.Info("PoC 模板缓存构建完成: %d 个模板, 用时 %s", len(nextDB), time.Since(started).Round(time.Millisecond))
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
