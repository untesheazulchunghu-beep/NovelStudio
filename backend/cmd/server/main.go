package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"novelstudio/internal/auth"
	"novelstudio/internal/config"
	"novelstudio/internal/database"
	"novelstudio/internal/llm"
	"novelstudio/internal/novel"
	"novelstudio/internal/task"
)

func main() {
	settings, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	db, err := database.Open(settings.DatabasePath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	modelRuntime := llm.NewRuntime(llm.RuntimeSettings{
		BaseURL:     settings.LLMBaseURL,
		APIKey:      settings.LLMAPIKey,
		Model:       settings.LLMModel,
		Temperature: settings.LLMTemperature,
		MaxTokens:   settings.LLMMaxTokens,
		TimeoutSec:  settings.LLMTimeoutSec,
	}, ".env")
	manager, err := task.NewManager(db, modelRuntime)
	if err != nil {
		log.Fatal(err)
	}
	staticDir := resolveStaticDir(settings.StaticDir, "../frontend/dist", "frontend/dist")
	if staticDir == "" {
		log.Printf("no frontend build found, serving API only")
	} else {
		log.Printf("serving frontend from %s", staticDir)
	}
	application := novel.NewServer(db, auth.NewSessions(settings.SessionSecret, settings.SessionSecure), manager, settings.AllowedOrigins, staticDir)
	application.SetLLMRuntime(modelRuntime)
	server := &http.Server{
		Addr:              settings.Addr,
		Handler:           application.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	shutdown, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdown.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("server shutdown failed: %v", err)
		}
	}()
	log.Printf("novel studio backend listening on %s", settings.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func resolveStaticDir(candidates ...string) string {
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(filepath.Join(candidate, "index.html")); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}
