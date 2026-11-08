package crack

import (
	"context"
	"net/url"
	"sort"
	"strings"
)

var scanners = map[string]crackFunc{
	"ftp":        FtpScan,
	"ssh":        SshScan,
	"rdp":        RdpScan,
	"telnet":     TelnetScan,
	"smb":        SmbScan,
	"oracle":     OracleScan,
	"dameng":     DamengScan,
	"pop3":       Pop3Scan,
	"smtp":       SmtpScan,
	"mssql":      MssqlScan,
	"mysql":      MysqlScan,
	"postgresql": PostgresScan,
	"mongodb":    MongodbScan,
	"ldap":       LdapScan,
	"mqtt":       MqttScan,
	"rabbitmq":   RabbitMQScan,
	"socks5":     Socks5Scan,
	"vnc":        VncScan,
	"redis":      RedisScan,
	"memcached":  MemcachedScan,
	"jdwp":       JdwpScan,
	"adb":        AdbScan,
	"java-rmi":   RmiScan,
	"activemq":   ActiveMQScan,
	"rsync":      RsyncScan,
	"kafka":      KafkaScan,
	"clickhouse": ClickHouseScan,
	"zookeeper":  ZookeeperScan,
}

// RegisterScanner registers or overrides a protocol crack implementation.
func RegisterScanner(protocol string, scanFunc crackFunc) {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" || scanFunc == nil {
		return
	}
	scanners[protocol] = scanFunc
}

// SupportedProtocols returns the registered crack protocol list.
func SupportedProtocols() []string {
	protocols := make([]string, 0, len(scanners))
	for proto := range scanners {
		protocols = append(protocols, proto)
	}
	sort.Strings(protocols)
	return protocols
}

// RunWithContext dispatches a crack scan for a protocol://host:port target.
func RunWithContext(ctx context.Context, target string, credentials []string, vulnCallback VulnerabilityCallback) {
	if ctx.Err() != nil {
		return
	}

	u, err := url.Parse(target)
	if err != nil {
		errorf("parse target error: %s", err)
		return
	}

	protocol := strings.ToLower(strings.TrimSpace(u.Scheme))
	scanFunc, ok := scanners[protocol]
	if !ok {
		warningf("unsupported protocol: %s", protocol)
		return
	}

	dial := dialContextFromContext(ctx)
	if dial == nil {
		if proxyURL := proxyURLFromContext(ctx); proxyURL != "" {
			if built, err := NewProxyDialContext(proxyURL); err == nil {
				dial = built
			}
		}
	}
	if dial != nil && protocolUsesProxyRelay(protocol) {
		relay, err := newTCPRelay(ctx, u.Host, dial)
		if err != nil {
			errorf("create proxy relay for %s failed: %v", u.Host, err)
			return
		}
		defer relay.Close()
		relayHost := relay.Address()
		infof("crack target %s via proxy relay %s", u.Host, relayHost)
		callback := wrapProxyRelayCallback(vulnCallback, u.Host, relayHost)
		scanFunc(ctx, relayHost, credentials, callback)
		if protocol == "smb" {
			_ = MS17010(relayHost, callback)
		}
		return
	}

	scanFunc(ctx, u.Host, credentials, vulnCallback)

	switch protocol {
	case "smb":
		_ = MS17010(u.Host, vulnCallback)
	}
}

// protocolUsesProxyRelay reports whether a protocol should be tunneled through
// the loopback relay. Protocols that can natively apply the proxy dialer (or
// that need the real host for HTTP/Kafka/Mongo semantics) are excluded.
func protocolUsesProxyRelay(protocol string) bool {
	switch protocol {
	case "ftp", "kafka", "mongodb", "clickhouse", "rabbitmq", "jdwp":
		return false
	default:
		return true
	}
}

func wrapProxyRelayCallback(callback VulnerabilityCallback, originalHost, relayHost string) VulnerabilityCallback {
	if callback == nil {
		return nil
	}
	if relayHost == "" || relayHost == originalHost {
		return callback
	}
	return func(v VulnerabilityInfo) {
		v.URL = strings.ReplaceAll(v.URL, relayHost, originalHost)
		callback(v)
	}
}

// Run dispatches a crack scan without caller-supplied context.
func Run(target string, credentials []string, vulnCallback VulnerabilityCallback) {
	RunWithContext(context.Background(), target, credentials, vulnCallback)
}
