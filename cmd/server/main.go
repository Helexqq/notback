package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"contest/internal/app"
)

func main() {
	ctx := context.Background()

	application, err := app.New(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize app: %v", err)
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := application.Run(); err != nil {
			log.Fatalf("Application failed: %v", err)
		}
	}()

	<-shutdown

	log.Println("Starting graceful shutdown")
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := application.Stop(ctx); err != nil {
		log.Fatalf("Shutdown failed: %v", err)
	}

	log.Println("Shutdown completed")
}
