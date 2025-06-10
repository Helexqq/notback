package app

import (
	"context"
	"log"
	"sync"
	_ "time"

	"contest/internal/config"
	"contest/internal/service"
	"contest/internal/storage"
	"contest/internal/transport/http"
)

type App struct {
	httpServer *http.Server
	saleSvc    *service.SaleService
	redis      *storage.RedisClient
	pgPool     *storage.PostgresPool
	wg         sync.WaitGroup
}

func New(ctx context.Context) (*App, error) {
	cfg := config.Load()

	redisClient, err := storage.NewRedisClient(ctx, cfg.Redis)

	if err != nil {
		return nil, err
	}

	pgPool, err := storage.NewPostgresPool(ctx, cfg.Postgres)
	if err != nil {
		return nil, err
	}

	if err := storage.MigratePostgres(ctx, pgPool); err != nil {
		return nil, err
	}

	saleSvc := service.NewSaleService(redisClient, pgPool)

	httpServer := http.NewServer(saleSvc)

	return &App{
		httpServer: httpServer,
		saleSvc:    saleSvc,
		redis:      redisClient,
		pgPool:     pgPool,
	}, nil
}

func (a *App) Run() error {

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.saleSvc.StartBackgroundTasks(context.Background())
	}()

	log.Println("Starting HTTP server on portt 8080")
	return a.httpServer.Start()
}

func (a *App) Stop(ctx context.Context) error {
	if err := a.httpServer.Stop(ctx); err != nil {
		return err
	}
	a.redis.Close()
	a.pgPool.Close()

	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
