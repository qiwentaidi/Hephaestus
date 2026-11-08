package types

// DirsearchOption 目录扫描选项
type DirsearchOption struct {
	Method                 string   `json:"method"`                 // HTTP 方法 (GET, POST, HEAD, OPTIONS)
	URLs                   []string `json:"urls"`                   // 要扫描的 URL 列表
	Paths                  []string `json:"paths"`                  // 字典路径列表（如果为空，将使用字典文件路径）
	DictPaths              []string `json:"dictPaths"`              // 字典文件路径列表（用于加载字典）
	Exts                   string   `json:"exts"`                   // 扩展名列表，逗号分隔（如: php,aspx,asp），用于替换 %EXT%
	Workers                int      `json:"workers"`                // 并发线程数
	Timeout                int      `json:"timeout"`                // 超时时间（秒）
	BodyExclude            string   `json:"bodyExclude"`            // 过滤响应体内容
	BodyLengthExcludeTimes int      `json:"bodyLengthExcludeTimes"` // 过滤响应包长度数据次数
	StatusCodeExclude      []int    `json:"statusCodeExclude"`      // 排除的状态码列表
	Redirect               bool     `json:"redirect"`               // 是否跟随重定向
	Interval               int      `json:"interval"`               // 请求间隔（秒）
	CustomHeader           string   `json:"customHeader"`           // 自定义请求头（每行一个键值对）
	Recursion              int      `json:"recursion"`              // 递归层级
	Backupscan             bool     `json:"backupscan"`             // 是否启用备份文件扫描
	Proxy                  string   `json:"proxy"`                  // 代理地址（如: http://127.0.0.1:8080 或 socks5://127.0.0.1:1080）
}
