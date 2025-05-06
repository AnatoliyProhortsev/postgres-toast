package main

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

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

type DBStats struct {
	Datname      sql.NullString `db:"datname"`
	Numbackends  int            `json:"numbackends"`
	XactCommit   int64          `json:"xact_commit"`
	XactRollback int64          `json:"xact_rollback"`
	BlksRead     int64          `json:"blks_read"`
	BlksHit      int64          `json:"blks_hit"`
}

// ToastStats – статистика по TOAST
type ToastStats struct {
	TableName       string `json:"table_name"`
	ToastSizeBytes  int64  `json:"toast_size_bytes"`
	ToastSizePretty string `json:"toast_size_pretty"`
}

// FullStats объединяет обе статистики
type AllStats struct {
	DBStats            []DBStats    `json:"db_stats"`
	ToastStats         []ToastStats `json:"toast_stats"`
	AvgSelectTimeMs    float64      `json:"avg_select_time_ms"`
	AvgInsertTimeMs    float64      `json:"avg_insert_time_ms"`
	AvgUpdateTimeMs    float64      `json:"avg_update_time_ms"`
	AvgDeleteTimeMs    float64      `json:"avg_delete_time_ms"`
	AvgInsertSizeBytes float64      `json:"avg_insert_size_bytes"`
	AvgUpdateSizeBytes float64      `json:"avg_update_size_bytes"`
	AvgSelectSizeBytes float64      `json:"avg_select_size_bytes"`
}

// Storage обёртка над sqlx.DB, хранит соединение и методы для работы с данными
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
		return fmt.Errorf("failed to get working directory: %w", err)
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
	start := time.Now()

	// Вычисляем размер JSONB в байтах
	infoBytes, err := json.Marshal(r.Info)
	if err != nil {
		log.Printf("failed to marshal JSONB: %v", err)
		return fmt.Errorf("failed to marshal JSONB: %w", err)
	}
	jsonbSize := len(infoBytes)

	_, err = s.DB.NamedExec(
		`INSERT INTO test_table (name, info) VALUES (:name, :info)`,
		r,
	)

	if err != nil {
		return err
	}

	durationMs := int64(time.Since(start).Nanoseconds())

	_, err = s.DB.Exec(`INSERT INTO request_log (request_type, duration_ms, raw_bytes) VALUES ($1, $2, $3)`, 4, durationMs, jsonbSize)
	if err != nil {
		log.Printf("(Add) Error while inserting request_log: %v", err)
	}

	return err
}

// Delete удаляет запись по ID
func (s *Storage) Delete(id int) error {
	start := time.Now()

	_, err := s.DB.Exec(`DELETE FROM test_table WHERE id = $1`, id)

	if err != nil {
		return err
	}

	durationMs := int64(time.Since(start).Nanoseconds())

	_, err = s.DB.Exec(`INSERT INTO request_log (request_type, duration_ms) VALUES ($1, $2)`, 3, durationMs)
	if err != nil {
		log.Printf("(Delete) Error while inserting request_log: %v", err)
	}

	return err
}

// Update обновляет запись по ID, используя абстракцию Row
func (s *Storage) Update(r Row) error {
	start := time.Now()

	// Вычисляем размер JSONB в байтах
	infoBytes, err := json.Marshal(r.Info)
	if err != nil {
		log.Printf("failed to marshal JSONB: %v", err)
		return fmt.Errorf("failed to marshal JSONB: %w", err)
	}
	jsonbSize := len(infoBytes)

	_, err = s.DB.NamedExec(`UPDATE test_table SET name = :name, info = :info WHERE id = :id`, r)

	if err != nil {
		return err
	}

	durationMs := int64(time.Since(start).Nanoseconds())

	_, err = s.DB.Exec(`INSERT INTO request_log (request_type, duration_ms, raw_bytes) VALUES ($1, $2, $3)`, 2, durationMs, jsonbSize)
	if err != nil {
		log.Printf("(Update) Error while inserting request_log: %v", err)
	}

	return err
}

// GetAll возвращает все записи из test_table
func (s *Storage) GetAll() ([]Row, error) {
	start := time.Now()
	var rows []Row
	err := s.DB.Select(&rows, `SELECT id, name, info FROM test_table`)

	if err != nil {
		return rows, err
	}

	durationMs := int64(time.Since(start).Nanoseconds())

	// Считаем суммарный размер всех jsonb
	totalSize := 0
	for _, row := range rows {
		infoBytes, err := json.Marshal(row.Info)
		if err != nil {
			log.Printf("(GetAll) failed to marshal JSONB: %v", err)
			continue
		}
		totalSize += len(infoBytes)
	}

	_, err = s.DB.Exec(`INSERT INTO request_log (request_type, duration_ms, raw_bytes) VALUES ($1, $2, $3)`, 1, durationMs, totalSize)

	if err != nil {
		log.Print("(GetAll) Error while inserting request_log: %w", err)
		return rows, err
	}

	return rows, err
}

