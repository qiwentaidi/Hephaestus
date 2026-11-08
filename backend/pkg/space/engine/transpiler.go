package engine

import (
	"regexp"
	"strings"
)

// QueryType 查询类型
type QueryType string

const (
	QueryTypeDefault QueryType = "default" // 默认查询

	// 基础字段
	QueryTypeIP           QueryType = "ip"            // IP 地址
	QueryTypePort         QueryType = "port"          // 端口
	QueryTypeProtocol     QueryType = "protocol"      // 协议名
	QueryTypeBaseProtocol QueryType = "base_protocol" // 基础协议（tcp/udp）
	QueryTypeHost         QueryType = "host"          // 主机名
	QueryTypeDomain       QueryType = "domain"        // 域名

	// 地理位置
	QueryTypeCountry     QueryType = "country"      // 国家代码
	QueryTypeCountryName QueryType = "country_name" // 国家名
	QueryTypeRegion      QueryType = "region"       // 区域/省份
	QueryTypeCity        QueryType = "city"         // 城市

	// 网络信息
	QueryTypeASN QueryType = "asn" // ASN 编号
	QueryTypeOrg QueryType = "org" // ASN 组织
	QueryTypeISP QueryType = "isp" // 运营商

	// 网站信息
	QueryTypeTitle  QueryType = "title"  // 网站标题
	QueryTypeServer QueryType = "server" // 网站 Server
	QueryTypeHeader QueryType = "header" // 网站 Header
	QueryTypeBanner QueryType = "banner" // 协议 Banner
	QueryTypeBody   QueryType = "body"   // 网站正文内容
	QueryTypeLink   QueryType = "link"   // 资产 URL 链接

	// ICP 备案（区分备案号和备案名称）
	QueryTypeICP       QueryType = "icp"        // ICP 备案（通用，默认为备案号）
	QueryTypeICPNumber QueryType = "icp.number" // ICP 备案号
	QueryTypeICPName   QueryType = "icp.name"   // ICP 备案名称

	// 证书相关
	QueryTypeCert           QueryType = "cert"             // 证书（通用）
	QueryTypeCertIssuerOrg  QueryType = "cert.issuer.org"  // 证书颁发者组织
	QueryTypeCertIssuerCN   QueryType = "cert.issuer.cn"   // 证书颁发者通用名称
	QueryTypeCertSubjectOrg QueryType = "cert.subject.org" // 证书持有者组织
	QueryTypeCertSubjectCN  QueryType = "cert.subject.cn"  // 证书持有者通用名称
	QueryTypeCertDomain     QueryType = "cert.domain"      // 证书中的根域名

	// 指纹相关
	QueryTypeApp        QueryType = "app"         // 应用/组件/产品
	QueryTypeProduct    QueryType = "product"     // 产品名
	QueryTypeOS         QueryType = "os"          // 操作系统
	QueryTypeJarm       QueryType = "jarm"        // JARM 指纹
	QueryTypeIconHash   QueryType = "icon_hash"   // Icon Hash
	QueryTypeHeaderHash QueryType = "header_hash" // Header Hash
	QueryTypeBannerHash QueryType = "banner_hash" // Banner Hash
	QueryTypeFid        QueryType = "fid"         // FID 指纹
)

// SyntaxRule 语法规则映射
type SyntaxRule struct {
	Fofa    string // FOFA 语法
	Hunter  string // Hunter 语法
	Quake   string // Quake 语法
	Censys  string // Censys 语法
	Zoomeye string // ZoomEye 语法
}

// Transpiler 查询语法转换引擎
// 将统一的元语法（meta syntax）转换为各引擎特定的查询语法
type Transpiler struct {
	// 语法映射表
	syntaxMap map[QueryType]SyntaxRule
}

// NewTranspiler 创建新的查询转换器
func NewTranspiler() *Transpiler {
	t := &Transpiler{
		syntaxMap: make(map[QueryType]SyntaxRule),
	}
	t.initSyntaxMap()
	return t
}

