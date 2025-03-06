package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/anchel/mini-seckill/lib/redisop"
	"github.com/anchel/mini-seckill/mysqldb"
	pb "github.com/anchel/mini-seckill/proto"
	"github.com/charmbracelet/log"
	"github.com/redis/go-redis/v9"
)

type CacheSeckill struct {
	Id        int64      `json:"id"`
	Name      string     `json:"name"`
	StartTime int64      `json:"start_time"`
	EndTime   int64      `json:"end_time"`
	Total     int64      `json:"total"`
	Remaining int64      `json:"remaining"`
	Finished  int32      `json:"finished"`
	CreateAt  *time.Time `json:"create_at"`
}

func writeSeckillToRedis(ctx context.Context, sk *CacheSeckill) error {
	// now := time.Now()
	key := fmt.Sprintf("seckill:info:%d", sk.Id)

	// expiration := time.UnixMilli(sk.EndTime).Sub(now)

	vals := map[string]interface{}{
		"StartTime": sk.StartTime, // 开始时间
		"EndTime":   sk.EndTime,   // 结束时间
		"Total":     sk.Total,     // 总数量
		"Remaining": sk.Remaining, // 剩余数量
		"Finished":  sk.Finished,  // 是否已结束，这是一个开关，不是根据剩余数量计算的
	}

	err := redisop.HMSet(ctx, key, vals)
	if err != nil {
		log.Warn("writeSeckillToRedis redisop.HMSet", "err", err)
		return err
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

func checkCanJoinSeckill(ctx context.Context, seckillID int64, userID int64) (pb.JoinSeckillStatus, error) {

	ldata, ok := SeckillInfoLocalCacheMap.Load(seckillID)
	if ok {
		log.Info("checkCanJoinSeckill seckill found in localcache", "SeckillID", seckillID)
		lcsk := ldata.(*LocalCacheSeckill)

		if checkSeckillEnd(lcsk.Finished, lcsk.EndTime) {
			return pb.JoinSeckillStatus_JOIN_FINISHED, nil
		}
		if checkSeckillNotStart(lcsk.StartTime) {
			return pb.JoinSeckillStatus_JOIN_NOT_START, nil
		}
		if checkSeckillNoRemaining(lcsk.Remaining) {
			return pb.JoinSeckillStatus_JOIN_NO_REMAINING, nil
		}
	}

	// 从redis来判断秒杀活动的状态
	fields := []string{"StartTime", "EndTime", "Remaining", "Finished"}
	key := fmt.Sprintf("seckill:info:%d", seckillID)
	length := len(fields)
	vals, e := redisop.HMGet(ctx, key, fields...)
	if e != nil {
		if e == redis.Nil {
			log.Info("checkCanJoinSeckill seckill not found in redis", "SeckillID", seckillID)

			// 从数据库来判断秒杀活动的状态
			doc, err := mysqldb.QuerySeckillByID(ctx, seckillID, false)
			if err != nil {
				log.Error("checkCanJoinSeckill QuerySeckillByID", "err", err)
				return pb.JoinSeckillStatus_JOIN_UNKNOWN, err
			}
			if doc == nil {
				log.Info("checkCanJoinSeckill not found in database", "id", seckillID)
				return pb.JoinSeckillStatus_JOIN_UNKNOWN, nil
			}

			if checkSeckillEnd(doc.Finished, doc.EndTime.UnixMilli()) {
				return pb.JoinSeckillStatus_JOIN_FINISHED, nil
			}
			if checkSeckillNotStart(doc.StartTime.UnixMilli()) {
				return pb.JoinSeckillStatus_JOIN_NOT_START, nil
			}
			if checkSeckillNoRemaining(doc.Remaining) {
				return pb.JoinSeckillStatus_JOIN_NO_REMAINING, nil
			}
		} else {
			log.Error("checkCanJoinSeckill redisop.HMGet", "err", e)
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
					return pb.JoinSeckillStatus_JOIN_FINISHED, nil
				}
			}

			if okStartTime {
				startTime, e := strconv.ParseInt(StartTime, 10, 64)
				if e == nil && checkSeckillNotStart(startTime) {
					return pb.JoinSeckillStatus_JOIN_NOT_START, nil
				}
			}

			if okRemaining {
				remaining, e := strconv.ParseInt(Remaining, 10, 64)
				if e == nil && checkSeckillNoRemaining(remaining) {
					return pb.JoinSeckillStatus_JOIN_NO_REMAINING, nil
				}
			}
		} else {
			log.Warn("checkCanJoinSeckill seckill not found in redis, len(vals) < length", "SeckillID", seckillID, "vals", vals)
		}
	}

	// 判断是否已秒杀成功
	keyusers := fmt.Sprintf("seckill:users:%d", seckillID)
	success, err := redisop.SIsMember(ctx, keyusers, userID)
	if err != nil {
		log.Error("checkCanJoinSeckill redisop.SIsMember", "err", err)
	} else {
		if success {
			log.Info("checkCanJoinSeckill redisop.SIsMember", "SeckillID", seckillID, "UserID", userID, "success", success)
			return pb.JoinSeckillStatus_JOIN_IN_USERS_LIST, nil
		}
	}

	// 判断是否已加入过
	// keyticket := fmt.Sprintf("seckill:ticket:%d:%d", seckillID, userID)

	// status, err := redisop.Get(ctx, keyticket, false)
	// if err != nil {
	// 	log.Error("checkCanJoinSeckill redisop.Get", "err", err)
	// } else {
	// 	log.Info("checkCanJoinSeckill redisop.Get", "status", status)
	// 	// 如果是“排队中”，则返回加入成功，让前端继续轮询
	// 	// 此时不一定是真正在排队中，可能是数据库还为更新redis，不过此时继续轮询也没问题
	// 	if status == pb.InquireSeckillStatus_IS_QUEUEING.String() {
	// 		return pb.JoinSeckillStatus_JOIN_SUCCESS, nil
	// 	} else if status != "" {
	// 		return pb.JoinSeckillStatus_JOIN_FAILED, nil // 要么是秒杀成功或秒杀失败了，或者是其他状态
	// 	}
	// }

	return pb.JoinSeckillStatus_JOIN_UNKNOWN, nil
}
