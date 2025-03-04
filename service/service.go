package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

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
		log.Error("CreateSeckill mongodb.ModelSecKill.InsertOne", "err", err)
		return nil, err
	}

	// write to redis
	err = writeSeckillToRedis(ctx, &CacheSeckill{
		Id:        id,
		StartTime: doc.StartTime,
		EndTime:   doc.EndTime,
		Total:     doc.Total,
		Remaining: doc.Remaining,
		Finished:  doc.Finished,
	})
	if err != nil {
		log.Error("CreateSeckill writeSeckillToRedis", "err", err)
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

	doc, err := mongodb.ModelSecKill.FindByID(ctx, req.Id)
	if err != nil {
		log.Error("GetSeckill mongodb.ModelSecKill.FindByID", "err", err)
		return nil, err
	}
	if doc == nil {
		log.Info("GetSeckill not found in database", "id", req.Id)
		return nil, nil
	}

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

type JoinSeckillRequest struct {
	SeckillID string
	UserID    string
}

type JoinSeckillResponse struct {
	Status pb.JoinSeckillStatus
}

var localCache sync.Map

// JoinSeckill joins a seckill activity
func JoinSeckill(ctx context.Context, req *JoinSeckillRequest) (*JoinSeckillResponse, error) {

	status, err := checkCanJoinSeckill(ctx, req.SeckillID, req.UserID)
	if err != nil {
		log.Error("JoinSeckill checkCanJoinSeckill", "SeckillID", req.SeckillID, "UserID", req.UserID, "err", err)
		return nil, err
	}

	if status != pb.JoinSeckillStatus_JS_UNKNOWN {
		return &JoinSeckillResponse{Status: status}, nil
	}

	// 加入队列
	nowmill := time.Now().UnixMilli()
	keyticket := fmt.Sprintf("seckill:ticket:%s:%s", req.SeckillID, req.UserID)
	fieldCount := "count"
	fieldStatus := "status"

	keyqueue := "seckill:queue"
	val := fmt.Sprintf("%s:%s:%d", req.SeckillID, req.UserID, nowmill)

	cmds, err := redisclient.Rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HIncrBy(ctx, keyticket, fieldCount, 1)
		pipe.HSet(ctx, keyticket, fieldStatus, pb.InquireSeckillStatus_IS_QUEUEING.String()) // 默认状态是排队中
		pipe.Expire(ctx, keyticket, time.Duration(rand.Intn(30)+60)*time.Second)
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