// initSyntaxMap 初始化语法映射表
func (t *Transpiler) initSyntaxMap() {
	// IP 查询
	t.syntaxMap[QueryTypeIP] = SyntaxRule{
		Fofa:    `ip="%s"`,
		Hunter:  `ip="%s"`,
		Quake:   `ip:"%s"`,
		Censys:  `host.ip=%s`,
		Zoomeye: `ip:"%s"`,
	}

	// 端口查询
	t.syntaxMap[QueryTypePort] = SyntaxRule{
		Fofa:    `port="%s"`,
		Hunter:  `ip.port="%s"`,
		Quake:   `port:%s`,
		Censys:  `host.services.port=%s`,
		Zoomeye: `port:%s`,
	}

	// 协议查询
	t.syntaxMap[QueryTypeProtocol] = SyntaxRule{
		Fofa:    `protocol="%s"`,
		Hunter:  `protocol="%s"`,
		Quake:   `service:"%s"`,
		Censys:  `host.services.service_name=%s`,
		Zoomeye: `service:"%s"`,
	}

	// 基础协议查询（tcp/udp）
	t.syntaxMap[QueryTypeBaseProtocol] = SyntaxRule{
		Fofa:   `base_protocol="%s"`,
		Hunter: `base_protocol="%s"`,
		Quake:  `transport:"%s"`,
		Censys: `host.services.transport_protocol=%s`,
	}

	// 主机名查询
	t.syntaxMap[QueryTypeHost] = SyntaxRule{
		Fofa:    `host="%s"`,
		Hunter:  `ip="%s"`, // Hunter 用 ip 代替
		Quake:   `hostname:"%s"`,
		Censys:  `host.name=%s`,
		Zoomeye: `hostname:"%s"`,
	}

	// 域名查询
	t.syntaxMap[QueryTypeDomain] = SyntaxRule{
		Fofa:    `domain="%s"`,
		Hunter:  `domain.suffix="%s"`,
		Quake:   `domain:"%s"`,
		Censys:  `host.dns.names=%s`,
		Zoomeye: `domain:"%s"`,
	}

	// 国家代码查询
	t.syntaxMap[QueryTypeCountry] = SyntaxRule{
		Fofa:    `country="%s"`,
		Hunter:  `ip.country="%s"`,
		Quake:   `country:"%s"`,
		Censys:  `host.location.country_code=%s`,
		Zoomeye: `country:"%s"`,
	}

	// 国家名查询
	t.syntaxMap[QueryTypeCountryName] = SyntaxRule{
		Fofa:   `country_name="%s"`,
		Hunter: `ip.country="%s"`,
		Quake:  `country:"%s"`,
		Censys: `host.location.country=%s`,
	}

	// 区域查询
	t.syntaxMap[QueryTypeRegion] = SyntaxRule{
		Fofa:    `region="%s"`,
		Hunter:  `ip.province="%s"`,
		Quake:   `province:"%s"`,
		Censys:  `host.location.province=%s`,
		Zoomeye: `subdivisions:"%s"`,
	}

	// 城市查询
	t.syntaxMap[QueryTypeCity] = SyntaxRule{
		Fofa:    `city="%s"`,
		Hunter:  `ip.city="%s"`,
		Quake:   `city:"%s"`,
		Censys:  `host.location.city=%s`,
		Zoomeye: `city:"%s"`,
	}

	// ASN 编号查询
	t.syntaxMap[QueryTypeASN] = SyntaxRule{
		Fofa:    `asn="%s"`,
		Hunter:  `ip.asn="%s"`,
		Quake:   `asn:%s`,
		Censys:  `host.autonomous_system.asn=%s`,
		Zoomeye: `asn:%s`,
	}

	// ASN 组织查询
	t.syntaxMap[QueryTypeOrg] = SyntaxRule{
		Fofa:    `org="%s"`,
		Hunter:  `ip.org="%s"`,
		Quake:   `org:"%s"`,
		Censys:  `host.autonomous_system.organization=%s`,
		Zoomeye: `org:"%s"`,
	}

	// ISP 查询
	t.syntaxMap[QueryTypeISP] = SyntaxRule{
		Fofa:    `org="%s"`, // FOFA 没有单独的 ISP 字段
		Hunter:  `ip.isp="%s"`,
		Quake:   `isp:"%s"`,
		Censys:  `host.autonomous_system.organization=%s`,
		Zoomeye: `isp:"%s"`,
	}

	// 标题查询
	t.syntaxMap[QueryTypeTitle] = SyntaxRule{
		Fofa:    `title="%s"`,
		Hunter:  `web.title="%s"`,
		Quake:   `title:"%s"`,
		Censys:  `host.services.http.response.html_title=%s`,
		Zoomeye: `title:"%s"`,
	}

	// Server 查询
	t.syntaxMap[QueryTypeServer] = SyntaxRule{
		Fofa:    `server="%s"`,
		Hunter:  `web.server="%s"`,
		Quake:   `server:"%s"`,
		Censys:  `host.services.http.response.headers.server=%s`,
		Zoomeye: `server:"%s"`,
	}

	// Header 查询
	t.syntaxMap[QueryTypeHeader] = SyntaxRule{
		Fofa:    `header="%s"`,
		Hunter:  `header="%s"`,
		Quake:   `header:"%s"`,
		Censys:  `host.services.http.response.headers=%s`,
		Zoomeye: `header:"%s"`,
	}

	// Banner 查询
	t.syntaxMap[QueryTypeBanner] = SyntaxRule{
		Fofa:    `banner="%s"`,
		Hunter:  `protocol.banner="%s"`,
		Quake:   `response:"%s"`,
		Censys:  `host.services.banner=%s`,
		Zoomeye: `banner:"%s"`,
	}

	// Body 查询
	t.syntaxMap[QueryTypeBody] = SyntaxRule{
		Fofa:    `body="%s"`,
		Hunter:  `web.body="%s"`,
		Quake:   `body:"%s"`,
		Censys:  `host.services.http.response.body=%s`,
		Zoomeye: `body:"%s"`,
	}

	// Link 查询
	t.syntaxMap[QueryTypeLink] = SyntaxRule{
		Fofa:    `link="%s"`,
		Hunter:  `url="%s"`,
		Quake:   `url:"%s"`,
		Censys:  "", // Censys 不支持
		Zoomeye: `site:"%s"`,
	}

	// ICP 备案号查询（FOFA 只支持备案号）
	t.syntaxMap[QueryTypeICP] = SyntaxRule{
		Fofa:   `icp="%s"`,
		Hunter: `icp.number="%s"`,
		Quake:  `icp:"%s"`,
		Censys: "", // Censys 不支持
	}

	// ICP 备案号查询（明确指定）
	t.syntaxMap[QueryTypeICPNumber] = SyntaxRule{
		Fofa:   `icp="%s"`, // FOFA 只支持备案号
		Hunter: `icp.number="%s"`,
		Quake:  `icp:"%s"`,
		Censys: "", // Censys 不支持
	}

	// ICP 备案名称查询
	t.syntaxMap[QueryTypeICPName] = SyntaxRule{
		Fofa:   "", // FOFA 不支持备案名称查询
		Hunter: `icp.name="%s"`,
		Quake:  `icp_keywords:"%s"`,
		Censys: "", // Censys 不支持
	}

	// 证书查询（通用）
	t.syntaxMap[QueryTypeCert] = SyntaxRule{
		Fofa:    `cert="%s"`,
		Hunter:  `cert="%s"`,
		Quake:   `cert:"%s"`,
		Censys:  `host.services.tls.certificates.leaf.names=%s`,
		Zoomeye: `ssl:"%s"`,
	}

	// 证书颁发者组织
	t.syntaxMap[QueryTypeCertIssuerOrg] = SyntaxRule{
		Fofa:   `cert.issuer.org="%s"`,
		Hunter: `cert.issuer.org="%s"`,
		Quake:  `cert.issuer.org:"%s"`,
		Censys: `host.services.tls.certificates.leaf.issuer.organization=%s`,
	}

	// 证书颁发者通用名称
	t.syntaxMap[QueryTypeCertIssuerCN] = SyntaxRule{
		Fofa:   `cert.issuer.cn="%s"`,
		Hunter: `cert.issuer.cn="%s"`,
		Quake:  `cert.issuer.cn:"%s"`,
		Censys: `host.services.tls.certificates.leaf.issuer.common_name=%s`,
	}

	// 证书持有者组织
	t.syntaxMap[QueryTypeCertSubjectOrg] = SyntaxRule{
		Fofa:   `cert.subject.org="%s"`,
		Hunter: `cert.subject.org="%s"`,
		Quake:  `cert.subject.org:"%s"`,
		Censys: `host.services.tls.certificates.leaf.subject.organization=%s`,
	}

	// 证书持有者通用名称
	t.syntaxMap[QueryTypeCertSubjectCN] = SyntaxRule{
		Fofa:   `cert.subject.cn="%s"`,
		Hunter: `cert.subject.cn="%s"`,
		Quake:  `cert.subject.cn:"%s"`,
		Censys: `host.services.tls.certificates.leaf.subject.common_name=%s`,
	}

	// 证书域名
	t.syntaxMap[QueryTypeCertDomain] = SyntaxRule{
		Fofa:   `cert.domain="%s"`,
		Hunter: `cert.domain="%s"`,
		Quake:  `cert.domain:"%s"`,
		Censys: `host.services.tls.certificates.leaf.names=%s`,
	}

	// 应用/产品查询
	t.syntaxMap[QueryTypeApp] = SyntaxRule{
		Fofa:    `app="%s"`,
		Hunter:  `app.name="%s"`,
		Quake:   `app:"%s"`,
		Censys:  `host.services.software.product="%s"`,
		Zoomeye: `app="%s"`,
	}

	// 产品名查询
	t.syntaxMap[QueryTypeProduct] = SyntaxRule{
		Fofa:    `product="%s"`,
		Hunter:  `app.name="%s"`,
		Quake:   `app:"%s"`,
		Censys:  `host.services.software.product="%s"`,
		Zoomeye: `app="%s"`,
	}

	// 操作系统查询
	t.syntaxMap[QueryTypeOS] = SyntaxRule{
		Fofa:    `os="%s"`,
		Hunter:  `ip.os="%s"`,
		Quake:   `os:"%s"`,
		Censys:  `host.services.software.os=%s`,
		Zoomeye: `os:"%s"`,
	}

	// JARM 指纹查询
	t.syntaxMap[QueryTypeJarm] = SyntaxRule{
		Fofa:    `jarm="%s"`,
		Hunter:  `tls.jarm="%s"`,
		Quake:   `jarm:"%s"`,
		Censys:  `host.services.tls.ja3s=%s`,
		Zoomeye: `ssl.jarm:"%s"`,
	}

	// Icon Hash 查询
	t.syntaxMap[QueryTypeIconHash] = SyntaxRule{
		Fofa:    `icon_hash="%s"`,
		Hunter:  `web.icon="%s"`,
		Quake:   `favicon.hash:"%s"`,
		Censys:  "", // Censys 不支持
		Zoomeye: `iconhash_md5:"%s"`,
	}

	// Header Hash 查询
	t.syntaxMap[QueryTypeHeaderHash] = SyntaxRule{
		Fofa:   `header_hash="%s"`,
		Hunter: "", // Hunter 不支持
		Quake:  "", // Quake 不支持
		Censys: "", // Censys 不支持
	}

	// Banner Hash 查询
	t.syntaxMap[QueryTypeBannerHash] = SyntaxRule{
		Fofa:   `banner_hash="%s"`,
		Hunter: "", // Hunter 不支持
		Quake:  "", // Quake 不支持
		Censys: "", // Censys 不支持
	}

	// FID 查询
	t.syntaxMap[QueryTypeFid] = SyntaxRule{
		Fofa:   `fid="%s"`,
		Hunter: "", // Hunter 不支持
		Quake:  "", // Quake 不支持
		Censys: "", // Censys 不支持
	}
}

