package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	webhookSecret := os.Getenv("WEBHOOK_SECRET")
	host := envOr("MOCK_HOST", "0.0.0.0")
	port := envOr("MOCK_PORT", "8080")

	mux := http.NewServeMux()
	mux.HandleFunc("/authorize", authorizeHandler(webhookSecret))
	mux.HandleFunc("/events", eventsHandler())

	addr := fmt.Sprintf("%s:%s", host, port)
	log.Printf("mock-services: listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, requestLogger(mux)))
}

func authorizeHandler(secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if secret != "" {
			expected := "Bearer " + secret
			if r.Header.Get("Authorization") != expected {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusInternalServerError)
			return
		}
		if len(body) > 0 {
			log.Printf("mock-services: authorize payload=%s", body)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":200}`))
	}
}

func eventsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusInternalServerError)
			return
		}
		if len(body) > 0 {
			log.Printf("mock-services: ingest payload=%s", body)
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("mock-services: %s %s %s", r.Method, r.URL.Path, r.Proto)
		next.ServeHTTP(w, r)
	})
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
