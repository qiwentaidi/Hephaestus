// Package ipmetadata resolves public IP address metadata from locally cached
// GeoLite ASN and QQWry IPDB databases.
package ipmetadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ipipdotnet/ipdb-go"
	"github.com/oschwald/geoip2-golang/v2"
)

const (
	// GeoLite release tags are publication dates, so a pinned URL goes stale
	// once the project publishes the next build. The /releases/latest/download
	// path always redirects to the current release's asset.
	geoLiteASNURL = "https://github.com/P3TERX/GeoLite.mmdb/releases/latest/download/GeoLite2-ASN.mmdb"
	// When the latest-download path cannot be used, ask the Release API for the
	// current tag and retry with the pinned URL it describes.
	geoLiteASNReleaseAPI = "https://api.github.com/repos/P3TERX/GeoLite.mmdb/releases/latest"
	qqwryIPDBURL         = "https://cdn.jsdelivr.net/npm/qqwry.ipdb/qqwry.ipdb"

	geoLiteASNLatestMarker = "/releases/latest/download/"
	geoLiteASNPinnedPrefix = "/releases/download/"

	databaseDownloadTimeout = 2 * time.Minute
	releaseAPITimeout       = 30 * time.Second

	StateIdle             = "idle"
	StateDownloadingQQWry = "downloading_qqwry"
	StateDownloadingASN   = "downloading_asn"
	StateLoading          = "loading"
	StateReady            = "ready"
	StateFailed           = "failed"
)

// Metadata contains the display fields used by IP asset records.
type Metadata struct {
	Operator string
	Location string
	ASN      string
}

// Status reports the lazy IP metadata bootstrap state to authenticated UI
// clients, including the manual-install instructions for intranet deployments.
type Status struct {
	State         string `json:"state"`
	Ready         bool   `json:"ready"`
	DatabaseDir   string `json:"databaseDir"`
	QQWryURL      string `json:"qqwryUrl"`
	GeoLiteASNURL string `json:"geoLiteAsnUrl"`
}

// Resolver loads the two public databases from the local data directory and
// downloads any missing file in the background. Lookups never wait for a
// download: they use whichever database is already open.
type Resolver struct {
	mu       sync.Mutex
	statusMu sync.RWMutex
	status   Status
	cityDB   *ipdb.City
	asnDB    *geoip2.Reader

	loading       bool
	qqwryURL      string
	asnURL        string
	asnReleaseAPI string
	client        *http.Client
}

func NewResolver(dataDir string) *Resolver {
	databaseDir := filepath.Join(dataDir, "ipdb")
	return &Resolver{
		status: Status{
			State:         StateIdle,
			DatabaseDir:   databaseDir,
			QQWryURL:      qqwryIPDBURL,
			GeoLiteASNURL: geoLiteASNURL,
		},
		qqwryURL:      qqwryIPDBURL,
		asnURL:        geoLiteASNURL,
		asnReleaseAPI: geoLiteASNReleaseAPI,
		client:        &http.Client{Timeout: databaseDownloadTimeout},
	}
}

// Status returns a snapshot without starting a download itself.
func (r *Resolver) Status() Status {
	r.statusMu.RLock()
	defer r.statusMu.RUnlock()
	return r.status
}

// Lookup returns data only for routable public IP addresses. It never starts
// or waits for a download: StartWarmup loads the databases in the background,
// and a missing database leaves the corresponding field blank.
func (r *Resolver) Lookup(value string) Metadata {
	ip := net.ParseIP(strings.TrimSpace(value))
	if !isPublicIP(ip) {
		return Metadata{}
	}

	r.mu.Lock()
	city := r.cityDB
	asn := r.asnDB
	r.mu.Unlock()

	metadata := Metadata{}
	if city != nil {
		if info, err := city.FindInfo(ip.String(), "CN"); err == nil && info != nil {
			metadata.Location = MergeLocation(info.CountryName, info.RegionName, info.CityName, info.DistrictName)
			metadata.Operator = NormalizeISP(info.IspDomain)
		}
	}
	if asn != nil {
		if address, err := netip.ParseAddr(ip.String()); err == nil {
			if record, err := asn.ASN(address); err == nil && record != nil && record.HasData() {
				metadata.ASN = fmt.Sprintf("AS%d", record.AutonomousSystemNumber)
			}
		}
	}
	return metadata
}

// StartWarmup loads the databases in a background goroutine at application
// startup, so asset requests never wait for a download. Failures are logged
// with the manual download URL and target directory.
func (r *Resolver) StartWarmup() {
	r.mu.Lock()
	if r.loading || (r.cityDB != nil && r.asnDB != nil) {
		r.mu.Unlock()
		return
	}
	r.loading = true
	r.mu.Unlock()

	go r.load()
}

