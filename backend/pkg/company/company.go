package company

import (
	"context"
	"fmt"
	"strings"
	"time"

	"hephaestus-backend/pkg/company/riskbird"
	"hephaestus-backend/pkg/logger"
)

// CompanyInfo 公司信息
type CompanyInfo struct {
	CompanyName      string            `json:"companyName"`
	Trademark        string            `json:"trademark"`
	Investment       float64           `json:"investment"`
	Amount           string            `json:"amount"`
	RegStatus        string            `json:"regStatus"`
	Domains          []string          `json:"domains"`
	Subsidiaries     []CompanyInfo     `json:"subsidiaries"`
	OfficialAccounts []OfficialAccount `json:"officialAccounts"`
	Emails           []EmailAccount    `json:"emails"`
	Apps             []App             `json:"apps"`
	Applets          []Applet          `json:"applets"`
}

// OfficialAccount 企业公众号信息
type OfficialAccount struct {
	Name          string `json:"name"`
	Logo          string `json:"logo"`
	Numbers       string `json:"numbers"`
	Qrcode        string `json:"qrcode"`
	Introduction  string `json:"introduction"`
	BelongCompany string `json:"belongCompany"`
}

// EmailAccount 企业邮箱信息
type EmailAccount struct {
	Address          string   `json:"address"`
	Type             string   `json:"type"`
	Company          string   `json:"company"`
	Domains          []string `json:"domains"`
	Sources          []string `json:"sources"`
	Tags             []string `json:"tags"`
	LeakageCount     int      `json:"leakageCount"`
	LeakageAccounts  []string `json:"leakageAccounts"`
	LeakageUpdatedAt string   `json:"leakageUpdatedAt"`
}

// DataSource 数据源配置
type DataSource struct {
	Riskbird struct {
		Enable bool
		Cookie string
	}
	ZeroZone struct {
		Enable bool
		APIKey string
	}
	Miit struct {
		Enable bool
		API    string
	}
}

func dedupeTrimmedStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func NormalizeCompanyInfoTree(info *CompanyInfo) {
	if info == nil {
		return
	}
	info.Domains = dedupeTrimmedStrings(info.Domains)
	for i := range info.Subsidiaries {
		NormalizeCompanyInfoTree(&info.Subsidiaries[i])
	}
}

func NormalizeCompanyInfos(items []CompanyInfo) []CompanyInfo {
	for i := range items {
		NormalizeCompanyInfoTree(&items[i])
	}
	return items
}

// FetchCompanyInfo 收集企业信息
func FetchCompanyInfo(companyName string, ratio int, ds *DataSource, maxDepth int, miitApi string) (*CompanyInfo, error) {
	var result *CompanyInfo

	// 使用风鸟收集信息（优先）
	if ds.Riskbird.Enable {
		client := riskbird.NewClient(context.Background(), ds.Riskbird.Cookie)
		if client.CheckLogin() {
			info, err := FetchCompanyWithRiskbird(client, companyName, ratio, 1, maxDepth)
			if err != nil {
				logger.Error("[riskbird] fetch company info error: %s", err.Error())
				result = &CompanyInfo{
					CompanyName: companyName,
				}
			} else {
				result = info
			}
		} else {
			logger.Warning("riskbird cookie is invalid")
			result = &CompanyInfo{
				CompanyName: companyName,
			}
		}
	} else {
		result = &CompanyInfo{
			CompanyName: companyName,
		}
	}

	// 使用 0.zone 补充公众号
	if ds.ZeroZone.Enable && result != nil {
		var enrich func(comp *CompanyInfo)
		enrich = func(comp *CompanyInfo) {
			if comp == nil {
				return
			}
			officials, applets, apps, err := FetchZeroZoneResources(ds.ZeroZone.APIKey, comp.CompanyName)
			if err != nil {
				logger.Warning("[0.zone] %s 获取应用信息失败: %s", comp.CompanyName, err.Error())
			} else {
				if len(officials) > 0 {
					comp.OfficialAccounts = append(comp.OfficialAccounts, officials...)
				}
				if len(applets) > 0 {
					comp.Applets = append(comp.Applets, applets...)
				}
				if len(apps) > 0 {
					comp.Apps = append(comp.Apps, apps...)
				}
			}
			emails, err := FetchZeroZoneEmails(ds.ZeroZone.APIKey, comp.CompanyName)
			if err != nil {
				logger.Warning("[0.zone] %s 获取邮箱失败: %s", comp.CompanyName, err.Error())
			} else if len(emails) > 0 {
				comp.Emails = append(comp.Emails, emails...)
			}
			for i := range comp.Subsidiaries {
				enrich(&comp.Subsidiaries[i])
			}
		}
		enrich(result)
	}

	// 使用工信部API补充信息（仅在明确启用时执行）
	miitEnabled := false
	if ds != nil {
		miitEnabled = ds.Miit.Enable
		if strings.TrimSpace(ds.Miit.API) != "" {
			miitApi = ds.Miit.API
		}
	}
	if miitEnabled && strings.TrimSpace(miitApi) != "" {
		miitApi = strings.TrimRight(strings.TrimSpace(miitApi), "/")
		// 给主公司填充ICP信息
		EnrichCompanyWithMiit(result, miitApi)
		time.Sleep(2 * time.Second)
		// 给所有子公司递归填充
		var enrichSubsidiaries func(subs []CompanyInfo)
		enrichSubsidiaries = func(subs []CompanyInfo) {
			for i := range subs {
				EnrichCompanyWithMiit(&subs[i], miitApi)
				if len(subs[i].Subsidiaries) > 0 {
					enrichSubsidiaries(subs[i].Subsidiaries)
				}
			}
		}
		enrichSubsidiaries(result.Subsidiaries)
	}

	NormalizeCompanyInfoTree(result)
	return result, nil
}

