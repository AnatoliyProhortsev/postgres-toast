package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
)

// DBStats определяет структуру статистики по базе данных.
type DBStats struct {
	Datname      sql.NullString `json:"datname" db:"datname"`
	Numbackends  int            `json:"numbackends" db:"numbackends"`
	XactCommit   int64          `json:"xact_commit" db:"xact_commit"`
	XactRollback int64          `json:"xact_rollback" db:"xact_rollback"`
	BlksRead     int64          `json:"blks_read" db:"blks_read"`
	BlksHit      int64          `json:"blks_hit" db:"blks_hit"`
}

// ToastStats – статистика для TOAST-таблиц.
type ToastStats struct {
	TableName       string `json:"table_name"`
	ToastSizeBytes  int64  `json:"toast_size_bytes"`
	ToastSizePretty string `json:"toast_size_pretty"`
}

// Snapshot объединяет полученную статистику со временем снимка.
type Snapshot struct {
	Timestamp          time.Time    `json:"timestamp"`
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

var (
	snapshotsMu sync.Mutex
	snapshots   []Snapshot
)

// addSnapshot сохраняет новый снимок в памяти (оставляем последние 100).
func addSnapshot(s Snapshot) {
	snapshotsMu.Lock()
	defer snapshotsMu.Unlock()
	snapshots = append(snapshots, s)
	if len(snapshots) > 100 {
		snapshots = snapshots[len(snapshots)-100:]
	}
}

// fetchStats выполняет запрос к основному сервису и возвращает Snapshot.
func fetchStats(mainServiceURL string) (*Snapshot, error) {
	resp, err := http.Get(mainServiceURL + "/stats")
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("неверный статус ответа: %d", resp.StatusCode)
	}

	// Ожидается, что эндпоинт /stats возвращает JSON вида:
	// { "db_stats": [...], "toast_stats": [...] }
	var allStats struct {
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
	if err := json.NewDecoder(resp.Body).Decode(&allStats); err != nil {
		return nil, fmt.Errorf("ошибка декодирования: %w", err)
	}

	snapshot := &Snapshot{
		Timestamp:          time.Now(),
		DBStats:            allStats.DBStats,
		ToastStats:         allStats.ToastStats,
		AvgSelectTimeMs:    allStats.AvgSelectTimeMs,
		AvgInsertTimeMs:    allStats.AvgInsertTimeMs,
		AvgUpdateTimeMs:    allStats.AvgUpdateTimeMs,
		AvgDeleteTimeMs:    allStats.AvgDeleteTimeMs,
		AvgInsertSizeBytes: allStats.AvgInsertSizeBytes,
		AvgUpdateSizeBytes: allStats.AvgUpdateSizeBytes,
		AvgSelectSizeBytes: allStats.AvgSelectSizeBytes,
	}
	return snapshot, nil
}

// saveSnapshotToFile сохраняет снимок в указанный каталог.
func saveSnapshotToFile(s Snapshot, exportDir string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("ошибка маршалинга JSON: %w", err)
	}
	filename := filepath.Join(exportDir, "stats_"+s.Timestamp.Format("20060102_150405")+".json")
	return ioutil.WriteFile(filename, data, 0644)
}

// startStatsCollector периодически собирает статистику и сохраняет снимки.
func startStatsCollector(mainServiceURL, exportDir string) {
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		log.Fatalf("Ошибка создания каталога экспорта: %v", err)
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		snapshot, err := fetchStats(mainServiceURL)
		if err != nil {
			log.Printf("Ошибка получения статистики: %v", err)
			continue
		}

		if err := saveSnapshotToFile(*snapshot, exportDir); err != nil {
			log.Printf("Ошибка записи файла: %v", err)
		} else {
			log.Printf("Статистика экспортирована в файл: stats_%s.json", snapshot.Timestamp.Format("20060102_150405"))
		}

		for _, s := range snapshot.DBStats {
			log.Printf("БД: %s | Соединения: %d | Коммиты: %d | Откаты: %d | Чтения блоков: %d | Кэш попаданий: %d",
				s.Datname.String, s.Numbackends, s.XactCommit, s.XactRollback, s.BlksRead, s.BlksHit)
		}

		// Логирование TOAST-метрик (если есть)
		if len(snapshot.ToastStats) > 0 {
			for _, tstat := range snapshot.ToastStats {
				log.Printf("TOAST - Таблица: %s | Размер (байт): %d | Размер (читаемо): %s",
					tstat.TableName, tstat.ToastSizeBytes, tstat.ToastSizePretty)
			}
		} else {
			log.Printf("Нет данных TOAST в данном снимке")
		}

		addSnapshot(*snapshot)
	}
}

