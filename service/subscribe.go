package service

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anchel/mini-seckill/redisclient"
	"github.com/charmbracelet/log"
)

var countReceive int64
var notifyMap sync.Map

func init() {
	go func() {
		for {
			time.Sleep(3 * time.Second)
			log.Warn("subscribe loop receive", "countReceive", countReceive)
		}
	}()
}

func StartSubscribeNotify(ctx context.Context) {
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	subkey := "seckill:notify"
	pubsub := redisclient.Rdb.Subscribe(subCtx, subkey)
	defer pubsub.Close()

	ch := pubsub.Channel()

	var exit bool

loop:
	for {
		var ctxChan <-chan struct{}
		if !exit {
			ctxChan = subCtx.Done()
		}
		select {
		case <-ctxChan:
			exit = true
		case msg, readOK := <-ch:
			if !readOK {
				break loop
			}
			atomic.AddInt64(&countReceive, 1)
			log.Info("subscribe notify receive msg", "msg", msg.Payload)
			var snm SubscribeNotifyMessage
			err := json.Unmarshal([]byte(msg.Payload), &snm)
			if err != nil {
				log.Error("subscribe notify Unmarshal error", "err", err)
				continue loop
			}
			val, ok := notifyMap.Load(snm.Ticket)
			if ok {
				ch := val.(chan string)
				select {
				case ch <- snm.Status:
				default:
				}
			}
		}
	}
}

type SubscribeNotifyMessage struct {
	Ticket string `json:"ticket"`
	Status string `json:"status"`
}

func (snm SubscribeNotifyMessage) MarshalBinary() ([]byte, error) {
	bs, err := json.Marshal(snm)
	// log.Error("MarshalBinary", "str", string(bs))
	return bs, err
}
