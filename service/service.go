package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/anchel/mini-seckill/mongodb"
	"github.com/anchel/mini-seckill/redisclient"
	"github.com/charmbracelet/log"
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

func CreateSeckill(ctx context.Context, req *CreateSeckillRequest) (*CreateSeckillResponse, error) {
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
