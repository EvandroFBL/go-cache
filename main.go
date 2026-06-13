package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Configuration (hardcoded for simplicity — no external deps)
	port := getEnv("PORT", "8080")
	cleanupInterval := getEnvDuration("CLEANUP_INTERVAL", 10*time.Second)
	maxKeys := getEnvInt("MAX_KEYS", 0) // 0 = unlimited

	log.Printf("Starting go-cache on :%s", port)
	log.Printf("  Cleanup interval: %v", cleanupInterval)
	log.Printf("  Max keys: %d (0=unlimited)", maxKeys)

	// Create the store
	store := NewStore(maxKeys)

	// Start background cleanup goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go StartCleanup(ctx, store, cleanupInterval)

	// Set up HTTP server
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("Received signal %v — shutting down...", sig)

		cancel() // Stop cleanup goroutine

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx)
	}()

	// Print available routes
	fmt.Println("\n📋 Available endpoints:")
	fmt.Println("  GET    /health              — Health check")
	fmt.Println("  GET    /cache               — List all keys")
	fmt.Println("  GET    /cache/{key}         — Get a value")
	fmt.Println("  PUT    /cache/{key}         — Set a value (JSON body: {\"value\": ..., \"ttl\": \"5m\"})")
	fmt.Println("  DELETE /cache/{key}         — Delete a key")
	fmt.Println("  GET    /stats               — Cache statistics")
	fmt.Println()

	// Start serving
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	log.Println("Server stopped")
}

// --- Helper functions to avoid external config dependencies ---

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("invalid duration for %s: %v — using default %v", key, err, fallback)
		return fallback
	}
	return d
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		log.Printf("invalid int for %s: %v — using default %d", key, err, fallback)
		return fallback
	}
	return n
}
