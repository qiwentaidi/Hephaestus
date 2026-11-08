package asset

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"hephaestus-backend/pkg/ipmetadata"
)

var assetMapProvinceAliases = []struct {
	key     string
	label   string
	aliases []string
}{
	{"北京", "北京市", []string{"北京", "北京市"}},
	{"天津", "天津市", []string{"天津", "天津市"}},
	{"上海", "上海市", []string{"上海", "上海市"}},
	{"重庆", "重庆市", []string{"重庆", "重庆市"}},
	{"河北", "河北省", []string{"河北", "河北省"}},
	{"山西", "山西省", []string{"山西", "山西省"}},
	{"辽宁", "辽宁省", []string{"辽宁", "辽宁省"}},
	{"吉林", "吉林省", []string{"吉林", "吉林省"}},
	{"黑龙江", "黑龙江省", []string{"黑龙江", "黑龙江省"}},
	{"江苏", "江苏省", []string{"江苏", "江苏省"}},
	{"浙江", "浙江省", []string{"浙江", "浙江省"}},
	{"安徽", "安徽省", []string{"安徽", "安徽省"}},
	{"福建", "福建省", []string{"福建", "福建省"}},
	{"江西", "江西省", []string{"江西", "江西省"}},
	{"山东", "山东省", []string{"山东", "山东省"}},
	{"河南", "河南省", []string{"河南", "河南省"}},
	{"湖北", "湖北省", []string{"湖北", "湖北省"}},
	{"湖南", "湖南省", []string{"湖南", "湖南省"}},
	{"广东", "广东省", []string{"广东", "广东省"}},
	{"海南", "海南省", []string{"海南", "海南省"}},
	{"四川", "四川省", []string{"四川", "四川省"}},
	{"贵州", "贵州省", []string{"贵州", "贵州省"}},
	{"云南", "云南省", []string{"云南", "云南省"}},
	{"陕西", "陕西省", []string{"陕西", "陕西省"}},
	{"甘肃", "甘肃省", []string{"甘肃", "甘肃省"}},
	{"青海", "青海省", []string{"青海", "青海省"}},
	{"台湾", "台湾省", []string{"台湾", "台湾省"}},
	{"内蒙古", "内蒙古自治区", []string{"内蒙古", "内蒙古自治区"}},
	{"广西", "广西壮族自治区", []string{"广西", "广西壮族自治区"}},
	{"西藏", "西藏自治区", []string{"西藏", "西藏自治区"}},
	{"宁夏", "宁夏回族自治区", []string{"宁夏", "宁夏回族自治区"}},
	{"新疆", "新疆维吾尔自治区", []string{"新疆", "新疆维吾尔自治区"}},
	{"香港", "香港特别行政区", []string{"香港", "香港特别行政区"}},
	{"澳门", "澳门特别行政区", []string{"澳门", "澳门特别行政区"}},
}

func normalizeAssetMapProvince(value string) (string, string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false
	}
	value = strings.ReplaceAll(value, " ", "")
	for _, province := range assetMapProvinceAliases {
		for _, alias := range province.aliases {
			if value == alias {
				return province.key, province.label, true
			}
		}
	}
	for _, province := range assetMapProvinceAliases {
		for _, alias := range province.aliases {
			if strings.Contains(value, alias) {
				return province.key, province.label, true
			}
		}
	}
	return "", "", false
}

func isAssetMapMunicipality(province string) bool {
	switch province {
	case "北京", "天津", "上海", "重庆", "香港", "澳门":
		return true
	default:
		return false
	}
}

