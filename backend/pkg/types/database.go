package types

// DatabaseConnection 数据库连接信息
type DatabaseConnection struct {
	Nanoid        string `json:"nanoid"`        // 唯一标识符
	Scheme        string `json:"scheme"`        // 数据库类型: mysql, mssql, oracle, postgres, mongodb, redis
	Host          string `json:"host"`          // 主机地址
	Port          int    `json:"port"`          // 端口号
	Username      string `json:"username"`      // 用户名
	Password      string `json:"password"`      // 密码
	ServerName    string `json:"serverName"`    // 服务器名（Oracle 使用）
	Notes         string `json:"notes"`         // 备注
	DatabaseCount int    `json:"databaseCount"` // 数据库数量
	TableCount    int    `json:"tableCount"`    // 表数量
	UpdatedAt     string `json:"updatedAt"`     // 更新时间
}

// RowData 表数据信息
type RowData struct {
	Columns   []string        `json:"columns"`   // 列名
	Rows      [][]interface{} `json:"rows"`      // 行数据（前3行）
	RowsCount int             `json:"rowsCount"` // 总行数
}

// DatabaseInfo 数据库信息（数据库名 -> 表列表）
type DatabaseInfo map[string][]string

// TableInfo 表信息请求
type TableInfoRequest struct {
	Nanoid     string `json:"nanoid"`     // 连接标识符
	SchemaName string `json:"schemaName"` // 数据库/模式名
	TableName  string `json:"tableName"`  // 表名
}

// CollectResult 采集结果
type CollectResult struct {
	ID               string          `json:"id"`                  // 唯一标识符
	ConnectionNanoid string          `json:"connectionNanoid"`    // 连接标识符
	TableName        string          `json:"tableName"`           // 表名（格式: dbName.tableName）
	Columns          []string        `json:"columns"`             // 采样列名
	Rows             [][]interface{} `json:"rows"`                // 采样值，不包含可执行 HTML
	DataError        string          `json:"dataError,omitempty"` // 无法转换的历史记录仍保留元信息
	RowsCount        int             `json:"rowsCount"`           // 总行数
	MatchedType      string          `json:"matchedType"`         // 匹配类型
	CreatedAt        string          `json:"createdAt"`           // 创建时间
}

// CollectCustomRule 数据库信息采集自定义行匹配规则
type CollectCustomRule struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Pattern string `json:"pattern"`
}
