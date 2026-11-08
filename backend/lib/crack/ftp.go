package crack

import (
	"context"
	"net"
	"strings"

	"github.com/jlaffaye/ftp"
)

func FtpScan(ctx context.Context, address string, credentials []string, vulnCallback VulnerabilityCallback) {
	flag, directories, err := ftpConn(ctx, address, "anonymous", "")
	if flag && err == nil {
		if vulnCallback != nil {
			vulnCallback(VulnerabilityInfo{
				ID:       "ftp-unauthorized",
				Name:     "ftp unauthorized",
				URL:      address,
				Type:     "FTP",
				Severity: "HIGH",
				Response: strings.Join(directories, "\n"),
			})
		}
		return
	}
	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[ftp] User exits crack scanning")
			return
		}
		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}
		user := parts[0]
		pass := parts[1]

		flag, directories, err := ftpConn(ctx, address, user, pass)
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "ftp weak password",
					Name:     "ftp weak password",
					URL:      address,
					Type:     "FTP",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
					Response: strings.Join(directories, "\n"),
				})
			}
			return
		}
	}
}

func FtpConn(address, user, pass string) (flag bool, directories []string, err error) {
	return ftpConn(context.Background(), address, user, pass)
}

func ftpConn(ctx context.Context, address, user, pass string) (flag bool, directories []string, err error) {
	options := []ftp.DialOption{
		ftp.DialWithTimeout(defaultCrackConnectTimeout),
		ftp.DialWithContext(ctx),
	}
	if dial := dialContextFromContext(ctx); dial != nil {
		options = append(options, ftp.DialWithDialFunc(func(network, addr string) (net.Conn, error) {
			return dial(ctx, network, addr)
		}))
	}
	conn, err := ftp.Dial(address, options...)
	if err != nil {
		return false, nil, err
	}
	defer func() {
		if conn != nil {
			conn.Quit()
		}
	}()

	err = conn.Login(user, pass)
	if err != nil {
		return false, nil, err
	}

	// 获取目录信息
	dirs, err := conn.List("")
	if err == nil && len(dirs) > 0 {
		directories = make([]string, 0, min(6, len(dirs)))
		for i := 0; i < len(dirs) && i < 6; i++ {
			name := dirs[i].Name
			if len(name) > 50 {
				name = name[:50]
			}
			directories = append(directories, name)
		}
	}

	return true, directories, nil
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
