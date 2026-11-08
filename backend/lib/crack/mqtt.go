package crack

import (
	"context"
	"fmt"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func MqttScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	flag, err := MqttUnauth(host)
	if flag && err == nil {
		if vulnCallback != nil {
			vulnCallback(VulnerabilityInfo{
				ID:       "mqtt unauthorized",
				Name:     "mqtt unauthorized",
				URL:      host,
				Type:     "MQTT",
				Severity: "HIGH",
			})

		}
		return
	}
	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[mqtt] User exits crack scanning")
			return
		}
		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}
		user := parts[0]
		pass := parts[1]
		flag, err := MqttConn(host, user, pass)
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "mqtt-weak-password",
					Name:     "mqtt weak password",
					URL:      host,
					Type:     "mqtt",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
				})

			}
			return
		}
	}
}

func MqttUnauth(host string) (bool, error) {
	opts := newMqttClientOptions(host)
	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(defaultCrackConnectTimeout) {
		return false, fmt.Errorf("mqtt connect timeout")
	}
	if token.Error() != nil {
		return false, token.Error()
	}
	client.Disconnect(250)
	return true, nil
}

func MqttConn(host, user, pass string) (bool, error) {
	opts := newMqttClientOptions(host).SetUsername(user).SetPassword(pass)
	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(defaultCrackConnectTimeout) {
		return false, fmt.Errorf("mqtt connect timeout")
	}
	if token.Error() != nil {
		return false, token.Error()
	}
	client.Disconnect(250)
	return true, nil
}

func newMqttClientOptions(host string) *mqtt.ClientOptions {
	return mqtt.NewClientOptions().
		AddBroker(fmt.Sprintf("tcp://%s", host)).
		SetConnectTimeout(defaultCrackConnectTimeout).
		SetAutoReconnect(false).
		SetConnectRetry(false)
}
