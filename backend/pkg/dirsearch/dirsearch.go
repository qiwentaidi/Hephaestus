package dirsearch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hephaestus-backend/pkg/logger"

	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qiwentaidi/clients"
	arrayutil "github.com/qiwentaidi/utils/array"
	httputil "github.com/qiwentaidi/utils/http"

	"github.com/go-resty/resty/v2"
	"github.com/panjf2000/ants/v2"
)

// CallbackFunc 回调函数类型，用于替代 Wails runtime.EventsEmit
type CallbackFunc func(event string, data interface{})

var maxReponseLength = 1024 * 100 // 最大响应包长度，100kb
const progressEmitStep = 25

type Dirsearch struct {
	ctx           context.Context
	options       Options
	bodyLengthMap map[int]int
	mutex         sync.Mutex
	client        *resty.Client
	headers       map[string]string
	callback      CallbackFunc // 回调函数，用于发送事件
}

type Result struct {
	Status      int
	URL         string
	Title       string
	Location    string
	ContentType string
	Length      int
	Body        string
	Recursion   int
}

type Options struct {
	Method                 string
	URLs                   []string
	Paths                  []string
	Workers                int
	Timeout                int
	BodyExclude            string
	BodyLengthExcludeTimes int // 过滤响应包长度数据次数出现了多少次，就过滤相同的
	StatusCodeExclude      []int
	Redirect               bool
	Interval               int
	CustomHeader           string
	Recursion              int
	Backupscan             bool
	Proxy                  string // 代理地址
}

// NewDirsearchEngine 创建目录扫描引擎
// ctx: 上下文
// o: 扫描选项
// callback: 回调函数，用于发送事件（dirsearchCounts, dirsearchLoading, dirsearchProgressID, dirsearchComplete）
func NewDirsearchEngine(ctx context.Context, o Options, callback CallbackFunc) *Dirsearch {
	headers := clients.Str2HeadersMap(o.CustomHeader)

	return &Dirsearch{
		ctx:           ctx,
		options:       o,
		bodyLengthMap: make(map[int]int),
		client:        clients.NewRestyClientWithProxy(nil, o.Redirect, o.Proxy),
		headers:       headers,
		callback:      callback,
	}
}

// Run executes the configured directory scan and, when requested, follows it
// with the built-in backup-file scan. Backupscan is an additional phase: it
// must not suppress the normal dictionary/custom-path scan.
func (s *Dirsearch) Run(ctrlCtx context.Context) {
	s.Runner(ctrlCtx)
	if s.options.Backupscan && ctrlCtx.Err() == nil {
		s.BackupRunner(ctrlCtx)
	}
}

func (s *Dirsearch) Runner(ctrlCtx context.Context) {
	// 初始化请求信息
	if s.options.Timeout == 0 {
		s.options.Timeout = 8
	}
	reachableURLs, skippedTargets := s.reachableURLs(ctrlCtx)

	totalCount := len(reachableURLs) * len(s.options.Paths)
	if s.callback != nil {
		s.callback("dirsearchCounts", totalCount)
	}
	var completedCount int32
	var errorCount int32 = int32(skippedTargets)
	single := make(chan struct{})
	retChan := make(chan Result)
	var wg sync.WaitGroup

	// 定时推送进度的 ticker（每5秒）
	progressTicker := time.NewTicker(5 * time.Second)
	defer progressTicker.Stop()

	// 进度推送 goroutine
	go func() {
		for {
			select {
			case <-progressTicker.C:
				// 每5秒推送一次进度
				if s.callback != nil {
					completed := int(atomic.LoadInt32(&completedCount))
					errors := int(atomic.LoadInt32(&errorCount))
					s.callback("dirsearchProgress", map[string]interface{}{
						"completed": completed,
						"total":     totalCount,
						"errors":    errors,
					})
				}
			case <-single:
				// 扫描完成，退出
				return
			case <-ctrlCtx.Done():
				// 上下文取消，退出
				return
			}
		}
	}()

	// 处理结果的 goroutine（只推送有效响应）
	go func() {
		for pr := range retChan {
			pr.Recursion = s.options.Recursion
			// 只推送有效的响应（Status > 1，排除 Status = 0 和 Status = 1）
			if pr.Status > 1 {
				if s.callback != nil {
					s.callback("dirsearchLoading", pr)
				}
			} else if pr.Status == 0 {
				// Status = 0 表示请求失败，增加错误计数
				atomic.AddInt32(&errorCount, 1)
			}
			// Status = 1 表示被过滤，不做任何处理
		}
		close(single)

		// 扫描完成，推送最终进度和完成事件
		if s.callback != nil {
			completed := int(atomic.LoadInt32(&completedCount))
			errors := int(atomic.LoadInt32(&errorCount))
			s.callback("dirsearchProgress", map[string]interface{}{
				"completed": completed,
				"total":     totalCount,
				"errors":    errors,
			})
			s.callback("dirsearchComplete", "done")
		}
	}()

	dirScan := func(url string) {
		if ctrlCtx.Err() != nil {
			completed := atomic.AddInt32(&completedCount, 1)
			if s.callback != nil && completed%progressEmitStep == 0 {
				errors := int(atomic.LoadInt32(&errorCount))
				s.callback("dirsearchProgress", map[string]interface{}{
					"completed": int(completed),
					"total":     totalCount,
					"errors":    errors,
				})
			}
			retChan <- Result{Status: 0, URL: url}
			return
		}
		r := s.Scan(s.ctx, url)
		completed := atomic.AddInt32(&completedCount, 1)
		if s.callback != nil && completed%progressEmitStep == 0 {
			errors := int(atomic.LoadInt32(&errorCount))
			s.callback("dirsearchProgress", map[string]interface{}{
				"completed": int(completed),
				"total":     totalCount,
				"errors":    errors,
			})
		}
		retChan <- r
	}

	threadPool, _ := ants.NewPoolWithFunc(s.options.Workers, func(p interface{}) {
		if ctrlCtx.Err() != nil {
			wg.Done()
			return
		}
		path := p.(string)
		dirScan(path)
		wg.Done()
	})
	defer threadPool.Release()
	canceled := false
	for _, url := range reachableURLs {
		for _, path := range s.options.Paths {
			if ctrlCtx.Err() != nil {
				canceled = true
				break
			}
			path = httputil.PrettyPath(path)
			wg.Add(1)
			threadPool.Invoke(url + path)
			if s.options.Interval != 0 {
				time.Sleep(time.Second * time.Duration(s.options.Interval))
			}
		}
		if canceled {
			break
		}
	}
	wg.Wait()
	close(retChan)
	<-single
}

