package database

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose"
)

type Storage struct {
	DB DB
}

type DB interface {
	Prepare(query string) (*sql.Stmt, error)
	Close() error
}

func InitDB(connectionString string) (*sql.DB, error) {
	connStr := "host=db port=5432 user=app_user password=app_password dbname=app_db sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %v", err)
	}

	// Run migrations
	if err := goose.Up(db.DB, "/app/migrations"); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %v", err)
	}

	return db, nil
}
