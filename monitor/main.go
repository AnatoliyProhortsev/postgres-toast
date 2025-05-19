package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
)

func main() {
	mainServiceURL := os.Getenv("MAIN_SERVICE_URL") // например, http://localhost:8080
	if mainServiceURL == "" {
		mainServiceURL = "http://localhost:8080"
	}

	// Обработчик статики
	fs := http.FileServer(http.Dir(filepath.Join(".", "gui")))
	http.Handle("/", fs)

	// Прокси для /stats
	target, _ := url.Parse(mainServiceURL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/stats"
		proxy.ServeHTTP(w, r)
	})

	addr := ":8190"
	log.Printf("Monitor GUI запущен на %s и проксирует к %s", addr, mainServiceURL)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("monitor failed: %v", err)
	}
}
