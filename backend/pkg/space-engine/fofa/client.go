package fofa

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"hephaestus-backend/pkg/types"

	"github.com/qiwentaidi/clients"
	"github.com/spaolacci/murmur3"
)

type FofaClient struct {
	Auth *types.FofaAuth
}

const fofaSimpleOrChunkSize = 20

var (
	fofaBatchClausePattern = regexp.MustCompile(`^(ip|domain|host)="[^"]+"$`)
	fofaRequestDelay       = time.Second
)

func NewFofaClient(auth *types.FofaAuth) *FofaClient {
	if auth.Address == "" {
		auth.Address = "https://fofa.info"
	}

	return &FofaClient{
		Auth: auth,
	}
}

// FOFABaseEncode FOFA Base64 编码
func FOFABaseEncode(str string) string {
	return base64.StdEncoding.EncodeToString([]byte(str))
}

func (f *FofaClient) buildSearchURL(search, pageSize, pageNum string, fraud, cert bool) string {
	params := url.Values{}
	params.Set("email", f.Auth.Email)
	params.Set("key", f.Auth.Key)
	params.Set("qbase64", FOFABaseEncode(search))
	params.Set("page", pageNum)
	params.Set("size", pageSize)
	params.Set("fields", "host,title,ip,domain,port,protocol,country_name,region,city,icp,link,product")
	if cert {
		params.Set("cert.is_valid", "true")
	}
	if fraud {
		params.Set("is_fraud", "false")
		params.Set("is_honeypot", "false")
	}
	return strings.TrimRight(f.Auth.Address, "/") + "/api/v1/search/all?" + params.Encode()
}

// FofaApiSearch FOFA API 搜索
func (f *FofaClient) FofaApiSearch(search, pageSize, pageNum string, fraud, cert bool) *types.FofaSearchResult {
	if chunks, ok := splitSimpleOrQuery(search, fofaSimpleOrChunkSize); ok {
		return f.fofaApiSearchChunks(chunks, pageSize, pageNum, fraud, cert)
	}

	result := f.fofaApiSearchSingle(search, pageSize, pageNum, fraud, cert)
	sleepAfterFofaRequest()
	return result
}

func (f *FofaClient) fofaApiSearchSingle(search, pageSize, pageNum string, fraud, cert bool) *types.FofaSearchResult {
	var fs types.FofaSearchResult
	var fr types.FofaRawResult

	address := f.buildSearchURL(search, pageSize, pageNum, fraud, cert)

	// 使用 clients 包创建客户端，支持跟随重定向
	client := clients.DefaultRestyClient()
	client.SetTimeout(time.Second * 15)
	resp, err := clients.SimpleGet(address, client)
	if err != nil {
		fs.Error = true
		fs.Message = "请求失败: " + err.Error()
		return &fs
	}

	responseBody := strings.TrimSpace(resp.String())
	if resp.StatusCode() >= 400 {
		fs.Error = true
		fs.Message = fmt.Sprintf("FOFA API HTTP %d: %s", resp.StatusCode(), truncateMessage(responseBody, 300))
		return &fs
	}

	if err := json.Unmarshal(resp.Body(), &fr); err != nil {
		fs.Error = true
		fs.Message = "解析响应失败: " + err.Error() + "; 响应内容: " + truncateMessage(responseBody, 300)
		return &fs
	}

	fs.Size = fr.Size
	fs.Error = fr.Error

	if fr.Errmsg == "[820001] 没有权限搜索product字段" {
		user := f.FofaApiSearchByUserInfo()
		if user.Error || !user.Isvip {
			fs.Message = "[FOFA] 当前用户非VIP权限, 无法使用API查询"
		}
	} else {
		fs.Message = fr.Errmsg
	}

	if fs.Error && strings.TrimSpace(fs.Message) == "" {
		fs.Message = fmt.Sprintf("FOFA API 返回错误但未提供 errmsg，HTTP %d，响应内容: %s", resp.StatusCode(), truncateMessage(responseBody, 300))
	}

	if !fs.Error {
		if fs.Size == 0 {
			fs.Message = "未查询到数据"
		} else {
			for i := range fr.Results {
				if len(fr.Results[i]) < 12 {
					continue
				}
				// 拼接地区信息：国家/省份/城市
				var regionParts []string
				if fr.Results[i][6] != "" {
					regionParts = append(regionParts, fr.Results[i][6])
				}
				if fr.Results[i][7] != "" {
					regionParts = append(regionParts, fr.Results[i][7])
				}
				if fr.Results[i][8] != "" {
					regionParts = append(regionParts, fr.Results[i][8])
				}
				region := ""
				if len(regionParts) > 0 {
					region = regionParts[0]
					for j := 1; j < len(regionParts); j++ {
						region += "/" + regionParts[j]
					}
				}

				fs.Results = append(fs.Results, types.FofaResult{
					URL:      fr.Results[i][10],
					Host:     fr.Results[i][0],
					Title:    fr.Results[i][1],
					IP:       fr.Results[i][2],
					Port:     fr.Results[i][4],
					Domain:   fr.Results[i][3],
					Protocol: fr.Results[i][5],
					Country:  region,
					ICP:      fr.Results[i][9],
					Product:  fr.Results[i][11],
				})
			}
		}
	}

	return &fs
}

