package redisop

import (
	"context"
	"time"

	"github.com/anchel/mini-seckill/redisclient"
	"github.com/charmbracelet/log"
)

func Del(ctx context.Context, key string) (int64, error) {
	val, err := redisclient.Rdb.Del(ctx, key).Result()
	if err != nil {
		log.Debug("redisop Del", "key", key, "err", err)
		return 0, err
	}
	return val, nil
}

func Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	_, err := redisclient.Rdb.Set(ctx, key, value, expiration).Result()
	if err != nil {
		log.Debug("redisop Set", "key", key, "err", err)
		return err
	}
	return nil
}

func SetNX(ctx context.Context, key string, value any, expiration time.Duration) (bool, error) {
	result, err := redisclient.Rdb.SetNX(ctx, key, value, expiration).Result()
	if err != nil {
		log.Debug("redisop SetNX", "key", key, "err", err)
		return false, err
	}
	return result, nil
}

func HSet(ctx context.Context, key, field string, value any) error {
	_, err := redisclient.Rdb.HSet(ctx, key, field, value).Result()
	if err != nil {
		log.Debug("redisop HSet", "key", key, "field", field, "err", err)
		return err
	}
	return nil
}

func HSetNX(ctx context.Context, key, field string, value any) (bool, error) {
	result, err := redisclient.Rdb.HSetNX(ctx, key, field, value).Result()
	if err != nil {
		log.Debug("redisop HSetNX", "key", key, "field", field, "err", err)
		return false, err
	}
	return result, nil
}

func RPush(ctx context.Context, key string, value any) error {
	_, err := redisclient.Rdb.RPush(ctx, key, value).Result()
	if err != nil {
		log.Debug("redisop RPush", "key", key, "err", err)
		return err
	}
	return nil
}

func LPop(ctx context.Context, key string) (string, error) {
	val, err := redisclient.Rdb.LPop(ctx, key).Result()
	if err != nil {
		log.Debug("redisop LPop", "key", key, "err", err)
		return "", err
	}
	return val, nil
}

func LPopCount(ctx context.Context, key string, count int) ([]string, error) {
	val, err := redisclient.Rdb.LPopCount(ctx, key, count).Result()
	if err != nil {
		log.Debug("redisop LPopCount", "key", key, "count", count, "err", err)
		return nil, err
	}
	return val, nil
}
