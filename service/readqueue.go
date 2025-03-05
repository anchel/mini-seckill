package service

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anchel/mini-seckill/lib/redisop"
	"github.com/anchel/mini-seckill/mysqldb"
	pb "github.com/anchel/mini-seckill/proto"
	"github.com/anchel/mini-seckill/redisclient"
	"github.com/charmbracelet/log"
	"github.com/redis/go-redis/v9"
)

var MaxQueueSize = 10
var LocalQueueSize = 50
var RedisBatchSize = 5

func InitLogicReadQueue(ctx context.Context, wgExit *sync.WaitGroup) {
	defer wgExit.Done()

	wg := sync.WaitGroup{}
	for range MaxQueueSize {
		wg.Add(1)
		go func(w *sync.WaitGroup) {
			defer w.Done()
			startReadQueue(ctx)
		}(&wg)
	}

	wg.Wait()
}

type LocalQueueSeckillJoin struct {
	SeckillID   int64 `json:"seckill_id"`
	UserID      int64 `json:"user_id"`
	SecKillTime int64 `json:"seckill_time"`
}

type ReadResult struct {
	delay bool
	err   error
}

type TryDoSeckillResult struct {
	err error
}

func startReadQueue(ctx context.Context) {
	keyqueue := "seckill:queue"

	localQueue := make(chan *LocalQueueSeckillJoin, LocalQueueSize)

	readFromRedis := func() (bool, error) {
		// 从redis读取数据
		vals, err := redisop.LPopCount(ctx, keyqueue, RedisBatchSize, false)
		if err != nil {
			if err == redis.Nil {
				log.Debug("startReadQueue redisop.LPop", "keyqueue", keyqueue, "err", err)
				return true, nil
			}
			return true, err
		}

		if len(vals) == 0 {
			return true, nil
		}

		log.Info("startReadQueue redisop.LPop", "keyqueue", keyqueue, "vals.length", len(vals))

		for _, val := range vals {
			arr := strings.Split(val, ":")
			if len(arr) < 3 {
				log.Error("startReadQueue strings.Split", "keyqueue", keyqueue, "val", val)
				continue
			}

			seckillIdStr := arr[0]
			userIdStr := arr[1]
			secKillTimeStr := arr[2]

			seckillId, err := strconv.ParseInt(seckillIdStr, 10, 64)
			if err != nil {
				log.Error("startReadQueue strconv.ParseInt", "seckillIdStr", seckillIdStr, "err", err)
				continue
			}

			userId, err := strconv.ParseInt(userIdStr, 10, 64)
			if err != nil {
				log.Error("startReadQueue strconv.ParseInt", "userIdStr", userIdStr, "err", err)
				continue
			}

			secKillTime, err := strconv.ParseInt(secKillTimeStr, 10, 64)
			if err != nil {
				log.Error("startReadQueue strconv.ParseInt", "secKillTimeStr", secKillTimeStr, "err", err)
				continue
			}

			lqs := &LocalQueueSeckillJoin{
				SeckillID:   seckillId,
				UserID:      userId,
				SecKillTime: secKillTime,
			}

			select {
			case <-ctx.Done():
				return false, nil
			case localQueue <- lqs:
				log.Debug("startReadQueue localQueue <-", "seckillId", seckillId, "userId", userId, "secKillTime", secKillTime)
			}
		}

		return false, nil
	}

	go func() {
		var delay bool
		var done chan ReadResult

		for {
			var startRead <-chan time.Time
			if done == nil {
				if delay {
					startRead = time.After(500 * time.Millisecond)
				} else {
					startRead = time.After(5 * time.Millisecond)
				}
			}

			select {
			case <-ctx.Done():
				return
			case <-startRead:
				done = make(chan ReadResult, 1)
				go func() {
					d, err := readFromRedis()
					if err != nil {
						log.Error("startReadQueue readFromRedis", "err", err)
					}

					if d {
						log.Debug("startReadQueue readFromRedis", "delay", d)
					}
					done <- ReadResult{delay: d, err: err}
				}()
			case rr := <-done:
				done = nil
				delay = rr.delay
			}
		}
	}()

	var localQueueCond chan *LocalQueueSeckillJoin
	var done chan any

	for {
		if done == nil {
			localQueueCond = localQueue
		}

		select {
		case <-ctx.Done():
			return
		case lqs := <-localQueueCond:
			log.Info("startReadQueue <-localQueue", "seckillId", lqs.SeckillID, "userId", lqs.UserID, "secKillTime", lqs.SecKillTime)
			done = make(chan any, 1)
			go func() {
				// 业务逻辑
				err := tryDoSeckill(ctx, lqs.SeckillID, lqs.UserID, lqs.SecKillTime)
				if err != nil {
					log.Warn("startReadQueue business logic", "seckillId", lqs.SeckillID, "userId", lqs.UserID, "secKillTime", lqs.SecKillTime, "err", err)
				}

				done <- struct{}{}
			}()
		case <-done:
			done = nil
		}
	}
}

