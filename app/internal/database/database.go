package database

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose"
)

type Storage struct {
	DB *sql.DB
}

type DB interface {
	Prepare(query string) (*sql.Stmt, error)
	Close() error
}

func New(connectionString string) (*Storage, error) {
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", "opening database connection: ", err)
	}

	return &Storage{DB: db}, nil
}

func (s *Storage) Stop() error {
	return s.DB.Close()
}

func ApplyMigrations(s *Storage) error {
	if err := goose.Up(s.DB, "migrations.sql"); err != nil {
		return fmt.Errorf("failed to run migrations %v", err)
	}
	return nil
}

func NewRow(s *Storage, id int, name string, info JSONB) error {
	stmt, err := s.DB.Prepare()
}
