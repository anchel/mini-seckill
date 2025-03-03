package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/anchel/mini-seckill/lib/redisop"
	"github.com/anchel/mini-seckill/mongodb"
	"github.com/anchel/mini-seckill/redisclient"
	"github.com/charmbracelet/log"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"

	pb "github.com/anchel/mini-seckill/proto"
)

type CreateSeckillRequest struct {
	Name        string
	Description string
	StartTime   int64
	EndTime     int64
	Total       int64
}

type CreateSeckillResponse struct {
	Id string
}

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

// CreateSeckill creates a seckill activity
func CreateSeckill(ctx context.Context, req *CreateSeckillRequest) (*CreateSeckillResponse, error) {
	if time.Now().UnixMilli() > req.EndTime {
		return nil, errors.New("end_time is invalid")
	}

	doc := &mongodb.EntitySecKill{
		Name:      req.Name,
		Desc:      req.Description,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Total:     req.Total,
		Remaining: req.Total,
		Finished:  0,
	}
	id, err := mongodb.ModelSecKill.InsertOne(ctx, doc)
	if err != nil {
		log.Error("mongodb.ModelSecKill.InsertOne", "err", err)
		return nil, err
	}

	// redis
	key := "seckill:" + id
	bs, err := json.Marshal(&CacheSeckill{
		Id:        id,
		Name:      doc.Name,
		StartTime: doc.StartTime,
		EndTime:   doc.EndTime,
		Total:     doc.Total,
		Remaining: doc.Remaining,
		Finished:  doc.Finished,
		CreateAt:  &doc.EntityBase.CreatedAt,
	})
	if err != nil {
		log.Error("json.Marshal", "err", err)
		return nil, err
	}
	expiration := time.UnixMilli(doc.EndTime).Sub(time.Now())
	log.Info("redisclient.Rdb.Set", "key", key, "expiration", expiration.Seconds())

	err = redisop.Set(ctx, key, bs, expiration)
	if err != nil {
		log.Error("redisclient.Rdb.Set", "err", err)
		return nil, err
	}

	return &CreateSeckillResponse{Id: id}, nil
}

type GetSeckillRequest struct {
	Id string
}

type GetSeckillResponse struct {
	Id          string
	Name        string
	Description string
	StartTime   int64
	EndTime     int64
	Total       int64
	Remaining   int64
	Finished    int32
	CreatedAt   *time.Time
}

// GetSeckill gets a seckill activity
func GetSeckill(ctx context.Context, req *GetSeckillRequest) (*GetSeckillResponse, error) {
	key := "seckill:" + req.Id
	str, err := redisclient.Rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			log.Info("GetSeckill not found in redis, look for it in the database", "key", key, "err", err)
			// 降级从mongodb中获取
			doc, err := mongodb.ModelSecKill.FindByID(ctx, req.Id)
			if err != nil {
				log.Error("GetSeckill mongodb.ModelSecKill.FindByID", "err", err)
				return nil, err
			}
			if doc == nil {
				log.Info("GetSeckill not found in database", "id", req.Id)
				return nil, nil
			}

			// todo: 保存到redis

			return &GetSeckillResponse{
				Id:          req.Id,
				Name:        doc.Name,
				Description: doc.Desc,
				StartTime:   doc.StartTime,
				EndTime:     doc.EndTime,
				Total:       doc.Total,
				Remaining:   doc.Remaining,
				Finished:    doc.Finished,
				CreatedAt:   &doc.EntityBase.CreatedAt,
			}, nil
		}
		log.Error("redisclient.Rdb.Get", "err", err)
		return nil, err
	}
	var cache CacheSeckill
	err = json.Unmarshal([]byte(str), &cache)
	if err != nil {
		log.Error("json.Unmarshal", "err", err)
		return nil, err
	}

	return &GetSeckillResponse{
		Id:          cache.Id,
		Name:        cache.Name,
		Description: "",
		StartTime:   cache.StartTime,
		EndTime:     cache.EndTime,
		Total:       cache.Total,
		Remaining:   cache.Remaining,
		Finished:    cache.Finished,
		CreatedAt:   cache.CreateAt,
	}, nil
}

type JoinSeckillRequest struct {
	SeckillID string
	UserID    string
}

