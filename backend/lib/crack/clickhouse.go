package crack

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func ClickHouseScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	flag, err := clickHouseQueryContext(ctx, host, "", "", false)
	if flag && err == nil {
		if vulnCallback != nil {
			vulnCallback(VulnerabilityInfo{
				ID:       "clickhouse-unauthorized",
				Name:     "clickhouse unauthorized",
				URL:      host,
				Type:     "ClickHouse",
				Severity: "HIGH",
			})
		}
		return
	}

	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[clickhouse] User exits crack scanning")
			return
		}

		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}
		user := parts[0]
		pass := parts[1]

		flag, err := clickHouseQueryContext(ctx, host, user, pass, true)
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "clickhouse-weak-password",
					Name:     "clickhouse weak password",
					URL:      host,
					Type:     "ClickHouse",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
				})
			}
			return
		}
	}
}

func ClickHouseUnauth(host string) (bool, error) {
	return clickHouseQueryContext(context.Background(), host, "", "", false)
}

func ClickHouseConn(host, user, pass string) (bool, error) {
	return clickHouseQueryContext(context.Background(), host, user, pass, true)
}

func clickHouseQueryContext(ctx context.Context, host, user, pass string, useAuth bool) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, defaultCrackConnectTimeout+defaultCrackIOTimeout)
	defer cancel()

	target := url.URL{
		Scheme:   "http",
		Host:     host,
		Path:     "/",
		RawQuery: "query=SELECT+1",
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return false, err
	}
	if useAuth {
		req.SetBasicAuth(user, pass)
	}

	client := newHTTPClient(ctx, defaultCrackConnectTimeout+defaultCrackIOTimeout)
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return false, err
	}

	if resp.StatusCode == http.StatusOK && strings.TrimSpace(string(body)) == "1" {
		return true, nil
	}
	return false, fmt.Errorf("clickhouse query failed: status=%d", resp.StatusCode)
}
