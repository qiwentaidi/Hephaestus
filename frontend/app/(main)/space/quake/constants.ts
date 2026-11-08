import type { SyntaxItem, SyntaxCategory } from './types'

export const DEFAULT_PAGE_SIZE = 10

// 布尔逻辑语法（单独展示）
export const booleanLogicData: SyntaxItem[] = [
	{ key: 'port:"80" AND app:" 群晖NAS"', description: '查询开放80端口的"Synology NAS"应用资产' },
	{ key: 'title:" 管理系统" OR body:" 登录页"', description: '查询网页标题包含"management system"或网页正文包含"登录页"的网站' },
	{ key: 'title:"登录" AND(NOT title:"管理系统")', description: '查询网页标题包含"login"但不包含"管理系统"的网站' },
]

// Quake 语法提示数据（排除布尔逻辑）
export const quakeOptions: SyntaxCategory[] = [
	{
		title: "基本信息",
		data: [
			{ key: 'ip:1.1.1.1/22', description: '基本信息' },
			{ key: 'is_ipv6:true', description: '查询IPv6数据' },
			{ key: 'port:80', description: '查询开放80端口的资产' },
			{ key: 'port:[50 TO 60]', description: '查询开放端口在50至60之间的资产' },
			{ key: 'transport:udp', description: '查询udp数据' },
			{ key: 'domain:google.com', description: '查询资产域名' },
			{ key: 'asn:12345', description: '查询自治域号码为"12345"的资产' },
			{ key: 'org:No.31,Jin-rong Street', description: '查询自治域归属组织为"No.31,Jin-rong Street"的资产' },
			{ key: 'hostname:unifiedlayer.com', description: '查询主机名包含"unifiedlayer.com"的资产' }
		]
	},
	{
		title: "服务数据",
		data: [
			{ key: 'service:http', description: '查询服务名称为"http"的资产' },
			{ key: 'app:Apache', description: '查询应用名称为"Apache"的资产' },
			{ key: 'response:奇虎科技', description: '查询端口原生返回数据中包含"奇虎科技"的资产' },
			{ key: 'cert:奇虎科技', description: '查询包含"奇虎科技"的证书资产' },
			{ key: 'catalog:IoT物联网', description: '查询应用类别为"IoT物联网"的资产', isVip: true },
			{ key: 'type:VPN OR 防火墙', description: '查询应用类型为"VPN 或 防火墙"的资产', isVip: true },
			{ key: 'level:硬件设备层', description: '查询应用层级为"硬件设备层"', isVip: true },
			{ key: 'vendor:Sangfor OR 微软', description: '查询应用生产商为"Sangfor 或 微软"的资产', isVip: true }
		]
	},
	{
		title: "IP归属与定位",
		data: [
			{ key: 'country:China', description: '搜索国家，可使用简写CN' },
			{ key: 'country_cn:中国', description: '搜索中文国家名称' },
			{ key: 'province:Sichuan', description: '搜索英文省份名称' },
			{ key: 'province_cn:四川', description: '搜索中文省份名称' },
			{ key: 'city:Chengdu', description: '搜索英文城市名称' },
			{ key: 'city_cn:成都', description: '搜索中文城市名称' },
			{ key: 'owner:tencent.com', description: '搜索IP归属单位' },
			{ key: 'isp:amazon.com', description: '搜索IP归属运营商' },
		]
	},
	{
		title: "网页深度识别",
		data: [
			{ key: 'icp:京ICP备08010314号', description: '查询ICP备案号', isVip: true },
			{ key: 'icp_nature:企业', description: '查询备案主体性质', isVip: true },
			{ key: 'icp_keywords:奇虎', description: '查询备案网站中的关键词或域名', isVip: true },
			{ key: 'link_url:/login', description: '查询网页url中包含"/login"的资产', isVip: true },
			{ key: 'script_variable:csrfToken', description: '查询script标签变量中包含"csrfToken"的资产', isVip: true },
			{ key: 'script_function:indexOf', description: '查询script标签函数中包含"indexOf"的资产', isVip: true },
			{ key: 'css_class:login', description: '查询css标签class选择器中包含"login"的资产', isVip: true },
			{ key: 'css_id:login', description: '查询css标签id选择器中包含"login"的资产', isVip: true },
			{ key: 'iframe_title:企业微信登录', description: '查询iframe链接标题中包含"Enterprise wechat login"的资产', isVip: true },
			{ key: 'iframe_url:/wwopen/sso/v1/', description: '查询iframe链接中包含"/wwopen/sso/v1/"的资产', isVip: true },
			{ key: 'url_load:/login', description: '查询网页加载流中包含"/login"的资产', isVip: true },
			{ key: 'page_type:门户网站', description: '查询类型是"门户网站"的资产', isVip: true }
		]
	}
]
