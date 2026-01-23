package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/singh-gur/api_cache/internal/cache"
	"github.com/singh-gur/api_cache/internal/config"
	"github.com/singh-gur/api_cache/internal/logger"
	"github.com/singh-gur/api_cache/internal/middleware"
	"github.com/singh-gur/api_cache/internal/proxy"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := logger.Init(cfg.Logging); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	logger.Log.Info("Starting API Cache Proxy")

	// Initialize cache client
	cacheClient, err := cache.NewClient(cfg)
	if err != nil {
		logger.Log.Fatalf("Failed to initialize cache client: %v", err)
	}
	defer cacheClient.Close()

	// Create proxy handler
	proxyHandler := proxy.NewHandler(cacheClient, cfg)

	// Create rate limiter
	rateLimiter := middleware.NewRateLimiter(&cfg.RateLimit)

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", proxyHandler.Health())
	mux.Handle("/", rateLimiter.Middleware(cfg)(proxyHandler))

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in a goroutine
	go func() {
		logger.Log.Infof("Server listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Log.Info("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Log.Errorf("Server forced to shutdown: %v", err)
	}

	logger.Log.Info("Server stopped")
}
