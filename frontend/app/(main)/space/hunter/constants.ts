import type { SyntaxCategory } from './types'

export const DEFAULT_PAGE_SIZE = 10

export const hunterSyntaxData: SyntaxCategory[] = [
	{
		title: 'IP',
		data: [
			{ key: 'ip="192.0.2.10"', description: '搜索IP为 "192.0.2.10"的资产' },
			{ key: 'ip="198.51.100.0/24"', description: '搜索网段为"198.51.100.0"的C段资产' },
			{ key: 'ip.port="80"', description: '搜索开放端口为"80"的资产' },
			{ key: 'ip.country="中国" 或 ip.country="CN"', description: '搜索IP对应主机所在国为"中国"的资产' },
			{ key: 'ip.province="江苏"', description: '搜索IP对应主机在江苏省的资产' },
			{ key: 'ip.city="北京"', description: '搜索IP对应主机所在城市为"北京"市的资产' },
			{ key: 'ip.isp="电信"', description: '搜索运营商为"中国电信"的资产' },
			{ key: 'ip.os="Windows"', description: '搜索操作系统标记为"Windows"的资产' },
			{ key: 'app="Hikvision 海康威视 Firmware 5.0+" && ip.ports="8000"', description: '检索使用了Hikvision且ip开放8000端口的资产' },
			{ key: 'ip.port_count>"2"', description: '搜索开放端口大于2的IP（支持等于、大于、小于）', hot: true },
			{ key: 'ip.ports="80" && ip.ports="443"', description: '查询开放了80和443端口号的资产' },
			{ key: 'ip.tag="CDN"', description: '查询包含IP标签"CDN"的资产', hot: true, characteristic: true },
		],
	},
	{
		title: 'domain域名',
		data: [
			{ key: 'is_domain=true', description: '搜索域名标记不为空的资产' },
			{ key: 'domain="qianxin.com"', description: '搜索域名包含"qianxin.com"的网站' },
			{ key: 'domain.suffix="qianxin.com"', description: '搜索主域为"qianxin.com"的网站', hot: true },
			{ key: 'domain.status="clientDeleteProhibited"', description: '搜索域名状态为"client Delete Prohibited"的网站' },
			{ key: 'domain.whois_server="whois.markmonitor.com"', description: '搜索whois服务器为"whois.markmonitor.com"的网站' },
			{ key: 'domain.name_server="ns1.qq.com"', description: '搜索名称服务器为"ns1.qq.com"的网站' },
			{ key: 'domain.created_date="2022-06-01"', description: '搜索域名创建时间为"2022-06-01"的网站' },
			{ key: 'domain.expires_date="2022-06-01"', description: '搜索域名到期时间为"2022-06-01"的网站' },
			{ key: 'domain.updated_date="2022-06-01"', description: '搜索域名更新时间为"2022-06-01"的网站' },
			{ key: 'domain.cname="a6c56dbcc1f22283.qaxanyu.com"', description: '搜索cname包含"a6c56dbcc1f22283.qaxanyu.com"的网站' },
			{ key: 'is_domain.cname=true', description: '搜索含有cname解析记录的网站' },
		],
	},
	{
		title: 'header响应头',
		data: [
			{ key: 'header.server=="Microsoft-IIS/10"', description: '搜索server全名为"Microsoft-IIS/10"的服务器' },
			{ key: 'header.content_length="691"', description: '搜索HTTP消息主体的大小为691的网站' },
			{ key: 'header.status_code="402"', description: '搜索HTTP请求返回状态码为"402"的资产' },
			{ key: 'header="elastic"', description: '搜索HTTP响应头中含有"elastic"的资产' },
		],
	},
	{
		title: 'web网站信息',
		data: [
			{ key: 'is_web=true', description: '搜索web资产', hot: true },
			{ key: 'web.title="北京"', description: '从网站标题中搜索"北京"' },
			{ key: 'web.body="网络空间测绘"', description: '搜索网站正文包含"网络空间测绘"的资产' },
			{ key: 'after="2021-01-01" && before="2021-12-31"', description: '搜索2021年的资产' },
			{ key: 'web.similar="baidu.com:443"', description: '查询与baidu.com:443网站的特征相似的资产', hot: true, characteristic: true },
			{ key: 'web.similar_icon=="17262739310191283300"', description: '查询网站icon与该icon相似的资产', hot: true, characteristic: true },
			{ key: 'web.icon="22eeab765346f14faf564a4709f98548"', description: '查询网站icon与该icon相同的资产', hot: true },
			{ key: 'web.similar_id="3322dfb483ea6fd250b29de488969b35"', description: '查询与该网页相似的资产', hot: true, characteristic: true },
			{ key: 'web.tag="登录页面"', description: '查询包含资产标签"登录页面"的资产', hot: true, characteristic: true },
		],
	},
	{
		title: 'icp备案信息',
		data: [
			{ key: 'icp.number="京ICP备16020626号-8"', description: '搜索通过域名关联的ICP备案号为"京ICP备16020626号-8"的网站资产' },
			{ key: 'icp.web_name="奇安信"', description: '搜索ICP备案网站名中含有"奇安信"的资产' },
			{ key: 'icp.name="奇安信"', description: '搜索ICP备案单位名中含有"奇安信"的资产' },
			{ key: 'icp.type="企业"', description: '搜索ICP备案主体为"企业"的资产' },
			{ key: 'icp.industry="软件和信息技术服务业"', description: '搜索ICP备案行业为"软件和信息技术服务业"的资产' },
			{ key: 'icp.is_exception=true', description: '搜索含有ICP备案异常的资产', characteristic: true },
		],
	},
	{
		title: 'protocol协议/端口响应',
		data: [
			{ key: 'protocol="http"', description: '搜索协议为"http"的资产' },
			{ key: 'protocol.transport="udp"', description: '搜索传输层协议为"udp"的资产' },
			{ key: 'protocol.banner="nginx"', description: '查询端口响应中包含"nginx"的资产' },
		],
	},
	{
		title: 'app组件信息',
		data: [
			{ key: 'app.name="小米 Router"', description: '搜索标记为"小米 Router"的资产' },
			{ key: 'app.type="开发与运维"', description: '查询包含组件分类为"开发与运维"的资产' },
			{ key: 'app.vendor="PHP"', description: '查询包含组件厂商为"PHP"的资产' },
			{ key: 'app.version="1.8.1"', description: '查询包含组件版本为"1.8.1"的资产' },
		],
	},
	{
		title: 'cert证书',
		data: [
			{ key: 'cert="baidu"', description: '搜索证书中带有baidu的资产' },
			{ key: 'cert.subject="qianxin.com"', description: '搜索证书使用者包含qianxin.com的资产' },
			{ key: 'cert.subject.suffix="qianxin.com"', description: '搜索证书使用者为qianxin.com的资产' },
			{ key: 'cert.subject_org="奇安信科技集团股份有限公司"', description: '搜索证书使用者组织是奇安信科技集团股份有限公司的资产' },
			{ key: 'cert.issuer="Let\'s Encrypt Authority X3"', description: '搜索证书颁发者是Let\'s Encrypt Authority X3的资产' },
			{ key: 'cert.issuer_org="Let\'s Encrypt"', description: '搜索证书颁发者组织是Let\'s Encrypt的资产' },
			{ key: 'cert.sha-1="be7605a3b72b60fcaa6c58b6896b9e2e7442ec50"', description: '搜索证书签名哈希算法sha1为be7605a3b72b60fcaa6c58b6896b9e2e7442ec50的资产' },
			{ key: 'cert.sha-256="4e529a65512029d77a28cbe694c7dad1e60f98b5cb89bf2aa329233acacc174e"', description: '搜索证书签名哈希算法sha256为4e529a65512029d77a28cbe694c7dad1e60f98b5cb89bf2aa329233acacc174e的资产' },
			{ key: 'cert.sha-md5="aeedfb3c1c26b90d08537523bbb16bf1"', description: '搜索证书签名哈希算法shamd5为aeedfb3c1c26b90d08537523bbb16bf1的资产' },
			{ key: 'cert.serial_number="35351242533515273557482149369"', description: '搜索证书序列号是35351242533515273557482149369的资产' },
			{ key: 'cert.is_expired=true', description: '搜索证书已过期的资产' },
			{ key: 'cert.is_trust=true', description: '搜索证书可信的资产', hot: true },
		],
	},
]
