package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"
)

// Config определяет параметры нагрузки и генерации JSON
type Config struct {
	BaseURL        string `json:"base_url"` // URL основного сервиса, например, http://localhost:8080
	AddRowRPS      int    `json:"add_row_rps"`
	UpdateRowRPS   int    `json:"update_row_rps"`
	DeleteRowRPS   int    `json:"delete_row_rps"`
	GetRowsRPS     int    `json:"get_rows_rps"`
	NumJSONFields  int    `json:"num_json_fields"`
	StrFieldLength int    `json:"str_field_length"`
}

// LoadGenerator управляет генерацией нагрузки путем отправки HTTP запросов
type LoadGenerator struct {
	cfg     Config
	cfgLock sync.RWMutex
	stopCh  chan struct{}
	wg      sync.WaitGroup
	client  *http.Client
}

// NewLoadGenerator создает экземпляр load generator с исходной конфигурацией
func NewLoadGenerator(cfg Config) *LoadGenerator {
	return &LoadGenerator{
		cfg:    cfg,
		stopCh: make(chan struct{}),
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// updateConfig позволяет динамически изменять конфигурацию нагрузки
func (lg *LoadGenerator) updateConfig(newCfg Config) {
	lg.cfgLock.Lock()
	defer lg.cfgLock.Unlock()
	lg.cfg = newCfg
	log.Printf("Конфигурация обновлена: %+v\n", lg.cfg)
}

// generateJSON генерирует случайный JSON объект с заданным числом полей и длиной строк
func (lg *LoadGenerator) generateJSON() []byte {
	lg.cfgLock.RLock()
	numFields := lg.cfg.NumJSONFields
	strLen := lg.cfg.StrFieldLength
	lg.cfgLock.RUnlock()

	obj := make(map[string]interface{})
	for i := 0; i < numFields; i++ {
		key := fmt.Sprintf("field_%d", i)
		// Вариация типов значений: целое число, число с плавающей точкой или строка
		switch rand.Intn(3) {
		case 0:
			obj[key] = rand.Intn(1000)
		case 1:
			obj[key] = rand.Float64() * 100
		default:
			obj[key] = randomString(strLen)
		}
	}
	data, _ := json.Marshal(obj)
	return data
}

// randomString возвращает случайную строку указанной длины
func randomString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	s := make([]rune, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

// Start запускает горутины для каждого типа запроса и веб-сервер для обновления конфигурации
func (lg *LoadGenerator) Start() {
	lg.wg.Add(4)
	go lg.runAddRowRequests()
	go lg.runUpdateRowRequests()
	go lg.runDeleteRowRequests()
	go lg.runGetRowsRequests()

	// API для динамического обновления конфигурации нагрузки
	go lg.configServer()
}

// Stop завершает работу load generator
func (lg *LoadGenerator) Stop() {
	close(lg.stopCh)
	lg.wg.Wait()
	log.Println("Load generator остановлен.")
}

// runAddRowRequests отправляет POST запросы на endpoint /addRow согласно заданной частоте
func (lg *LoadGenerator) runAddRowRequests() {
	defer lg.wg.Done()
	lg.cfgLock.RLock()
	interval := time.Second / time.Duration(lg.cfg.AddRowRPS)
	baseURL := lg.cfg.BaseURL
	lg.cfgLock.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-lg.stopCh:
			return
		case <-ticker.C:
			go lg.doAddRow(baseURL)
		}
	}
}

func (lg *LoadGenerator) doAddRow(baseURL string) {
	url := baseURL + "/addRow"
	// Формируем тестовую запись. id генерируем случайно, name – на основе id, info – случайный JSON.
	payload := map[string]interface{}{
		"id":   rand.Intn(1000000),
		"name": fmt.Sprintf("User_%d", rand.Intn(1000000)),
		"info": json.RawMessage(lg.generateJSON()),
	}
	data, _ := json.Marshal(payload)
	resp, err := lg.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("[AddRow] Ошибка запроса: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		log.Printf("[AddRow] Неверный статус: %d", resp.StatusCode)
	}
}

// runUpdateRowRequests отправляет PUT запросы на endpoint /updateRow согласно заданной частоте
func (lg *LoadGenerator) runUpdateRowRequests() {
	defer lg.wg.Done()
	lg.cfgLock.RLock()
	interval := time.Second / time.Duration(lg.cfg.UpdateRowRPS)
	baseURL := lg.cfg.BaseURL
	lg.cfgLock.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-lg.stopCh:
			return
		case <-ticker.C:
			go lg.doUpdateRow(baseURL)
		}
	}
}

