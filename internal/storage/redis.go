package storage

import (
	"contest/internal/config"
	"context"

	"contest/pkg/util"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	*redis.Client
}

func NewRedisClient(ctx context.Context, cfg config.RedisConfig) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.GetRedisURL(),
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, util.Wrap(err)
	}

	return &RedisClient{client}, nil
}

func (r *RedisClient) Close() {
	r.Client.Close()
}