func splitAssetMapLocation(value string) []string {
	parts := make([]string, 0, 4)
	for _, part := range strings.Split(value, "/") {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func isAssetMapChinaCountry(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "中国", "china", "cn", "chn", "中华人民共和国":
		return true
	default:
		return false
	}
}

func assetMapLocationCountry(value string) (string, bool) {
	parts := splitAssetMapLocation(value)
	if len(parts) == 0 {
		return "", false
	}
	if _, _, ok := normalizeAssetMapProvince(parts[0]); ok {
		return "中国", true
	}
	return parts[0], true
}

func assetMapOverseasCity(value string) string {
	parts := splitAssetMapLocation(value)
	if len(parts) == 0 {
		return ""
	}
	for index := len(parts) - 1; index >= 1; index-- {
		if parts[index] != "" {
			return parts[index]
		}
	}
	return parts[0]
}

func isAssetMapPublicIP(value string) bool {
	ip := net.ParseIP(strings.TrimSpace(value))
	return ip != nil && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsUnspecified() &&
		!ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsMulticast()
}

func parseAssetMapLocation(value string) (assetMapLocation, bool) {
	parts := splitAssetMapLocation(value)
	if len(parts) == 0 {
		return assetMapLocation{}, false
	}
	startIndex := 0
	country := "中国"
	if isAssetMapChinaCountry(parts[0]) {
		startIndex = 1
	} else {
		if _, _, ok := normalizeAssetMapProvince(parts[0]); !ok {
			return assetMapLocation{}, false
		}
	}
	if startIndex >= len(parts) {
		return assetMapLocation{}, false
	}

	province, provinceLabel, ok := normalizeAssetMapProvince(parts[startIndex])
	if !ok {
		return assetMapLocation{}, false
	}
	city := ""
	if isAssetMapMunicipality(province) {
		city = provinceLabel
	} else if len(parts) > startIndex+1 {
		city = strings.TrimSpace(parts[startIndex+1])
	}
	if city == "" && isAssetMapMunicipality(province) {
		city = provinceLabel
	}
	return assetMapLocation{Country: country, Province: province, ProvinceLabel: provinceLabel, City: city}, true
}

// normalizeAssetMapCity removes administrative suffixes so that "杭州" and
// "杭州市" are grouped together.
func normalizeAssetMapCity(value string) string {
	value = strings.TrimSpace(value)
	for _, suffix := range []string{"特别行政区", "自治州", "地区", "市"} {
		if strings.HasSuffix(value, suffix) && len([]rune(value)) > len([]rune(suffix)) {
			return strings.TrimSuffix(value, suffix)
		}
	}
	return value
}

func assetMapOperator(value string) string {
	return ipmetadata.NormalizeISP(value)
}

// assetMapCityName drops province-level pseudo-cities that some GeoIP rows
// report for addresses without city detail (for example "中国/浙江省").
func assetMapCityName(location assetMapLocation) string {
	raw := strings.TrimSpace(location.City)
	if raw == "" {
		return ""
	}
	if !isAssetMapMunicipality(location.Province) {
		if raw == location.Province || raw == location.ProvinceLabel {
			return ""
		}
	}
	return normalizeAssetMapCity(raw)
}

func assetMapDimensionStats(values map[string]int, limit int) []AssetMapDimensionStat {
	stats := make([]AssetMapDimensionStat, 0, len(values))
	for name, count := range values {
		if strings.TrimSpace(name) == "" {
			continue
		}
		stats = append(stats, AssetMapDimensionStat{Name: name, Count: count})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Count == stats[j].Count {
			return stats[i].Name < stats[j].Name
		}
		return stats[i].Count > stats[j].Count
	})
	if limit > 0 && len(stats) > limit {
		return stats[:limit]
	}
	return stats
}

func assetMapCountryStats(countries map[string]*assetMapCountryAggregate) []AssetMapCountryStat {
	stats := make([]AssetMapCountryStat, 0, len(countries))
	for _, country := range countries {
		country.AssetCount = country.Count
		country.TaskCount = len(country.tasks)
		country.Cities = assetMapDimensionStats(country.cities, 8)
		country.Operators = assetMapDimensionStats(country.operators, 8)
		country.ASNs = assetMapDimensionStats(country.asns, 8)
		stats = append(stats, country.AssetMapCountryStat)
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].AssetCount == stats[j].AssetCount {
			return stats[i].Country < stats[j].Country
		}
		return stats[i].AssetCount > stats[j].AssetCount
	})
	return stats
}

// assetMapRiskRollup attributes findings observed on domains to the IPs those
// domains resolve to. Without this an IP shows no risk even though every
// vulnerability on its hosted web assets belongs to it.
func assetMapRiskRollup(items []AssetCenterItem) map[string]map[string]struct{} {
	rollup := make(map[string]map[string]struct{})
	for _, item := range items {
		if item.Type != "domain" || len(item.riskFindings) == 0 {
			continue
		}
		for _, ip := range item.IPs {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			set := rollup[ip]
			if set == nil {
				set = make(map[string]struct{})
				rollup[ip] = set
			}
			for key := range item.riskFindings {
				set[key] = struct{}{}
			}
		}
	}
	return rollup
}