type JoinSeckillResponse struct {
	Status pb.JoinSeckillStatus
}

var localCache sync.Map

// JoinSeckill joins a seckill activity
// todo: 判断是否已经秒杀成功再决定是否加入队列
func JoinSeckill(ctx context.Context, req *JoinSeckillRequest) (*JoinSeckillResponse, error) {
	// 先从本地缓存中获取
	ldata, ok := SeckillInfoLocalCacheMap.Load(req.SeckillID)
	if ok {
		log.Info("JoinSeckill seckill found in localcache", "SeckillID", req.SeckillID)
		lcsk := ldata.(*LocalCacheSeckill)
		status := checkSeckillCanJoin(&CacheSeckill{
			Id:        lcsk.Id,
			StartTime: lcsk.StartTime,
			EndTime:   lcsk.EndTime,
			Total:     lcsk.Total,
			Remaining: lcsk.Remaining,
			Finished:  lcsk.Finished,
		})
		if status != pb.JoinSeckillStatus_JS_UNKNOWN {
			return &JoinSeckillResponse{Status: status}, nil
		}
	}

	// 本地缓存中没有，再从redis，以及数据库中获取
	sk, err := GetSeckill(ctx, &GetSeckillRequest{Id: req.SeckillID})
	if err != nil {
		log.Error("GetSeckill", "err", err)
		return nil, err
	}
	if sk == nil {
		return nil, errors.New("seckill is not found")
	}

	status := checkSeckillCanJoin(&CacheSeckill{
		Id:        sk.Id,
		StartTime: sk.StartTime,
		EndTime:   sk.EndTime,
		Total:     sk.Total,
		Remaining: sk.Remaining,
		Finished:  sk.Finished,
	})
	if status != pb.JoinSeckillStatus_JS_UNKNOWN {
		return &JoinSeckillResponse{Status: status}, nil
	}

	nowmill := time.Now().UnixMilli()
	keyticket := fmt.Sprintf("seckill:ticket:%s:%s", req.SeckillID, req.UserID)
	field1 := "count"
	field2 := "status"

	keyqueue := "seckill:queue"
	val := fmt.Sprintf("%s:%s:%d", req.SeckillID, req.UserID, nowmill)

	cmds, err := redisclient.Rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HIncrBy(ctx, keyticket, field1, 1)
		pipe.HSet(ctx, keyticket, field2, pb.InquireSeckillStatus_IS_QUEUEING.String()) // 默认状态是排队中
		pipe.Expire(ctx, keyticket, time.Duration(rand.Intn(30)+90)*time.Second)
		pipe.RPush(ctx, keyqueue, val)
		return nil
	})
	if err != nil {
		log.Error("JoinSeckill redisclient.Rdb.TxPipelined", "err", err)
		return nil, err
	}
	if len(cmds) != 4 {
		log.Error("JoinSeckill redisclient.Rdb.TxPipelined", "len(cmds)", len(cmds))
		return nil, errors.New("len(cmds) != 3")
	}

	// 是否有必要依次检查每个命令的错误？
	if cmd0 := cmds[0]; cmd0.Err() != nil {
		log.Error("JoinSeckill redisclient.Rdb.HIncrBy", "err", cmd0.Err())
		return nil, cmd0.Err()
	}

	count := cmds[0].(*redis.IntCmd).Val()
	log.Info("JoinSeckill", "seckillid", req.SeckillID, "userid", req.UserID, "count", count)

	return &JoinSeckillResponse{Status: pb.JoinSeckillStatus_JS_SUCCESS}, nil
}

func checkSeckillCanJoin(csk *CacheSeckill) pb.JoinSeckillStatus {
	now := time.Now().UnixMilli()

	if csk.Finished == 1 || csk.EndTime < now {
		return pb.JoinSeckillStatus_JS_FINISHED
	}

	if csk.Remaining < 1 {
		return pb.JoinSeckillStatus_JS_EMPTY
	}

	if csk.StartTime > now {
		return pb.JoinSeckillStatus_JS_NOT_START
	}

	return pb.JoinSeckillStatus_JS_UNKNOWN
}

type InquireSeckillRequest struct {
	SeckillID string
	UserID    string
}