func (s *Dirsearch) reachableURLs(ctx context.Context) ([]string, int) {
	reachable := make([]string, 0, len(s.options.URLs))
	skipped := 0
	for _, rawURL := range s.options.URLs {
		if ctx.Err() != nil {
			break
		}
		url := httputil.PrettyURL(rawURL)
		if err := s.probeURL(ctx, url); err != nil {
			skipped++
			logger.Warning("[dirsearch] 目标不可访问，跳过目录扫描: %s, error: %v", url, err)
			continue
		}
		reachable = append(reachable, url)
	}
	return reachable, skipped
}

func (s *Dirsearch) probeURL(ctx context.Context, url string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	client := s.client.Clone()
	if s.options.Timeout > 0 {
		client.SetTimeout(time.Duration(s.options.Timeout) * time.Second)
	}

	req := client.R().SetContext(ctx)
	if s.headers != nil {
		req.SetHeaders(s.headers)
	}

	resp, err := req.Get(url)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return err
		}
		return err
	}
	if resp == nil {
		return fmt.Errorf("empty response")
	}
	return nil
}

// status 1 表示被排除显示在外，不计入前端ERROR请求中
func (s *Dirsearch) Scan(ctx context.Context, url string) Result {
	var result Result
	result.URL = url
	if ctx == nil {
		ctx = context.Background()
	}

	client := s.client.Clone()
	if s.options.Timeout > 0 {
		client.SetTimeout(time.Duration(s.options.Timeout) * time.Second)
	}

	req := client.R().SetContext(ctx)
	if s.headers != nil {
		req.SetHeaders(s.headers)
	}

	resp, err := req.Execute(s.options.Method, url)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return result
		}
		logger.Error("[dirsearch] 请求失败: %v", err)
		return result
	}
	if resp == nil {
		return result
	}
	if s.options.BodyExclude != "" && bytes.Contains(resp.Body(), []byte(s.options.BodyExclude)) {
		result.Status = 1
		return result
	} else {
		result.Status = resp.StatusCode()
	}
	if arrayutil.ArrayContains(result.Status, s.options.StatusCodeExclude) {
		result.Status = 1
		return result
	}
	result.Length = len(resp.Body())
	// 记录同一状态码下长度出现次数，当次数超过o.BodyLengthExcludeTimes时，将状态码设置为1，过滤显示
	s.mutex.Lock()
	s.bodyLengthMap[result.Length]++
	if s.bodyLengthMap[result.Length] > s.options.BodyLengthExcludeTimes {
		result.Status = 1
	}
	s.mutex.Unlock()
	result.Body = httputil.LimitResponse(string(resp.Body()), maxReponseLength, "响应包过大, 请自行打开连接查看")
	result.Location = resp.Header().Get("Location")
	result.ContentType = resp.Header().Get("Content-Type")
	result.Title = clients.GetTitle(resp.Body())
	return result
}

type ScanTask struct {
	URL  string
	Path string
}

