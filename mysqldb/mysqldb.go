package mysqldb

import (
	"database/sql"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"
)

var DB *sql.DB

func InitDB() error {
	conf := mysql.NewConfig()
	conf.Addr = os.Getenv("MYSQL_HOST")
	conf.User = os.Getenv("MYSQL_USER")
	conf.Passwd = os.Getenv("MYSQL_PASSWORD")
	conf.DBName = os.Getenv("MYSQL_DB")
	conf.Net = "tcp"
	conf.Loc = time.Local
	conf.ParseTime = true

	db, err := sql.Open("mysql", conf.FormatDSN())
	if err != nil {
		return err
	}
	// See "Important settings" section.
	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

	err = db.Ping()
	if err != nil {
		return err
	}

	DB = db
	return nil
}