func (lg *LoadGenerator) doUpdateRow(baseURL string) {
	url := baseURL + "/updateRow"
	// Обновляем случайную запись: id выбираем случайно, остальные поля – новые данные
	payload := map[string]interface{}{
		"id":   rand.Intn(1000000),
		"name": fmt.Sprintf("User_%d", rand.Intn(1000000)),
		"info": json.RawMessage(lg.generateJSON()),
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		log.Printf("[UpdateRow] Ошибка создания запроса: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := lg.client.Do(req)
	if err != nil {
		log.Printf("[UpdateRow] Ошибка запроса: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[UpdateRow] Неверный статус: %d", resp.StatusCode)
	}
}

// runDeleteRowRequests отправляет DELETE запросы на endpoint /deleteRow/{id}
func (lg *LoadGenerator) runDeleteRowRequests() {
	defer lg.wg.Done()
	lg.cfgLock.RLock()
	interval := time.Second / time.Duration(lg.cfg.DeleteRowRPS)
	baseURL := lg.cfg.BaseURL
	lg.cfgLock.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-lg.stopCh:
			return
		case <-ticker.C:
			go lg.doDeleteRow(baseURL)
		}
	}
}

func (lg *LoadGenerator) doDeleteRow(baseURL string) {
	// Удаляем запись с случайным id
	id := rand.Intn(1000000)
	url := fmt.Sprintf("%s/deleteRow/%d", baseURL, id)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		log.Printf("[DeleteRow] Ошибка создания запроса: %v", err)
		return
	}
	resp, err := lg.client.Do(req)
	if err != nil {
		log.Printf("[DeleteRow] Ошибка запроса: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		log.Printf("[DeleteRow] Неверный статус: %d", resp.StatusCode)
	}
}

// runGetRowsRequests отправляет GET запросы на endpoint /getRows
func (lg *LoadGenerator) runGetRowsRequests() {
	defer lg.wg.Done()
	lg.cfgLock.RLock()
	interval := time.Second / time.Duration(lg.cfg.GetRowsRPS)
	baseURL := lg.cfg.BaseURL
	lg.cfgLock.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-lg.stopCh:
			return
		case <-ticker.C:
			go lg.doGetRows(baseURL)
		}
	}
}

func (lg *LoadGenerator) doGetRows(baseURL string) {
	url := baseURL + "/getRows"
	resp, err := lg.client.Get(url)
	if err != nil {
		log.Printf("[GetRows] Ошибка запроса: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[GetRows] Неверный статус: %d", resp.StatusCode)
	}
	// Дополнительно можно обработать данные, если это необходимо
}

// configServer предоставляет HTTP API для динамического обновления конфигурации нагрузки
func (lg *LoadGenerator) configServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/updateConfig", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
			return
		}
		var newCfg Config
		if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		lg.updateConfig(newCfg)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Конфигурация обновлена"))
	})
	srv := &http.Server{
		Addr:    ":8090", // Порт для управления конфигурацией
		Handler: mux,
	}
	log.Println("Config API сервер запущен на :8090")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Config server error: %v", err)
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())

	// Загрузка конфигурации. URL основного сервиса может передаваться через переменные окружения.
	baseURL := os.Getenv("MAIN_SERVICE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	// Начальная конфигурация нагрузочного тестирования
	cfg := Config{
		BaseURL:        baseURL,
		AddRowRPS:      10,
		UpdateRowRPS:   5,
		DeleteRowRPS:   2,
		GetRowsRPS:     3,
		NumJSONFields:  3,
		StrFieldLength: 15,
	}

	loadGen := NewLoadGenerator(cfg)
	loadGen.Start()

	// Работает в течение 10 минут, затем останавливается
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	<-ctx.Done()
	loadGen.Stop()
}