func tryDoSeckill(parentCtx context.Context, seckillId, userId, secKillTime int64) error {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	keyticket := fmt.Sprintf("seckill:ticket:%d:%d", seckillId, userId)

	// 从本地缓存来判断秒杀活动的状态
	ldata, ok := SeckillInfoLocalCacheMap.Load(seckillId)
	if ok {
		log.Info("tryDoSeckill seckill found in localcache", "SeckillID", seckillId)
		lcsk := ldata.(*LocalCacheSeckill)

		if lcsk.Remaining <= 0 {
			log.Info("tryDoSeckill localcache Remaining <= 0", "SeckillID", seckillId)
			err := redisop.Set(ctx, keyticket, pb.InquireSeckillStatus_IS_FAILED.String(), time.Duration(rand.Intn(30)+80)*time.Second)
			if err != nil {
				log.Error("tryDoSeckill redisop.HSet", "err", err)
			}
			return nil
		}
	} else {
		log.Debug("tryDoSeckill seckill not found in localcache", "SeckillID", seckillId)
	}

	// TODO: just for test, remember to remove
	// err := redisop.Set(ctx, keyticket, pb.InquireSeckillStatus_IS_FAILED.String(), time.Duration(rand.Intn(30)+80)*time.Second)
	// if err != nil {
	// 	log.Error("tryDoSeckill redisop.HSet", "err", err)
	// }
	// return nil

	keyusers := fmt.Sprintf("seckill:users:%d", seckillId)

	status, lastInsertID, err := executeTransaction(ctx, seckillId, userId, secKillTime)

	if err != nil {
		log.Warn("tryDoSeckill Transaction", "err", err)
		if status != pb.InquireSeckillStatus_IS_UNKNOWN {
			// 更新redis上的状态
			innererr := redisop.Set(ctx, keyticket, status.String(), time.Duration(rand.Intn(30)+80)*time.Second)
			if innererr != nil {
				log.Error("tryDoSeckill WithTransaction redisop.HSet", "err", innererr)
			}
		}
		return err
	}

	log.Info("tryDoSeckill Transaction", "status", status, "lastInsertID", lastInsertID)

	if status != pb.InquireSeckillStatus_IS_SUCCESS {
		log.Warn("tryDoSeckill Transaction", "status", status, "seckillId", seckillId, "userId", userId)
		// 更新redis上的状态
		err = redisop.Set(ctx, keyticket, status.String(), time.Duration(rand.Intn(30)+80)*time.Second)
		if err != nil {
			log.Error("tryDoSeckill redisop.HSet", "err", err)
		}
		return nil
	}

	// 更新本地缓存，加快性能
	doc, err := mysqldb.QuerySeckillByID(ctx, seckillId, true)
	if err != nil {
		log.Error("tryDoSeckill mysqldb.QuerySeckillByID", "err", err)
		return err
	}
	SeckillInfoLocalCacheMap.Store(seckillId, &LocalCacheSeckill{
		StartTime: doc.StartTime.UnixMilli(),
		EndTime:   doc.EndTime.UnixMilli(),
		Remaining: doc.Remaining,
		Finished:  doc.Finished,
	})

	_, err = redisclient.Rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		// 更新缓存里面秒杀活动的库存，这里不要求强一致性，只是用来判断大概的库存情况
		cmd := pipe.HIncrBy(ctx, fmt.Sprintf("seckill:info:%d", seckillId), "Remaining", -1)
		if cmd.Err() != nil {
			log.Error("tryDoSeckill redisop.HIncrBy", "err", err)
		}

		// 把用户加入到已秒杀用户列表
		err = redisop.SAdd(ctx, keyusers, userId)
		if err != nil {
			log.Error("tryDoSeckill redisop.SAdd", "err", err)
		}

		// 更新redis上用户排队的状态
		err = redisop.Set(ctx, keyticket, status.String(), time.Duration(rand.Intn(30)+80)*time.Second)
		if err != nil {
			log.Error("tryDoSeckill redisop.HSet", "err", err)
		}
		return nil
	})

	if err != nil {
		log.Error("JoinSeckill redisclient.Rdb.TxPipelined", "err", err)
		return err
	}

	return nil
}

