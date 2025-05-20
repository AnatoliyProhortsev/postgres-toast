package main

import (
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

// -----------------------------JSONB interface
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

// ----------------------DB API --------------------
// test_table : {id int, info jsonb}
type Row struct {
	ID   int   `json:"id" db:"id"`
	Info JSONB `json:"info" db:"info"`
}

// request_log : {id }

// Storage: *sqlx.DB
type Storage struct {
	DB *sqlx.DB
}

// ----------------------Monitor API ---------------
// - Пакет с размером TOST таблиц
type ToastSizePacket struct {
	ToastSizeBytes int64 `json:"toast_size_bytes"`
}

type InfoKey int

const (
	k_imdb_id InfoKey = iota // Воскресенью присваивается 0
	k_height                 // Понедельнику присваивается 1, прирост относительно воскресенья
	k_roles                  // Вторнику присваивается 2, прирост относительно понедельника
)

// - информация об одном запросе - время и размер jsonb
type QueryInfo struct {
	Size int64   `json:"size"`
	Time int64   `json:"time"`
	Key  InfoKey `json:"key"`
}

// - Пакет с набором точек (размер,время)
type QueryStatPacket struct {
	Points []QueryInfo `json:"points"`
}

// - Итоговый пакет с готовой статистикой
type StatPacket struct {
	ToastStat ToastSizePacket `json:"toast_stat"`
	QueryStat QueryStatPacket `json:"query_stat"`
}

// ----------------------- WorkLoad API -----------
// - Полное поле jsonb
// - info {
// -- imdb_id
// -- height
// -- roles [] }

type BaseRecord struct {
	IMDBID string        `json:"imdb_id"`
	Height string        `json:"height"`
	Roles  []RoleDetails `json:"roles"`
}

// - Объект Роли
// roles [{role, title}, {..}]
type RoleDetails struct {
	Role  string `json:"role"`
	Title string `json:"title"`
}

// ------------------------ Internal functions ------------
// - INSERT request log (duration_ms, raw_bytes)
func (s *Storage) InsertRequestLog(duration_ms int64, raw_bytes int64) {
	_, err := s.DB.Exec(
		`INSERT INTO request_log (duration_ms, raw_bytes)
             VALUES ($1, $2)`,
		duration_ms, raw_bytes,
	)
	if err != nil {
		log.Printf("(InsertRequestLog) error inserting request_log: %v", err)
	}
}

// - Получить info->imdb_id
func (s *Storage) GetImdbId(id int) (string, error) {
	var imdb string
	err := s.DB.Get(&imdb, `SELECT info ->> 'imdb_id' FROM test_table WHERE id = $1`, id)
	if err != nil {
		log.Printf("(GetImdbId) failed to SELECT info -> 'imdb_id'. id:%d err:%v", id, err)
		return "", err
	}
	return imdb, nil
}

// - Получить info->height
func (s *Storage) GetHeight(id int) (string, error) {
	var height string
	err := s.DB.Get(&height, `SELECT info -> 'height' FROM test_table WHERE id = $1`, id)
	if err != nil {
		log.Printf("(GetHeight) failed to SELECT info ->> 'imdb_id'. id:%d err:%v", id, err)
		return "", err
	}
	return height, nil
}

// - Получить info->roles
func (s *Storage) GetRoles(id int) ([]RoleDetails, error) {
	// Получение json
	var raw json.RawMessage
	err := s.DB.QueryRow(
		`SELECT info -> 'roles' FROM test_table WHERE id = $1`,
		id,
	).Scan(&raw)
	if err != nil {
		return nil, fmt.Errorf("(GetRoles) failed. id:%d err:%w", id, err)
	}

	// Unmarshall в []{role, title}
	var roles []RoleDetails
	if err := json.Unmarshal(raw, &roles); err != nil {
		return nil, fmt.Errorf("failed to unmarshal roles JSON, id:%d err:%w", id, err)
	}

	return roles, nil
}

// - Измерить доступ к jsonb полям по всем ключам
func (s *Storage) MeasureSelectPerformance(limit int) (QueryStatPacket, error) {
	var points []QueryInfo

	// Получаем все ID и размеры JSONB
	type rowInfo struct {
		ID             int   `db:"id"`
		JsonbSizeBytes int64 `db:"jsonb_size_bytes"`
	}
	var rowInfos []rowInfo
	query := `
        SELECT id, octet_length(info::text) AS jsonb_size_bytes
        FROM test_table
        WHERE info IS NOT NULL
        ORDER BY id
    `
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	err := s.DB.Select(&rowInfos, query)
	if err != nil {
		return QueryStatPacket{}, fmt.Errorf("failed to fetch row IDs and sizes: %w", err)
	}

	// Выполняем SELECT для каждой записи и измеряем время
	for _, ri := range rowInfos {
		// imdb_id ключ
		// - старт таймера
		start := time.Now()
		// - запрос imdb_id
		_, err := s.GetImdbId(ri.ID)
		if err != nil {
			log.Printf("(MeasureSelectPerformance) failed to select imdb_id. row id: %d err:%v", ri.ID, err)
		}
		// - значение таймера
		timeDur := time.Since(start).Microseconds()
		durationUs := timeDur

		// - добавляем в массив точек
		points = append(points, QueryInfo{
			Size: ri.JsonbSizeBytes,
			Time: durationUs,
			Key:  k_imdb_id,
		})

		// height ключ
		// - старт таймера
		start = time.Now()
		// - запрос imdb_id
		_, err = s.GetHeight(ri.ID)
		if err != nil {
			log.Printf("(MeasureSelectPerformance) failed to select height. row id: %d err:%v", ri.ID, err)
		}
		// - значение таймера
		timeDur = time.Since(start).Microseconds()
		durationUs = timeDur

		// - добавляем в массив точек
		points = append(points, QueryInfo{
			Size: ri.JsonbSizeBytes,
			Time: durationUs,
			Key:  k_height,
		})

		// roles ключ
		// - старт таймера
		start = time.Now()
		// - запрос imdb_id
		_, err = s.GetRoles(ri.ID)
		if err != nil {
			log.Printf("(MeasureSelectPerformance) failed measure 'roles' key. row id: %d err:%v", ri.ID, err)
		}
		// - значение таймера
		timeDur = time.Since(start).Microseconds()
		durationUs = timeDur

		// - добавляем в массив точек
		points = append(points, QueryInfo{
			Size: ri.JsonbSizeBytes,
			Time: durationUs,
			Key:  k_roles,
		})
	}
	// Проверка на пустой массив точек
	if len(points) == 0 {
		return QueryStatPacket{}, fmt.Errorf("empty points array")
	}

	// Заворачиваем массив точек в пакет
	packet := QueryStatPacket{
		Points: points,
	}

	return packet, nil
}

// БД
// - Подключение к БД
func New(connectionString string) (*Storage, error) {
	db, err := sqlx.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("opening database connection: %w", err)
	}
	return &Storage{DB: db}, nil
}