// assetMapEffectiveRisk returns the de-duplicated direct + resolved-domain
// finding counts for one IP asset.
func assetMapEffectiveRisk(item AssetCenterItem, rollup map[string]map[string]struct{}) (int, int) {
	extra := rollup[item.Value]
	if len(item.riskFindings) == 0 && len(extra) == 0 {
		return item.CriticalCount, item.HighCount
	}
	merged := make(map[string]struct{}, len(item.riskFindings)+len(extra))
	for key := range item.riskFindings {
		merged[key] = struct{}{}
	}
	for key := range extra {
		merged[key] = struct{}{}
	}
	return countAssetRiskFindings(merged)
}

func assetMapWindowSince(window string, now time.Time) time.Time {
	switch strings.ToLower(strings.TrimSpace(window)) {
	case "7d":
		return now.AddDate(0, 0, -7)
	case "30d":
		return now.AddDate(0, 0, -30)
	case "90d":
		return now.AddDate(0, 0, -90)
	default:
		return time.Time{}
	}
}

func buildAssetMapOverview(items []AssetCenterItem) AssetMapOverviewData {
	return buildAssetMapOverviewSince(items, time.Time{})
}

func buildAssetMapOverviewSince(items []AssetCenterItem, since time.Time) AssetMapOverviewData {
	return buildAssetMapOverviewWithRisk(items, since, nil)
}

