package crack

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "gitee.com/chunanyong/dm"
)

func DamengScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[dameng] User exits crack scanning")
			return
		}

		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}
		user := parts[0]
		pass := parts[1]

		flag, err := DamengConn(host, user, pass)
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "dameng-weak-password",
					Name:     "dameng weak password",
					URL:      host,
					Type:     "Dameng",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
				})
			}
			return
		}
	}
}

func DamengConn(host, user, pass string) (flag bool, err error) {
	flag = false
	for _, dataSourceName := range []string{
		buildDamengDSN(host, user, pass, user),
		buildDamengDSN(host, user, pass, ""),
	} {
		db, openErr := sql.Open("dm", dataSourceName)
		if openErr != nil {
			err = openErr
			continue
		}

		db.SetConnMaxLifetime(defaultCrackConnectTimeout)
		db.SetConnMaxIdleTime(defaultCrackConnectTimeout)
		db.SetMaxIdleConns(0)

		pingCtx, cancel := context.WithTimeout(context.Background(), defaultCrackConnectTimeout)
		err = db.PingContext(pingCtx)
		cancel()
		db.Close()
		if err == nil {
			return true, nil
		}
	}
	return flag, err
}

func buildDamengDSN(host, user, pass, schema string) string {
	dsn := fmt.Sprintf(
		"dm://%s:%s@%s",
		url.PathEscape(user),
		url.PathEscape(pass),
		host,
	)
	if schema != "" {
		return dsn + "/" + url.PathEscape(schema)
	}
	return dsn
}
