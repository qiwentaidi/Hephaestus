package crack

import (
	"context"
	"strings"
	"time"

	"hephaestus-backend/lib/grdp/glog"
	"hephaestus-backend/lib/grdp/login"
)

func RdpScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	login.LogLever = glog.NONE

	timeoutSeconds := int64(defaultCrackConnectTimeout / time.Second)
	if timeoutSeconds <= 0 {
		timeoutSeconds = 3
	}

	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[rdp] User exits crack scanning")
			return
		}

		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}
		domain, user := parseRdpUser(parts[0])
		pass := parts[1]

		flag, err := RdpConn(host, domain, user, pass, timeoutSeconds)
		if flag && err == nil {
			if vulnCallback != nil {
				extract := user + "/" + pass
				if domain != "" {
					extract = domain + `\` + extract
				}
				vulnCallback(VulnerabilityInfo{
					ID:       "rdp-weak-password",
					Name:     "rdp weak password",
					URL:      host,
					Type:     "RDP",
					Severity: "HIGH",
					Extract:  extract,
				})
			}
			return
		}
	}
}

func RdpConn(host, domain, user, pass string, timeoutSeconds int64) (bool, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 3
	}
	return login.NlaAuth(host, domain, user, pass, timeoutSeconds)
}

func parseRdpUser(raw string) (domain, user string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}

	if parts := strings.SplitN(raw, `\`, 2); len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	if parts := strings.SplitN(raw, "/", 2); len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	if parts := strings.SplitN(raw, "@", 2); len(parts) == 2 {
		return strings.TrimSpace(parts[1]), strings.TrimSpace(parts[0])
	}
	return "", raw
}