// chartHandler строит страницу с несколькими графиками.
// Создаются графики для каждой метрики из DBStats и отдельный график для суммарного размера TOAST.
func chartHandler(w http.ResponseWriter, r *http.Request) {
	dbMetrics := []string{"numbackends", "xact_commit", "xact_rollback", "blks_read", "blks_hit"}
	metricTitles := map[string]string{
		"numbackends":   "Соединения",
		"xact_commit":   "Коммиты",
		"xact_rollback": "Откаты",
		"blks_read":     "Чтения блоков",
		"blks_hit":      "Кэш попаданий",
	}

	snapshotsMu.Lock()
	localSnapshots := make([]Snapshot, len(snapshots))
	copy(localSnapshots, snapshots)
	snapshotsMu.Unlock()

	if len(localSnapshots) == 0 {
		http.Error(w, "Нет данных для построения графиков", http.StatusServiceUnavailable)
		return
	}

	page := components.NewPage()
	page.PageTitle = "Графики статистики БД"

	// Графики DBStats
	for _, metric := range dbMetrics {
		times := make([]string, 0, len(localSnapshots))
		seriesMap := make(map[string][]opts.LineData)

		for _, snap := range localSnapshots {
			times = append(times, snap.Timestamp.Format("15:04:05"))
			for _, stat := range snap.DBStats {
				dbName := stat.Datname.String
				var value interface{}
				switch metric {
				case "numbackends":
					value = stat.Numbackends
				case "xact_commit":
					value = stat.XactCommit
				case "xact_rollback":
					value = stat.XactRollback
				case "blks_read":
					value = stat.BlksRead
				case "blks_hit":
					value = stat.BlksHit
				}
				seriesMap[dbName] = append(seriesMap[dbName], opts.LineData{Value: value})
			}
		}

		line := charts.NewLine()
		line.SetGlobalOptions(
			charts.WithTitleOpts(opts.Title{
				Title:    "Метрика " + metricTitles[metric],
				Subtitle: "Обновляется каждые 10 секунд",
			}),
			charts.WithXAxisOpts(opts.XAxis{Type: "category", Data: times}),
			charts.WithYAxisOpts(opts.YAxis{Type: "value"}),
		)
		for dbName, data := range seriesMap {
			line.AddSeries(dbName, data)
		}
		page.AddCharts(line)
	}

	// График: суммарный размер TOAST
	times := make([]string, 0, len(localSnapshots))
	var toastValues []opts.LineData
	for _, snap := range localSnapshots {
		times = append(times, snap.Timestamp.Format("15:04:05"))
		var sum int64
		for _, tstat := range snap.ToastStats {
			sum += tstat.ToastSizeBytes
		}
		toastValues = append(toastValues, opts.LineData{Value: sum})
	}
	toastLine := charts.NewLine()
	toastLine.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "Общий размер TOAST (байт)",
			Subtitle: "Агрегированно по всем TOAST-таблицам",
		}),
		charts.WithXAxisOpts(opts.XAxis{Type: "category", Data: times}),
		charts.WithYAxisOpts(opts.YAxis{Type: "value"}),
	)
	toastLine.AddSeries("TOAST", toastValues)
	page.AddCharts(toastLine)

	// График: среднее время SELECT / UPDATE / DELETE
	var selectValues, insertValues, updateValues, deleteValues []opts.LineData
	for _, snap := range localSnapshots {
		selectValues = append(selectValues, opts.LineData{Value: snap.AvgSelectTimeMs})
		insertValues = append(insertValues, opts.LineData{Value: snap.AvgInsertTimeMs})
		updateValues = append(updateValues, opts.LineData{Value: snap.AvgUpdateTimeMs})
		deleteValues = append(deleteValues, opts.LineData{Value: snap.AvgDeleteTimeMs})
	}
	avgTimeLine := charts.NewLine()
	avgTimeLine.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "Среднее время запросов (мс)",
			Subtitle: "SELECT / INSERT / UPDATE / DELETE",
		}),
		charts.WithXAxisOpts(opts.XAxis{Type: "category", Data: times}),
		charts.WithYAxisOpts(opts.YAxis{Type: "value"}),
	)
	avgTimeLine.AddSeries("SELECT", selectValues)
	avgTimeLine.AddSeries("INSERT", insertValues)
	avgTimeLine.AddSeries("UPDATE", updateValues)
	avgTimeLine.AddSeries("DELETE", deleteValues)
	page.AddCharts(avgTimeLine)

	// График: зависимость времени от размера JSONB
	var selectTimePerSize []opts.ScatterData
	for _, snap := range localSnapshots {
		if snap.AvgSelectSizeBytes > 0 {
			selectTimePerSize = append(selectTimePerSize, opts.ScatterData{
				Value: [2]interface{}{snap.AvgSelectSizeBytes, snap.AvgSelectTimeMs},
			})
		}
	}

	scatter := charts.NewScatter()
	scatter.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: "Время запроса vs. Размер JSONB",
		}),
		charts.WithXAxisOpts(opts.XAxis{Type: "value", Name: "Размер JSONB (байт)"}),
		charts.WithYAxisOpts(opts.YAxis{Type: "value", Name: "Время (нс)"}),
	)
	// scatter.AddSeries("INSERT", insertTimePerSize)
	// scatter.AddSeries("UPDATE", updateTimePerSize)
	scatter.AddSeries("SELECT", selectTimePerSize)

	page.AddCharts(scatter)

	// Рендерим страницу
	if err := page.Render(w); err != nil {
		http.Error(w, fmt.Sprintf("Ошибка рендера страницы: %v", err), http.StatusInternalServerError)
	}
}

func main() {
	// URL основного сервиса статистики (если сервис находится локально, можно использовать "http://localhost:8080")
	mainServiceURL := "http://localhost:8080"
	// Каталог для экспорта файлов статистики.
	exportDir := "export"

	// Запускаем сбор статистики в отдельной горутине.
	go startStatsCollector(mainServiceURL, exportDir)

	// HTTP-сервер для графиков.
	http.HandleFunc("/chart", chartHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html>
<head>
	<meta http-equiv="refresh" content="15">
	<title>Графики статистики БД</title>
</head>
<body>
	<h1>Графики статистики БД</h1>
	<p>Перейдите по <a href="/chart">/chart</a> для просмотра всех графиков.</p>
</body>
</html>`)
	})
	addr := ":8190"
	log.Printf("Сервер графиков запущен на %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Ошибка сервера: %v", err)
	}
}
