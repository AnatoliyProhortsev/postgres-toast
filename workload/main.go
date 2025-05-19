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

func main() {
	fmt.Printf("Started\n")
	// Параметры генерации
	var (
		serviceURL = flag.String("url", "http://app:8080", "Base URL основного сервиса")
		nRecords   = flag.Int("n", 100, "Number of records to send")
		minKB      = flag.Float64("min_kb", 0.1, "Минимальный размер JSON на запись, КБ")
		maxMB      = flag.Float64("max_mb", 0.8, "Максимальный размер JSON на запись, МБ")
		randomSize = flag.Bool("rand", true, "Использовать случайный размер между min и max")
	)
	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	fmt.Printf("Initted\n")

	for i := 1; i <= *nRecords; i++ {
		// выбираем целевой размер в байтах
		target := chooseSize(*minKB, *maxMB, *randomSize)
		rec := generateRecord(target)
		payload := LoadPayload{
			Info: rec,
		}

		// сериализуем
		body, err := json.Marshal(payload)
		if err != nil {
			fmt.Printf("marshal payload error: %v\n", err)
			continue
		}

		// POST /addRow
		resp, err := http.Post(*serviceURL+"/addRow", "application/json", bytes.NewReader(body))
		if err != nil {
			fmt.Printf("HTTP POST error: %v\n", err)
			continue
		}
		b, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			fmt.Printf("addRow failed: status=%d, body=%s\n", resp.StatusCode, string(b))
		} else {
			fmt.Printf("Sent %d/%d (target ~%.2f KB)\n", i, *nRecords, float64(len(body))/1024)
		}
		// небольшой рандомный сон
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("Loading done!\n")
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