func buildAssetMapOverviewWithRisk(items []AssetCenterItem, since time.Time, riskIndex *findingRiskIndex) AssetMapOverviewData {
	overview := AssetMapOverviewData{UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	regions := make(map[string]*assetMapRegionAggregate)
	operators := make(map[string]int)
	asns := make(map[string]int)
	countries := make(map[string]*assetMapCountryAggregate)
	riskRollup := assetMapRiskRollup(items)

	for _, item := range items {
		if item.Type != "ip" {
			continue
		}
		if !since.IsZero() && !item.LastSeenAt.IsZero() && item.LastSeenAt.Before(since) {
			continue
		}
		overview.TotalIPAssets++
		operator := assetMapOperator(item.Operator)
		if operator != "" {
			operators[operator]++
		}
		if item.ASN != "" {
			asns[item.ASN]++
		}
		critical, high := assetMapEffectiveRisk(item, riskRollup)
		directCritical, directHigh, inheritedCritical, inheritedHigh := critical, high, 0, 0
		medium := 0
		directMedium, inheritedMedium := 0, 0
		if riskIndex != nil && riskIndex.hasFindings() {
			counts, _ := riskIndex.countsFor("ip", item.Value)
			directCritical = counts.DirectCritical
			directHigh = counts.DirectHigh
			directMedium = counts.DirectMedium
			inheritedCritical = counts.InheritedCritical
			inheritedHigh = counts.InheritedHigh
			inheritedMedium = counts.InheritedMedium
			critical = directCritical + inheritedCritical
			high = directHigh + inheritedHigh
			medium = directMedium + inheritedMedium
		}

		location, ok := parseAssetMapLocation(item.Location)
		if !ok {
			overview.UnknownIPAssets++
			if !isAssetMapPublicIP(item.Value) {
				overview.NonPublicIPAssets++
			} else if country, hasCountry := assetMapLocationCountry(item.Location); hasCountry && !isAssetMapChinaCountry(country) {
				overview.OverseasIPAssets++
				countryAggregate := countries[country]
				if countryAggregate == nil {
					countryAggregate = &assetMapCountryAggregate{
						AssetMapCountryStat: AssetMapCountryStat{
							Name:         country,
							Country:      country,
							CountryLabel: country,
						},
						tasks:     make(map[string]struct{}),
						cities:    make(map[string]int),
						operators: make(map[string]int),
						asns:      make(map[string]int),
					}
					countries[country] = countryAggregate
				}
				countryAggregate.Count++
				countryAggregate.CriticalCount += critical
				countryAggregate.HighCount += high
				countryAggregate.MediumCount += medium
				countryAggregate.DirectCritical += directCritical
				countryAggregate.DirectHigh += directHigh
				countryAggregate.DirectMedium += directMedium
				countryAggregate.InheritedCritical += inheritedCritical
				countryAggregate.InheritedHigh += inheritedHigh
				countryAggregate.InheritedMedium += inheritedMedium
				for _, taskID := range item.SourceTaskIDs {
					countryAggregate.tasks[taskID] = struct{}{}
				}
				if city := normalizeAssetMapCity(assetMapOverseasCity(item.Location)); city != "" && city != country {
					countryAggregate.cities[city]++
				}
				if operator != "" {
					countryAggregate.operators[operator]++
				}
				if item.ASN != "" {
					countryAggregate.asns[item.ASN]++
				}
			} else {
				overview.UnresolvedIPAssets++
			}
			continue
		}
		overview.LocatedIPAssets++

		region := regions[location.Province]
		if region == nil {
			region = &assetMapRegionAggregate{
				AssetMapRegionStat: AssetMapRegionStat{
					Province:      location.Province,
					ProvinceLabel: location.ProvinceLabel,
				},
				tasks:     make(map[string]struct{}),
				cities:    make(map[string]int),
				operators: make(map[string]int),
				asns:      make(map[string]int),
			}
			regions[location.Province] = region
		}
		region.AssetCount++
		region.CriticalCount += critical
		region.HighCount += high
		region.MediumCount += medium
		region.DirectCritical += directCritical
		region.DirectHigh += directHigh
		region.DirectMedium += directMedium
		region.InheritedCritical += inheritedCritical
		region.InheritedHigh += inheritedHigh
		region.InheritedMedium += inheritedMedium
		for _, taskID := range item.SourceTaskIDs {
			region.tasks[taskID] = struct{}{}
		}
		if city := assetMapCityName(location); city != "" {
			region.cities[city]++
		}
		if operator != "" {
			region.operators[operator]++
		}
		if item.ASN != "" {
			region.asns[item.ASN]++
		}
	}

	overview.Regions = make([]AssetMapRegionStat, 0, len(regions))
	for _, region := range regions {
		region.TaskCount = len(region.tasks)
		region.Cities = assetMapDimensionStats(region.cities, 8)
		region.Operators = assetMapDimensionStats(region.operators, 8)
		region.ASNs = assetMapDimensionStats(region.asns, 8)
		overview.Regions = append(overview.Regions, region.AssetMapRegionStat)
		overview.TotalCritical += region.CriticalCount
		overview.TotalHigh += region.HighCount
		overview.TotalMedium += region.MediumCount
	}
	sort.Slice(overview.Regions, func(i, j int) bool {
		if overview.Regions[i].AssetCount == overview.Regions[j].AssetCount {
			return overview.Regions[i].Province < overview.Regions[j].Province
		}
		return overview.Regions[i].AssetCount > overview.Regions[j].AssetCount
	})
	overview.Operators = assetMapDimensionStats(operators, 10)
	overview.ASNs = assetMapDimensionStats(asns, 10)
	overview.Countries = assetMapCountryStats(countries)
	for _, country := range overview.Countries {
		overview.TotalCritical += country.CriticalCount
		overview.TotalHigh += country.HighCount
		overview.TotalMedium += country.MediumCount
	}
	return overview
}

type assetMapAssetQuery struct {
	Scope    string
	Province string
	City     string
	Country  string
	Risk     string
	Keyword  string
	Since    time.Time
	Page     int
	PageSize int
}

func assetMapPortLabel(port int, protocol string) string {
	if port <= 0 {
		return ""
	}
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" {
		protocol = "tcp"
	}
	return fmt.Sprintf("%d/%s", port, protocol)
}

func assetMapPortLess(first, second string) bool {
	firstPort := 0
	secondPort := 0
	_, _ = fmt.Sscanf(first, "%d", &firstPort)
	_, _ = fmt.Sscanf(second, "%d", &secondPort)
	if firstPort == secondPort {
		return first < second
	}
	return firstPort < secondPort
}

type assetMapHostServices struct {
	ports    map[string]struct{}
	services map[string]struct{}
}

