package crack

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/sijms/go-ora/v2"
)

const defaultOracleServerName = "orcl"

func OracleScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[oracle] User exits crack scanning")
			return
		}
		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}
		user := parts[0]
		pass := parts[1]
		flag, err := OracleConn(host, defaultOracleServerName, user, pass)
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "oracle-weak-password",
					Name:     "oracle weak password",
					URL:      host,
					Type:     "Oracle",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
				})
			}
			return
		}
	}
}

func OracleConn(host, servername, user, pass string) (flag bool, err error) {
	flag = false
	dataSourceName := fmt.Sprintf("oracle://%s:%s@%s/%s", user, pass, host, servername)
	db, err := sql.Open("oracle", dataSourceName)
	if err == nil {
		db.SetConnMaxLifetime(defaultCrackConnectTimeout)
		db.SetConnMaxIdleTime(defaultCrackConnectTimeout)
		db.SetMaxIdleConns(0)
		defer db.Close()
		pingCtx, cancel := context.WithTimeout(context.Background(), defaultCrackConnectTimeout)
		defer cancel()
		err = db.PingContext(pingCtx)
		if err == nil {
			flag = true
		}
	}
	return flag, err
}
