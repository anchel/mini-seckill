package server

import (
	"context"
	"errors"

	pb "github.com/anchel/mini-seckill/proto"
	"github.com/anchel/mini-seckill/service"
	"github.com/charmbracelet/log"
)

type SeckillServer struct {
	pb.UnimplementedSeckillServiceServer
}

func (s *SeckillServer) CreateSeckill(ctx context.Context, in *pb.CreateSeckillRequest) (*pb.CreateSeckillResponse, error) {
	log.Infof("Received: %v", in)
	if in.Name == "" {
		return nil, errors.New("name is empty")
	}
	if in.Description == "" {
		return nil, errors.New("description is empty")
	}
	if in.StartTime == 0 {
		return nil, errors.New("start_time is empty")
	}
	if in.EndTime == 0 {
		return nil, errors.New("end_time is empty")
	}
	if in.Total == 0 {
		return nil, errors.New("total is empty")
	}
	req := &service.CreateSeckillRequest{
		Name:        in.Name,
		Description: in.Description,
		StartTime:   in.StartTime,
		EndTime:     in.EndTime,
		Total:       in.Total,
	}
	rsp, err := service.CreateSeckill(ctx, req)
	if err != nil {
		log.Error("service.CreateSeckill", "err", err)
		return nil, err
	}
	return &pb.CreateSeckillResponse{Id: rsp.Id}, nil
}
