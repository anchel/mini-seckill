package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anchel/mini-seckill/lib/redisop"
	"github.com/anchel/mini-seckill/mongodb"
	pb "github.com/anchel/mini-seckill/proto"
	"github.com/charmbracelet/log"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

var MaxQueueSize = 1

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
	SeckillID   string `json:"seckill_id"`
	UserID      string `json:"user_id"`
	SecKillTime int64  `json:"seckill_time"`
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

	localQueue := make(chan *LocalQueueSeckillJoin, 10)

	readFromRedis := func() (bool, error) {
		// 从redis读取数据
		vals, err := redisop.LPopCount(ctx, keyqueue, 5)
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
			if len(arr) != 3 {
				log.Error("startReadQueue strings.Split", "keyqueue", keyqueue, "val", val)
				continue
			}

			seckillId := arr[0]
			userId := arr[1]
			secKillTimeStr := arr[2]

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
					startRead = time.After(5000 * time.Millisecond)
				} else {
					startRead = time.After(10 * time.Millisecond)
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
						log.Info("startReadQueue readFromRedis", "delay", d)
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
	var done chan TryDoSeckillResult

	for {
		if done == nil {
			localQueueCond = localQueue
		}

		select {
		case <-ctx.Done():
			return
		case lqs := <-localQueueCond:
			log.Info("startReadQueue <-localQueue", "seckillId", lqs.SeckillID, "userId", lqs.UserID, "secKillTime", lqs.SecKillTime)
			done = make(chan TryDoSeckillResult, 1)
			go func() {
				// 业务逻辑
				err := tryDoSeckill(ctx, lqs.SeckillID, lqs.UserID, lqs.SecKillTime)
				if err != nil {
					log.Warn("startReadQueue business logic", "seckillId", lqs.SeckillID, "userId", lqs.UserID, "secKillTime", lqs.SecKillTime, "err", err)
				}

				done <- TryDoSeckillResult{err: err}
			}()
		case <-done:
			done = nil
		}
	}
}

func tryDoSeckill(parentCtx context.Context, seckillId, userId string, secKillTime int64) error {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	keyticket := fmt.Sprintf("seckill:ticket:%s:%s", seckillId, userId)
	fieldStatus := "status"

	wc := writeconcern.Majority()
	txnOptions := options.Transaction().SetWriteConcern(wc)

	// Starts a session on the client
	session, err := mongodb.MongoClientInstance.Client.StartSession()
	if err != nil {
		log.Error("tryDoSeckill StartSession", "err", err)
		return err
	}
	// Defers ending the session after the transaction is committed or ended
	defer session.EndSession(ctx)

	tresult, err := session.WithTransaction(ctx, func(ctx mongo.SessionContext) (interface{}, error) {
		ID, err := primitive.ObjectIDFromHex(seckillId)
		if err != nil {
			return nil, err
		}

		collectionSeckill, err := mongodb.MongoClientInstance.GetCollection("seckill")
		if err != nil {
			log.Error("tryDoSeckill GetCollection", "collection", collectionSeckill.Name(), "err", err)
			return nil, err
		}

		collectionSeckillResult, err := mongodb.MongoClientInstance.GetCollection("seckill_result")
		if err != nil {
			log.Error("tryDoSeckill GetCollection", "collection", collectionSeckillResult.Name(), "err", err)
			return nil, err
		}

		// 减库存
		filter1 := bson.D{{Key: "_id", Value: ID}, {Key: "finished", Value: 0}, {Key: "remaining", Value: bson.D{{Key: "$gt", Value: 0}}}}
		update1 := bson.D{{Key: "$inc", Value: bson.D{{Key: "remaining", Value: -1}}}}
		result1, err := collectionSeckill.UpdateOne(ctx, filter1, update1)
		if err != nil {
			log.Error("tryDoSeckill UpdateOne", "collection", collectionSeckill.Name(), "filter", filter1, "update", update1, "err", err)
			return nil, err
		}

		log.Info("tryDoSeckill UpdateOne", "collection", collectionSeckill.Name(), "seckillId", seckillId, "result", result1)

		if result1.ModifiedCount == 0 {
			return pb.InquireSeckillStatus_IS_FAILED, errors.New("seckill is not found or finished or remaining is 0")
		}

		// 插入秒杀结果
		// 下面注释的这个做法能否更合理？如果已经存在记录，是认为秒杀成功还是失败？所以还是直接插入，如果已经存在记录，会报错，不存在插入成功并返回秒杀成功
		// filter2 := bson.D{{Key: "seckill_id", Value: seckillId}, {Key: "user_id", Value: userId}}
		// update2 := bson.D{{Key: "$set", Value: bson.D{{Key: "updated_at", Value: time.Now()}}}}
		// opts := options.Update().SetUpsert(true)

		// result2, err := collectionSeckillResult.UpdateOne(ctx, filter2, update2, opts)
		// if err != nil {
		// 	log.Error("tryDoSeckill UpdateOne", "collection", collectionSeckillResult.Name(), "filter", filter2, "update", update2, "err", err)
		// 	return nil, err
		// }

		now := time.Now()
		doc := mongodb.EntitySecKillResult{
			SeckillID:   seckillId,
			UserID:      userId,
			SecKillTime: &now,
		}
		doc.CreatedAt = now
		result2, err := collectionSeckillResult.InsertOne(ctx, doc)
		if err != nil {
			log.Warn("tryDoSeckill InsertOne", "collection", collectionSeckillResult.Name(), "doc", doc, "err", err)
			// 是否要区分是普通错误，还是由于唯一索引冲突导致的错误？
			return pb.InquireSeckillStatus_IS_FAILED, err
		}

		log.Info("tryDoSeckill InsertOne", "collection", collectionSeckillResult.Name(), "seckillId", seckillId, "userId", userId, "result", result2)

		return pb.InquireSeckillStatus_IS_SUCCESS, err
	}, txnOptions)

	if err != nil {
		log.Warn("tryDoSeckill WithTransaction", "err", err)
		if tresult != nil {
			status := tresult.(pb.InquireSeckillStatus)
			innererr := redisop.HSet(ctx, keyticket, fieldStatus, status.String())
			if innererr != nil {
				log.Error("tryDoSeckill WithTransaction redisop.HSet", "err", innererr)
			}
		}
		return err
	}
	log.Info("tryDoSeckill WithTransaction", "result", tresult)

	if tresult == nil {
		tresult = pb.InquireSeckillStatus_IS_FAILED
	}

	// 更新redis上的状态
	// 这里更新失败，用户得不到真正的秒杀结果
	err = redisop.HSet(ctx, keyticket, fieldStatus, tresult.(pb.InquireSeckillStatus).String())
	if err != nil {
		log.Error("tryDoSeckill redisop.HSet", "err", err)
		return err
	}

	// 更新缓存里面秒杀活动的库存，这里不要求强一致性，只是用来判断大概的库存情况
	_, err = redisop.HIncrBy(ctx, fmt.Sprintf("seckill:%s", seckillId), "Remaining", -1)
	if err != nil {
		log.Error("tryDoSeckill redisop.HIncrBy", "err", err)
		return err
	}

	return nil
}
