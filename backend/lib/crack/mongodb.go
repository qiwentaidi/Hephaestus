package crack

import (
	"context"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

func MongodbScan(ctx context.Context, host string, credentials []string, vulnCallback VulnerabilityCallback) {
	flag, err := mongodbUnauthorizedConn(ctx, host)
	if flag && err == nil {
		if vulnCallback != nil {
			vulnCallback(VulnerabilityInfo{
				ID:       "mongodb-unauthorized",
				Name:     "mongodb unauthorized",
				URL:      host,
				Type:     "Mongodb",
				Severity: "HIGH",
			})
		}
		return
	}
	for _, cred := range credentials {
		if ctx.Err() != nil {
			warningf("[mongodb] User exits crack scanning")
			return
		}
		parts := strings.SplitN(cred, ":", 2)
		if len(parts) != 2 {
			continue
		}
		user := parts[0]
		pass := parts[1]

		flag, err := mongodbConn(ctx, host, user, pass, func(ctx context.Context, client *mongo.Client) error {
			return client.Ping(ctx, readpref.Primary())
		})
		if flag && err == nil {
			if vulnCallback != nil {
				vulnCallback(VulnerabilityInfo{
					ID:       "mongodb-weak-password",
					Name:     "mongodb weak password",
					URL:      host,
					Type:     "Mongodb",
					Severity: "HIGH",
					Extract:  user + "/" + pass,
				})
			}
			return
		}
	}
}

func MongodbConn(host, user, pass string) (flag bool, err error) {
	return mongodbConn(context.Background(), host, user, pass, func(ctx context.Context, client *mongo.Client) error {
		return client.Ping(ctx, readpref.Primary())
	})
}

// MongodbUnauthorizedConn verifies that an anonymous connection can access
// database metadata. A successful ping alone is not proof of unauthorized
// access because MongoDB permits it before authentication.
func MongodbUnauthorizedConn(host string) (flag bool, err error) {
	return mongodbUnauthorizedConn(context.Background(), host)
}

func mongodbUnauthorizedConn(ctx context.Context, host string) (flag bool, err error) {
	return mongodbConn(ctx, host, "", "", func(ctx context.Context, client *mongo.Client) error {
		_, err := client.ListDatabaseNames(ctx, bson.D{})
		return err
	})
}

func mongodbConn(ctx context.Context, host, user, pass string, verify func(context.Context, *mongo.Client) error) (flag bool, err error) {
	flag = false
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, defaultCrackConnectTimeout)
	defer cancel()

	// Create MongoDB URI
	mongoURI := fmt.Sprintf("mongodb://%s", host)

	// Define client options with or without credentials
	clientOpts := options.Client().ApplyURI(mongoURI)
	if user != "" && pass != "" {
		credentials := options.Credential{
			Username: user,
			Password: pass,
		}
		clientOpts.SetAuth(credentials)
	}
	clientOpts.SetConnectTimeout(defaultCrackConnectTimeout)
	if dial := dialContextFromContext(ctx); dial != nil {
		clientOpts.SetDialer(contextProxyDialer{dial: dial})
	}

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return flag, err
	}
	defer client.Disconnect(context.Background())

	err = verify(ctx, client)
	if err == nil {
		flag = true
	}
	return flag, err
}
