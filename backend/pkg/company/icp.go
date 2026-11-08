package company

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"time"

	"hephaestus-backend/pkg/logger"

	"github.com/qiwentaidi/clients"
)

// WebApiResponse 工信部Web查询响应
type WebApiResponse struct {
	Code   int    `json:"code"`
	Msg    string `json:"msg"`
	Params struct {
		PageNum  int       `json:"pageNum"`
		PageSize int       `json:"pageSize"`
		Total    int       `json:"total"`
		NextPage int       `json:"nextPage"`
		HasNext  bool      `json:"hasNextPage"`
		List     []WebData `json:"list"`
	} `json:"params"`
	Success bool `json:"success"`
}

// WebData 域名备案数据
type WebData struct {
	Domain           string `json:"domain"`
	DomainID         int64  `json:"domainId"`
	MainID           int64  `json:"mainId"`
	MainLicence      string `json:"mainLicence"`
	ServiceID        int64  `json:"serviceId"`
	ServiceLicence   string `json:"serviceLicence"`
	UnitName         string `json:"unitName"`
	UpdateRecordTime string `json:"updateRecordTime"`
	NatureName       string `json:"natureName"`
	LimitAccess      string `json:"limitAccess"`
}

// AppApiResponse 工信部App查询响应
type AppApiResponse struct {
	Code   int    `json:"code"`
	Msg    string `json:"msg"`
	Params struct {
		EndRow          int   `json:"endRow"`
		FirstPage       int   `json:"firstPage"`
		HasNextPage     bool  `json:"hasNextPage"`
		HasPreviousPage bool  `json:"hasPreviousPage"`
		IsFirstPage     bool  `json:"isFirstPage"`
		IsLastPage      bool  `json:"isLastPage"`
		LastPage        int   `json:"lastPage"`
		List            []App `json:"list"`
		NavigatePages   int   `json:"navigatePages"`
		NextPage        int   `json:"nextPage"`
		PageNum         int   `json:"pageNum"`
		PageSize        int   `json:"pageSize"`
		Pages           int   `json:"pages"`
		PrePage         int   `json:"prePage"`
		Size            int   `json:"size"`
		StartRow        int   `json:"startRow"`
		Total           int   `json:"total"`
	} `json:"params"`
	Success bool `json:"success"`
}

// AppletApiResponse 工信部小程序查询响应
type AppletApiResponse struct {
	Code   int    `json:"code"`
	Msg    string `json:"msg"`
	Params struct {
		EndRow          int      `json:"endRow"`
		FirstPage       int      `json:"firstPage"`
		HasNextPage     bool     `json:"hasNextPage"`
		HasPreviousPage bool     `json:"hasPreviousPage"`
		IsFirstPage     bool     `json:"isFirstPage"`
		IsLastPage      bool     `json:"isLastPage"`
		LastPage        int      `json:"lastPage"`
		List            []Applet `json:"list"`
		NavigatePages   int      `json:"navigatePages"`
		NextPage        int      `json:"nextPage"`
		PageNum         int      `json:"pageNum"`
		PageSize        int      `json:"pageSize"`
		Pages           int      `json:"pages"`
		PrePage         int      `json:"prePage"`
		Size            int      `json:"size"`
		StartRow        int      `json:"startRow"`
		Total           int      `json:"total"`
	} `json:"params"`
	Success bool `json:"success"`
}

// App App信息
type App struct {
	CityID           int    `json:"cityId"`
	CountyID         int    `json:"countyId"`
	DataID           int    `json:"dataId"`
	LeaderName       string `json:"leaderName"`
	MainID           string `json:"mainId"`
	MainLicence      string `json:"mainLicence"`
	MainUnitAddress  string `json:"mainUnitAddress"`
	MainUnitCertNo   string `json:"mainUnitCertNo"`
	MainUnitCertType int    `json:"mainUnitCertType"`
	NatureID         int    `json:"natureId"`
	NatureName       string `json:"natureName"`
	ProvinceID       int    `json:"provinceId"`
	ServiceID        int64  `json:"serviceId"`
	ServiceLicence   string `json:"serviceLicence"`
	ServiceName      string `json:"serviceName"`
	ServiceType      int    `json:"serviceType"`
	UnitName         string `json:"unitName"`
	UpdateRecordTime string `json:"updateRecordTime"`
	Version          string `json:"version"`
}

