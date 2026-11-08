package crack

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"

	_ "github.com/go-sql-driver/mysql"
)

const mysqlScanConcurrency = 8

func MysqlScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type mysqlCredential struct {
		user string
		pass string
	}

	taskChan := make(chan mysqlCredential)
	var wg sync.WaitGroup
	var successOnce sync.Once

	workerCount := mysqlScanConcurrency
	if len(credentials) > 0 && len(credentials) < workerCount {
		workerCount = len(credentials)
	}
	if workerCount == 0 {
		return
	}

	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for cred := range taskChan {
				if ctx.Err() != nil {
					return
				}

				flag, err := MysqlConn(host, cred.user, cred.pass)
				if flag && err == nil {
					successOnce.Do(func() {
						if vulnCallback != nil {
							vulnCallback(VulnerabilityInfo{
								ID:       "mysql-weak-password",
								Name:     "mysql weak password",
								URL:      host,
								Type:     "Mysql",
								Severity: "HIGH",
								Extract:  cred.user + "/" + cred.pass,
							})
						}
						cancel()
					})
				}
			}
		}()
	}

dispatchLoop:
	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[mysql] User exits crack scanning")
			break
		}

		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}

		select {
		case <-ctx.Done():
			break dispatchLoop
		case taskChan <- mysqlCredential{user: parts[0], pass: parts[1]}:
		}
	}

	close(taskChan)
	wg.Wait()
}

func MysqlConn(host, user, pass string) (flag bool, err error) {
	flag = false
	for _, database := range []string{"information_schema", "mysql"} {
		dataSourceName := fmt.Sprintf("%v:%v@tcp(%v)/%v?charset=utf8&timeout=%v&readTimeout=%v&writeTimeout=%v", user, pass, host, database, defaultCrackConnectTimeout, defaultCrackIOTimeout, defaultCrackIOTimeout)
		db, openErr := sql.Open("mysql", dataSourceName)
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
