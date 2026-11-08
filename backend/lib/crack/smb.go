package crack

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/stacktitan/smb/smb"
)

func SmbScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[smb] User exits crack scanning")
			return
		}
		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}
		user := parts[0]
		pass := parts[1]

		flag, err := doWithTimeOut(host, user, pass)
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "smb-weak-password",
					Name:     "smb weak password",
					URL:      host,
					Type:     "SMB",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
				})
			}
			return
		}
	}
}

func SmblConn(host, user, pass string, signal chan struct{}) (flag bool, err error) {
	flag = false
	Host := strings.Split(host, ":")[0]
	Port, _ := strconv.Atoi(strings.Split(host, ":")[1])
	options := smb.Options{
		Host:        Host,
		Port:        Port,
		User:        user,
		Password:    pass,
		Domain:      "",
		Workstation: "",
	}
	session, err := smb.NewSession(options, false)
	if err == nil {
		session.Close()
		if session.IsAuthenticated {
			flag = true
		}
	}
	signal <- struct{}{}
	return flag, err
}

func doWithTimeOut(host, user, pass string) (flag bool, err error) {
	signal := make(chan struct{})
	go func() {
		flag, err = SmblConn(host, user, pass, signal)
	}()
	select {
	case <-signal:
		return flag, err
	case <-time.After(10 * time.Second):
		return false, errors.New("time out")
	}
}
