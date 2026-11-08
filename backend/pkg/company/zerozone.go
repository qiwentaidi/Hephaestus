package company

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const zeroZoneAPI = "https://0.zone/api/data/"

type zeroZoneRequest struct {
	Query           string `json:"query"`
	QueryType       string `json:"query_type"`
	Page            int    `json:"page,omitempty"`
	Next            int64  `json:"next,omitempty"`
	PageSize        int    `json:"pagesize"`
	ZoneKeyID       string `json:"zone_key_id"`
	TimestampSort   string `json:"timestamp_sort,omitempty"`
	TimestampUpdate string `json:"timestamp_update_sort,omitempty"`
	ZbPay           int    `json:"zb_pay,omitempty"`
}

type zeroZoneEnvelope struct {
	Code     int             `json:"code"`
	Message  string          `json:"message"`
	Page     int             `json:"page"`
	Next     any             `json:"next"`
	PageSize int             `json:"pagesize"`
	Total    int             `json:"total"`
	Data     json.RawMessage `json:"data"`
}

type zeroZoneOfficialItem struct {
	Title           string              `json:"title"`
	Group           string              `json:"group"`
	Company         string              `json:"company"`
	Timestamp       string              `json:"timestamp"`
	TimestampUpdate string              `json:"timestamp_update"`
	Type            string              `json:"type"`
	ICP             string              `json:"icp"`
	CheckTime       string              `json:"check_time"`
	Message         zeroZoneOfficialMsg `json:"msg"`
}

type zeroZoneOfficialMsg struct {
	AppID        any         `json:"app_id"`
	WechatFakeID string      `json:"wechat_fakeid"`
	WechatID     string      `json:"wechat_id"`
	IconURL      string      `json:"iconUrl"`
	Introduction string      `json:"introduction"`
	ServiceType  interface{} `json:"service_type"`
	Code         string      `json:"code"`
	AppURL       string      `json:"app_url"`
	ExtInfo      any         `json:"ext_information"`
}

type zeroZoneEmailItem struct {
	Email             string                  `json:"email"`
	EmailSource       zeroZoneEmailSources    `json:"email_source"`
	EmailType         string                  `json:"email_type"`
	Group             string                  `json:"group"`
	MailDomain        zeroZoneStringList      `json:"mail_domain"`
	Source            any                     `json:"source"`
	Message           zeroZoneEmailMsg        `json:"msg"`
	LeakageAccount    zeroZoneLeakageAccounts `json:"leakage_account"`
	LeakageUpdateTime string                  `json:"leakage_update_time"`
	LeakageNum        int                     `json:"leakage_num"`
	Timestamp         string                  `json:"timestamp"`
	Extra             map[string]any          `json:"extra"`
	Raw               json.RawMessage         `json:"-"`
}

type zeroZoneEmailMsg struct {
	Tags  []string `json:"tags"`
	Num   int      `json:"num"`
	Cycle int      `json:"cycle"`
}

// zeroZoneStringList 兼容 0.zone 字段类型漂移：
// 支持 string / []string / object / []object / number / bool。
type zeroZoneStringList []string

func (l *zeroZoneStringList) UnmarshalJSON(data []byte) error {
	items := decodeZeroZoneStringList(data)
	*l = zeroZoneStringList(items)
	return nil
}

func decodeZeroZoneStringList(data []byte) []string {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}

	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		return dedupeStrings(arr)
	}

	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		if strings.TrimSpace(single) == "" {
			return nil
		}
		return []string{single}
	}

	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil
	}
	return dedupeStrings(collectStrings(v))
}

func collectStrings(v any) []string {
	switch val := v.(type) {
	case nil:
		return nil
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return nil
		}
		return []string{s}
	case float64:
		return []string{fmt.Sprintf("%v", val)}
	case bool:
		return []string{fmt.Sprintf("%v", val)}
	case []any:
		var out []string
		for _, item := range val {
			out = append(out, collectStrings(item)...)
		}
		return out
	case map[string]any:
		// 对 object，优先收集 value 文本；若没有可用 value，则保底收集 key。
		var out []string
		for _, item := range val {
			out = append(out, collectStrings(item)...)
		}
		if len(out) == 0 {
			for k := range val {
				k = strings.TrimSpace(k)
				if k != "" {
					out = append(out, k)
				}
			}
		}
		return out
	default:
		s := strings.TrimSpace(fmt.Sprintf("%v", val))
		if s == "" || s == "<nil>" {
			return nil
		}
		return []string{s}
	}
}

func dedupeStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

type zeroZoneEmailSourceItem struct {
	SourceDomain string `json:"source_domain"`
	URL          string `json:"url"`
}

// zeroZoneEmailSources 兼容 email_source 的多种格式：
// []object / []string / string / object / number / bool
type zeroZoneEmailSources []string

func (s *zeroZoneEmailSources) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*s = nil
		return nil
	}

	var sourceItems []zeroZoneEmailSourceItem
	if err := json.Unmarshal(data, &sourceItems); err == nil {
		out := make([]string, 0, len(sourceItems))
		for _, item := range sourceItems {
			if v := strings.TrimSpace(item.URL); v != "" {
				out = append(out, v)
				continue
			}
			if v := strings.TrimSpace(item.SourceDomain); v != "" {
				out = append(out, v)
			}
		}
		*s = zeroZoneEmailSources(dedupeStrings(out))
		return nil
	}

	*s = zeroZoneEmailSources(decodeZeroZoneStringList(data))
	return nil
}

type zeroZoneLeakageAccountItem struct {
	Name     string `json:"Name"`
	Title    string `json:"Title"`
	Domain   string `json:"Domain"`
	EmailKey string `json:"email_key"`
}

// zeroZoneLeakageAccounts 兼容 leakage_account 的对象数组与字符串数组
type zeroZoneLeakageAccounts []string

func (a *zeroZoneLeakageAccounts) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*a = nil
		return nil
	}

	var accountItems []zeroZoneLeakageAccountItem
	if err := json.Unmarshal(data, &accountItems); err == nil {
		out := make([]string, 0, len(accountItems))
		for _, item := range accountItems {
			if v := strings.TrimSpace(item.Name); v != "" {
				out = append(out, v)
				continue
			}
			if v := strings.TrimSpace(item.Title); v != "" {
				out = append(out, v)
				continue
			}
			if v := strings.TrimSpace(item.Domain); v != "" {
				out = append(out, v)
				continue
			}
			if v := strings.TrimSpace(item.EmailKey); v != "" {
				out = append(out, v)
			}
		}
		*a = zeroZoneLeakageAccounts(dedupeStrings(out))
		return nil
	}

	*a = zeroZoneLeakageAccounts(decodeZeroZoneStringList(data))
	return nil
}

func parseZeroZoneNext(next any) int64 {
	switch v := next.(type) {
	case nil:
		return 0
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		n := strings.TrimSpace(v)
		if n == "" {
			return 0
		}
		if iv, err := strconv.ParseInt(n, 10, 64); err == nil {
			return iv
		}
		return 0
	default:
		s := strings.TrimSpace(fmt.Sprintf("%v", v))
		if s == "" || s == "<nil>" {
			return 0
		}
		if iv, err := strconv.ParseInt(s, 10, 64); err == nil {
			return iv
		}
		return 0
	}
}

var zeroZoneHTTPClient = &http.Client{
	Timeout: 20 * time.Second,
}

func doZeroZoneRequest(reqBody zeroZoneRequest) (*zeroZoneEnvelope, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, zeroZoneAPI, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := zeroZoneHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("zerozone api error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var env zeroZoneEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("zerozone response parse error: %w", err)
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("zerozone api returned code %d: %s", env.Code, env.Message)
	}
	return &env, nil
}

