package server

import (
	"context"

	pb "github.com/anchel/mini-seckill/proto"
	"github.com/charmbracelet/log"
)

type SeckillServer struct {
	pb.UnimplementedSeckillServiceServer
}

func (s *SeckillServer) CreateSeckill(_ context.Context, in *pb.CreateSeckillRequest) (*pb.CreateSeckillResponse, error) {
	log.Infof("Received: %v", in)
	return &pb.CreateSeckillResponse{Id: "seckill001"}, nil
}
