package db

import (
	"database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type Options struct {
	DSN          string
	MaxOpenConns int
	MaxIdleConns int
	ConnMaxLife  time.Duration
	ConnMaxIdle  time.Duration
}

type DB struct {
	DB *sql.DB
}

func Open(opt Options) (*DB, error) {
	d, err := sql.Open("mysql", opt.DSN)
	if err != nil {
		return nil, err
	}
	if opt.MaxOpenConns > 0 {
		d.SetMaxOpenConns(opt.MaxOpenConns)
	}
	if opt.MaxIdleConns > 0 {
		d.SetMaxIdleConns(opt.MaxIdleConns)
	}
	if opt.ConnMaxLife > 0 {
		d.SetConnMaxLifetime(opt.ConnMaxLife)
	}
	if opt.ConnMaxIdle > 0 {
		d.SetConnMaxIdleTime(opt.ConnMaxIdle)
	}
	if err := d.Ping(); err != nil {
		_ = d.Close()
		return nil, err
	}
	return &DB{DB: d}, nil
}

func (d *DB) Close() error { return d.DB.Close() }
