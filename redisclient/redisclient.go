package redisclient

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

var Rdb *redis.Client

func InitRedis(addr, pwd string, db int) error {
	client := redis.NewClient(&redis.Options{
		Addr:        addr,
		Password:    pwd,
		DB:          db,
		ReadTimeout: time.Duration(8) * time.Second,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		return err
	}

	Rdb = client

	return nil
}

func GetPubSubClient(ctx context.Context, addr, pwd string) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: pwd,
	})
	return client
}

func Close() {
	Rdb.Close()
}