// monitorDatabaseStats собирает статистику из базы каждые 10 секунд
func monitorDatabaseStats(s *Storage) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Пример запроса к представлению pg_stat_database
		query := `
			SELECT COALESCE(datname, ''), numbackends, xact_commit, xact_rollback, blks_read, blks_hit
			FROM pg_stat_database;
		`
		rows, err := s.DB.Query(query)
		if err != nil {
			log.Printf("Ошибка при сборе статистики: %v", err)
			continue
		}

		log.Println("Статистика базы данных:")
		for rows.Next() {
			var datname string
			var numbackends int
			var xactCommit, xactRollback, blksRead, blksHit int64
			if err := rows.Scan(&datname, &numbackends, &xactCommit, &xactRollback, &blksRead, &blksHit); err != nil {
				log.Printf("Ошибка сканирования статистики: %v", err)
				continue
			}
			log.Printf("БД: %s | Соединения: %d | Коммиты: %d | Откаты: %d | Чтения блоков: %d | Кэш попаданий: %d",
				datname, numbackends, xactCommit, xactRollback, blksRead, blksHit)
		}
		rows.Close()
	}
}

// statsFullHandler собирает и возвращает расширенную статистику, включая статистику TOAST
func statsHandler(s *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var fullStats AllStats

		// Стандартная статистика базы
		dbStatsRows, err := s.DB.Query(`
			SELECT datname, numbackends, xact_commit, xact_rollback, blks_read, blks_hit
			FROM pg_stat_database;
		`)
		if err != nil {
			http.Error(w, "Ошибка при сборе статистики: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer dbStatsRows.Close()

		for dbStatsRows.Next() {
			var stats DBStats
			if err := dbStatsRows.Scan(&stats.Datname, &stats.Numbackends, &stats.XactCommit, &stats.XactRollback, &stats.BlksRead, &stats.BlksHit); err != nil {
				log.Printf("Failed to fetch database stats: %v", err)
				http.Error(w, "Ошибка чтения статистики: "+err.Error(), http.StatusInternalServerError)
				return
			}
			fullStats.DBStats = append(fullStats.DBStats, stats)
		}

		// Статистика по TOAST
		toastRows, err := s.DB.Query(`
			SELECT
			  c.relname AS table_name,
			  pg_relation_size(c.reltoastrelid) AS toast_size_bytes,
			  pg_size_pretty(pg_relation_size(c.reltoastrelid)) AS toast_size_pretty
			FROM pg_class c
			WHERE c.reltoastrelid <> 0;
		`)
		if err != nil {
			log.Printf("Failed to fetch table stats: %v", err)
			http.Error(w, "Ошибка при сборе статистики TOAST: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer toastRows.Close()

		for toastRows.Next() {
			var ts ToastStats
			if err := toastRows.Scan(&ts.TableName, &ts.ToastSizeBytes, &ts.ToastSizePretty); err != nil {
				log.Printf("Failed to read TOAST statistics: %v", err)
				http.Error(w, "Ошибка чтения статистики TOAST: "+err.Error(), http.StatusInternalServerError)
				return
			}
			fullStats.ToastStats = append(fullStats.ToastStats, ts)
		}

		// Вычисление среднего времени по каждому типу запроса
		type avgResult struct {
			RequestType int     `db:"request_type"`
			AvgDuration float64 `db:"avg_duration"`
			AvgRawBytes float64 `db:"avg_raw_bytes"`
		}
		var averages []avgResult
		err = s.DB.Select(&averages, `
			SELECT request_type,
			 COALESCE(AVG(duration_ms), 0) AS avg_duration,
			 COALESCE(AVG(raw_bytes), 0) AS avg_raw_bytes
			FROM request_log
			GROUP BY request_type
		`)
		if err != nil {
			log.Printf("(statsHandler) error: %v", err)
			http.Error(w, "Ошибка при чтении логов запросов: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Заполняем поля среднего времени
		for _, avg := range averages {
			switch avg.RequestType {
			case 1:
				fullStats.AvgSelectTimeMs = avg.AvgDuration
				fullStats.AvgSelectSizeBytes = avg.AvgRawBytes
			case 2:
				fullStats.AvgUpdateTimeMs = avg.AvgDuration
				fullStats.AvgUpdateSizeBytes = avg.AvgRawBytes
			case 3:
				fullStats.AvgDeleteTimeMs = avg.AvgDuration
			case 4:
				fullStats.AvgInsertSizeBytes = avg.AvgRawBytes
				fullStats.AvgInsertTimeMs = avg.AvgDuration
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(fullStats); err != nil {
			http.Error(w, "Ошибка сериализации JSON: "+err.Error(), http.StatusInternalServerError)
		}
	}
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
			log.Printf("(addRowHandler) error: %v", err)
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
			log.Printf("(deleteRowHandler) error: %v", err)
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
			log.Printf("(updateRowHandler) 1 error: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.Update(row); err != nil {
			log.Printf("(updateRowHandler) 2 error: %v", err)
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
			log.Printf("(getRowsHandler) error: %v", err)
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

	// Пытаемся установить подключение к базе данных
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
		// Если не удалось подключиться, повторяем попытку
		time.Sleep(2 * time.Second)
	}

	// Выполнение миграций
	if err = ApplyMigrations(storage); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	// Запуск горутины для сбора статистики с БД каждые 10 секунд
	go monitorDatabaseStats(storage)

	// Инициализация маршрутов HTTP API
	r := mux.NewRouter()
	r.HandleFunc("/addRow", addRowHandler(storage)).Methods("POST")
	r.HandleFunc("/deleteRow/{id}", deleteRowHandler(storage)).Methods("DELETE")
	r.HandleFunc("/updateRow", updateRowHandler(storage)).Methods("PUT")
	r.HandleFunc("/getRows", getRowsHandler(storage)).Methods("GET")
	r.HandleFunc("/stats", statsHandler(storage)).Methods("GET")

	addr := ":8080"
	log.Printf("Listening on %s...", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
