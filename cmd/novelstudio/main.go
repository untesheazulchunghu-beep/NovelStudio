package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"novelstudio/internal/app"
)

func main() {
	appDir := findAppDir()
	addr := flag.String("addr", envOr("NOVELSTUDIO_ADDR", "127.0.0.1:8080"), "HTTP listen address")
	dataFile := flag.String("data", envOr("NOVELSTUDIO_DATA", filepath.Join(appDir, "data", "novelstudio.db")), "workspace SQLite database (legacy JSON is imported on first start)")
	webDir := flag.String("web", envOr("NOVELSTUDIO_WEB", filepath.Join(appDir, "frontend", "dist")), "built frontend directory")
	flag.Parse()

	store, err := app.NewStore(*dataFile)
	if err != nil {
		log.Fatalf("load workspace: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("close workspace database: %v", err)
		}
	}()

	server := app.NewServer(store, *webDir)
	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           withLogging(withCORS(server.Handler())),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	listenURL := *addr
	if strings.HasPrefix(listenURL, ":") {
		listenURL = "localhost" + listenURL
	}
	log.Printf("NovelStudio listening on http://%s", listenURL)
	log.Printf("workspace database: %s", store.DatabasePath())
	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-shutdownContext.Done()
		shutdownTimeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownTimeout); err != nil {
			log.Printf("graceful server shutdown: %v", err)
			_ = httpServer.Close()
		}
	}()
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("server: %v", err)
	}
}

func findAppDir() string {
	workingDir, err := os.Getwd()
	if err != nil {
		workingDir = "."
	}
	starts := make([]string, 0, 2)
	if executable, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(executable))
	}
	starts = append(starts, workingDir)

	seen := make(map[string]struct{})
	for _, start := range starts {
		absolute, err := filepath.Abs(start)
		if err != nil {
			continue
		}
		for directory := absolute; ; directory = filepath.Dir(directory) {
			if _, exists := seen[directory]; !exists {
				seen[directory] = struct{}{}
				if isAppDir(directory) {
					return directory
				}
			}
			parent := filepath.Dir(directory)
			if parent == directory {
				break
			}
		}
	}
	absolute, err := filepath.Abs(workingDir)
	if err != nil {
		return workingDir
	}
	return absolute
}

func isAppDir(directory string) bool {
	if info, err := os.Stat(filepath.Join(directory, "frontend", "dist", "index.html")); err == nil && !info.IsDir() {
		return true
	}
	_, goModErr := os.Stat(filepath.Join(directory, "go.mod"))
	_, frontendManifestErr := os.Stat(filepath.Join(directory, "frontend", "package.json"))
	return goModErr == nil && frontendManifestErr == nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; object-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; form-action 'self'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		origin := r.Header.Get("Origin")
		if origin != "" && !allowedBrowserOrigin(r, origin) {
			writeOriginError(w)
			return
		}
		if isDevelopmentOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Workspace-ID")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func allowedBrowserOrigin(r *http.Request, origin string) bool {
	if isDevelopmentOrigin(origin) {
		return isLoopbackAuthority(r.Host)
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" || parsed.Path != "" || parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	requestScheme := "http"
	if r.TLS != nil {
		requestScheme = "https"
	}
	if !strings.EqualFold(parsed.Scheme, requestScheme) || !strings.EqualFold(parsed.Host, r.Host) {
		return false
	}
	return isLoopbackAuthority(parsed.Host)
}

func isLoopbackAuthority(authority string) bool {
	host := authority
	if parsedHost, _, err := net.SplitHostPort(authority); err == nil {
		host = parsedHost
	}
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isDevelopmentOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || parsed.Opaque != "" || parsed.Path != "" || parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if !strings.EqualFold(parsed.Scheme, "http") || parsed.Host == "" {
		return false
	}
	return isLoopbackAuthority(parsed.Host)
}

func writeOriginError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":"request origin is not allowed"}` + "\n"))
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, wrapped.status, time.Since(started).Round(time.Millisecond))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	return w.ResponseWriter.Write(body)
}

func (w *statusWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