func assetMapHostServiceIndex(items []AssetCenterItem) map[string]*assetMapHostServices {
	index := make(map[string]*assetMapHostServices)
	for _, item := range items {
		if item.Type != "endpoint" || item.EndpointHost == "" {
			continue
		}
		entry := index[item.EndpointHost]
		if entry == nil {
			entry = &assetMapHostServices{ports: make(map[string]struct{}), services: make(map[string]struct{})}
			index[item.EndpointHost] = entry
		}
		if label := assetMapPortLabel(item.Port, item.Protocol); label != "" {
			entry.ports[label] = struct{}{}
		}
		for _, service := range item.ServiceLabels {
			if service = strings.TrimSpace(service); service != "" {
				entry.services[service] = struct{}{}
			}
		}
	}
	return index
}

func buildAssetMapAssets(items []AssetCenterItem, query assetMapAssetQuery) AssetMapAssetListData {
	return buildAssetMapAssetsWithRisk(items, query, nil)
}

func buildAssetMapAssetsWithRisk(items []AssetCenterItem, query assetMapAssetQuery, riskIndex *findingRiskIndex) AssetMapAssetListData {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		query.PageSize = 20
	}
	scope := strings.ToLower(strings.TrimSpace(query.Scope))
	if scope == "" {
		scope = "located"
	}
	riskRollup := assetMapRiskRollup(items)
	serviceIndex := assetMapHostServiceIndex(items)
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	provinceFilter := strings.TrimSpace(query.Province)
	cityFilter := strings.TrimSpace(query.City)
	countryFilter := strings.TrimSpace(query.Country)
	riskFilter := strings.ToLower(strings.TrimSpace(query.Risk))
	results := make([]AssetMapAssetItem, 0)

	for _, item := range items {
		if item.Type != "ip" {
			continue
		}
		if !query.Since.IsZero() && !item.LastSeenAt.IsZero() && item.LastSeenAt.Before(query.Since) {
			continue
		}
		location, located := parseAssetMapLocation(item.Location)
		country, hasCountry := assetMapLocationCountry(item.Location)
		public := isAssetMapPublicIP(item.Value)
		itemScope := "unresolved"
		switch {
		case located:
			itemScope = "located"
		case !public:
			itemScope = "internal"
		case hasCountry && !isAssetMapChinaCountry(country):
			itemScope = "overseas"
		}
		if scope != "all" && scope != itemScope {
			continue
		}
		if provinceFilter != "" && (!located || location.Province != provinceFilter) {
			continue
		}
		if cityFilter != "" && (!located || normalizeAssetMapCity(location.City) != normalizeAssetMapCity(cityFilter)) {
			continue
		}
		if countryFilter != "" && (!hasCountry || country != countryFilter) {
			continue
		}
		critical, high := assetMapEffectiveRisk(item, riskRollup)
		directCritical, directHigh, inheritedCritical, inheritedHigh := critical, high, 0, 0
		medium := 0
		directMedium, inheritedMedium := 0, 0
		if riskIndex != nil && riskIndex.hasFindings() {
			counts, _ := riskIndex.countsFor("ip", item.Value)
			directCritical = counts.DirectCritical
			directHigh = counts.DirectHigh
			directMedium = counts.DirectMedium
			inheritedCritical = counts.InheritedCritical
			inheritedHigh = counts.InheritedHigh
			inheritedMedium = counts.InheritedMedium
			critical = directCritical + inheritedCritical
			high = directHigh + inheritedHigh
			medium = directMedium + inheritedMedium
		}
		switch riskFilter {
		case "critical":
			if critical == 0 {
				continue
			}
		case "high":
			if high == 0 {
				continue
			}
		case "medium":
			if medium == 0 {
				continue
			}
		case "any":
			if critical+high+medium == 0 {
				continue
			}
		case "none":
			if critical+high > 0 {
				continue
			}
		}
		if keyword != "" {
			haystack := strings.ToLower(strings.Join([]string{
				item.Value, item.Location, item.Operator, item.ASN,
			}, " "))
			if !strings.Contains(haystack, keyword) {
				continue
			}
		}

		entry := AssetMapAssetItem{
			AssetID:           item.AssetID,
			Value:             item.Value,
			Location:          item.Location,
			Operator:          assetMapOperator(item.Operator),
			ASN:               item.ASN,
			TaskCount:         item.TaskCount,
			CriticalCount:     critical,
			HighCount:         high,
			MediumCount:       medium,
			DirectCritical:    directCritical,
			DirectHigh:        directHigh,
			DirectMedium:      directMedium,
			InheritedCritical: inheritedCritical,
			InheritedHigh:     inheritedHigh,
			InheritedMedium:   inheritedMedium,
		}
		if !item.UpdatedAt.IsZero() {
			entry.UpdatedAt = item.UpdatedAt.UTC().Format(time.RFC3339)
		}
		if located {
			entry.Province = location.Province
			entry.ProvinceLabel = location.ProvinceLabel
			entry.City = assetMapCityName(location)
		}
		if hasCountry {
			entry.Country = country
		}
		if itemScope == "overseas" {
			if city := normalizeAssetMapCity(assetMapOverseasCity(item.Location)); city == country {
				entry.City = ""
			} else {
				entry.City = city
			}
		}
		if !item.LastSeenAt.IsZero() {
			entry.LastSeenAt = item.LastSeenAt.UTC().Format(time.RFC3339)
		}
		if services := serviceIndex[item.Value]; services != nil {
			entry.Ports = make([]string, 0, len(services.ports))
			for port := range services.ports {
				entry.Ports = append(entry.Ports, port)
			}
			sort.Slice(entry.Ports, func(i, j int) bool { return assetMapPortLess(entry.Ports[i], entry.Ports[j]) })
			entry.Services = make([]string, 0, len(services.services))
			for service := range services.services {
				entry.Services = append(entry.Services, service)
			}
			sort.Strings(entry.Services)
			if len(entry.Services) > 4 {
				entry.Services = entry.Services[:4]
			}
		}
		results = append(results, entry)
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].CriticalCount != results[j].CriticalCount {
			return results[i].CriticalCount > results[j].CriticalCount
		}
		if results[i].HighCount != results[j].HighCount {
			return results[i].HighCount > results[j].HighCount
		}
		if results[i].MediumCount != results[j].MediumCount {
			return results[i].MediumCount > results[j].MediumCount
		}
		if results[i].LastSeenAt != results[j].LastSeenAt {
			return results[i].LastSeenAt > results[j].LastSeenAt
		}
		return results[i].Value < results[j].Value
	})

	total := len(results)
	start := (query.Page - 1) * query.PageSize
	if start > total {
		start = total
	}
	end := start + query.PageSize
	if end > total {
		end = total
	}
	return AssetMapAssetListData{
		Items:    results[start:end],
		Total:    total,
		Page:     query.Page,
		PageSize: query.PageSize,
	}
}