func executeTransaction(ctx context.Context, seckillId, userId, secKillTime int64) (pb.InquireSeckillStatus, int64, error) {
	// tx, err := mysqldb.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	tx, err := mysqldb.DB.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		log.Error("tryDoSeckill BeginTx", "err", err)
		return pb.InquireSeckillStatus_IS_UNKNOWN, 0, err
	}

	// 减库存，库存数量大于0时减1
	sqlRet, err := tx.ExecContext(ctx, "UPDATE seckill SET remaining = remaining - 1 WHERE id = ? AND finished = 0 AND remaining > 0", seckillId)
	if err != nil {
		log.Error("tryDoSeckill UpdateOne", "err", err)
		e := tx.Rollback()
		if e != nil {
			log.Error("tryDoSeckill Rollback", "err", e)
		}
		return pb.InquireSeckillStatus_IS_UNKNOWN, 0, err
	}

	affected, err := sqlRet.RowsAffected()
	if err != nil {
		log.Error("tryDoSeckill RowsAffected", "err", err)
		e := tx.Rollback()
		if e != nil {
			log.Error("tryDoSeckill Rollback", "err", e)
		}
		return pb.InquireSeckillStatus_IS_UNKNOWN, 0, err
	}

	log.Info("tryDoSeckill RowsAffected", "affected", affected)
	if affected == 0 {
		e := tx.Rollback()
		if e != nil {
			log.Error("tryDoSeckill Rollback", "err", e)
		}
		return pb.InquireSeckillStatus_IS_FAILED, 0, nil
	}

	// 插入秒杀结果
	now := time.Now()
	insertRet, err := tx.ExecContext(ctx, "INSERT INTO seckill_order (seckill_id, user_id, created_at) VALUES (?, ?, ?)", seckillId, userId, now)
	if err != nil {
		log.Error("tryDoSeckill InsertOne", "err", err)
		// TODO: 区分是普通错误，还是由于唯一索引冲突导致的错误？
		e := tx.Rollback()
		if e != nil {
			log.Error("tryDoSeckill Rollback", "err", e)
		}
		return pb.InquireSeckillStatus_IS_FAILED, 0, err
	}

	if e := tx.Commit(); e != nil {
		log.Error("tryDoSeckill Commit", "err", e)
		return pb.InquireSeckillStatus_IS_UNKNOWN, 0, e
	}

	lastInsertID, err := insertRet.LastInsertId()
	if err != nil {
		log.Error("tryDoSeckill LastInsertId", "err", err)
		return pb.InquireSeckillStatus_IS_UNKNOWN, 0, err
	}

	return pb.InquireSeckillStatus_IS_SUCCESS, lastInsertID, nil
}
