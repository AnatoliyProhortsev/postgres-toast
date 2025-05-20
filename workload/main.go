// load_service.go
// CLI для нагрузки через HTTP-рэндпоинты основного сервиса

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// BaseRecord соответствует полю Info в запросе addRow
// Дополнительно основной сервис ожидает поле Name, заполним его фиксированно или по шаблону.
type LoadPayload struct {
	Info BaseRecord `json:"info"`
}

type BaseRecord struct {
	IMDBID string        `json:"imdb_id"`
	Height string        `json:"height"`
	Roles  []RoleDetails `json:"roles"`
}

type RoleDetails struct {
	Role  string `json:"role"`
	Title string `json:"title"`
}

type AppParams struct {
	ApiUrl   string
	NRecords int
	MinKB    float64
	MaxMB    float64
	IsRandom bool
	Counter  int
}

// Отправляет одну запись по HTTP
func postRandomRecord(apiURL string, minKB, maxMB float64, isRandom bool, idx, total int) error {
	// 1) Выбираем размер
	target := chooseSize(minKB, maxMB, isRandom)
	rec := generateRecord(target)
	payload := LoadPayload{Info: rec}

	// 2) Сериализуем
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload error: %w", err)
	}

	// 3) Делаем POST
	resp, err := http.Post(apiURL+"/addRow", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("HTTP POST error: %w", err)
	}
	defer resp.Body.Close()

	b, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("addRow failed: status=%d, body=%s", resp.StatusCode, string(b))
	}

	// 4) Логируем успех
	fmt.Printf("[%d/%d] Sent ~%.2f KB\n", idx, total, float64(len(body))/1024.0)
	return nil
}

func main() {
	// Парсим флаги
	url := flag.String("url", "http://app:8080", "Base URL основного сервиса")
	n := flag.Int("n", 100000, "Number of records to send")
	minKB := flag.Float64("min_kb", 0.001, "Минимальный размер JSON на запись, КБ")
	maxMB := flag.Float64("max_mb", 1, "Максимальный размер JSON на запись, МБ")
	isRand := flag.Bool("rand", true, "Использовать случайный размер между min и max")
	flag.Parse()

	params := AppParams{
		ApiUrl:   *url,
		NRecords: *n,
		MinKB:    *minKB,
		MaxMB:    *maxMB,
		IsRandom: *isRand,
	}
	rand.Seed(time.Now().UnixNano())

	fmt.Println("Started sending", params.NRecords, "records to", params.ApiUrl)

	var wg sync.WaitGroup
	// Семофор: не более 5 горутин одновременно
	sem := make(chan struct{}, 5)

	for i := 1; i <= params.NRecords; i++ {
		wg.Add(1)
		// Блокируемся, если уже 5 горутин "в работе"
		sem <- struct{}{}

		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }() // освобождаем слот по окончании

			if err := postRandomRecord(
				params.ApiUrl,
				params.MinKB,
				params.MaxMB,
				params.IsRandom,
				idx,
				params.NRecords,
			); err != nil {
				fmt.Printf("[%d/%d] error: %v\n", idx, params.NRecords, err)
			}
		}(i)
	}

	wg.Wait()
	fmt.Println("Loading done!")
}

// chooseSize возвращает целевой размер JSON в байтах
func chooseSize(minKB, maxMB float64, random bool) int {
	minB := int(minKB * 1024)
	maxB := int(maxMB * 1024 * 1024)
	if random {
		return rand.Intn(maxB-minB+1) + minB
	}
	// линейный от min к max по времени
	return minB
}

// generateRecord создаёт BaseRecord с общим размером JSON не менее targetBytes
func generateRecord(targetBytes int) BaseRecord {
	// базовый объект
	rec := BaseRecord{
		IMDBID: fmt.Sprintf("tt%07d", rand.Intn(10000000)),
		Height: fmt.Sprintf("%d cm", rand.Intn(50)+150),
		Roles:  []RoleDetails{},
	}

	// шаблон роли
	tmpl := RoleDetails{Role: "actor", Title: "Sample Movie"}

	// добавляем роли до достижения размера
	data, _ := json.Marshal(rec)
	current := len(data)
	for current < targetBytes {
		rec.Roles = append(rec.Roles, tmpl)
		// слегка поменяем title
		tmpl.Title = fmt.Sprintf("Sample Movie %d", rand.Intn(1000000))
		data, _ = json.Marshal(rec)
		current = len(data)
		if len(rec.Roles) > 10_000_000 {
			break
		}
	}

	return rec
}