// FetchCompanyWithRiskbird 使用风鸟递归获取公司信息
func FetchCompanyWithRiskbird(client *riskbird.RiskbirdClient, companyName string, ratio int, currentDepth, maxDepth int) (*CompanyInfo, error) {
	var companyInfo CompanyInfo

	// Step 1: 获取公司基本信息和 orderNo
	basicInfo, orderNo, err := client.FetchBasicCompanyInfo(companyName)
	if err != nil {
		return nil, err
	}

	companyInfo.CompanyName = basicInfo.CompanyName
	companyInfo.RegStatus = basicInfo.RegStatus
	companyInfo.Amount = basicInfo.Amount
	companyInfo.Investment = 0

	logger.Info("[riskbird] 正在查询 %s 的信息", basicInfo.CompanyName)

	// Step 2: 获取公众号信息
	if basicInfo.RegStatus != "注销" && basicInfo.RegStatus != "吊销" {
		accounts, err := client.FetchWeChatOfficialAccounts(orderNo)
		if err != nil {
			logger.Warning("[riskbird] %s 获取公众号失败: %s", basicInfo.CompanyName, err.Error())
		} else if len(accounts) > 0 {
			for _, acc := range accounts {
				companyInfo.OfficialAccounts = append(companyInfo.OfficialAccounts, OfficialAccount{
					Name:          acc.Name,
					Numbers:       acc.AccountNo,
					Logo:          acc.HeadImageUrl,
					Qrcode:        acc.QrcodeUrl,
					Introduction:  acc.Introduction,
					BelongCompany: basicInfo.CompanyName,
				})
			}
		}
		time.Sleep(1 * time.Second)
	}

	// Step 3: 获取子公司信息
	subsidiaries, err := client.FetchSubsidiary(orderNo)
	if err != nil {
		logger.Warning("[riskbird] %s 获取子公司失败: %s", basicInfo.CompanyName, err.Error())
	}

	for _, sub := range subsidiaries {
		// 解析投资比例
		percentStr := strings.TrimSuffix(sub.FunderRatio, "%")
		var percent float64
		fmt.Sscanf(percentStr, "%f", &percent)

		// 过滤低于阈值的投资
		if ratio > 0 && percent < float64(ratio) {
			continue
		}

		child := CompanyInfo{
			CompanyName: sub.EntName,
			Investment:  percent,
			Amount:      sub.RegCap,
			RegStatus:   sub.EntStatus,
			Trademark:   sub.EntPic,
		}

		// 递归获取子公司信息
		if sub.EntStatus != "注销" && sub.EntStatus != "吊销" && currentDepth < maxDepth {
			time.Sleep(2 * time.Second)
			subInfo, err := FetchCompanyWithRiskbird(client, sub.EntName, ratio, currentDepth+1, maxDepth)
			if err == nil {
				child.OfficialAccounts = subInfo.OfficialAccounts
				child.Subsidiaries = subInfo.Subsidiaries
			}
		}

		companyInfo.Subsidiaries = append(companyInfo.Subsidiaries, child)
	}

	return &companyInfo, nil
}

// EnrichCompanyWithMiit 使用工信部API补充公司信息
func EnrichCompanyWithMiit(company *CompanyInfo, miitApi string) {
	// 吊销或注销则跳过
	if company.RegStatus == "吊销" || company.RegStatus == "注销" {
		return
	}

	logger.Info("[icp] 正在查询%s域名信息", company.CompanyName)
	if webResp, err := FetchWebInfo(miitApi, company.CompanyName); err == nil {
		var domains []string
		for _, data := range webResp.Params.List {
			domains = append(domains, data.Domain)
		}
		company.Domains = dedupeTrimmedStrings(domains)
	} else {
		logger.Warning("%s fetch web info error: %s", company.CompanyName, err)
	}

	time.Sleep(2 * time.Second)
	logger.Info("[icp] 正在查询%sApp信息", company.CompanyName)
	if appResp, err := FetchAppInfo(miitApi, company.CompanyName); err == nil {
		mergeApps(company, appResp.Params.List)
	} else {
		logger.Warning("%s fetch app info error: %s", company.CompanyName, err)
	}

	time.Sleep(2 * time.Second)
	logger.Info("[icp] 正在查询%s小程序信息", company.CompanyName)
	if appletResp, err := FetchAppletInfo(miitApi, company.CompanyName); err == nil {
		mergeApplets(company, appletResp.Params.List)
	} else {
		logger.Warning("%s fetch applet info error: %s", company.CompanyName, err)
	}
}

func mergeApps(company *CompanyInfo, apps []App) {
	existing := make(map[string]bool)
	for _, app := range company.Apps {
		existing[app.ServiceName+app.ServiceLicence] = true
	}
	for _, app := range apps {
		key := app.ServiceName + app.ServiceLicence
		if !existing[key] {
			company.Apps = append(company.Apps, app)
			existing[key] = true
		}
	}
}

func mergeApplets(company *CompanyInfo, applets []Applet) {
	existing := make(map[string]bool)
	for _, applet := range company.Applets {
		existing[applet.ServiceName+applet.ServiceLicence] = true
	}
	for _, applet := range applets {
		key := applet.ServiceName + applet.ServiceLicence
		if !existing[key] {
			company.Applets = append(company.Applets, applet)
			existing[key] = true
		}
	}
}
