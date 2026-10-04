package main

import (
	"context"
	"log"
	"log/slog"
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
	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	settings, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	db, err := database.Open(settings.DatabasePath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	providerCatalog, err := llm.NewProviderCatalog(db, settings.LLMEncryptionKey, settings.LLMEncryptionKeyNew, ".env", llm.RuntimeSettings{
		BaseURL:     settings.LLMBaseURL,
		APIKey:      settings.LLMAPIKey,
		Model:       settings.LLMModel,
		Temperature: settings.LLMTemperature,
		MaxTokens:   settings.LLMMaxTokens,
		TimeoutSec:  settings.LLMTimeoutSec,
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := config.ClearLegacyLLMAPIKey(".env"); err != nil {
		log.Fatal(err)
	}
	modelRuntime := llm.NewRuntimeWithCatalog(providerCatalog, ".env")
	manager, err := task.NewManager(db, modelRuntime)
	if err != nil {
		log.Fatal(err)
	}
	manager.SetUserQuota(settings.UserTaskQuota)
	staticDir := resolveStaticDir(settings.StaticDir, "../frontend/dist", "frontend/dist")
	if staticDir == "" {
		slog.Info("no frontend build found, serving API only")
	} else {
		slog.Info("serving frontend", "path", staticDir)
	}
	application := novel.NewServer(db, auth.NewSessions(settings.SessionSecret, settings.SessionSecure), manager, settings.AllowedOrigins, staticDir)
	application.SetLLMRuntime(modelRuntime)
	application.SetContextBudget(settings.ContextBudgetRunes)
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
		slog.Info("shutdown signal received, stopping server")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			slog.Error("server shutdown failed", "error", err)
		}
	}()
	slog.Info("novel studio backend listening", "addr", settings.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	shutdownTasks, cancelTasks := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelTasks()
	if err := manager.Shutdown(shutdownTasks); err != nil {
		slog.Error("task manager shutdown failed", "error", err)
	}
	slog.Info("server stopped gracefully")
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