func (r *Resolver) load() {
	defer func() {
		r.mu.Lock()
		r.loading = false
		r.mu.Unlock()
	}()

	databaseDir := r.Status().DatabaseDir
	if err := os.MkdirAll(databaseDir, 0755); err != nil {
		log.Printf("[IP元数据] 创建数据库目录失败: %v", err)
		r.setStatus(StateFailed)
		return
	}

	r.mu.Lock()
	city := r.cityDB
	asn := r.asnDB
	r.mu.Unlock()

	if city == nil {
		qqwryPath := filepath.Join(databaseDir, "qqwry.ipdb")
		if databaseFileExists(qqwryPath) {
			r.setStatus(StateLoading)
		} else {
			r.setStatus(StateDownloadingQQWry)
		}
		path, err := r.ensureDatabaseFile(databaseDir, "qqwry.ipdb", r.qqwryURL)
		if err != nil {
			r.logManualInstall("qqwry.ipdb", r.qqwryURL, err)
		} else {
			r.setStatus(StateLoading)
			opened, openErr := ipdb.NewCity(path)
			if openErr != nil {
				r.logCorruptDatabase("qqwry.ipdb", path, openErr)
			} else {
				city = opened
				r.mu.Lock()
				r.cityDB = opened
				r.mu.Unlock()
			}
		}
	}

	if asn == nil {
		asnPath := filepath.Join(databaseDir, "GeoLite2-ASN.mmdb")
		if databaseFileExists(asnPath) {
			r.setStatus(StateLoading)
		} else {
			r.setStatus(StateDownloadingASN)
		}
		path, err := r.ensureASNDatabaseFile(databaseDir)
		if err != nil {
			r.logManualInstall("GeoLite2-ASN.mmdb", r.Status().GeoLiteASNURL, err)
		} else {
			r.setStatus(StateLoading)
			opened, openErr := geoip2.Open(path)
			if openErr != nil {
				r.logCorruptDatabase("GeoLite2-ASN.mmdb", path, openErr)
			} else {
				asn = opened
				r.mu.Lock()
				r.asnDB = opened
				r.mu.Unlock()
			}
		}
	}

	if city != nil && asn != nil {
		r.setStatus(StateReady)
		log.Printf("[IP元数据] IP 归属地库已加载: %s", databaseDir)
		return
	}
	r.setStatus(StateFailed)
}

// logManualInstall tells operators how to supply a database file when the
// automatic download fails, which is common on intranet deployments.
func (r *Resolver) logManualInstall(filename, sourceURL string, cause error) {
	target := filepath.Join(r.Status().DatabaseDir, filename)
	log.Printf("[IP元数据] 下载 %s 失败: %v", filename, cause)
	log.Printf("[IP元数据] 请手动下载 %s 并放到: %s（放好后重启服务生效）", filename, target)
	log.Printf("[IP元数据] 下载地址: %s", sourceURL)
}

func (r *Resolver) logCorruptDatabase(filename, path string, cause error) {
	log.Printf("[IP元数据] 打开 %s 失败（文件可能损坏）: %v", filename, cause)
	log.Printf("[IP元数据] 请删除 %s 后重启服务重新下载，或手动替换该文件", path)
}

func (r *Resolver) setStatus(state string) {
	r.statusMu.Lock()
	defer r.statusMu.Unlock()
	r.status.State = state
	r.status.Ready = state == StateReady
}

func databaseFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

func (r *Resolver) ensureDatabaseFile(directory, filename, sourceURL string) (string, error) {
	path := filepath.Join(directory, filename)
	if databaseFileExists(path) {
		return path, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), databaseDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Hephaestus-IP-Metadata/1.0")
	client := r.client
	if client == nil {
		client = &http.Client{Timeout: databaseDownloadTimeout}
	}
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: unexpected HTTP status %s", filename, response.Status)
	}

	temporaryFile, err := os.CreateTemp(directory, filename+"-*.download")
	if err != nil {
		return "", err
	}
	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)
	if _, err := io.Copy(temporaryFile, response.Body); err != nil {
		_ = temporaryFile.Close()
		return "", err
	}
	if err := temporaryFile.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return "", err
	}
	return path, nil
}

// ensureASNDatabaseFile downloads the ASN database from the latest release,
// retrying once with a tag-pinned URL resolved through the Release API when the
// latest-download path fails.
func (r *Resolver) ensureASNDatabaseFile(directory string) (string, error) {
	path, err := r.ensureDatabaseFile(directory, "GeoLite2-ASN.mmdb", r.asnURL)
	if err == nil {
		return path, nil
	}
	pinnedURL, resolveErr := r.latestASNReleaseURL()
	if resolveErr != nil {
		log.Printf("[IP元数据] 解析 ASN 库最新 release 失败: %v", resolveErr)
		return "", err
	}
	log.Printf("[IP元数据] ASN 库最新下载地址为 %s，重试下载", pinnedURL)
	return r.ensureDatabaseFile(directory, "GeoLite2-ASN.mmdb", pinnedURL)
}

