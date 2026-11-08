// Package ipmetadata resolves public IP address metadata from locally cached
// GeoLite ASN and QQWry IPDB databases.
package ipmetadata

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ipipdotnet/ipdb-go"
	"github.com/oschwald/geoip2-golang/v2"
)

const (
	geoLiteASNURL = "https://github.com/P3TERX/GeoLite.mmdb/releases/download/2026.07.16/GeoLite2-ASN.mmdb"
	qqwryIPDBURL  = "https://cdn.jsdelivr.net/npm/qqwry.ipdb/qqwry.ipdb"

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

// Resolver downloads the two public databases once when missing, then keeps
// their readers in memory for the process lifetime.
type Resolver struct {
	once     sync.Once
	statusMu sync.RWMutex
	status   Status
	cityDB   *ipdb.City
	asnDB    *geoip2.Reader
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
	}
}

// Status returns a snapshot without starting a download itself.
func (r *Resolver) Status() Status {
	r.statusMu.RLock()
	defer r.statusMu.RUnlock()
	return r.status
}

// Lookup returns data only for routable public IP addresses. A missing or
// temporarily unreachable database leaves the corresponding field blank.
func (r *Resolver) Lookup(value string) Metadata {
	ip := net.ParseIP(strings.TrimSpace(value))
	if !isPublicIP(ip) {
		return Metadata{}
	}

	r.once.Do(r.load)
	metadata := Metadata{}
	if r.cityDB != nil {
		if info, err := r.cityDB.FindInfo(ip.String(), "CN"); err == nil && info != nil {
			metadata.Location = MergeLocation(info.CountryName, info.RegionName, info.CityName, info.DistrictName)
			metadata.Operator = NormalizeISP(info.IspDomain)
		}
	}
	if r.asnDB != nil {
		if address, err := netip.ParseAddr(ip.String()); err == nil {
			if record, err := r.asnDB.ASN(address); err == nil && record != nil && record.HasData() {
				metadata.ASN = fmt.Sprintf("AS%d", record.AutonomousSystemNumber)
			}
		}
	}
	return metadata
}

func (r *Resolver) load() {
	databaseDir := r.Status().DatabaseDir
	if err := os.MkdirAll(databaseDir, 0755); err != nil {
		r.setStatus(StateFailed)
		return
	}

	qqwryPath := filepath.Join(databaseDir, "qqwry.ipdb")
	if databaseFileExists(qqwryPath) {
		r.setStatus(StateLoading)
	} else {
		r.setStatus(StateDownloadingQQWry)
	}
	if path, err := ensureDatabaseFile(databaseDir, "qqwry.ipdb", qqwryIPDBURL); err == nil {
		r.setStatus(StateLoading)
		r.cityDB, _ = ipdb.NewCity(path)
	}

	asnPath := filepath.Join(databaseDir, "GeoLite2-ASN.mmdb")
	if databaseFileExists(asnPath) {
		r.setStatus(StateLoading)
	} else {
		r.setStatus(StateDownloadingASN)
	}
	if path, err := ensureDatabaseFile(databaseDir, "GeoLite2-ASN.mmdb", geoLiteASNURL); err == nil {
		r.setStatus(StateLoading)
		r.asnDB, _ = geoip2.Open(path)
	}

	if r.cityDB != nil && r.asnDB != nil {
		r.setStatus(StateReady)
		return
	}
	r.setStatus(StateFailed)
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

func ensureDatabaseFile(directory, filename, sourceURL string) (string, error) {
	path := filepath.Join(directory, filename)
	if databaseFileExists(path) {
		return path, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Hephaestus-IP-Metadata/1.0")
	response, err := (&http.Client{}).Do(req)
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