// Applet 小程序信息
type Applet struct {
	CityID           int    `json:"cityId"`
	CountyID         int    `json:"countyId"`
	DataID           int    `json:"dataId"`
	LeaderName       string `json:"leaderName"`
	MainID           string `json:"mainId"`
	MainLicence      string `json:"mainLicence"`
	MainUnitAddress  string `json:"mainUnitAddress"`
	MainUnitCertNo   string `json:"mainUnitCertNo"`
	MainUnitCertType int    `json:"mainUnitCertType"`
	NatureID         int    `json:"natureId"`
	NatureName       string `json:"natureName"`
	ProvinceID       int    `json:"provinceId"`
	ServiceID        int64  `json:"serviceId"`
	ServiceLicence   string `json:"serviceLicence"`
	ServiceName      string `json:"serviceName"`
	ServiceType      int    `json:"serviceType"`
	UnitName         string `json:"unitName"`
	UpdateRecordTime string `json:"updateRecordTime"`
	Version          string `json:"version"`
}

// FetchWebInfo 查询域名备案信息
func FetchWebInfo(apiAddress string, query string) (*WebApiResponse, error) {
	var allList []WebData
	pageSize := 10
	pageNum := 1
	totalPages := 1
	var lastResponse WebApiResponse
	retryCount := 0
	maxRetries := 5

	apiAddress = strings.TrimRight(apiAddress, "/")

	for {
		queryURL := fmt.Sprintf("%s/query/web?search=%s&pageNum=%d&pageSize=%d", apiAddress, url.QueryEscape(query), pageNum, pageSize)
		resp, err := clients.SimpleGet(queryURL, clients.NewRestyClient(nil, true))
		if err != nil {
			retryCount++
			if retryCount > maxRetries {
				logger.Error("[icp] %s查询备案信息重试超过%d次，返回空结果", query, maxRetries)
				return &WebApiResponse{}, nil
			}
			logger.Warning("[icp] %s查询备案信息出现错误: %v，重试第%d次", query, err, retryCount)
			time.Sleep(time.Duration(1+rand.Intn(2)) * time.Second)
			continue
		}

		bodyStr := string(resp.Body())
		// 如果不包含params参数时代码查询失败，需要重新查询
		if !strings.Contains(bodyStr, "params") {
			retryCount++
			if retryCount > maxRetries {
				logger.Error("[icp] %s查询备案信息重试超过%d次，返回空结果", query, maxRetries)
				return &WebApiResponse{}, nil
			}
			logger.Warning("[icp] %s查询备案信息时出现错误: %s，重试第%d次", query, bodyStr, retryCount)
			time.Sleep(time.Duration(1+rand.Intn(2)) * time.Second)
			continue
		}

		var response WebApiResponse
		if err := json.Unmarshal(resp.Body(), &response); err != nil {
			retryCount++
			if retryCount > maxRetries {
				logger.Error("[icp] %s查询备案信息重试超过%d次，返回空结果", query, maxRetries)
				return &WebApiResponse{}, nil
			}
			logger.Warning("[icp] %s解析备案信息时出现错误: %v，重试第%d次", query, err, retryCount)
			time.Sleep(time.Duration(1+rand.Intn(2)) * time.Second)
			continue
		}
		if !response.Success {
			break
		}

		if pageNum == 1 {
			lastResponse = response
			total := response.Params.Total
			if total > 0 {
				totalPages = (total + pageSize - 1) / pageSize
			}
		}

		allList = append(allList, response.Params.List...)

		if pageNum >= totalPages {
			break
		}

		pageNum++
		retryCount = 0 // 重置重试计数器
		time.Sleep(3 * time.Second)
	}

	lastResponse.Params.List = allList
	return &lastResponse, nil
}