// TranslateQuery 将元语法查询转换为特定引擎的语法
// metaQuery: 元语法查询，如 "ip:1.1.1.1 AND port:80"
// targetEngine: 目标引擎名称（fofa/hunter/quake/censys/zoomeye）
// queryType: 查询类型（用于简单查询）
func (t *Transpiler) TranslateQuery(metaQuery string, targetEngine string, queryType QueryType) string {
	query, _ := t.TranslateQueryChecked(metaQuery, targetEngine, queryType)
	return query
}

// formatSimpleQuery 格式化简单查询
func (t *Transpiler) formatSimpleQuery(value string, engine string, queryType QueryType) string {
	rule, ok := t.syntaxMap[queryType]
	if !ok {
		// 未知查询类型，直接返回原值
		return value
	}

	var format string
	switch engine {
	case "fofa":
		format = rule.Fofa
	case "hunter":
		format = rule.Hunter
	case "quake":
		format = rule.Quake
	case "censys":
		format = rule.Censys
	case "zoomeye":
		format = rule.Zoomeye
	default:
		return value
	}

	if format == "" {
		// 该引擎不支持此查询类型
		return value
	}

	return strings.Replace(format, "%s", value, 1)
}

// translateComplexQuery 转换复杂查询
// 支持的元语法格式：
//   - ip:1.1.1.1
//   - domain:example.com
//   - title:"关键词"
//   - ip:1.1.1.1 AND port:80
//   - (title:xxx OR body:xxx) AND country:CN
func (t *Transpiler) translateComplexQuery(query string, engine string) string {
	// 定义元语法到引擎语法的正则替换规则
	replacements := t.getReplacementRules(engine)

	result := query
	for metaPattern, enginePattern := range replacements {
		re := regexp.MustCompile(metaPattern)
		result = re.ReplaceAllString(result, enginePattern)
	}

	return result
}