func (f *FofaClient) fofaApiSearchChunks(chunks []string, pageSize, pageNum string, fraud, cert bool) *types.FofaSearchResult {
	requestedPageSize := parsePositiveInt(pageSize, 100)
	requestedPageNum := parsePositiveInt(pageNum, 1)
	requestedEnd := requestedPageSize * requestedPageNum
	chunkPageSize := strconv.Itoa(requestedEnd)

	var merged []types.FofaResult
	seen := make(map[string]struct{})
	total := 0

	for index, chunk := range chunks {
		result := f.fofaApiSearchSingle(chunk, chunkPageSize, "1", fraud, cert)
		sleepAfterFofaRequest()
		if result.Error {
			return &types.FofaSearchResult{
				Error:   true,
				Message: fmt.Sprintf("批量查询第 %d/%d 批失败: %s", index+1, len(chunks), result.Message),
			}
		}

		total += result.Size
		for _, item := range result.Results {
			key := fofaResultKey(item)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, item)
		}
	}

	start := requestedPageSize * (requestedPageNum - 1)
	end := start + requestedPageSize
	pageResults := []types.FofaResult{}
	if start < len(merged) {
		if end > len(merged) {
			end = len(merged)
		}
		pageResults = merged[start:end]
	}

	message := fmt.Sprintf("批量查询完成，已拆分为 %d 批", len(chunks))
	if total == 0 {
		message += "，未查询到数据"
	} else {
		message += fmt.Sprintf("，共找到 %d 条结果", total)
	}

	return &types.FofaSearchResult{
		Error:   false,
		Message: message,
		Size:    total,
		Results: pageResults,
	}
}