// latestASNReleaseURL asks the Release API for the current GeoLite tag and
// returns the tag-pinned asset URL next to the configured latest-release URL.
func (r *Resolver) latestASNReleaseURL() (string, error) {
	if !strings.Contains(r.asnURL, geoLiteASNLatestMarker) {
		return "", fmt.Errorf("ASN 下载地址缺少 %s 路径: %s", geoLiteASNLatestMarker, r.asnURL)
	}
	tag, err := r.fetchLatestASNReleaseTag()
	if err != nil {
		return "", err
	}
	pinnedURL := strings.Replace(r.asnURL, geoLiteASNLatestMarker,
		geoLiteASNPinnedPrefix+url.PathEscape(tag)+"/", 1)
	if pinnedURL == r.asnURL {
		return "", fmt.Errorf("无法从 %s 生成固定 release 地址", r.asnURL)
	}
	return pinnedURL, nil
}

func (r *Resolver) fetchLatestASNReleaseTag() (string, error) {
	apiURL := r.asnReleaseAPI
	if apiURL == "" {
		apiURL = geoLiteASNReleaseAPI
	}
	ctx, cancel := context.WithTimeout(context.Background(), releaseAPITimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Hephaestus-IP-Metadata/1.0")
	client := r.client
	if client == nil {
		client = &http.Client{Timeout: databaseDownloadTimeout}
	}
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("查询最新 release: unexpected HTTP status %s", response.Status)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return "", fmt.Errorf("解析最新 release 响应: %w", err)
	}
	tag := strings.TrimSpace(payload.TagName)
	if tag == "" {
		return "", fmt.Errorf("最新 release 响应缺少 tag_name")
	}
	return tag, nil
}

func isPublicIP(ip net.IP) bool {
	return ip != nil && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsUnspecified() &&
		!ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsMulticast()
}

// MergeLocation omits duplicates such as Beijing/Beijing while retaining the
// country, province, city, and district sequence used by dnsx-plus.
func MergeLocation(country, province, city, district string) string {
	fields := make([]string, 0, 4)
	if country = strings.TrimSpace(country); country != "" {
		fields = append(fields, country)
	}
	province = strings.TrimSpace(province)
	city = strings.TrimSpace(city)
	district = strings.TrimSpace(district)
	if province != "" && province != city {
		fields = append(fields, province)
	}
	if city != "" {
		fields = append(fields, city)
	}
	if district != "" {
		fields = append(fields, district)
	}
	return strings.Join(fields, "/")
}

// NormalizeISP maps carrier and cloud-provider aliases onto one display name.
// GeoIP databases mix short names, legal entities and domain brands, so
// grouping only by the raw string splits the same provider into several
// buckets (for example "阿里云" and "阿里云计算有限公司").
func NormalizeISP(isp string) string {
	value := strings.TrimSpace(isp)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	switch {
	case strings.Contains(value, "阿里云") || strings.Contains(lower, "alibaba") || strings.Contains(lower, "aliyun"):
		return "阿里云"
	case strings.Contains(value, "腾讯云") || strings.Contains(lower, "tencent"):
		return "腾讯云"
	case strings.Contains(value, "华为云") || strings.Contains(lower, "huawei"):
		return "华为云"
	case strings.Contains(value, "百度云") || strings.Contains(lower, "baidu"):
		return "百度云"
	case strings.Contains(value, "金山云") || strings.Contains(lower, "kingsoft"):
		return "金山云"
	case strings.Contains(value, "京东云") || strings.Contains(lower, "jdcloud"):
		return "京东云"
	case strings.Contains(value, "移动") || strings.Contains(lower, "cmnet") || strings.Contains(lower, "china mobile"):
		return "中国移动"
	case strings.Contains(value, "联通") || strings.Contains(lower, "unicom") || strings.Contains(lower, "china169"):
		return "中国联通"
	case strings.Contains(value, "电信") || strings.Contains(lower, "chinanet") || strings.Contains(lower, "china telecom"):
		return "中国电信"
	case strings.Contains(value, "教育网") || strings.Contains(lower, "cernet"):
		return "教育网"
	case strings.Contains(value, "鹏博士") || strings.Contains(lower, "drpeng"):
		return "鹏博士"
	case strings.Contains(value, "长城宽带") || strings.Contains(lower, "great wall broadband"):
		return "长城宽带"
	default:
		return value
	}
}
