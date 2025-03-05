package mysqldb

import "time"

type EntitySeckill struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Total       int64     `json:"total"`
	Remaining   int64     `json:"remaining"`
	Finished    int32     `json:"finished"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	DeleteAt    time.Time `json:"delete_at"`
}

type EntitySeckillOrder struct {
	ID        int64     `json:"id"`
	SeckillID int64     `json:"seckill_id"`
	UserID    int64     `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	DeleteAt  time.Time `json:"delete_at"`
}
