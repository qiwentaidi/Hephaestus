// https://github.com/shadow1ng/fscan/blob/main/Plugins/Kafka.go
package crack

import (
	"context"
	"fmt"
	"strings"

	"github.com/IBM/sarama"
)

func KafkaScan(ctx context.Context, address string, credentials []string, vulnCallback VulnerabilityCallback) {
	flag, err := kafkaConn(ctx, address, "", "")
	if flag && err == nil {
		if vulnCallback != nil {
			vulnCallback(VulnerabilityInfo{
				ID:       "kafka-unauthorized",
				Name:     "kafka unauthorized",
				URL:      address,
				Type:     "Kafka",
				Severity: "HIGH",
				Extract:  "",
			})

		}
		return
	}
	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[kafka] User exits crack scanning")
			return
		}
		// 解析 user:pass 格式
		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}
		user := parts[0]
		pass := parts[1]
		flag, err := kafkaConn(ctx, address, user, pass)
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "kafka-weak-password",
					Name:     "kafka weak password",
					URL:      address,
					Type:     "Kafka",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
				})
			}
			return
		}
	}
}

// KafkaConn 尝试 Kafka 连接
func KafkaConn(address, user, pass string) (bool, error) {
	return kafkaConn(context.Background(), address, user, pass)
}

func kafkaConn(ctx context.Context, address, user, pass string) (bool, error) {
	config := sarama.NewConfig()
	config.Net.DialTimeout = defaultCrackConnectTimeout
	config.Net.ReadTimeout = defaultCrackIOTimeout
	config.Net.WriteTimeout = defaultCrackIOTimeout
	config.Net.TLS.Enable = false
	config.Version = sarama.V2_0_0_0
	if dial := dialContextFromContext(ctx); dial != nil {
		config.Net.Proxy.Enable = true
		config.Net.Proxy.Dialer = proxyDialerAdapter{dial: dial}
	}

	// 设置 SASL 配置
	if user != "" || pass != "" {
		config.Net.SASL.Enable = true
		config.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		config.Net.SASL.User = user
		config.Net.SASL.Password = pass
		config.Net.SASL.Handshake = true
	}

	brokers := []string{address}

	// 尝试作为消费者连接测试
	consumer, err := sarama.NewConsumer(brokers, config)
	if err == nil {
		defer consumer.Close()
		return true, nil
	}

	// 如果消费者连接失败，尝试作为客户端连接
	client, err := sarama.NewClient(brokers, config)
	if err == nil {
		defer client.Close()
		return true, nil
	}

	// 检查错误类型
	if strings.Contains(err.Error(), "SASL") ||
		strings.Contains(err.Error(), "authentication") ||
		strings.Contains(err.Error(), "credentials") {
		return false, fmt.Errorf("认证失败")
	}

	return false, err
}