func (s *Dirsearch) BackupRunner(ctrlCtx context.Context) {
	var tasks []ScanTask
	if s.options.Timeout == 0 {
		s.options.Timeout = 8
	}
	reachableURLs, skippedTargets := s.reachableURLs(ctrlCtx)
	for _, url := range reachableURLs {
		paths := generateBackupPaths(url)
		for _, path := range paths {
			prettyPath := httputil.PrettyPath(path)
			tasks = append(tasks, ScanTask{
				URL:  url,
				Path: prettyPath,
			})
		}
	}

	totalCount := len(tasks)
	if s.callback != nil {
		s.callback("dirsearchCounts", totalCount)
	}

	var completedCount int32
	var errorCount int32 = int32(skippedTargets)
	var wg sync.WaitGroup
	retChan := make(chan Result)
	single := make(chan struct{})

	// 定时推送进度的 ticker（每5秒）
	progressTicker := time.NewTicker(5 * time.Second)
	defer progressTicker.Stop()

	// 进度推送 goroutine
	go func() {
		for {
			select {
			case <-progressTicker.C:
				// 每5秒推送一次进度
				if s.callback != nil {
					completed := int(atomic.LoadInt32(&completedCount))
					errors := int(atomic.LoadInt32(&errorCount))
					s.callback("dirsearchProgress", map[string]interface{}{
						"completed": completed,
						"total":     totalCount,
						"errors":    errors,
					})
				}
			case <-single:
				// 扫描完成，退出
				return
			case <-ctrlCtx.Done():
				// 上下文取消，退出
				return
			}
		}
	}()

	go func() {
		for pr := range retChan {
			// 备份文件扫描：排除 HTML、图片、文本等常见内容类型
			if strings.Contains(pr.ContentType, "html") ||
				strings.Contains(pr.ContentType, "image") ||
				strings.Contains(pr.ContentType, "xml") ||
				strings.Contains(pr.ContentType, "text") ||
				strings.Contains(pr.ContentType, "json") ||
				strings.Contains(pr.ContentType, "javascript") {
				continue
			}
			pr.Recursion = s.options.Recursion
			// 只推送有效的响应（Status > 1）
			if pr.Status > 1 {
				if s.callback != nil {
					s.callback("dirsearchLoading", pr)
				}
			} else if pr.Status == 0 {
				// Status = 0 表示请求失败，增加错误计数
				atomic.AddInt32(&errorCount, 1)
			}
		}
		close(single)

		// 扫描完成，推送最终进度和完成事件
		if s.callback != nil {
			completed := int(atomic.LoadInt32(&completedCount))
			errors := int(atomic.LoadInt32(&errorCount))
			s.callback("dirsearchProgress", map[string]interface{}{
				"completed": completed,
				"total":     totalCount,
				"errors":    errors,
			})
			s.callback("dirsearchComplete", "done")
		}
	}()

	threadPool, _ := ants.NewPoolWithFunc(s.options.Workers, func(p interface{}) {
		if ctrlCtx.Err() != nil {
			wg.Done()
			return
		}
		task := p.(ScanTask)
		r := s.Scan(s.ctx, task.URL+task.Path)
		completed := atomic.AddInt32(&completedCount, 1)
		if s.callback != nil && completed%progressEmitStep == 0 {
			errors := int(atomic.LoadInt32(&errorCount))
			s.callback("dirsearchProgress", map[string]interface{}{
				"completed": int(completed),
				"total":     totalCount,
				"errors":    errors,
			})
		}
		retChan <- r
		wg.Done()
	})
	defer threadPool.Release()

	canceled := false
	for _, task := range tasks {
		if ctrlCtx.Err() != nil {
			canceled = true
			break
		}
		wg.Add(1)
		threadPool.Invoke(task)
		if s.options.Interval != 0 {
			time.Sleep(time.Second * time.Duration(s.options.Interval))
		}
	}
	if canceled {
		wg.Wait()
		close(retChan)
		<-single
		return
	}
	wg.Wait()
	close(retChan)
	<-single
}
func generateBackupPaths(baseURL string) []string {
	suffixFormat := []string{
		".zip", ".rar", ".tar.gz", ".tgz", ".tar.bz2", ".tar", ".jar", ".war",
		".7z", ".bak", ".sql", ".gz", ".sql.gz", ".tar.tgz", ".backup",
	}
	infoDict := []string{
		"1", "127.0.0.1", "admin", "backup", "backups", "db", "sql", "root", "www", "website",
		"2020", "2021", "2022", "2023", "2024", "2025", "old", "new", "test", "data", "dump",
		"web", "wordpress", "wp", "localhost", "users", "store", "auth",
	}

	// 提取主域名用于组合（如：www.example.com -> example）
	host := strings.TrimPrefix(strings.TrimPrefix(baseURL, "http://"), "https://")
	if strings.Contains(host, "/") {
		host = strings.Split(host, "/")[0]
	}
	domainParts := strings.Split(host, ".")

	domainVariants := map[string]struct{}{
		host:                               {},
		strings.ReplaceAll(host, ".", ""):  {},
		strings.ReplaceAll(host, ".", "_"): {},
	}

	if len(domainParts) > 1 {
		domainVariants[strings.Join(domainParts[1:], "")] = struct{}{}
		domainVariants[strings.Join(domainParts[1:], "_")] = struct{}{}
		domainVariants[domainParts[0]] = struct{}{}
	}

	for d := range domainVariants {
		infoDict = append(infoDict, d)
	}

	paths := []string{}
	for _, info := range infoDict {
		for _, suffix := range suffixFormat {
			paths = append(paths, info+suffix)
		}
	}
	return paths
}