func splitSimpleOrQuery(search string, chunkSize int) ([]string, bool) {
	if chunkSize <= 0 {
		return nil, false
	}

	trimmed := strings.TrimSpace(search)
	if !strings.Contains(trimmed, "||") || strings.Contains(trimmed, "&&") || strings.ContainsAny(trimmed, "()") {
		return nil, false
	}

	parts := strings.Split(trimmed, "||")
	clauses := make([]string, 0, len(parts))
	for _, part := range parts {
		clause := strings.TrimSpace(part)
		if clause == "" || !fofaBatchClausePattern.MatchString(clause) {
			return nil, false
		}
		clauses = append(clauses, clause)
	}
	if len(clauses) <= chunkSize {
		return nil, false
	}

	chunks := make([]string, 0, (len(clauses)+chunkSize-1)/chunkSize)
	for start := 0; start < len(clauses); start += chunkSize {
		end := start + chunkSize
		if end > len(clauses) {
			end = len(clauses)
		}
		chunks = append(chunks, strings.Join(clauses[start:end], " || "))
	}
	return chunks, true
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func fofaResultKey(item types.FofaResult) string {
	return item.Host + "\x00" + item.IP + "\x00" + item.Port + "\x00" + item.URL
}

func sleepAfterFofaRequest() {
	if fofaRequestDelay > 0 {
		time.Sleep(fofaRequestDelay)
	}
}

// FofaApiSearchByUserInfo 获取用户信息
func (f *FofaClient) FofaApiSearchByUserInfo() *types.FofaUserInfo {
	var user types.FofaUserInfo
	// 使用 clients 包的 SimpleGet，不需要 context
	client := clients.DefaultRestyClient()
	resp, err := clients.SimpleGet("https://fofa.info/api/v1/info/my?key="+f.Auth.Key, client)
	if err != nil {
		user.Error = true
		return &user
	}

	if err := json.Unmarshal(resp.Body(), &user); err != nil {
		user.Error = true
		return &user
	}
	return &user
}

// FofaApiStats 统计聚合
func (f *FofaClient) FofaApiStats(query, fields string) *types.StatsResponse {
	var result types.StatsResponse

	address := f.Auth.Address + "/api/v1/search/stats?email=" + f.Auth.Email + "&key=" + f.Auth.Key + "&qbase64=" +
		FOFABaseEncode(query) + "&fields=" + url.QueryEscape(fields)

	// 使用 clients 包创建客户端，支持跟随重定向
	client := clients.DefaultRestyClient()
	// 统计接口要增加请求响应时长
	client.SetTimeout(time.Second * 20)
	resp, err := clients.SimpleGet(address, client)
	if err != nil {
		result.Error = true
		result.Message = "请求失败: " + err.Error()
		return &result
	}

	// 解析响应
	var apiResp types.FofaStatsAPIResponse
	body := resp.Body()
	if err := json.Unmarshal(body, &apiResp); err != nil {
		result.Error = true
		result.Message = "解析响应失败: " + err.Error()
		return &result
	}

	// 检查错误
	if apiResp.Error {
		result.Error = true
		result.Message = "统计聚合失败"
		return &result
	}

	// 解析统计数据
	result.Data = make(map[string][]types.StatsItem)

	// 解析每个字段的统计
	fieldList := strings.SplitSeq(fields, ",")
	for field := range fieldList {
		field = strings.TrimSpace(field)
		var items []types.StatsItem

		switch field {
		case "domain":
			for _, item := range apiResp.Aggs.Domain {
				items = append(items, types.StatsItem{
					Name:  item.Name,
					Count: item.Count,
				})
			}
		case "port":
			for _, item := range apiResp.Aggs.Port {
				items = append(items, types.StatsItem{
					Name:  item.Name,
					Count: item.Count,
				})
			}
		case "protocol":
			for _, item := range apiResp.Aggs.Protocol {
				items = append(items, types.StatsItem{
					Name:  item.Name,
					Count: item.Count,
				})
			}
		case "server":
			for _, item := range apiResp.Aggs.Server {
				items = append(items, types.StatsItem{
					Name:  item.Name,
					Count: item.Count,
				})
			}
		case "country", "countries":
			// countries 字段在 API 中可能是空数组或其他格式
			// 暂时跳过，如果需要可以后续添加解析逻辑
		}

		if len(items) > 0 {
			result.Data[field] = items
		}
	}

	return &result
}

func truncateMessage(message string, limit int) string {
	message = strings.TrimSpace(message)
	if limit <= 0 || len(message) <= limit {
		return message
	}
	return message[:limit] + "..."
}

// Mmh3Hash32 计算 mmh3 hash
func Mmh3Hash32(raw []byte) string {
	var h32 hash.Hash32 = murmur3.New32()
	_, err := h32.Write(base64Encode(raw))
	if err == nil {
		return fmt.Sprint(int32(h32.Sum32()))
	} else {
		return "0"
	}
}

// base64 encode
func base64Encode(braw []byte) []byte {
	bckd := base64.StdEncoding.EncodeToString(braw)
	var buffer bytes.Buffer
	for i := 0; i < len(bckd); i++ {
		ch := bckd[i]
		buffer.WriteByte(ch)
		if (i+1)%76 == 0 {
			buffer.WriteByte('\n')
		}
	}
	buffer.WriteByte('\n')
	return buffer.Bytes()
}