// getReplacementRules 获取正则替换规则
func (t *Transpiler) getReplacementRules(engine string) map[string]string {
	rules := make(map[string]string)

	switch engine {
	case "fofa":
		// 基础字段
		rules[`\bip:([^\s"]+)`] = `ip="$1"`
		rules[`\bip:"([^"]+)"`] = `ip="$1"`
		rules[`\bport:(\d+)`] = `port="$1"`
		rules[`\bprotocol:([^\s"]+)`] = `protocol="$1"`
		rules[`\bhost:([^\s"]+)`] = `host="$1"`
		rules[`\bdomain:([^\s"]+)`] = `domain="$1"`
		rules[`\bdomain:"([^"]+)"`] = `domain="$1"`

		// 地理位置
		rules[`\bcountry:([^\s"]+)`] = `country="$1"`
		rules[`\bregion:([^\s"]+)`] = `region="$1"`
		rules[`\bcity:([^\s"]+)`] = `city="$1"`

		// 网络信息
		rules[`\basn:(\d+)`] = `asn="$1"`
		rules[`\borg:([^\s"]+)`] = `org="$1"`
		rules[`\borg:"([^"]+)"`] = `org="$1"`

		// 网站信息
		rules[`\btitle:([^\s"]+)`] = `title="$1"`
		rules[`\btitle:"([^"]+)"`] = `title="$1"`
		rules[`\bserver:([^\s"]+)`] = `server="$1"`
		rules[`\bserver:"([^"]+)"`] = `server="$1"`
		rules[`\bheader:([^\s"]+)`] = `header="$1"`
		rules[`\bheader:"([^"]+)"`] = `header="$1"`
		rules[`\bbanner:([^\s"]+)`] = `banner="$1"`
		rules[`\bbanner:"([^"]+)"`] = `banner="$1"`
		rules[`\bbody:([^\s"]+)`] = `body="$1"`
		rules[`\bbody:"([^"]+)"`] = `body="$1"`

		// ICP 备案（FOFA 只支持备案号）
		rules[`\bicp:([^\s"]+)`] = `icp="$1"`
		rules[`\bicp:"([^"]+)"`] = `icp="$1"`
		rules[`\bicp\.number:([^\s"]+)`] = `icp="$1"`
		rules[`\bicp\.name:([^\s"]+)`] = `$0` // FOFA 不支持，保持原样

		// 证书相关
		rules[`\bcert:([^\s"]+)`] = `cert="$1"`
		rules[`\bcert:"([^"]+)"`] = `cert="$1"`
		rules[`\bcert\.issuer\.org:([^\s"]+)`] = `cert.issuer.org="$1"`
		rules[`\bcert\.issuer\.cn:([^\s"]+)`] = `cert.issuer.cn="$1"`
		rules[`\bcert\.subject\.org:([^\s"]+)`] = `cert.subject.org="$1"`
		rules[`\bcert\.subject\.cn:([^\s"]+)`] = `cert.subject.cn="$1"`

		// 指纹
		rules[`\bapp:([^\s"]+)`] = `app="$1"`
		rules[`\bapp:"([^"]+)"`] = `app="$1"`
		rules[`\bproduct:([^\s"]+)`] = `product="$1"`
		rules[`\bproduct:"([^"]+)"`] = `product="$1"`
		rules[`\bos:([^\s"]+)`] = `os="$1"`
		rules[`\bjarm:([^\s"]+)`] = `jarm="$1"`
		rules[`\bicon_hash:([^\s"]+)`] = `icon_hash="$1"`

		// 逻辑运算符
		rules[`\bAND\b`] = `&&`
		rules[`\bOR\b`] = `||`
		rules[`\bNOT\b`] = `!=`

	case "hunter":
		// 基础字段
		rules[`\bip:([^\s"]+)`] = `ip="$1"`
		rules[`\bip:"([^"]+)"`] = `ip="$1"`
		rules[`\bport:(\d+)`] = `ip.port="$1"`
		rules[`\bprotocol:([^\s"]+)`] = `protocol="$1"`
		rules[`\btransport:([^\s"]+)`] = `protocol.transport="$1"`
		rules[`\bdomain:([^\s"]+)`] = `domain.suffix="$1"`
		rules[`\bdomain:"([^"]+)"`] = `domain.suffix="$1"`

		// 地理位置
		rules[`\bcountry:([^\s"]+)`] = `ip.country="$1"`
		rules[`\bregion:([^\s"]+)`] = `ip.province="$1"`
		rules[`\bcity:([^\s"]+)`] = `ip.city="$1"`

		// 网络信息
		rules[`\basn:(\d+)`] = `ip.asn="$1"`
		rules[`\borg:([^\s"]+)`] = `ip.org="$1"`
		rules[`\borg:"([^"]+)"`] = `ip.org="$1"`
		rules[`\bisp:([^\s"]+)`] = `ip.isp="$1"`

		// 网站信息
		rules[`\btitle:([^\s"]+)`] = `web.title="$1"`
		rules[`\btitle:"([^"]+)"`] = `web.title="$1"`
		rules[`\bserver:([^\s"]+)`] = `header.server="$1"`
		rules[`\bserver:"([^"]+)"`] = `header.server="$1"`
		rules[`\bheader:([^\s"]+)`] = `header="$1"`
		rules[`\bheader:"([^"]+)"`] = `header="$1"`
		rules[`\bbanner:([^\s"]+)`] = `protocol.banner="$1"`
		rules[`\bbanner:"([^"]+)"`] = `protocol.banner="$1"`
		rules[`\bbody:([^\s"]+)`] = `web.body="$1"`
		rules[`\bbody:"([^"]+)"`] = `web.body="$1"`

		// ICP 备案（Hunter 支持备案号和备案名称）
		rules[`\bicp:([^\s"]+)`] = `icp.number="$1"`
		rules[`\bicp:"([^"]+)"`] = `icp.number="$1"`
		rules[`\bicp\.number:([^\s"]+)`] = `icp.number="$1"`
		rules[`\bicp\.name:([^\s"]+)`] = `icp.name="$1"`
		rules[`\bicp\.name:"([^"]+)"`] = `icp.name="$1"`

		// 证书相关
		rules[`\bcert:([^\s"]+)`] = `cert="$1"`
		rules[`\bcert:"([^"]+)"`] = `cert="$1"`
		rules[`\bcert\.issuer_org:([^\s"]+)`] = `cert.issuer_org="$1"`
		rules[`\bcert\.issuer_org:"([^"]+)"`] = `cert.issuer_org="$1"`
		rules[`\bcert\.subject_org:([^\s"]+)`] = `cert.subject_org="$1"`
		rules[`\bcert\.subject_org:"([^"]+)"`] = `cert.subject_org="$1"`

		// 指纹
		rules[`\bapp:([^\s"]+)`] = `app.name="$1"`
		rules[`\bapp:"([^"]+)"`] = `app.name="$1"`
		rules[`\bproduct:([^\s"]+)`] = `app.name="$1"`
		rules[`\bproduct:"([^"]+)"`] = `app.name="$1"`
		rules[`\bos:([^\s"]+)`] = `ip.os="$1"`

		// 逻辑运算符
		rules[`\bAND\b`] = `&&`
		rules[`\bOR\b`] = `||`

	case "quake":
		// 基础字段
		rules[`\bip:([^\s"]+)`] = `ip:"$1"`
		rules[`\bip:"([^"]+)"`] = `ip:"$1"`
		rules[`\bport:(\d+)`] = `port:$1`
		rules[`\bprotocol:([^\s"]+)`] = `service:"$1"`
		rules[`\btransport:([^\s"]+)`] = `transport:"$1"`
		rules[`\bhost:([^\s"]+)`] = `hostname:"$1"`
		rules[`\bdomain:([^\s"]+)`] = `domain:"$1"`
		rules[`\bdomain:"([^"]+)"`] = `domain:"$1"`

		// 地理位置（Quake 支持中英文）
		rules[`\bcountry:([^\s"]+)`] = `country:"$1"`
		rules[`\bcountry_cn:([^\s"]+)`] = `country_cn:"$1"`
		rules[`\bregion:([^\s"]+)`] = `province:"$1"`
		rules[`\bprovince:([^\s"]+)`] = `province:"$1"`
		rules[`\bprovince_cn:([^\s"]+)`] = `province_cn:"$1"`
		rules[`\bcity:([^\s"]+)`] = `city:"$1"`
		rules[`\bcity_cn:([^\s"]+)`] = `city_cn:"$1"`

		// 网络信息
		rules[`\basn:(\d+)`] = `asn:$1`
		rules[`\borg:([^\s"]+)`] = `org:"$1"`
		rules[`\borg:"([^"]+)"`] = `org:"$1"`
		rules[`\bisp:([^\s"]+)`] = `isp:"$1"`
		rules[`\bowner:([^\s"]+)`] = `owner:"$1"`

		// 网站信息
		rules[`\btitle:([^\s"]+)`] = `title:"$1"`
		rules[`\btitle:"([^"]+)"`] = `title:"$1"`
		rules[`\bserver:([^\s"]+)`] = `server:"$1"`
		rules[`\bserver:"([^"]+)"`] = `server:"$1"`
		rules[`\bheader:([^\s"]+)`] = `header:"$1"`
		rules[`\bheader:"([^"]+)"`] = `header:"$1"`
		rules[`\bbanner:([^\s"]+)`] = `response:"$1"`
		rules[`\bbanner:"([^"]+)"`] = `response:"$1"`
		rules[`\bresponse:([^\s"]+)`] = `response:"$1"`
		rules[`\bresponse:"([^"]+)"`] = `response:"$1"`
		rules[`\bbody:([^\s"]+)`] = `body:"$1"`
		rules[`\bbody:"([^"]+)"`] = `body:"$1"`

		// ICP 备案
		rules[`\bicp:([^\s"]+)`] = `icp:"$1"`
		rules[`\bicp:"([^"]+)"`] = `icp:"$1"`
		rules[`\bicp\.number:([^\s"]+)`] = `icp:"$1"`
		rules[`\bicp\.name:([^\s"]+)`] = `icp_keywords:"$1"`
		rules[`\bicp\.name:"([^"]+)"`] = `icp_keywords:"$1"`
		rules[`\bicp_keywords:([^\s"]+)`] = `icp_keywords:"$1"`
		rules[`\bicp_nature:([^\s"]+)`] = `icp_nature:"$1"`

		// 证书相关
		rules[`\bcert:([^\s"]+)`] = `cert:"$1"`
		rules[`\bcert:"([^"]+)"`] = `cert:"$1"`

		// 指纹
		rules[`\bapp:([^\s"]+)`] = `app:"$1"`
		rules[`\bapp:"([^"]+)"`] = `app:"$1"`
		rules[`\bproduct:([^\s"]+)`] = `app:"$1"`
		rules[`\bproduct:"([^"]+)"`] = `app:"$1"`
		rules[`\bservice:([^\s"]+)`] = `service:"$1"`
		rules[`\bos:([^\s"]+)`] = `os:"$1"`
		rules[`\bjarm:([^\s"]+)`] = `jarm:"$1"`
		rules[`\bcatalog:([^\s"]+)`] = `catalog:"$1"`
		rules[`\btype:([^\s"]+)`] = `type:"$1"`
		rules[`\blevel:([^\s"]+)`] = `level:"$1"`
		rules[`\bvendor:([^\s"]+)`] = `vendor:"$1"`
		// Quake 使用 AND/OR 语法，保持不变

	case "censys":
		// 基础字段
		rules[`\bip:([^\s"]+)`] = `host.ip=$1`
		rules[`\bip:"([^"]+)"`] = `host.ip="$1"`
		rules[`\bport:(\d+)`] = `host.services.port=$1`
		rules[`\bprotocol:([^\s"]+)`] = `host.services.service_name=$1`
		rules[`\bdomain:([^\s"]+)`] = `host.dns.names=$1`
		rules[`\bdomain:"([^"]+)"`] = `host.dns.names="$1"`

		// 地理位置
		rules[`\bcountry:([^\s"]+)`] = `host.location.country_code=$1`
		rules[`\bregion:([^\s"]+)`] = `host.location.province=$1`
		rules[`\bcity:([^\s"]+)`] = `host.location.city=$1`

		// 网络信息
		rules[`\basn:(\d+)`] = `host.autonomous_system.asn=$1`
		rules[`\borg:([^\s"]+)`] = `host.autonomous_system.organization=$1`
		rules[`\borg:"([^"]+)"`] = `host.autonomous_system.organization="$1"`

		// 网站信息
		rules[`\btitle:([^\s"]+)`] = `host.services.http.response.html_title=$1`
		rules[`\btitle:"([^"]+)"`] = `host.services.http.response.html_title="$1"`
		rules[`\bserver:([^\s"]+)`] = `host.services.http.response.headers.server=$1`
		rules[`\bserver:"([^"]+)"`] = `host.services.http.response.headers.server="$1"`
		rules[`\bbody:([^\s"]+)`] = `host.services.http.response.body=$1`
		rules[`\bbody:"([^"]+)"`] = `host.services.http.response.body="$1"`

		// 证书
		rules[`\bcert:([^\s"]+)`] = `host.services.tls.certificates.leaf.names=$1`

		// 指纹
		rules[`\bapp:([^\s"]+)`] = `host.services.software.product="$1"`
		rules[`\bapp:"([^"]+)"`] = `host.services.software.product="$1"`
		rules[`\bproduct:([^\s"]+)`] = `host.services.software.product="$1"`
		rules[`\bproduct:"([^"]+)"`] = `host.services.software.product="$1"`

		// 逻辑运算符
		rules[`\bAND\b`] = `and`
		rules[`\bOR\b`] = `or`
		rules[`\bNOT\b`] = `not`

	case "zoomeye":
		rules[`\bip:([^\s"]+)`] = `ip:"$1"`
		rules[`\bip:"([^"]+)"`] = `ip:"$1"`
		rules[`\bport:(\d+)`] = `port:$1`
		rules[`\bprotocol:([^\s"]+)`] = `service:"$1"`
		rules[`\bhost:([^\s"]+)`] = `hostname:"$1"`
		rules[`\bhost:"([^"]+)"`] = `hostname:"$1"`
		rules[`\bdomain:([^\s"]+)`] = `domain:"$1"`
		rules[`\bdomain:"([^"]+)"`] = `domain:"$1"`
		rules[`\bcountry:([^\s"]+)`] = `country:"$1"`
		rules[`\bregion:([^\s"]+)`] = `subdivisions:"$1"`
		rules[`\bregion:"([^"]+)"`] = `subdivisions:"$1"`
		rules[`\bcity:([^\s"]+)`] = `city:"$1"`
		rules[`\bcity:"([^"]+)"`] = `city:"$1"`
		rules[`\basn:(\d+)`] = `asn:$1`
		rules[`\borg:([^\s"]+)`] = `org:"$1"`
		rules[`\borg:"([^"]+)"`] = `org:"$1"`
		rules[`\bisp:([^\s"]+)`] = `isp:"$1"`
		rules[`\bisp:"([^"]+)"`] = `isp:"$1"`
		rules[`\btitle:([^\s"]+)`] = `title:"$1"`
		rules[`\btitle:"([^"]+)"`] = `title:"$1"`
		rules[`\bserver:([^\s"]+)`] = `server:"$1"`
		rules[`\bserver:"([^"]+)"`] = `server:"$1"`
		rules[`\bheader:([^\s"]+)`] = `header:"$1"`
		rules[`\bheader:"([^"]+)"`] = `header:"$1"`
		rules[`\bbanner:([^\s"]+)`] = `banner:"$1"`
		rules[`\bbanner:"([^"]+)"`] = `banner:"$1"`
		rules[`\bbody:([^\s"]+)`] = `body:"$1"`
		rules[`\bbody:"([^"]+)"`] = `body:"$1"`
		rules[`\bcert:([^\s"]+)`] = `ssl:"$1"`
		rules[`\bcert:"([^"]+)"`] = `ssl:"$1"`
		rules[`\bapp:([^\s"]+)`] = `app="$1"`
		rules[`\bapp:"([^"]+)"`] = `app="$1"`
		rules[`\bproduct:([^\s"]+)`] = `app="$1"`
		rules[`\bproduct:"([^"]+)"`] = `app="$1"`
		rules[`\bos:([^\s"]+)`] = `os:"$1"`
		rules[`\bos:"([^"]+)"`] = `os:"$1"`
		rules[`\bjarm:([^\s"]+)`] = `ssl.jarm:"$1"`
		rules[`\bjarm:"([^"]+)"`] = `ssl.jarm:"$1"`
		rules[`\bicon_hash:([^\s"]+)`] = `iconhash_md5:"$1"`
		rules[`\bicon_hash:"([^"]+)"`] = `iconhash_md5:"$1"`
		rules[`\bAND\b`] = ` AND `
		rules[`\bOR\b`] = ` OR `
		rules[`\bNOT\b`] = ` NOT `

	case "shodan":
		// Shodan 使用空格表示 AND，不支持 OR；过滤器名与元语法存在差异。
		rules[`\bip:([^\s"]+)`] = `ip:$1`
		rules[`\bip:"([^"]+)"`] = `ip:$1`
		rules[`\bport:(\d+)`] = `port:$1`
		rules[`\bhost:([^\s"]+)`] = `hostname:$1`
		rules[`\bhost:"([^"]+)"`] = `hostname:"$1"`
		rules[`\bdomain:([^\s"]+)`] = `hostname:$1`
		rules[`\bdomain:"([^"]+)"`] = `hostname:"$1"`

		// 地理位置与网络信息
		rules[`\bcountry:([^\s"]+)`] = `country:"$1"`
		rules[`\bcountry:"([^"]+)"`] = `country:"$1"`
		rules[`\bcity:([^\s"]+)`] = `city:"$1"`
		rules[`\bcity:"([^"]+)"`] = `city:"$1"`
		rules[`\borg:([^\s"]+)`] = `org:"$1"`
		rules[`\borg:"([^"]+)"`] = `org:"$1"`
		rules[`\bisp:([^\s"]+)`] = `isp:"$1"`
		rules[`\bisp:"([^"]+)"`] = `isp:"$1"`
		rules[`\basn:(\d+)`] = `asn:$1`

		// HTTP 与指纹
		rules[`\btitle:([^\s"]+)`] = `http.title:"$1"`
		rules[`\btitle:"([^"]+)"`] = `http.title:"$1"`
		rules[`\bserver:([^\s"]+)`] = `http.headers:"$1"`
		rules[`\bserver:"([^"]+)"`] = `http.headers:"$1"`
		rules[`\bheader:([^\s"]+)`] = `http.headers:"$1"`
		rules[`\bheader:"([^"]+)"`] = `http.headers:"$1"`
		rules[`\bbody:([^\s"]+)`] = `http.html:"$1"`
		rules[`\bbody:"([^"]+)"`] = `http.html:"$1"`
		rules[`\bbanner:([^\s"]+)`] = `$1`
		rules[`\bbanner:"([^"]+)"`] = `"$1"`
		rules[`\bcert:([^\s"]+)`] = `ssl:"$1"`
		rules[`\bcert:"([^"]+)"`] = `ssl:"$1"`
		rules[`\bapp:([^\s"]+)`] = `product:"$1"`
		rules[`\bapp:"([^"]+)"`] = `product:"$1"`
		rules[`\bproduct:([^\s"]+)`] = `product:"$1"`
		rules[`\bproduct:"([^"]+)"`] = `product:"$1"`
		rules[`\bos:([^\s"]+)`] = `os:"$1"`
		rules[`\bos:"([^"]+)"`] = `os:"$1"`
		rules[`\bicon_hash:([^\s"]+)`] = `http.favicon.hash:$1`
		rules[`\bicon_hash:"([^"]+)"`] = `http.favicon.hash:$1`
		rules[`\bjarm:([^\s"]+)`] = `ssl.jarm:$1`

		// 逻辑运算符：Shodan 以空格表示 AND，使用 - 表示排除
		rules[`\bAND\b`] = ` `
		rules[`\bNOT\b`] = `-`
	}

	return rules
}

