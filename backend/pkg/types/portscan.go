package types

// PortInfo 端口扫描结果结构体
type PortInfo struct {
	Host            string
	Port            int
	Transport       string
	Protocol        string
	Product         string
	Version         string
	RawResponse     string
	DiscoveryStatus string
}
