package db

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type MySQL struct {
	DB *sql.DB
}

type Options struct {
	DSN          string
	MaxOpenConns int
	MaxIdleConns int
	ConnMaxLife  time.Duration
	ConnMaxIdle  time.Duration
	PingTimeout  time.Duration
}

func Open(opt Options) (*MySQL, error) {
	if opt.MaxOpenConns <= 0 {
		opt.MaxOpenConns = 50
	}
	if opt.MaxIdleConns <= 0 {
		opt.MaxIdleConns = 25
	}
	if opt.ConnMaxLife == 0 {
		opt.ConnMaxLife = 30 * time.Minute
	}
	if opt.ConnMaxIdle == 0 {
		opt.ConnMaxIdle = 5 * time.Minute
	}
	if opt.PingTimeout == 0 {
		opt.PingTimeout = 2 * time.Second
	}

	db, err := sql.Open("mysql", opt.DSN)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(opt.MaxOpenConns)
	db.SetMaxIdleConns(opt.MaxIdleConns)
	db.SetConnMaxLifetime(opt.ConnMaxLife)
	db.SetConnMaxIdleTime(opt.ConnMaxIdle)

	ctx, cancel := context.WithTimeout(context.Background(), opt.PingTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &MySQL{DB: db}, nil
}

func (m *MySQL) Close() error {
	if m == nil || m.DB == nil {
		return nil
	}
	return m.DB.Close()
}
