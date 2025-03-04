package mysqldb

import (
	"context"
	"database/sql"

	"github.com/charmbracelet/log"
)

func QuerySeckillByID(ctx context.Context, id int64, nilIsError bool) (*EntitySeckill, error) {
	var seckill EntitySeckill
	err := DB.QueryRow("SELECT id, name, description, start_time, end_time, total, remaining, finished FROM seckill WHERE id = ?", id).Scan(&seckill.ID, &seckill.Name, &seckill.Description, &seckill.StartTime, &seckill.EndTime, &seckill.Total, &seckill.Remaining, &seckill.Finished)
	if err != nil {
		if err == sql.ErrNoRows && !nilIsError {
			return nil, nil
		}
		log.Error("mysqldb dao QuerySeckillByID", "err", err)
		return nil, err
	}
	return &seckill, nil
}

func InsertSeckill(ctx context.Context, seckill *EntitySeckill) (int64, error) {
	result, err := DB.Exec("INSERT INTO seckill (name, description, start_time, end_time, total, remaining, finished, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", seckill.Name, seckill.Description, seckill.StartTime, seckill.EndTime, seckill.Total, seckill.Remaining, seckill.Finished, seckill.CreatedAt)
	if err != nil {
		log.Error("mysqldb dao InsertSeckill", "err", err)
		return 0, err
	}
	return result.LastInsertId()
}
