package redisclient

import (
	"context"

	"github.com/redis/go-redis/v9"
)

var Rdb *redis.Client

func InitRedis(ctx context.Context, addr, pwd string, db int) error {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: pwd,
		DB:       db,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		return err
	}

	Rdb = client

	return nil
}
