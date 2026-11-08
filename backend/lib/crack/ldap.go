package crack

import (
	"context"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

func LdapScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[ldap] User exits crack scanning")
			return
		}
		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}
		user := parts[0]
		pass := parts[1]
		flag, err := Ldapconn(host, user, pass)
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "ldap-weak-password",
					Name:     "ldap weak password",
					URL:      host,
					Type:     "LDAP",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
				})

			}
			return
		}
	}
}

func Ldapconn(host, user, pass string) (bool, error) {
	conn, err := ldap.Dial("tcp", host)

	if err != nil {
		return false, err
	}

	err = conn.Bind(user, pass)
	if err != nil {
		return false, err
	}

	return true, nil
}
