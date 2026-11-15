package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"shopassistant/internal/extraction"
	"shopassistant/internal/identity"
	"shopassistant/internal/instagram"
	"shopassistant/internal/platform"
	"shopassistant/internal/processing"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		client := http.Client{Timeout: 4 * time.Second}
		r, err := client.Get("http://127.0.0.1:8080/readyz")
		if err != nil {
			os.Exit(1)
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	config, err := platform.LoadConfig()
	if err != nil {
		slog.Error("configuration rejected")
		os.Exit(1)
	}
	role := os.Getenv("SERVICE_ROLE")
	if role != "api" && role != "worker" {
		slog.Error("invalid service role")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	pool, err := pgxpool.NewWithConfig(ctx, config.DB)
	if err != nil {
		slog.Error("database configuration rejected")
		os.Exit(1)
	}
	defer pool.Close()
	queue := processing.PostgresQueue{Pool: pool}
	if role == "worker" && config.Mode == "demo" {
		go func() {
			worker := extraction.Worker{Queue: extraction.PostgresQueue{Pool: pool}, Extractor: extraction.ScriptedExtractor{}}
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
					if err := worker.ProcessOne(attempt); err != nil {
						slog.Warn("caption extraction unavailable")
					}
					cancel()
				}
			}
		}()
	}
	if role == "worker" {
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					attempt, cancel := context.WithTimeout(ctx, 3*time.Second)
					if err := processing.ProcessOne(attempt, queue); err != nil {
						slog.Warn("processing attempt unavailable")
					}
					cancel()
				}
			}
		}()
	}
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			attempt, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := platform.Migrate(attempt, pool)
			if err == nil && role == "worker" {
				err = platform.Beat(attempt, pool)
			}
			cancel()
			if err != nil {
				slog.Warn("storage preparation failed", "role", role)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	mux := http.NewServeMux()
	if role == "api" {
		mux.Handle("POST /auth/register", identity.RegisterHandler(pool))
		origin := os.Getenv("APP_ORIGIN")
		if origin == "" {
			origin = "http://localhost:3000"
		}
		publicURL, err := url.Parse(origin)
		if err != nil || publicURL.Host == "" || publicURL.User != nil || publicURL.RawQuery != "" || publicURL.Fragment != "" || publicURL.Path != "" || (publicURL.Scheme != "http" && publicURL.Scheme != "https") || (config.Mode == "live" && publicURL.Scheme != "https") {
			slog.Error("APP_ORIGIN must be an exact origin; live requires HTTPS")
			os.Exit(1)
		}
		auth := identity.NewAuth(pool, origin, config.Mode == "live")
		auth.Processing = &processing.Service{Pool: pool, Queue: queue}
		if config.Mode == "demo" {
			auth.Instagram = instagram.Fake{}
		}
		mux.Handle("POST /auth/login", auth)
		mux.Handle("POST /auth/logout", auth)
		mux.Handle("GET /me", auth)
		mux.Handle("POST /stores", auth)
		mux.Handle("GET /stores/{store}", auth)
		mux.Handle("POST /stores/{store}/connection/{action}", auth)
		mux.Handle("GET /stores/{store}/connection/status", auth)
		mux.Handle("GET /stores/{store}/processing", auth)
		mux.Handle("POST /stores/{store}/sample-events", auth)
		mux.Handle("GET /stores/{store}/post-selections/{selection}/mapping", auth)
		mux.Handle("POST /stores/{store}/post-selections/{selection}/mapping", auth)
		mux.Handle("POST /stores/{store}/post-selections/{selection}/retry", auth)
		mux.Handle("GET /stores/{store}/posts", auth)
		mux.Handle("GET /stores/{store}/post-selections", auth)
		mux.Handle("POST /stores/{store}/post-selections", auth)
		mux.Handle("GET /stores/{store}/products", auth)
		mux.Handle("POST /stores/{store}/products", auth)
		mux.Handle("GET /stores/{store}/products/{product}", auth)
		mux.Handle("POST /stores/{store}/products/{product}", auth)
		mux.Handle("POST /stores/{store}/products/{product}/{action}", auth)
	}
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		check, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		h := platform.Check(check, pool, config.Mode)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Request-ID", h.RequestID)
		if h.Status != "ready" {
			w.WriteHeader(503)
		}
		json.NewEncoder(w).Encode(h)
		slog.Info("readiness", "request_id", h.RequestID, "result", h.Status, "duration_ms", time.Since(started).Milliseconds())
	})
	server := http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server failed")
		os.Exit(1)
	}
}
