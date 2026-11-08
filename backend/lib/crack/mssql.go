package crack

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

func MssqlScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[mssql] User exits crack scanning")
			return
		}
		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}
		user := parts[0]
		pass := parts[1]

		flag, err := MssqlConn(host, user, pass)
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "mssql-weak-password",
					Name:     "mssql weak password",
					URL:      host,
					Type:     "Mssql",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
				})
			}
			return
		}
	}
}

func MssqlConn(host, user, pass string) (flag bool, err error) {
	flag = false
	Host, Port := strings.Split(host, ":")[0], strings.Split(host, ":")[1]
	dataSourceName := fmt.Sprintf("server=%s;user id=%s;password=%s;port=%v;encrypt=disable;timeout=%d", Host, user, pass, Port, int(defaultCrackConnectTimeout/time.Second))
	db, err := sql.Open("sqlserver", dataSourceName)
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
