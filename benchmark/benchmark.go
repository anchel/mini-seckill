package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/anchel/mini-seckill/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	addr = flag.String("addr", "localhost:50051", "the address to connect to")
	name = flag.String("name", "aaaaaname", "Name to greet")
)

func main() {
	var countError int64
	var countEmpty int64
	var countInUserList int64
	var countJoinSuccess int64
	var countJoinFailed int64
	var countOther int64

	var countISError int64
	var countISQueue int64
	var countISSuccess int64
	var countISFailed int64
	var countISOther int64

	delayArr := []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	delayLen := len(delayArr)
	delayMutex := sync.Mutex{}

	maxC := 20001
	seckillId := int64(6)

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewSeckillServiceClient(conn)

	now := time.Now()

	var wg sync.WaitGroup
	var wgInquire sync.WaitGroup

	for i := 0; i < maxC; i++ {
		wg.Add(1)
		go func(w *sync.WaitGroup, i int) {
			// userid := int64(i + 1)
			userid := rand.Int63n(1000000) + 1

			rsp, err := c.JoinSeckill(context.Background(), &pb.JoinSeckillRequest{
				SeckillId: seckillId,
				UserId:    userid,
			})

			if err != nil {

				atomic.AddInt64(&countError, 1)
				wg.Done()
				return
			}

			var done chan bool

			if rsp.Status == pb.JoinSeckillStatus_JOIN_NO_REMAINING {
				atomic.AddInt64(&countEmpty, 1)
			} else if rsp.Status == pb.JoinSeckillStatus_JOIN_SUCCESS {
				atomic.AddInt64(&countJoinSuccess, 1)
				done = make(chan bool, 1)
				wgInquire.Add(1)
				go func() {
					defer wgInquire.Done()
					now := time.Now()

					isret, err := c.InquireSeckill(context.Background(), &pb.InquireSeckillRequest{
						SeckillId: seckillId,
						UserId:    userid,
						Ticket:    rsp.Ticket,
					})
					if err != nil {
						atomic.AddInt64(&countISError, 1)
					} else {
						if isret.Status == pb.InquireSeckillStatus_IS_QUEUEING {
							atomic.AddInt64(&countISQueue, 1)
						} else if isret.Status == pb.InquireSeckillStatus_IS_SUCCESS {
							atomic.AddInt64(&countISSuccess, 1)
						} else if isret.Status == pb.InquireSeckillStatus_IS_FAILED {
							atomic.AddInt64(&countISFailed, 1)
						} else {
							// fmt.Println("isret.Status:", isret.Status)
							atomic.AddInt64(&countISOther, 1)
						}
					}

					delay := int(math.Round(float64(time.Since(now).Milliseconds()) / 1000))
					if delay >= delayLen {
						delay = delayLen - 1
					}
					delayMutex.Lock()
					delayArr[delay]++
					delayMutex.Unlock()

					done <- true
				}()

			} else if rsp.Status == pb.JoinSeckillStatus_JOIN_IN_USERS_LIST {
				atomic.AddInt64(&countInUserList, 1)

			} else if rsp.Status == pb.JoinSeckillStatus_JOIN_FAILED {
				atomic.AddInt64(&countJoinFailed, 1)
			} else {
				atomic.AddInt64(&countOther, 1)
			}
			wg.Done()

			if done != nil {
				<-done
			}

		}(&wg, i)
	}
	wg.Wait()

	fmt.Println("seckillId", seckillId)
	fmt.Println("--------------------------------------")
	fmt.Println("time used:", time.Since(now).Seconds())

	fmt.Printf("countError: %d\n", countError)
	fmt.Printf("countEmpty: %d\n", countEmpty)
	fmt.Printf("countInUserList: %d\n", countInUserList)
	fmt.Printf("countJoinSuccess: %d\n", countJoinSuccess)
	fmt.Printf("countJoinFailed: %d\n", countJoinFailed)
	fmt.Printf("countOther: %d\n", countOther)

	fmt.Println("--------------------------------------")

	wgInquire.Wait()
	fmt.Println("time used:", time.Since(now).Seconds())

	fmt.Printf("countISError: %d\n", countISError)
	fmt.Printf("countISQueue: %d\n", countISQueue)
	fmt.Printf("countISSuccess: %d\n", countISSuccess)
	fmt.Printf("countISFailed: %d\n", countISFailed)
	fmt.Printf("countISOther: %d\n", countISOther)
	fmt.Println("delayArr:", delayArr)
}
