package portscan

import (
	"context"

	"hephaestus-backend/lib/crack"
	"hephaestus-backend/pkg/types"
)

func wrapCrackCallback(taskId string, cb VulnerabilityCallback) crack.VulnerabilityCallback {
	if cb == nil {
		return nil
	}
	return func(vuln crack.VulnerabilityInfo) {
		cb(types.VulnerabilityInfo{
			TaskId:             taskId,
			ID:                 vuln.ID,
			Name:               vuln.Name,
			Description:        vuln.Description,
			Reference:          vuln.Reference,
			Type:               vuln.Type,
			Severity:           vuln.Severity,
			URL:                vuln.URL,
			Request:            vuln.Request,
			Response:           vuln.Response,
			ResponseTime:       vuln.ResponseTime,
			Extract:            vuln.Extract,
			PrincipleDetection: vuln.PrincipleDetection,
		})
	}
}

func ActiveMQScan(ctx context.Context, taskId, address string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.ActiveMQScan(ctx, address, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func ActiveMQConn(address, user, pass string) (bool, error) {
	return crack.ActiveMQConn(address, user, pass)
}

func AdbScan(ctx context.Context, taskId, address string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.AdbScan(ctx, address, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func FtpScan(ctx context.Context, taskId, address string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.FtpScan(ctx, address, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func FtpConn(address, user, pass string) (bool, []string, error) {
	return crack.FtpConn(address, user, pass)
}

func JdwpScan(ctx context.Context, taskId, address string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.JdwpScan(ctx, address, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func KafkaScan(ctx context.Context, taskId, address string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.KafkaScan(ctx, address, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func KafkaConn(address, user, pass string) (bool, error) {
	return crack.KafkaConn(address, user, pass)
}

func ClickHouseScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.ClickHouseScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func ClickHouseConn(host, user, pass string) (bool, error) {
	return crack.ClickHouseConn(host, user, pass)
}

func ClickHouseUnauth(host string) (bool, error) {
	return crack.ClickHouseUnauth(host)
}

func LdapScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.LdapScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func Ldapconn(host, user, pass string) (bool, error) {
	return crack.Ldapconn(host, user, pass)
}

func MemcachedScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.MemcachedScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func MongodbScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.MongodbScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func MongodbConn(host, user, pass string) (bool, error) {
	return crack.MongodbConn(host, user, pass)
}

func MqttScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.MqttScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func MqttUnauth(host string) (bool, error) {
	return crack.MqttUnauth(host)
}

func MqttConn(host, user, pass string) (bool, error) {
	return crack.MqttConn(host, user, pass)
}

func RabbitMQScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.RabbitMQScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func RabbitMQUnauth(host string) (bool, string, error) {
	return crack.RabbitMQUnauth(host)
}

func RabbitMQConn(host, user, pass string) (bool, error) {
	return crack.RabbitMQConn(host, user, pass)
}

func MS17010(taskId, host string, vulnCallback VulnerabilityCallback) error {
	return crack.MS17010(host, wrapCrackCallback(taskId, vulnCallback))
}

func MS17010Scan(taskId, host string, vulnCallback VulnerabilityCallback) error {
	return crack.MS17010Scan(host, wrapCrackCallback(taskId, vulnCallback))
}

func MssqlScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.MssqlScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func MssqlConn(host, user, pass string) (bool, error) {
	return crack.MssqlConn(host, user, pass)
}

func MysqlScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.MysqlScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func MysqlConn(host, user, pass string) (bool, error) {
	return crack.MysqlConn(host, user, pass)
}

func OracleScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.OracleScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func OracleConn(host, servername, user, pass string) (bool, error) {
	return crack.OracleConn(host, servername, user, pass)
}

func Pop3Scan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.Pop3Scan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func Pop3Conn(host, user, pass string) (bool, string, error) {
	return crack.Pop3Conn(host, user, pass)
}

func SmtpScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.SmtpScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func SmtpUnauth(host string) (bool, string, error) {
	return crack.SmtpUnauth(host)
}

func SmtpConn(host, user, pass string) (bool, string, error) {
	return crack.SmtpConn(host, user, pass)
}

func PostgresScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.PostgresScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func PostgresConn(host, user, pass string) (bool, error) {
	return crack.PostgresConn(host, user, pass)
}

func RedisScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.RedisScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func RedisConn(address, password string) (bool, error) {
	return crack.RedisConn(address, password)
}

func RedisUnauth(address string) (bool, error) {
	return crack.RedisUnauth(address)
}

func RdpScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.RdpScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func RdpConn(host, domain, user, pass string, timeoutSeconds int64) (bool, error) {
	return crack.RdpConn(host, domain, user, pass, timeoutSeconds)
}

func RmiScan(ctx context.Context, taskId, address string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.RmiScan(ctx, address, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func RsyncScan(ctx context.Context, taskId, address string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.RsyncScan(ctx, address, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func RsyncConn(address, user, pass string) (bool, string, error) {
	return crack.RsyncConn(address, user, pass)
}

func SmbScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.SmbScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func SmblConn(host, user, pass string, signal chan struct{}) (bool, error) {
	return crack.SmblConn(host, user, pass, signal)
}

func Socks5Scan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.Socks5Scan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func Socks5Conn(ip string, port, timeout int, username, password, aliveURL string) bool {
	return crack.Socks5Conn(ip, port, timeout, username, password, aliveURL)
}

func SshScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.SshScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func SshConn(host, user, pass string) (bool, error) {
	return crack.SshConn(host, user, pass)
}

func ExecuteSshCommand(host, username, password, command string) (string, error) {
	return crack.ExecuteSshCommand(host, username, password, command)
}

func TelnetScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.TelnetScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func TelnetUnauth(addr string, port int) (bool, error) {
	return crack.TelnetUnauth(addr, port)
}

func TelnetConn(addr, user, pass string, port, serverType int) (bool, error) {
	return crack.TelnetConn(addr, user, pass, port, serverType)
}

func VncScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.VncScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func VncConn(host, pass string) (bool, error) {
	return crack.VncConn(host, pass)
}

func ZookeeperScan(ctx context.Context, taskId, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	crack.ZookeeperScan(ctx, host, credentials, wrapCrackCallback(taskId, vulnCallback))
}

func ZookeeperUnauth(host string) (bool, string, error) {
	return crack.ZookeeperUnauth(host)
}
