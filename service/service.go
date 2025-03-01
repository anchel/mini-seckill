package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/anchel/mini-seckill/mongodb"
	"github.com/anchel/mini-seckill/redisclient"
	"github.com/charmbracelet/log"
	"github.com/redis/go-redis/v9"

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
		CreateAt:  &doc.EntityBase.CreatedAt,
	})
	if err != nil {
		log.Error("json.Marshal", "err", err)
		return nil, err
	}
	expiration := time.UnixMilli(doc.EndTime).Sub(time.Now())
	log.Info("redisclient.Rdb.Set", "key", key, "expiration", expiration.Seconds())
	err = redisclient.Rdb.Set(ctx, key, bs, expiration).Err()
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
	CreatedAt   *time.Time
}

// GetSeckill gets a seckill activity
func GetSeckill(ctx context.Context, req *GetSeckillRequest) (*GetSeckillResponse, error) {
	key := "seckill:" + req.Id
	str, err := redisclient.Rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
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
		CreatedAt:   cache.CreateAt,
	}, nil
}

type JoinSeckillRequest struct {
	SeckillID string
	UserID    string
}

type JoinSeckillResponse struct {
	Success bool
	Status  pb.JoinSeckillStatus
}

var localCache sync.Map

// JoinSeckill joins a seckill activity
func JoinSeckill(ctx context.Context, req *JoinSeckillRequest) (*JoinSeckillResponse, error) {
	// 优先从本地缓存中获取
	// lsk, ok := localCache.Load(req.SeckillID)
	// if ok {
	// 	csk, tc := lsk.(*CacheSeckill)
	// 	if tc {
	// 		canJoin, status := checkSeckillCanJoin(csk)
	// 		if !canJoin {
	// 			return &JoinSeckillResponse{success: false, status: status}, nil
	// 		}
	// 	}
	// }

	sk, err := GetSeckill(ctx, &GetSeckillRequest{Id: req.SeckillID})
	if err != nil {
		log.Error("GetSeckill", "err", err)
		return nil, err
	}
	if sk == nil {
		return nil, errors.New("seckill is not found")
	}

	canJoin, status := checkSeckillCanJoin(&CacheSeckill{
		Id:        sk.Id,
		StartTime: sk.StartTime,
		EndTime:   sk.EndTime,
		Total:     sk.Total,
	})
	if !canJoin {
		return &JoinSeckillResponse{Success: false, Status: status}, nil
	}

	keyticket := fmt.Sprintf("seckill:ticket:%s:%s", req.SeckillID, req.UserID)
	field1 := "count"
	field2 := "status"

	keyqueue := "seckill:queue"
	val := fmt.Sprintf("%s:%s", req.SeckillID, req.UserID)

	cmds, err := redisclient.Rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HIncrBy(ctx, keyticket, field1, 1)
		pipe.HSetNX(ctx, keyticket, field2, int32(pb.JoinSeckillStatus_JOIN_SECKILL_STATUS_PARTICIPATING))
		pipe.Expire(ctx, keyticket, time.Duration(rand.Intn(600)+7200)*time.Second)
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

	if cmd0 := cmds[0]; cmd0.Err() != nil {
		log.Error("JoinSeckill redisclient.Rdb.HIncrBy", "err", cmd0.Err())
		return nil, cmd0.Err()
	}

	return &JoinSeckillResponse{Success: true, Status: pb.JoinSeckillStatus_JOIN_SECKILL_STATUS_PARTICIPATING}, nil
}

func checkSeckillCanJoin(csk *CacheSeckill) (bool, pb.JoinSeckillStatus) {
	now := time.Now().UnixMilli()
	if csk.StartTime > now {
		return false, pb.JoinSeckillStatus_JOIN_SECKILL_STATUS_NOT_START
	}
	if csk.EndTime < now {
		return false, pb.JoinSeckillStatus_JOIN_SECKILL_STATUS_FINISHED
	}
	return true, pb.JoinSeckillStatus_JOIN_SECKILL_STATUS_UNKNOWN
}
