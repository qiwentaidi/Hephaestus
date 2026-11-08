package crack

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

func PostgresScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[postgres] User exits crack scanning")
			return
		}
		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}
		user := parts[0]
		pass := parts[1]
		flag, err := PostgresConn(host, user, pass)
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "postgres-weak-password",
					Name:     "postgres weak password",
					URL:      host,
					Type:     "Postgres",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
				})
			}
			return
		}
	}
}

func PostgresConn(host, user, pass string) (flag bool, err error) {
	flag = false
	dataSourceName := fmt.Sprintf("postgres://%v:%v@%v/postgres?sslmode=disable&connect_timeout=%d", user, pass, host, int(defaultCrackConnectTimeout/time.Second))
	db, err := sql.Open("postgres", dataSourceName)
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
