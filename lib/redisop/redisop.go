package redisop

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/anchel/mini-seckill/redisclient"
	"github.com/charmbracelet/log"
	"github.com/redis/go-redis/v9"
)

var countTotal int64
var countGet int64

func init() {
	go func() {
		for {
			time.Sleep(6 * time.Second)
			log.Warn("loop redis countTotal", "total", countTotal, "get", countGet)
		}
	}()
}

func Del(ctx context.Context, key string) (int64, error) {
	atomic.AddInt64(&countTotal, 1)
	val, err := redisclient.Rdb.Del(ctx, key).Result()
	if err != nil {
		log.Error("redisop Del", "key", key, "err", err)
		return 0, err
	}
	return val, nil
}

func Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	atomic.AddInt64(&countTotal, 1)
	_, err := redisclient.Rdb.Set(ctx, key, value, expiration).Result()
	if err != nil {
		log.Error("redisop Set", "key", key, "err", err)
		return err
	}
	return nil
}

func Get(ctx context.Context, key string, nilIsError bool) (string, error) {
	atomic.AddInt64(&countTotal, 1)
	atomic.AddInt64(&countGet, 1)
	val, err := redisclient.Rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil && !nilIsError {
			return "", nil
		}
		log.Error("redisop Get", "key", key, "err", err)
		return "", err
	}
	return val, nil
}

func SetNX(ctx context.Context, key string, value any, expiration time.Duration) (bool, error) {
	atomic.AddInt64(&countTotal, 1)
	result, err := redisclient.Rdb.SetNX(ctx, key, value, expiration).Result()
	if err != nil {
		log.Error("redisop SetNX", "key", key, "err", err)
		return false, err
	}
	return result, nil
}

func HSet(ctx context.Context, key, field string, value any) error {
	atomic.AddInt64(&countTotal, 1)
	_, err := redisclient.Rdb.HSet(ctx, key, field, value).Result()
	if err != nil {
		log.Error("redisop HSet", "key", key, "field", field, "err", err)
		return err
	}
	return nil
}

func HSetNX(ctx context.Context, key, field string, value any) (bool, error) {
	atomic.AddInt64(&countTotal, 1)
	result, err := redisclient.Rdb.HSetNX(ctx, key, field, value).Result()
	if err != nil {
		log.Error("redisop HSetNX", "key", key, "field", field, "err", err)
		return false, err
	}
	return result, nil
}

func HMSet(ctx context.Context, key string, fields map[string]any) error {
	atomic.AddInt64(&countTotal, 1)
	_, err := redisclient.Rdb.HMSet(ctx, key, fields).Result()
	if err != nil {
		log.Error("redisop HMSet", "key", key, "err", err)
	}
	return err
}

func HGet(ctx context.Context, key, field string, nilIsError bool) (string, error) {
	atomic.AddInt64(&countTotal, 1)
	val, err := redisclient.Rdb.HGet(ctx, key, field).Result()
	if err != nil {
		if err == redis.Nil && !nilIsError {
			return "", nil
		}
		log.Error("redisop HGet", "key", key, "field", field, "err", err)
		return "", err
	}
	return val, nil
}

func HMGet(ctx context.Context, key string, fields ...string) ([]interface{}, error) {
	atomic.AddInt64(&countTotal, 1)
	val, err := redisclient.Rdb.HMGet(ctx, key, fields...).Result()
	if err != nil {
		log.Error("redisop HMGet", "key", key, "fields", fields, "err", err)
		return nil, err
	}
	return val, nil
}

func HIncrBy(ctx context.Context, key, field string, incr int64) (int64, error) {
	atomic.AddInt64(&countTotal, 1)
	val, err := redisclient.Rdb.HIncrBy(ctx, key, field, incr).Result()
	if err != nil {
		log.Error("redisop HIncrBy", "key", key, "field", field, "incr", incr, "err", err)
		return 0, err
	}
	return val, nil
}

func RPush(ctx context.Context, key string, value any) error {
	atomic.AddInt64(&countTotal, 1)
	_, err := redisclient.Rdb.RPush(ctx, key, value).Result()
	if err != nil {
		log.Error("redisop RPush", "key", key, "err", err)
		return err
	}
	return nil
}

func LPopCount(ctx context.Context, key string, count int, nilIsError bool) ([]string, error) {
	atomic.AddInt64(&countTotal, 1)
	val, err := redisclient.Rdb.LPopCount(ctx, key, count).Result()
	if err != nil {
		if err == redis.Nil && !nilIsError {
			return nil, nil
		}
		log.Error("redisop LPopCount", "key", key, "count", count, "err", err)
		return nil, err
	}
	return val, nil
}

func SIsMember(ctx context.Context, key string, member any) (bool, error) {
	atomic.AddInt64(&countTotal, 1)
	val, err := redisclient.Rdb.SIsMember(ctx, key, member).Result()
	if err != nil {
		log.Error("redisop SIsMember", "key", key, "member", member, "err", err)
		return false, err
	}
	return val, nil
}

func SAdd(ctx context.Context, key string, members ...any) error {
	atomic.AddInt64(&countTotal, 1)
	_, err := redisclient.Rdb.SAdd(ctx, key, members...).Result()
	if err != nil {
		log.Error("redisop SAdd", "key", key, "members", members, "err", err)
		return err
	}
	return nil
}

func Publish(ctx context.Context, channel string, message any) (int64, error) {
	atomic.AddInt64(&countTotal, 1)
	n, err := redisclient.Rdb.Publish(ctx, channel, message).Result()
	if err != nil {
		log.Error("redisop Publish", "channel", channel, "message", message, "err", err)
		return n, err
	}
	return n, nil
}
