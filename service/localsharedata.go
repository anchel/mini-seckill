package service

import (
	"sync"
)

var SeckillInfoLocalCacheMap sync.Map

func init() {
	SeckillInfoLocalCacheMap = sync.Map{}
}

func InitLocalCacheLogic() error {
	// TODO: 每日凌晨3点清空

	return nil
}

type LocalCacheSeckill struct {
	StartTime int64 `json:"start_time"`
	EndTime   int64 `json:"end_time"`
	Finished  int32 `json:"finished"`
	Remaining int64 `json:"remaining"`
}
