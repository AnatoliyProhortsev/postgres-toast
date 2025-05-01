package main

import (
	"database/sql"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// DBStats определяет структуру статистики, которую возвращает основной сервис.
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

// Snapshot объединяет полученную статистику со временем снимка
type Snapshot struct {
	Timestamp time.Time `json:"timestamp"`
	Stats     []DBStats `json:"stats"`
}

func main() {
	// URL основного сервиса; используем docker-compose DNS-имя "app"
	mainServiceURL := "http://app:8080"

	// Папка для экспорта JSON со статистикой
	exportDir := "export"
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		log.Fatalf("Ошибка создания каталога экспорта: %v", err)
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C

		resp, err := http.Get(mainServiceURL + "/stats")
		if err != nil {
			log.Printf("Ошибка запроса к основному сервису: %v", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			log.Printf("Неверный статус ответа: %d", resp.StatusCode)
			resp.Body.Close()
			continue
		}

		var allStats struct {
			DBStats    []DBStats    `json:"db_stats"`
			ToastStats []ToastStats `json:"toast_stats"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&allStats); err != nil {
			log.Printf("Ошибка декодирования ответа: %v", err)
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		// Создаём снимок статистики с меткой времени
		snapshot := Snapshot{
			Timestamp: time.Now(),
			Stats:     allStats.DBStats,
		}

		// Преобразуем в JSON
		data, err := json.MarshalIndent(snapshot, "", "  ")
		if err != nil {
			log.Printf("Ошибка кодирования JSON: %v", err)
			continue
		}

		// Формируем имя файла, например: stats_20230501_123456.json
		filename := filepath.Join(exportDir, "stats_"+snapshot.Timestamp.Format("20060102_150405")+".json")
		if err := ioutil.WriteFile(filename, data, 0644); err != nil {
			log.Printf("Ошибка записи файла %s: %v", filename, err)
			continue
		}

		log.Printf("Статистика успешно экспортирована в файл: %s", filename)

		// Также выводим полученную статистику в лог
		log.Println("Статистика, полученная от основного сервиса:")
		for _, s := range allStats.DBStats {
			log.Printf("БД: %s | Соединения: %d | Коммиты: %d | Откаты: %d | Чтения блоков: %d | Кэш попаданий: %d",
				s.Datname.String, s.Numbackends, s.XactCommit, s.XactRollback, s.BlksRead, s.BlksHit)
		}
	}
}