// appendSyntheticRiskIPs keeps risk visible when a scanner produced a finding
// for a public IP that never became a catalog asset (for example a URL-only
// webscan target). The synthetic item only carries the data needed by the map
// and the risk list; it is not persisted into the unified catalog.
func appendSyntheticRiskIPs(items []AssetCenterItem, riskIndex *findingRiskIndex) []AssetCenterItem {
	if riskIndex == nil || len(riskIndex.assets) == 0 {
		return items
	}
	existing := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.Type == "ip" {
			existing[strings.ToLower(strings.TrimSpace(item.Value))] = struct{}{}
		}
	}
	extra := make([]AssetCenterItem, 0)
	for key, counts := range riskIndex.assets {
		if counts.totalCritical()+counts.totalHigh()+counts.totalMedium() == 0 {
			continue
		}
		parts := strings.SplitN(key, "\x00", 2)
		if len(parts) != 2 || parts[0] != "ip" {
			continue
		}
		ip := strings.ToLower(strings.TrimSpace(parts[1]))
		if ip == "" {
			continue
		}
		if _, exists := existing[ip]; exists {
			continue
		}
		existing[ip] = struct{}{}
		now := time.Now().UTC()
		item := AssetCenterItem{
			Type:        "ip",
			Value:       ip,
			Status:      "active",
			FirstSeenAt: now,
			LastSeenAt:  now,
			UpdatedAt:   now,
		}
		populateAssetCenterDisplayFields(&item)
		extra = append(extra, item)
	}
	if len(extra) == 0 {
		return items
	}
	return append(items, extra...)
}
