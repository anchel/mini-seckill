package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/anchel/mini-seckill/lib/redisop"
	"github.com/anchel/mini-seckill/mysqldb"
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
	Id int64
}

// CreateSeckill creates a seckill activity
func CreateSeckill(ctx context.Context, req *CreateSeckillRequest) (*CreateSeckillResponse, error) {
	if time.Now().UnixMilli() > req.EndTime {
		return nil, errors.New("end_time is invalid")
	}

	id, err := mysqldb.InsertSeckill(ctx, &mysqldb.EntitySeckill{
		Name:        req.Name,
		Description: req.Description,
		StartTime:   time.UnixMilli(req.StartTime),
		EndTime:     time.Unix(0, req.EndTime*int64(time.Millisecond)),
		Total:       req.Total,
		Remaining:   req.Total,
		Finished:    0,
		CreatedAt:   time.Now(),
	})

	// write to redis
	err = writeSeckillToRedis(ctx, &CacheSeckill{
		Id:        id,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Total:     req.Total,
		Remaining: req.Total,
		Finished:  0,
	})
	if err != nil {
		log.Error("CreateSeckill writeSeckillToRedis", "err", err)
		return nil, err
	}

	return &CreateSeckillResponse{Id: id}, nil
}

type GetSeckillRequest struct {
	Id int64
}

type GetSeckillResponse struct {
	Id          int64
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

	doc, err := mysqldb.QuerySeckillByID(ctx, req.Id, true)
	if err != nil {
		log.Error("GetSeckill mysqldb.QuerySeckillByID", "err", err)
		return nil, err
	}

	return &GetSeckillResponse{
		Id:          req.Id,
		Name:        doc.Name,
		Description: doc.Description,
		StartTime:   doc.StartTime.UnixMilli(),
		EndTime:     doc.EndTime.UnixMilli(),
		Total:       doc.Total,
		Remaining:   doc.Remaining,
		Finished:    doc.Finished,
		CreatedAt:   &doc.CreatedAt,
	}, nil
}

type JoinSeckillRequest struct {
	SeckillID int64
	UserID    int64
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

	if status != pb.JoinSeckillStatus_JOIN_UNKNOWN {
		return &JoinSeckillResponse{Status: status}, nil
	}

	// 加入队列
	nowmill := time.Now().UnixMilli()
	keyticket := fmt.Sprintf("seckill:ticket:%d:%d", req.SeckillID, req.UserID)

	keyqueue := "seckill:queue"
	val := fmt.Sprintf("%d:%d:%d", req.SeckillID, req.UserID, nowmill)

	_, err = redisclient.Rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		ret := pipe.Set(ctx, keyticket, pb.InquireSeckillStatus_IS_QUEUEING.String(), time.Duration(rand.Intn(30)+80)*time.Second)
		if ret.Err() != nil {
			log.Error("JoinSeckill pipe.Set", "err", ret.Err())
			return ret.Err()
		}
		ret2 := pipe.RPush(ctx, keyqueue, val)
		if ret2.Err() != nil {
			log.Error("JoinSeckill pipe.RPush", "err", ret2.Err())
			return ret2.Err()
		}
		return nil
	})
	if err != nil {
		log.Error("JoinSeckill redisclient.Rdb.TxPipelined", "err", err)
		return nil, err
	}

	return &JoinSeckillResponse{Status: pb.JoinSeckillStatus_JOIN_SUCCESS}, nil
}

type InquireSeckillRequest struct {
	SeckillID int64
	UserID    int64
}

type InquireSeckillResponse struct {
	Status  pb.InquireSeckillStatus
	OrderId int64
}

// InquireSeckill 查询秒杀状态
func InquireSeckill(ctx context.Context, req *InquireSeckillRequest) (*InquireSeckillResponse, error) {
	keyticket := fmt.Sprintf("seckill:ticket:%d:%d", req.SeckillID, req.UserID)

	status, err := redisop.Get(ctx, keyticket, true)
	if err != nil {
		if err == redis.Nil {
			// redis中没有记录，不一定是未参与，可能是已经过期，返回未参与状态，让用户重新加入队列也可以得到结果
			// 如果用户不再次加入队列，如何获取真实状态？
			return &InquireSeckillResponse{Status: pb.InquireSeckillStatus_IS_NOT_PARTICIPATING}, nil
		}
		log.Error("InquireSeckill redisop.Get", "err", err)
		return nil, err
	}

	log.Info("InquireSeckill", "status", status)

	// 缓存状态是秒杀成功
	if status == pb.InquireSeckillStatus_IS_SUCCESS.String() {

		// TODO: 从mysql中查询订单信息，这里为了性能测试暂时注释掉
		// doc, err := mysqldb.QuerySeckillOrder(ctx, req.SeckillID, req.UserID, false)
		// if err != nil {
		// 	log.Error("InquireSeckill mysqldb.QuerySeckillOrder", "seckill_id", req.SeckillID, "user_id", req.UserID, "err", err)
		// 	return nil, err
		// }

		// if doc == nil {
		// 	return &InquireSeckillResponse{Status: pb.InquireSeckillStatus_IS_FAILED}, nil
		// }

		return &InquireSeckillResponse{Status: pb.InquireSeckillStatus_IS_SUCCESS, OrderId: 0}, nil
	}

	// 缓存状态是秒杀失败
	if status == pb.InquireSeckillStatus_IS_FAILED.String() {
		return &InquireSeckillResponse{Status: pb.InquireSeckillStatus_IS_FAILED}, nil
	}

	// 缓存状态是排队中
	if status == pb.InquireSeckillStatus_IS_QUEUEING.String() {
		maxPollCount := 120
		for range maxPollCount {
			d := time.Duration(rand.Intn(2000)+8000) * time.Millisecond
			time.Sleep(d) // sleep

			status, err := redisop.Get(ctx, keyticket, false)
			if err != nil {
				if err == redis.Nil {
					return &InquireSeckillResponse{Status: pb.InquireSeckillStatus_IS_NOT_PARTICIPATING}, nil
				}
				log.Error("InquireSeckill redisop.Get", "err", err)
				return nil, err
			}
			log.Info("InquireSeckill loop", "status", status)
			if status != pb.InquireSeckillStatus_IS_QUEUEING.String() {
				return &InquireSeckillResponse{Status: pb.InquireSeckillStatus(pb.InquireSeckillStatus_value[status])}, nil
			}

		}
	}

	return nil, errors.New("InquireSeckill timeout")
}

type CheckSeckillResultRequest struct {
	SeckillID int64
	UserID    int64
}

type CheckSeckillResultResponse struct {
	Success bool
	OrderId int64
}

func CheckSeckillResult(ctx context.Context, req *CheckSeckillResultRequest) (*CheckSeckillResultResponse, error) {

	doc, err := mysqldb.QuerySeckillOrder(ctx, req.SeckillID, req.UserID, false)
	if err != nil {
		log.Error("CheckSeckillResult mysqldb.QuerySeckillOrder", "seckill_id", req.SeckillID, "user_id", req.UserID, "err", err)
		return nil, err
	}

	if doc == nil {
		return &CheckSeckillResultResponse{Success: false}, nil
	}

	return &CheckSeckillResultResponse{Success: true, OrderId: doc.ID}, nil
}