// - Отключение от БД
func (s *Storage) Stop() error {
	return s.DB.Close()
}

// - Применение sql миграций
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

// - INSERT
func (s *Storage) Add(info JSONB) error {
	start := time.Now()
	_, err := s.DB.NamedExec(`INSERT INTO test_table (info) VALUES (:info)`, map[string]interface{}{"info": info})
	if err != nil {
		return err
	}
	durationMs := int64(time.Since(start).Nanoseconds())
	_, _ = s.DB.Exec(`INSERT INTO request_log (duration_ms) VALUES ($1)`, durationMs)
	return nil
}

// - UPDATE
func (s *Storage) Update(id int, info JSONB) error {
	start := time.Now()
	_, err := s.DB.Exec(`UPDATE test_table SET info = $1 WHERE id = $2`, info, id)
	if err != nil {
		return err
	}
	durationMs := int64(time.Since(start).Nanoseconds())
	_, _ = s.DB.Exec(`INSERT INTO request_log (duration_ms) VALUES ($1)`, durationMs)
	return nil
}

// - DELETE
func (s *Storage) Delete(id int) error {
	start := time.Now()
	_, err := s.DB.Exec(`DELETE FROM test_table WHERE id=$1`, id)
	if err != nil {
		return err
	}
	durationMs := int64(time.Since(start).Nanoseconds())
	_, _ = s.DB.Exec(`INSERT INTO request_log (duration_ms) VALUES ($1)`, durationMs)
	return nil
}

// - SELECT *
func (s *Storage) GetAll() ([]Row, error) {
	var rows []Row
	start := time.Now()
	err := s.DB.Select(&rows, `SELECT id, info FROM test_table`)
	if err != nil {
		return nil, err
	}
	dur := int64(time.Since(start).Nanoseconds())
	_, _ = s.DB.Exec(`INSERT INTO request_log (duration_ms, raw_bytes) VALUES ($1,$2)`, dur, len(rows))
	return rows, nil
}