// FetchAppInfo 查询App信息
func FetchAppInfo(apiAddress string, companyName string) (*AppApiResponse, error) {
	var allList []App
	pageSize := 10
	pageNum := 1
	totalPages := 1
	var lastResponse AppApiResponse
	retryCount := 0
	maxRetries := 5

	apiAddress = strings.TrimRight(apiAddress, "/")

	for {
		queryURL := fmt.Sprintf("%s/query/app?search=%s&pageNum=%d&pageSize=%d", apiAddress, url.QueryEscape(companyName), pageNum, pageSize)
		resp, err := clients.SimpleGet(queryURL, clients.NewRestyClient(nil, true))
		if err != nil {
			retryCount++
			if retryCount > maxRetries {
				logger.Error("[icp] %s查询App信息重试超过%d次，返回空结果", companyName, maxRetries)
				return &AppApiResponse{}, nil
			}
			logger.Warning("[icp] %s查询App信息出现错误: %v，重试第%d次", companyName, err, retryCount)
			time.Sleep(time.Duration(1+rand.Intn(2)) * time.Second)
			continue
		}

		bodyStr := string(resp.Body())
		// 如果不包含params参数时代码查询失败，需要重新查询
		if !strings.Contains(bodyStr, "params") {
			retryCount++
			if retryCount > maxRetries {
				logger.Error("[icp] %s查询App信息重试超过%d次，返回空结果", companyName, maxRetries)
				return &AppApiResponse{}, nil
			}
			logger.Warning("[icp] %s查询App信息时出现错误: %s，重试第%d次", companyName, bodyStr, retryCount)
			time.Sleep(time.Duration(1+rand.Intn(2)) * time.Second)
			continue
		}

		var response AppApiResponse
		if err := json.Unmarshal(resp.Body(), &response); err != nil {
			retryCount++
			if retryCount > maxRetries {
				logger.Error("[icp] %s查询App信息重试超过%d次，返回空结果", companyName, maxRetries)
				return &AppApiResponse{}, nil
			}
			logger.Warning("[icp] %s解析App信息时出现错误: %v，重试第%d次", companyName, err, retryCount)
			time.Sleep(time.Duration(1+rand.Intn(2)) * time.Second)
			continue
		}
		if !response.Success {
			break
		}

		if pageNum == 1 {
			lastResponse = response
			total := response.Params.Total
			if total > 0 {
				totalPages = (total + pageSize - 1) / pageSize
			}
		}

		allList = append(allList, response.Params.List...)

		if pageNum >= totalPages {
			break
		}

		pageNum++
		retryCount = 0 // 重置重试计数器
		time.Sleep(3 * time.Second)
	}

	lastResponse.Params.List = allList
	return &lastResponse, nil
}

// FetchAppletInfo 查询小程序信息
func FetchAppletInfo(apiAddress string, companyName string) (*AppletApiResponse, error) {
	var allList []Applet
	pageSize := 10
	pageNum := 1
	totalPages := 1
	var lastResponse AppletApiResponse
	retryCount := 0
	maxRetries := 5

	apiAddress = strings.TrimRight(apiAddress, "/")

	for {
		queryURL := fmt.Sprintf("%s/query/mapp?search=%s&pageNum=%d&pageSize=%d", apiAddress, url.QueryEscape(companyName), pageNum, pageSize)
		resp, err := clients.SimpleGet(queryURL, clients.NewRestyClient(nil, true))
		if err != nil {
			retryCount++
			if retryCount > maxRetries {
				logger.Error("[icp] %s查询小程序信息重试超过%d次，返回空结果", companyName, maxRetries)
				return &AppletApiResponse{}, nil
			}
			logger.Warning("[icp] %s查询小程序信息出现错误: %v，重试第%d次", companyName, err, retryCount)
			time.Sleep(time.Duration(1+rand.Intn(2)) * time.Second)
			continue
		}

		bodyStr := string(resp.Body())
		// 如果不包含params参数时代码查询失败，需要重新查询
		if !strings.Contains(bodyStr, "params") {
			retryCount++
			if retryCount > maxRetries {
				logger.Error("[icp] %s查询小程序信息重试超过%d次，返回空结果", companyName, maxRetries)
				return &AppletApiResponse{}, nil
			}
			logger.Warning("[icp] %s查询小程序信息时出现错误: %s，重试第%d次", companyName, bodyStr, retryCount)
			time.Sleep(time.Duration(1+rand.Intn(2)) * time.Second)
			continue
		}

		var response AppletApiResponse
		if err := json.Unmarshal(resp.Body(), &response); err != nil {
			retryCount++
			if retryCount > maxRetries {
				logger.Error("[icp] %s查询小程序信息重试超过%d次，返回空结果", companyName, maxRetries)
				return &AppletApiResponse{}, nil
			}
			logger.Warning("[icp] %s解析小程序信息时出现错误: %v，重试第%d次", companyName, err, retryCount)
			time.Sleep(time.Duration(1+rand.Intn(2)) * time.Second)
			continue
		}
		if !response.Success {
			break
		}

		if pageNum == 1 {
			lastResponse = response
			total := response.Params.Total
			if total > 0 {
				totalPages = (total + pageSize - 1) / pageSize
			}
		}

		allList = append(allList, response.Params.List...)

		if pageNum >= totalPages {
			break
		}

		pageNum++
		retryCount = 0 // 重置重试计数器
		time.Sleep(3 * time.Second)
	}

	lastResponse.Params.List = allList
	return &lastResponse, nil
}
