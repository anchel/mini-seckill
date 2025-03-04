package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/anchel/mini-seckill/lib/redisop"
	"github.com/anchel/mini-seckill/mongodb"
	pb "github.com/anchel/mini-seckill/proto"
	"github.com/anchel/mini-seckill/redisclient"
	"github.com/charmbracelet/log"
	"github.com/redis/go-redis/v9"
)

type CacheSeckill struct {
	Id        string     `json:"id"`
	Name      string     `json:"name"`
	StartTime int64      `json:"start_time"`
	EndTime   int64      `json:"end_time"`
	Total     int64      `json:"total"`
	Remaining int64      `json:"remaining"`
	Finished  int32      `json:"finished"`
	CreateAt  *time.Time `json:"create_at"`
}

func writeSeckillToRedis(ctx context.Context, sk *CacheSeckill) error {
	now := time.Now()
	key := "seckill:" + sk.Id

	expiration := time.UnixMilli(sk.EndTime).Sub(now)

	vals := map[string]interface{}{
		"StartTime": sk.StartTime, // 开始时间
		"EndTime":   sk.EndTime,   // 结束时间
		"Total":     sk.Total,     // 总数量
		"Remaining": sk.Remaining, // 剩余数量
		"Finished":  sk.Finished,  // 是否已结束，这是一个开关，不是根据剩余数量计算的
	}

	cmds, err := redisclient.Rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HMSet(ctx, key, vals)
		pipe.Expire(ctx, key, expiration)
		return nil
	})
	if err != nil {
		log.Warn("writeSeckillToRedis redisclient.Rdb.TxPipelined", "err", err)
		return err
	}
	if len(cmds) != 2 {
		log.Warn("writeSeckillToRedis redisclient.Rdb.TxPipelined", "len(cmds)", len(cmds))
		return errors.New("writeSeckillToRedis len(cmds) != 3")
	}
	return nil
}

func checkSeckillEnd(finished int32, EndTime int64) bool {
	return finished == 1 || time.Now().UnixMilli() > EndTime
}

func checkSeckillNotStart(StartTime int64) bool {
	return time.Now().UnixMilli() < StartTime
}

func checkSeckillNoRemaining(Remaining int64) bool {
	return Remaining <= 0
}

func checkCanJoinSeckill(ctx context.Context, seckillID string, userID string) (pb.JoinSeckillStatus, error) {

	ldata, ok := SeckillInfoLocalCacheMap.Load(seckillID)
	if ok {
		log.Info("checkCanJoinSeckill seckill found in localcache", "SeckillID", seckillID)
		lcsk := ldata.(*LocalCacheSeckill)

		if checkSeckillEnd(lcsk.Finished, lcsk.EndTime) {
			return pb.JoinSeckillStatus_JS_FINISHED, nil
		}
		if checkSeckillNotStart(lcsk.StartTime) {
			return pb.JoinSeckillStatus_JS_NOT_START, nil
		}
	}

	// 从redis读取来判断
	fields := []string{"StartTime", "EndTime", "Remaining", "Finished"}
	length := len(fields)
	vals, e := redisop.HMGet(ctx, "seckill:"+seckillID, fields...)
	if e != nil {
		if e == redis.Nil {
			log.Info("checkCanJoinSeckill seckill not found in redis", "SeckillID", seckillID)
		} else {
			log.Warn("checkCanJoinSeckill redisop.HMGet", "err", e)
		}
	} else {
		if len(vals) >= length {
			log.Info("checkCanJoinSeckill seckill found in redis", "SeckillID", seckillID, "vals", vals)
			StartTime, okStartTime := vals[0].(string)
			EndTime, okEndTime := vals[1].(string)
			Remaining, okRemaining := vals[2].(string)
			Finished, okFinished := vals[3].(string)
			// log.Info("checkCanJoinSeckill seckill found in redis", "SeckillID", seckillID, "ok1", okStartTime, "ok2", okEndTime, "ok3", okRemaining, "ok4", okFinished)

			if okFinished && okEndTime {
				finished, e1 := strconv.ParseInt(Finished, 10, 32)
				endTime, e2 := strconv.ParseInt(EndTime, 10, 64)
				if e1 == nil && e2 == nil && checkSeckillEnd(int32(finished), endTime) {
					return pb.JoinSeckillStatus_JS_FINISHED, nil
				}
			}

			if okStartTime {
				startTime, e := strconv.ParseInt(StartTime, 10, 64)
				if e == nil && checkSeckillNotStart(startTime) {
					return pb.JoinSeckillStatus_JS_NOT_START, nil
				}
			}

			if okRemaining {
				remaining, e := strconv.ParseInt(Remaining, 10, 64)
				if e == nil && checkSeckillNoRemaining(remaining) {
					return pb.JoinSeckillStatus_JS_EMPTY, nil
				}
			}
		} else {
			log.Warn("checkCanJoinSeckill seckill not found in redis, len(vals) < length", "SeckillID", seckillID, "vals", vals)
		}
	}

	// 判断是否已加入过
	keyticket := fmt.Sprintf("seckill:ticket:%s:%s", seckillID, userID)
	fieldStatus := "status"
	status, err := redisop.HGet(ctx, keyticket, fieldStatus, false)
	if err != nil {
		log.Warn("checkCanJoinSeckill redisop.HGet", "err", err)
	} else {
		log.Info("checkCanJoinSeckill redisop.HGet", "status", status)
		// 如果是“排队中”，则返回加入成功，让前端继续轮询
		// 此时不一定是真正在排队中，可能是数据库还为更新redis，不过此时继续轮询也没问题
		if status == pb.InquireSeckillStatus_IS_QUEUEING.String() {
			return pb.JoinSeckillStatus_JS_SUCCESS, nil
		} else if status != "" {
			return pb.JoinSeckillStatus_JS_FAILED, nil // 要么是秒杀成功或秒杀失败了，或者是其他状态
		}
	}

	// 去数据库查询该秒杀活动的一些状态
	doc, err := mongodb.ModelSecKill.FindByID(ctx, seckillID)
	if err != nil {
		log.Error("checkCanJoinSeckill mongodb.ModelSecKill.FindByID", "err", err)
		return pb.JoinSeckillStatus_JS_UNKNOWN, err
	}
	if doc == nil {
		log.Info("checkCanJoinSeckill not found in database", "id", seckillID)
		return pb.JoinSeckillStatus_JS_UNKNOWN, errors.New("seckill not found")
	}

	if checkSeckillEnd(doc.Finished, doc.EndTime) {
		return pb.JoinSeckillStatus_JS_FINISHED, nil
	}
	if checkSeckillNotStart(doc.StartTime) {
		return pb.JoinSeckillStatus_JS_NOT_START, nil
	}
	if checkSeckillNoRemaining(doc.Remaining) {
		return pb.JoinSeckillStatus_JS_EMPTY, nil
	}

	return pb.JoinSeckillStatus_JS_UNKNOWN, nil
}