// - SELECT pg_relation_size
func (s *Storage) GetToastTablesSize() (int64, error) {
	toastRows, err := s.DB.Query(`
			SELECT 
			(pg_relation_size(reltoastrelid)) AS toast_table_size
			FROM pg_class
			WHERE relname = 'test_table';
		`)
	if err != nil {
		log.Printf("Failed to fetch table stats: %v", err)
		return 0, err
	}
	defer toastRows.Close()

	var resultSize int64

	for toastRows.Next() {
		if err := toastRows.Scan(&resultSize); err != nil {
			log.Printf("Failed to read TOAST statistics: %v", err)
			return 0, err
		}
		log.Printf("Readed %d bytes toast table", resultSize)
	}

	return resultSize, nil
}

// --------------------------------- Handlers -----------------------------
// - Запрос статистики -> {toast_stat, query_stat}
func statsHandler(s *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var toastPacket ToastSizePacket

		// toast_stat
		size, err := s.GetToastTablesSize()
		if err != nil {
			http.Error(w, "Ошибка при сборе статистики TOAST: "+err.Error(), http.StatusInternalServerError)
		}
		toastPacket.ToastSizeBytes = size

		// query_stat
		queryPacket, err := s.MeasureSelectPerformance(0)
		if err != nil {
			log.Printf("(statsHandler) error: %v", err)
			http.Error(w, "Ошибка при тестировании запросов: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// statPacket
		result := StatPacket{
			ToastStat: toastPacket,
			QueryStat: queryPacket,
		}

		// тип данных
		w.Header().Set("Content-Type", "application/json")
		// запись в json
		if err := json.NewEncoder(w).Encode(result); err != nil {
			http.Error(w, "Ошибка сериализации JSON: "+err.Error(), http.StatusInternalServerError)
		}
	}
}

// - /addRow -> AddRowHandler
func addRowHandler(s *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var row Row
		if err := json.NewDecoder(r.Body).Decode(&row); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.Add(row.Info); err != nil {
			log.Printf("(addRowHandler) error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

// - /deleteRow/{id}" -> deleteRowHandler
func deleteRowHandler(s *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var id int
		// забираем id из заголовка
		if err := mux.Vars(r)["id"]; err != "" {
			fmt.Sscanf(mux.Vars(r)["id"], "%d", &id)
		}
		// запрос delete
		if err := s.Delete(id); err != nil {
			log.Printf("(deleteRowHandler) error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// статус
		w.WriteHeader(http.StatusNoContent)
	}
}

// - /updateRow -> updateRowHandler
func updateRowHandler(s *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var row Row
		// декодируем структуру {id, info} из json
		if err := json.NewDecoder(r.Body).Decode(&row); err != nil {
			log.Printf("(updateRowHandler) decode error: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// update запрос
		if err := s.Update(row.ID, row.Info); err != nil {
			log.Printf("(updateRowHandler) update error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// статус
		w.WriteHeader(http.StatusOK)
	}
}

// - /getRows -> getRowsHandler
func getRowsHandler(s *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// select *
		rows, err := s.GetAll()
		if err != nil {
			log.Printf("(getRowsHandler) error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// тип ответа
		w.Header().Set("Content-Type", "application/json")
		// парсинг json в ответ
		json.NewEncoder(w).Encode(rows)
	}
}

// ------------------------- Entrypoint --------------------------
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
			fmt.Printf("Connected to DB\n")
			defer storage.Stop()
			break
		}
		// Если не удалось подключиться, повторяем попытку
		fmt.Printf("Attempting to reconnect to DB\n")
		time.Sleep(2 * time.Second)
	}

	// Выполнение миграций
	if err = ApplyMigrations(storage); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	// Инициализация маршрутов HTTP API
	r := mux.NewRouter()
	// Общие методы
	r.HandleFunc("/addRow", addRowHandler(storage)).Methods("POST")
	r.HandleFunc("/deleteRow/{id}", deleteRowHandler(storage)).Methods("DELETE")
	r.HandleFunc("/updateRow", updateRowHandler(storage)).Methods("PUT")
	r.HandleFunc("/getRows", getRowsHandler(storage)).Methods("GET")

	// Метод получения статистики по запросам
	r.HandleFunc("/stats", statsHandler(storage)).Methods("GET")

	addr := ":8080"
	log.Printf("Listening on %s...", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
