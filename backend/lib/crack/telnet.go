package crack

import (
	"context"
	"strconv"
	"strings"

	"github.com/qiwentaidi/telnet"
)

func TelnetScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {

	h := strings.Split(host, ":")[0]
	p, _ := strconv.Atoi(strings.Split(host, ":")[1])
	serverType := getTelnetServerType(h, p)
	if serverType == telnet.UnauthorizedAccess {
		if vulnCallback != nil {
			vulnCallback(VulnerabilityInfo{
				ID:       "telnet-unauthorized",
				Name:     "telnet unauthorized",
				URL:      host,
				Type:     "Telnet",
				Severity: "HIGH",
			})
		}
		return
	}
	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[telnet] User exits crack scanning")
			return
		}
		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}
		user := parts[0]
		pass := parts[1]

		flag, err := TelnetConn(h, user, pass, p, serverType)
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "telnet-weak-password",
					Name:     "telnet weak password",
					URL:      host,
					Type:     "Telnet",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
				})
			}
			return
		}
	}
}

func getTelnetServerType(ip string, port int) int {
	client := telnet.New(ip, port)
	err := client.Connect()
	if err != nil {
		return telnet.Closed
	}
	defer client.Close()
	return client.MakeServerType()
}

func TelnetUnauth(addr string, port int) (bool, error) {
	serverType := getTelnetServerType(addr, port)
	if serverType == telnet.Closed {
		return false, nil
	}
	return serverType == telnet.UnauthorizedAccess, nil
}

func TelnetConn(addr, user, pass string, port, serverType int) (flag bool, err error) {
	flag = false
	client := telnet.New(addr, port)
	err = client.Connect()
	if err != nil {
		return flag, err
	}
	defer client.Close()
	client.UserName = user
	client.Password = pass
	client.ServerType = serverType
	err = client.Login()
	if err != nil {
		return flag, err
	}
	flag = true
	return flag, err
}
