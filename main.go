package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/anchel/mini-seckill/lib/utils"
	"github.com/anchel/mini-seckill/mongodb"
	pb "github.com/anchel/mini-seckill/proto"
	"github.com/anchel/mini-seckill/redisclient"
	"github.com/anchel/mini-seckill/server"
	"github.com/anchel/mini-seckill/service"
	"github.com/charmbracelet/log"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
)

func main() {
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exit := func() {
		os.Exit(-1)
	}

	wgExit := sync.WaitGroup{}

	exe, err := os.Executable()
	if err != nil {
		log.Error("os.Executable error", "message", err)
		return
	}
	log.Info("executable", "path", exe)

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
	err = redisclient.InitRedis(rootCtx, os.Getenv("REDIS_ADDR"), os.Getenv("REDIS_PASSWORD"), 0)
	if err != nil {
		log.Error("init redis error", err)
		return
	}

	// init mongodb
	mongoClient, err := mongodb.InitMongoDB()
	if err != nil {
		log.Error("Error mongodb.InitMongoDB")
		return
	}
	defer mongoClient.Disconnect(context.Background())

	// 定期检查mongo数据库健康状态
	go func() {
		for {
			time.Sleep(60 * time.Second)
			if !mongoClient.HealthCheck(rootCtx) {
				log.Warn("Trying to reconnect to MongoDB...")
				if err := mongoClient.Reconnect(rootCtx); err != nil {
					log.Error("Failed to reconnect to MongoDB", err.Error())
					exit()
				}
			}
		}
	}()

	err = service.InitLogicLocalCache(rootCtx)
	if err != nil {
		log.Error("service.InitLogic error", "message", err)
		return
	}

	wgExit.Add(1)
	go service.InitLogicReadQueue(rootCtx, &wgExit)

	// init grpc server
	wgExit.Add(1)
	go func() {
		defer wgExit.Done()

		port := os.Getenv("GRPC_PORT")
		lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
		if err != nil {
			log.Errorf("failed to listen: %v", err)
			return
		}
		s := grpc.NewServer()
		pb.RegisterSeckillServiceServer(s, &server.SeckillServer{})
		log.Infof("server listening at %v", lis.Addr())
		if err := s.Serve(lis); err != nil {
			log.Errorf("failed to serve: %v", err)
		}
	}()

	wgExit.Wait()
}
