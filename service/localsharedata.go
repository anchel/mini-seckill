package service

import (
	"context"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

var SeckillInfoLocalCacheMap sync.Map

func init() {
	SeckillInfoLocalCacheMap = sync.Map{}
}

func InitLogicLocalCache(ctx context.Context) error {
	// 每隔一个小时，清空本地缓存
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Hour):
				log.Info("clear local cache")
				SeckillInfoLocalCacheMap.Clear()
			}
		}
	}()

	return nil
}

type LocalCacheSeckill struct {
	StartTime int64 `json:"start_time"`
	EndTime   int64 `json:"end_time"`
	Finished  int32 `json:"finished"`
	Remaining int64 `json:"remaining"`
}
