package types

// SpaceOption 空间引擎聚合搜索选项
type SpaceOption struct {
	FofaApi       string `json:"fofaApi"`
	FofaEmail     string `json:"fofaEmail"`
	FofaKey       string `json:"fofaKey"`
	HunterKey     string `json:"hunterKey"`
	QuakeKey      string `json:"quakeKey"`
	CensysToken   string `json:"censysToken"`
	CensysOrgID   string `json:"censysOrgId"`
	ZoomeyeApi    string `json:"zoomeyeApi"` // ZoomEye API 地址
	ZoomeyeKey    string `json:"zoomeyeKey"`
	ShodanApi     string `json:"shodanApi"` // Shodan API 地址
	ShodanKey     string `json:"shodanKey"`
	EnableFofa    bool   `json:"enableFofa"`    // 是否启用 FOFA
	EnableHunter  bool   `json:"enableHunter"`  // 是否启用 Hunter
	EnableQuake   bool   `json:"enableQuake"`   // 是否启用 Quake
	EnableCensys  bool   `json:"enableCensys"`  // 是否启用 Censys
	EnableZoomeye bool   `json:"enableZoomeye"` // 是否启用 ZoomEye
	EnableShodan  bool   `json:"enableShodan"`  // 是否启用 Shodan
	MaxPages      int    `json:"maxPages"`      // 最大查询页数，默认为1
}

// SpaceSearchRequest 空间引擎聚合搜索请求
type SpaceSearchRequest struct {
	Query  string      `json:"query" binding:"required"` // 查询内容
	Type   string      `json:"type" binding:"required"`  // 查询类型：IP、域名、标题、备案号、备案名称
	Option SpaceOption `json:"option"`                   // 空间引擎选项
}

// SpaceSearchResult 空间引擎聚合搜索结果
type SpaceSearchResult struct {
	Results []SpaceResult `json:"results"`
	Total   int           `json:"total"`
	Message string        `json:"message"`
}

// SpaceResult 单个搜索结果
type SpaceResult struct {
	URL        string `json:"URL"`
	IP         string `json:"IP"`
	Domain     string `json:"Domain"`
	Port       string `json:"Port"`
	Protocol   string `json:"Protocol"`
	Title      string `json:"Title"`
	Components string `json:"Components"`
	ICP        string `json:"ICP"`
	ICPName    string `json:"ICPName"`
	Source     string `json:"Source"`
}
