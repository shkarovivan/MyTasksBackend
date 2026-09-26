package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

type config struct {
	port           string
	appAPIKey      string
	providerAPIKey string
	llmBaseURL     string
	defaultModel   string
	llmTimeout     time.Duration
	googleClientID string
	acceptDevAuth  bool
	dbURL          string
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadConfig() config {
	cfg := config{
		port:           getEnv("PORT", "8080"),
		appAPIKey:      os.Getenv("APP_API_KEY"),
		providerAPIKey: os.Getenv("PROVIDER_API_KEY"),
		llmBaseURL:     getEnv("LLM_BASE_URL", "https://api.proxyapi.ru/openai/v1/chat/completions"),
		defaultModel:   getEnv("DEFAULT_MODEL", "gpt-5.4-mini"),
		llmTimeout:     120 * time.Second,
	}
	if v := os.Getenv("LLM_TIMEOUT_SECS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.llmTimeout = time.Duration(n) * time.Second
		}
	}
	if cfg.appAPIKey == "" {
		log.Fatal("APP_API_KEY is required (shared secret with the app, e.g. `openssl rand -hex 32`)")
	}
	if cfg.providerAPIKey == "" {
		log.Fatal("PROVIDER_API_KEY is required (LLM provider key, lives only on the backend)")
	}
	cfg.googleClientID = os.Getenv("GOOGLE_CLIENT_ID")
	cfg.acceptDevAuth = getEnv("ACCEPT_DEV_TOKENS", "") == "1"
	if cfg.googleClientID == "" && !cfg.acceptDevAuth {
		log.Fatal("GOOGLE_CLIENT_ID is required (web OAuth client ID, aud of Google ID-tokens)")
	}
	cfg.dbURL = os.Getenv("DATABASE_URL")
	if cfg.dbURL == "" {
		log.Fatal("DATABASE_URL is required (postgres://...; provided by docker compose)")
	}
	return cfg
}

func main() {
	cfg := loadConfig()

	db, err := connectDB(context.Background(), cfg.dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	log.Printf("database connected")

	srv := newServer(cfg, db)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.Handle("/v1/task", apiKeyMiddleware(postOnly(srv.handleTask), cfg.appAPIKey))
	mux.Handle("/v1/search", apiKeyMiddleware(postOnly(srv.handleSearch), cfg.appAPIKey))

	googleAuth := newGoogleAuth(cfg.googleClientID, cfg.acceptDevAuth)
	mux.Handle("/v1/auth/whoami", bearerAuthMiddleware(http.HandlerFunc(handleWhoami), googleAuth))
	mux.Handle("/v1/sync/push", bearerAuthMiddleware(postOnly(srv.handleSyncPush), googleAuth))
	mux.Handle("/v1/sync/pull", bearerAuthMiddleware(http.HandlerFunc(srv.handleSyncPull), googleAuth))

	server := &http.Server{
		Addr:         ":" + cfg.port,
		Handler:      loggingMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: cfg.llmTimeout + 60*time.Second,
		IdleTimeout:  30 * time.Second,
	}

	log.Printf("mytasks-backend listening on :%s (llm %s, default model %s)", cfg.port, cfg.llmBaseURL, cfg.defaultModel)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, wrapped.status, time.Since(start))
	})
}
