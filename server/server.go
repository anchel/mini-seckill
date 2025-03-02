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

// CreateSeckill 创建秒杀活动
func (s *SeckillServer) CreateSeckill(ctx context.Context, in *pb.CreateSeckillRequest) (*pb.CreateSeckillResponse, error) {
	log.Infof("CreateSeckill Received: %v", in)
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

// GetSeckill 获取秒杀活动信息
func (s *SeckillServer) GetSeckill(ctx context.Context, in *pb.GetSeckillRequest) (*pb.GetSeckillResponse, error) {
	log.Infof("GetSeckill Received: %v", in)
	if in.Id == "" {
		return nil, errors.New("id is empty")
	}
	req := &service.GetSeckillRequest{
		Id: in.Id,
	}
	rsp, err := service.GetSeckill(ctx, req)
	if err != nil {
		log.Error("service.GetSeckill", "err", err)
		return nil, err
	}
	if rsp == nil {
		// return nil, errors.New("seckill is not found")
		log.Info("seckill is not found", "id", in.Id)
		return &pb.GetSeckillResponse{}, nil
	}
	return &pb.GetSeckillResponse{
		Seckill: &pb.Seckill{
			Id:          rsp.Id,
			Name:        rsp.Name,
			Description: rsp.Description,
			StartTime:   rsp.StartTime,
			EndTime:     rsp.EndTime,
			Total:       rsp.Total,
			Remaining:   rsp.Remaining,
			CreatedAt:   rsp.CreatedAt.UnixMilli(),
		},
	}, nil
}

// JoinSeckill 加入秒杀活动
func (s *SeckillServer) JoinSeckill(ctx context.Context, in *pb.JoinSeckillRequest) (*pb.JoinSeckillResponse, error) {
	log.Infof("JoinSeckill Received: %v", in)
	if in.SeckillId == "" {
		return nil, errors.New("SeckillId is empty")
	}
	if in.UserId == "" {
		return nil, errors.New("UserId is empty")
	}

	req := &service.JoinSeckillRequest{
		SeckillID: in.SeckillId,
		UserID:    in.UserId,
	}
	rsp, err := service.JoinSeckill(ctx, req)
	if err != nil {
		log.Error("service.JoinSeckill", "err", err)
		return nil, err
	}
	return &pb.JoinSeckillResponse{
		Status: rsp.Status,
	}, nil
}

// InquireSeckill 查询秒杀状态
func (s *SeckillServer) InquireSeckill(ctx context.Context, in *pb.InquireSeckillRequest) (*pb.InquireSeckillResponse, error) {
	log.Infof("InquireSeckill Received: %v", in)
	if in.SeckillId == "" {
		return nil, errors.New("SeckillId is empty")
	}
	if in.UserId == "" {
		return nil, errors.New("UserId is empty")
	}

	req := &service.InquireSeckillRequest{
		SeckillID: in.SeckillId,
		UserID:    in.UserId,
	}
	rsp, err := service.InquireSeckill(ctx, req)
	if err != nil {
		log.Error("service.InquireSeckill", "err", err)
		return nil, err
	}
	return &pb.InquireSeckillResponse{
		Status:  rsp.Status,
		OrderId: rsp.OrderId,
	}, nil
}
