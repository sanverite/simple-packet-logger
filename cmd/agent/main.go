package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sanverite/simple-packet-logger/internal/api"
	"github.com/sanverite/simple-packet-logger/internal/core"
)

func main() {
	var (
		addr         = flag.String("listen", api.DefaultAddress, "HTTP listen address")
		shutdownSecs = flag.Int("shutdown-secs", 5, "graceful shutdown timeout in seconds")
	)
	flag.Parse()

	logger := log.Default()

	// Core state initialization
	state := core.NewState()

	// API Server
	srv := api.NewServer(state, api.ServerOptions{
		Addr:              *addr,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		ShutdownTimeout:   time.Duration(*shutdownSecs) * time.Second,
		Logger:            logger,
	})

	// Start API
	srv.Start()

	// Handle shutdown signals
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	sig := <-signals
	logger.Printf("agent: received signal %v, shutting down", sig)

	ctx := context.Background()
	if err := srv.Stop(ctx); err != nil {
		logger.Printf("agent: graceful shutdown error: %v", err)
	}
	logger.Printf("agent: stopped")
}
