package main

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

// JSONB реализует поддержку PostgreSQL jsonb
// Используется для автоматического сканирования и сохранения
// JSON-данных как map[string]interface{}
type JSONB map[string]interface{}

func (j *JSONB) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to scan JSONB: expected []byte, got %T", value)
	}
	return json.Unmarshal(bytes, j)
}

func (j JSONB) Value() (driver.Value, error) {
	return json.Marshal(j)
}

// Row представляет одну запись в test_table и служит абстрактным типом для CRUD операций
type Row struct {
	ID   int    `json:"id" db:"id"`
	Name string `json:"name" db:"name"`
	Info JSONB  `json:"info" db:"info"`
}

// Storage обёртка над sqlx.DB
// Хранит соединение и методы для работы через структуру Row
type Storage struct {
	DB *sqlx.DB
}

// New создаёт соединение к БД по строке подключения
func New(connectionString string) (*Storage, error) {
	db, err := sqlx.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("opening database connection: %w", err)
	}
	return &Storage{DB: db}, nil
}

// Stop закрывает соединение к БД
func (s *Storage) Stop() error {
	return s.DB.Close()
}

// ApplyMigrations выполняет SQL-миграции из файла migrations.sql
func ApplyMigrations(s *Storage) error {
	if s == nil || s.DB == nil || s.DB.DB == nil {
		return fmt.Errorf("ApplyMigrations: invalid storage provided")
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed t get working directory: %w", err)
	}
	fmt.Println("working directory: ", wd)

	files, err := os.ReadDir("./migrations")
	if err != nil {
		return fmt.Errorf("cannot read migrations directory: %w", err)
	}
	fmt.Printf("Found %d files in migrations:\n", len(files))
	for _, f := range files {
		fmt.Println("  -", f.Name())
	}

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}
	if err := goose.Up(s.DB.DB, "./migrations"); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}

// Add добавляет новую запись, используя абстракцию Row
func (s *Storage) Add(r Row) error {
	_, err := s.DB.NamedExec(
		`INSERT INTO test_table (id, name, info) VALUES (:id, :name, :info)`,
		r,
	)
	return err
}

// Delete удаляет запись по ID
func (s *Storage) Delete(id int) error {
	_, err := s.DB.Exec(
		`DELETE FROM test_table WHERE id = $1`, id,
	)
	return err
}

// Update обновляет запись по ID, используя абстракцию Row
func (s *Storage) Update(r Row) error {
	_, err := s.DB.NamedExec(
		`UPDATE test_table SET name = :name, info = :info WHERE id = :id`,
		r,
	)
	return err
}

// GetAll возвращает все записи из test_table
func (s *Storage) GetAll() ([]Row, error) {
	var rows []Row
	if err := s.DB.Select(&rows, `SELECT id, name, info FROM test_table`); err != nil {
		return nil, err
	}
	return rows, nil
}

// HTTP-обработчики
func addRowHandler(s *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var row Row
		if err := json.NewDecoder(r.Body).Decode(&row); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.Add(row); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

func deleteRowHandler(s *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var id int
		if err := mux.Vars(r)["id"]; err != "" {
			fmt.Sscanf(mux.Vars(r)["id"], "%d", &id)
		}
		if err := s.Delete(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func updateRowHandler(s *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var row Row
		if err := json.NewDecoder(r.Body).Decode(&row); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.Update(row); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func getRowsHandler(s *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := s.GetAll()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rows)
	}
}

func main() {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://postgres:pass@postgres:5432/postgres?sslmode=disable"
	}

	var storage *Storage
	var err error

	for {
		storage, err = New(connStr)

		if err == nil {
			err = storage.DB.Ping()
		}

		if err == nil {
			fmt.Println("Connected to DB")
			defer storage.Stop()
			break
		}
	}

	if err = ApplyMigrations(storage); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	r := mux.NewRouter()
	r.HandleFunc("/addRow", addRowHandler(storage)).Methods("POST")
	r.HandleFunc("/deleteRow/{id}", deleteRowHandler(storage)).Methods("DELETE")
	r.HandleFunc("/updateRow", updateRowHandler(storage)).Methods("PUT")
	r.HandleFunc("/getRows", getRowsHandler(storage)).Methods("GET")

	addr := ":8080"
	log.Printf("Listening on %s...", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
