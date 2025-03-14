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
	"github.com/google/uuid"
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
	Ticket string
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
		return &JoinSeckillResponse{Status: status, Ticket: ""}, nil
	}

	// 加入队列
	nowmill := time.Now().UnixMilli()
	uuidStr := uuid.NewString()
	// keyticket := fmt.Sprintf("seckill:ticket:%d:%d:%s", req.SeckillID, req.UserID, uuidStr)
	keyticket := fmt.Sprintf("seckill-ticket-%s", uuidStr)

	keyqueue := "seckill:queue"
	val := fmt.Sprintf("%d:%d:%d:%s", req.SeckillID, req.UserID, nowmill, keyticket)

	_, err = redisclient.Rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		_, err := pipe.Set(ctx, keyticket, pb.InquireSeckillStatus_IS_QUEUEING.String(), time.Duration(rand.Intn(30)+80)*time.Second).Result()
		if err != nil {
			log.Error("JoinSeckill pipe.Set", "err", err)
			return err
		}
		_, err = pipe.RPush(ctx, keyqueue, val).Result()
		if err != nil {
			log.Error("JoinSeckill pipe.RPush", "err", err)
			return err
		}
		return nil
	})
	if err != nil {
		log.Error("JoinSeckill redisclient.Rdb.TxPipelined", "err", err)
		return nil, err
	}

	// 提前load进去，避免后面沦陷的时候已经错过通知
	notifyChan := make(chan string, 1)
	notifyMap.LoadOrStore(keyticket, notifyChan)
	go func() {
		time.Sleep(20 * time.Second)
		notifyMap.Delete(keyticket)
	}()

	return &JoinSeckillResponse{Status: pb.JoinSeckillStatus_JOIN_SUCCESS, Ticket: keyticket}, nil
}

type InquireSeckillRequest struct {
	SeckillID int64
	UserID    int64
	Ticket    string
}

type InquireSeckillResponse struct {
	Status  pb.InquireSeckillStatus
	OrderId int64
}

// InquireSeckill 查询秒杀状态
func InquireSeckill(ctx context.Context, req *InquireSeckillRequest) (*InquireSeckillResponse, error) {
	keyticket := req.Ticket

	status, err := redisop.Get(ctx, keyticket, true)
	if err != nil {
		if err == redis.Nil {
			// redis中没有记录，不一定是未参与，可能是已经过期，返回未参与状态，让用户重新加入队列也可以得到结果
			// 如果用户不再次加入队列，如何获取真实状态？
			return &InquireSeckillResponse{Status: pb.InquireSeckillStatus_IS_NOT_PARTICIPATING}, nil
		}
		log.Error("InquireSeckill redisop.Get 11111", "err", err)
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
		subCtx, cancel := context.WithTimeout(ctx, 100*time.Second)
		defer cancel()

		errorChan := make(chan error, 1)
		statusChan := make(chan string, 1)

		sendError := func(err error) {
			select {
			case errorChan <- err:
			default:
			}
		}

		sendStatus := func(status string) {
			select {
			case statusChan <- status:
			default:
			}
		}

		// go func() {
		// 	keyticketsub := fmt.Sprintf("%s:channel", keyticket)
		// 	redisClient := redisclient.GetPubSubClient(ctx, os.Getenv("REDIS_ADDR"), os.Getenv("REDIS_PASSWORD"))
		// 	defer redisClient.Close()

		// 	pubsub := redisClient.Subscribe(subCtx, keyticketsub)
		// 	defer pubsub.Close()

		// 	ch := pubsub.Channel()

		// 	select {
		// 	case <-subCtx.Done():
		// 		return
		// 	case msg := <-ch:
		// 		log.Info("InquireSeckill subscribe", "msg", msg.Payload)
		// 		// if msg.Payload == pb.InquireSeckillStatus_IS_SUCCESS.String() {
		// 		// 	log.Error("InquireSeckill subscribe", "UserID", req.UserID, "msg", msg.Payload)
		// 		// }
		// 		sendStatus(msg.Payload)
		// 	}
		// }()

		go func() {
			notifyChan := make(chan string, 1)
			result, loaded := notifyMap.LoadOrStore(keyticket, notifyChan)
			if loaded {
				notifyChan = result.(chan string)
			}
			defer notifyMap.Delete(keyticket)
			select {
			case <-subCtx.Done():
			case status := <-notifyChan:
				log.Info("InquireSeckill notifyChan receive", "ticket", keyticket, "status", status)
				sendStatus(status)
			}
		}()

		go func() {
			for {
				delay := time.Duration(rand.Intn(2000)+12000) * time.Millisecond

				select {
				case <-subCtx.Done():
					return
				case <-time.After(delay):
					status, err := redisop.Get(context.Background(), keyticket, true)
					if err != nil {
						// log.Error("InquireSeckill loop redisop.Get error")
						if err == redis.Nil {
							status = pb.InquireSeckillStatus_IS_NOT_PARTICIPATING.String()
							sendStatus(status)
							return
						} else {
							sendError(err)
							return
						}
					}
					if status != pb.InquireSeckillStatus_IS_QUEUEING.String() {
						sendStatus(status)
						return
					}

					log.Info("InquireSeckill loop", "seckillID", req.SeckillID, "userID", req.UserID, "status", status)
				}
			}
		}()

		select {
		case <-subCtx.Done():
			return nil, errors.New("InquireSeckill timeout")
		case err := <-errorChan:
			return nil, err
		case status := <-statusChan:
			return &InquireSeckillResponse{Status: pb.InquireSeckillStatus(pb.InquireSeckillStatus_value[status])}, nil
		}
	}

	return nil, errors.New("InquireSeckill unknown status")
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