type InquireSeckillResponse struct {
	Status  pb.InquireSeckillStatus
	OrderId string
}

// InquireSeckill 查询秒杀状态
func InquireSeckill(ctx context.Context, req *InquireSeckillRequest) (*InquireSeckillResponse, error) {
	keyticket := fmt.Sprintf("seckill:ticket:%s:%s", req.SeckillID, req.UserID)
	field2 := "status"

	status, err := redisclient.Rdb.HGet(ctx, keyticket, field2).Result()
	if err != nil {
		if err == redis.Nil {
			// redis中没有记录，不一定是未参与，可能是已经过期，返回未参与状态，让用户重新加入队列也可以得到结果
			// 如果用户不再次加入队列，如何获取真实状态？
			return &InquireSeckillResponse{Status: pb.InquireSeckillStatus_IS_NOT_PARTICIPATING}, nil
		}
		log.Error("InquireSeckill redisclient.Rdb.HGet", "err", err)
		return nil, err
	}

	log.Info("InquireSeckill", "status", status)

	// 缓存状态是秒杀成功
	if status == pb.InquireSeckillStatus_IS_SUCCESS.String() {
		filter := bson.D{
			{Key: "seckill_id", Value: req.SeckillID},
			{Key: "user_id", Value: req.UserID},
		}
		doc, err := mongodb.ModelSecKillResult.FindOne(ctx, filter)
		if err != nil {
			log.Error("InquireSeckill mongodb.ModelSecKillResult.FindOne", "seckill_id", req.SeckillID, "user_id", req.UserID, "err", err)
			return nil, err
		}

		// 从redis中删除
		_, err = redisop.Del(ctx, keyticket)
		if err != nil {
			log.Error("InquireSeckill redisop.Del", "key", keyticket, "err", err)
			return nil, err
		}

		if doc == nil {
			return &InquireSeckillResponse{Status: pb.InquireSeckillStatus_IS_FAILED}, nil
		}
		return &InquireSeckillResponse{Status: pb.InquireSeckillStatus_IS_SUCCESS, OrderId: doc.ID.Hex()}, nil
	}

	// 缓存状态是秒杀失败
	if status == pb.InquireSeckillStatus_IS_FAILED.String() {
		return &InquireSeckillResponse{Status: pb.InquireSeckillStatus_IS_FAILED}, nil
	}

	// 缓存状态是排队中
	if status == pb.InquireSeckillStatus_IS_QUEUEING.String() {
		maxPollCount := 10
		for range maxPollCount {
			status, err = redisclient.Rdb.HGet(ctx, keyticket, field2).Result()
			if err != nil {
				if err == redis.Nil {
					return &InquireSeckillResponse{Status: pb.InquireSeckillStatus_IS_NOT_PARTICIPATING}, nil
				}
				log.Error("InquireSeckill redisclient.Rdb.HGet", "err", err)
				return nil, err
			}
			log.Info("InquireSeckill loop", "status", status)
			if status != pb.InquireSeckillStatus_IS_QUEUEING.String() {
				return &InquireSeckillResponse{Status: pb.InquireSeckillStatus(pb.InquireSeckillStatus_value[status])}, nil
			}
			time.Sleep(1000 * time.Millisecond) // sleep 1000ms
		}
	}

	return nil, errors.New("InquireSeckill timeout")
}

type CheckSeckillResultRequest struct {
	SeckillID string
	UserID    string
}

type CheckSeckillResultResponse struct {
	Success bool
	OrderId string
}

func CheckSeckillResult(ctx context.Context, req *CheckSeckillResultRequest) (*CheckSeckillResultResponse, error) {
	filter := bson.D{
		{Key: "seckill_id", Value: req.SeckillID},
		{Key: "user_id", Value: req.UserID},
	}
	doc, err := mongodb.ModelSecKillResult.FindOne(ctx, filter)
	if err != nil {
		log.Error("CheckSeckillResult mongodb.ModelSecKillResult.FindOne", "seckill_id", req.SeckillID, "user_id", req.UserID, "err", err)
		return nil, err
	}

	if doc == nil {
		return &CheckSeckillResultResponse{Success: false}, nil
	}

	return &CheckSeckillResultResponse{Success: true, OrderId: doc.ID.Hex()}, nil
}
