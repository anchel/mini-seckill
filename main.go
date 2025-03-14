package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/anchel/mini-seckill/lib/utils"
	"github.com/anchel/mini-seckill/mysqldb"
	pb "github.com/anchel/mini-seckill/proto"
	"github.com/anchel/mini-seckill/redisclient"
	"github.com/anchel/mini-seckill/server"
	"github.com/anchel/mini-seckill/service"
	"github.com/charmbracelet/log"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {

	log.SetLevel(log.WarnLevel)

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wgRoot := sync.WaitGroup{}

	// check .env file
	if utils.CheckEnvFile() {
		log.Debug("loading .env file")
		err := godotenv.Load()
		if err != nil {
			log.Error("Error loading .env file")
			return
		}
		log.Info("load .env successful")
	}

	// init redis
	err := redisclient.InitRedis(os.Getenv("REDIS_ADDR"), os.Getenv("REDIS_PASSWORD"), 0)
	if err != nil {
		log.Error("init redis error", err)
		return
	}
	wgRoot.Add(1)
	go func() {
		defer wgRoot.Done()
		<-rootCtx.Done()
		redisclient.Close()
	}()

	// init mysql
	err = mysqldb.InitDB()
	if err != nil {
		log.Error("service.InitDB error", "message", err)
		return
	}
	wgRoot.Add(1)
	go func() {
		defer wgRoot.Done()
		<-rootCtx.Done()
		mysqldb.CloseDB()
	}()

	wgRoot.Add(1)
	go func() {
		defer wgRoot.Done()
		service.StartSubscribeNotify(rootCtx)
	}()

	wgRoot.Add(1)
	go func() {
		defer wgRoot.Done()
		service.StartReadJoinQueue(rootCtx)
	}()

	port := os.Getenv("GRPC_PORT")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Errorf("failed to listen: %v", err)
		return
	}
	s := grpc.NewServer()
	pb.RegisterSeckillServiceServer(s, &server.SeckillServer{})
	// Register reflection service on gRPC server.
	reflection.Register(s)

	// init grpc server
	wgRoot.Add(1)
	go func() {
		defer wgRoot.Done()
		<-rootCtx.Done()
		s.GracefulStop()
	}()

	go func() {
		signalChan := make(chan os.Signal, 1)
		signal.Notify(signalChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		<-signalChan
		cancel()
	}()

	log.Infof("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Errorf("failed to serve: %v", err)
	}

	wgRoot.Wait()

	log.Info("server exit")
}
