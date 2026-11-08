package crack

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
)

func RabbitMQScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	flag, response, err := rabbitMQUnauthContext(ctx, host)
	if flag && err == nil {
		if vulnCallback != nil {
			vulnCallback(VulnerabilityInfo{
				ID:       "rabbitmq-unauthorized",
				Name:     "rabbitmq unauthorized",
				URL:      host,
				Type:     "RabbitMQ",
				Severity: "HIGH",
				Response: response,
			})
		}
		return
	}

	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[rabbitmq] User exits crack scanning")
			return
		}

		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}
		user := parts[0]
		pass := parts[1]

		flag, err := rabbitMQConnContext(ctx, host, user, pass)
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "rabbitmq-weak-password",
					Name:     "rabbitmq weak password",
					URL:      host,
					Type:     "RabbitMQ",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
				})
			}
			return
		}
	}
}

func RabbitMQUnauth(address string) (bool, string, error) {
	return rabbitMQUnauthContext(context.Background(), address)
}

func rabbitMQUnauthContext(ctx context.Context, address string) (bool, string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return false, "", err
	}
	if port == "5672" {
		return false, "", fmt.Errorf("rabbitmq amqp port does not support management unauthorized check")
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, defaultCrackConnectTimeout+defaultCrackIOTimeout)
	defer cancel()

	target := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, port),
		Path:   "/api/overview",
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return false, "", err
	}

	client := newHTTPClient(ctx, defaultCrackConnectTimeout+defaultCrackIOTimeout)
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return false, "", err
	}
	response := string(body)
	if resp.StatusCode == http.StatusOK &&
		(strings.Contains(response, "rabbitmq_version") || strings.Contains(response, "management_version")) {
		return true, response, nil
	}
	return false, response, fmt.Errorf("rabbitmq management unauthorized check failed: status=%d", resp.StatusCode)
}

func RabbitMQConn(address, user, pass string) (bool, error) {
	return rabbitMQConnContext(context.Background(), address, user, pass)
}

func rabbitMQConnContext(ctx context.Context, address, user, pass string) (bool, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return false, err
	}
	if _, err := strconv.Atoi(port); err != nil {
		return false, err
	}
	if port != "5672" {
		return rabbitMQHTTPAuthContext(ctx, address, user, pass)
	}

	amqpURL := url.URL{
		Scheme: "amqp",
		User:   url.UserPassword(user, pass),
		Host:   net.JoinHostPort(host, port),
		Path:   "/",
	}
	conn, err := amqp.DialConfig(amqpURL.String(), amqp.Config{
		Dial: func(network, addr string) (net.Conn, error) {
			return DialContext(ctx, network, addr)
		},
	})
	if err != nil {
		return false, err
	}
	defer conn.Close()
	return true, nil
}

func rabbitMQHTTPAuth(address, user, pass string) (bool, error) {
	return rabbitMQHTTPAuthContext(context.Background(), address, user, pass)
}

func rabbitMQHTTPAuthContext(ctx context.Context, address, user, pass string) (bool, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return false, err
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, defaultCrackConnectTimeout+defaultCrackIOTimeout)
	defer cancel()

	target := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, port),
		Path:   "/api/overview",
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return false, err
	}
	req.SetBasicAuth(user, pass)

	client := newHTTPClient(ctx, defaultCrackConnectTimeout+defaultCrackIOTimeout)
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return false, err
	}
	response := string(body)
	if resp.StatusCode == http.StatusOK &&
		(strings.Contains(response, "rabbitmq_version") || strings.Contains(response, "management_version")) {
		return true, nil
	}
	return false, fmt.Errorf("rabbitmq management auth failed: status=%d", resp.StatusCode)
}