// FetchZeroZoneOfficialAccounts 调用 0.zone API 搜集公众号信息
func FetchZeroZoneResources(apiKey, companyName string) ([]OfficialAccount, []Applet, []App, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, nil, nil, fmt.Errorf("zerozone api key is empty")
	}
	companyName = strings.TrimSpace(companyName)
	if companyName == "" {
		return nil, nil, nil, fmt.Errorf("company name is empty")
	}

	const pageSize = 100
	var (
		officials []OfficialAccount
		applets   []Applet
		apps      []App
		useNext   bool
		next      int64
		page      = 1
	)

	for {
		reqBody := zeroZoneRequest{
			Query:     fmt.Sprintf("company==%s", companyName),
			QueryType: "app",
			PageSize:  pageSize,
			ZoneKeyID: apiKey,
		}
		if useNext {
			reqBody.Next = next
		} else {
			reqBody.Page = page
		}

		env, err := doZeroZoneRequest(reqBody)
		if err != nil {
			return nil, nil, nil, err
		}

		var data []zeroZoneOfficialItem
		if len(env.Data) > 0 {
			if err := json.Unmarshal(env.Data, &data); err != nil {
				return nil, nil, nil, fmt.Errorf("zerozone official data parse error: %w", err)
			}
		}

		for _, item := range data {
			switch strings.TrimSpace(item.Type) {
			case "微信公众号":
				officials = append(officials, OfficialAccount{
					Name:          item.Title,
					Numbers:       item.Message.WechatID,
					Logo:          item.Message.IconURL,
					Qrcode:        item.Message.Code,
					Introduction:  item.Message.Introduction,
					BelongCompany: item.Company,
				})
			case "微信小程序":
				applets = append(applets, Applet{
					ServiceName:      item.Title,
					ServiceLicence:   item.ICP,
					UpdateRecordTime: item.Timestamp,
					UnitName:         item.Company,
				})
			case "安卓APK", "iOS":
				apps = append(apps, App{
					ServiceName:    item.Title,
					ServiceLicence: item.ICP,
					UnitName:       item.Company,
				})
			}
		}

		if nextValue := parseZeroZoneNext(env.Next); nextValue > 0 {
			useNext = true
			next = nextValue
			continue
		}

		if len(data) < pageSize || page*pageSize >= env.Total || env.Total == 0 {
			break
		}
		page++
	}

	return officials, applets, apps, nil
}

// FetchZeroZoneEmails 调用 0.zone API 搜集邮箱信息
func FetchZeroZoneEmails(apiKey, companyName string) ([]EmailAccount, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("zerozone api key is empty")
	}
	companyName = strings.TrimSpace(companyName)
	if companyName == "" {
		return nil, fmt.Errorf("company name is empty")
	}

	const pageSize = 100
	var (
		results []EmailAccount
		useNext bool
		next    int64
		page    = 1
	)

	for {
		query := fmt.Sprintf("group==%s", companyName)
		reqBody := zeroZoneRequest{
			Query:     query,
			QueryType: "email",
			PageSize:  pageSize,
			ZoneKeyID: apiKey,
		}
		if useNext {
			reqBody.Next = next
		} else {
			reqBody.Page = page
		}

		env, err := doZeroZoneRequest(reqBody)
		if err != nil {
			return nil, err
		}

		var data []zeroZoneEmailItem
		if len(env.Data) > 0 {
			if err := json.Unmarshal(env.Data, &data); err != nil {
				return nil, fmt.Errorf("zerozone email data parse error: %w", err)
			}
		}

		for _, item := range data {
			sources := []string(item.EmailSource)
			if len(sources) == 0 {
				sources = collectStrings(item.Source)
			}
			if len(sources) == 0 {
				sources = []string{"0zone"}
			}
			results = append(results, EmailAccount{
				Address:          item.Email,
				Type:             item.EmailType,
				Company:          item.Group,
				Domains:          []string(item.MailDomain),
				Sources:          dedupeStrings(sources),
				Tags:             item.Message.Tags,
				LeakageCount:     item.LeakageNum,
				LeakageAccounts:  []string(item.LeakageAccount),
				LeakageUpdatedAt: item.LeakageUpdateTime,
			})
		}

		if nextValue := parseZeroZoneNext(env.Next); nextValue > 0 {
			useNext = true
			next = nextValue
			continue
		}

		if len(data) < pageSize || page*pageSize >= env.Total || env.Total == 0 {
			break
		}
		page++
	}

	return results, nil
}