// DetectQueryType 从查询字符串检测查询类型
func (t *Transpiler) DetectQueryType(query string) QueryType {
	query = strings.TrimSpace(query)

	// 检测是否是复合查询
	if strings.Contains(query, " AND ") || strings.Contains(query, " OR ") ||
		strings.Contains(query, "&&") || strings.Contains(query, "||") {
		return QueryTypeDefault
	}

	// 检测是否包含字段前缀
	if strings.Contains(query, ":") || strings.Contains(query, "=") {
		return QueryTypeDefault
	}

	// 检测是否是纯 IP
	ipPattern := regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(/\d{1,2})?$`)
	if ipPattern.MatchString(query) {
		return QueryTypeIP
	}

	// 检测是否是域名
	domainPattern := regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	if domainPattern.MatchString(query) {
		return QueryTypeDomain
	}

	return QueryTypeDefault
}

// ValidateQuery 验证查询语法
func (t *Transpiler) ValidateQuery(query string) error {
	// TODO: 实现查询语法验证
	return nil
}

// QueryTypeFromString 从字符串转换为 QueryType
func QueryTypeFromString(s string) QueryType {
	switch strings.ToLower(s) {
	// 基础字段
	case "ip":
		return QueryTypeIP
	case "port":
		return QueryTypePort
	case "protocol":
		return QueryTypeProtocol
	case "base_protocol":
		return QueryTypeBaseProtocol
	case "host":
		return QueryTypeHost
	case "domain":
		return QueryTypeDomain

	// 地理位置
	case "country":
		return QueryTypeCountry
	case "country_name":
		return QueryTypeCountryName
	case "region", "province":
		return QueryTypeRegion
	case "city":
		return QueryTypeCity

	// 网络信息
	case "asn":
		return QueryTypeASN
	case "org":
		return QueryTypeOrg
	case "isp":
		return QueryTypeISP

	// 网站信息
	case "title":
		return QueryTypeTitle
	case "server":
		return QueryTypeServer
	case "header":
		return QueryTypeHeader
	case "banner":
		return QueryTypeBanner
	case "body":
		return QueryTypeBody
	case "link", "url":
		return QueryTypeLink

	// ICP 备案
	case "icp":
		return QueryTypeICP
	case "icp.number", "icp_number", "备案号":
		return QueryTypeICPNumber
	case "icp.name", "icp_name", "备案名称":
		return QueryTypeICPName

	// 证书相关
	case "cert":
		return QueryTypeCert
	case "cert.issuer.org":
		return QueryTypeCertIssuerOrg
	case "cert.issuer.cn":
		return QueryTypeCertIssuerCN
	case "cert.subject.org":
		return QueryTypeCertSubjectOrg
	case "cert.subject.cn":
		return QueryTypeCertSubjectCN
	case "cert.domain":
		return QueryTypeCertDomain

	// 指纹相关
	case "app", "应用", "组件":
		return QueryTypeApp
	case "product", "产品":
		return QueryTypeProduct
	case "os":
		return QueryTypeOS
	case "jarm":
		return QueryTypeJarm
	case "icon_hash":
		return QueryTypeIconHash
	case "header_hash":
		return QueryTypeHeaderHash
	case "banner_hash":
		return QueryTypeBannerHash
	case "fid":
		return QueryTypeFid

	default:
		return QueryTypeDefault
	}
}

// DefaultTranspiler 全局默认转换器实例
var DefaultTranspiler = NewTranspiler()
