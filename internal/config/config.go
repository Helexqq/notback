package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Redis    RedisConfig
	Postgres PostgresConfig
}

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

type PostgresConfig struct {
	Host           string
	Port           int
	User           string
	Password       string
	DBName         string
	MaxConnections int
	SSLMode        string
}

func Load() *Config {
	return &Config{
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnvAsInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvAsInt("REDIS_DB", 0),
		},
		Postgres: PostgresConfig{
			Host:           getEnv("POSTGRES_HOST", "localhost"),
			Port:           getEnvAsInt("POSTGRES_PORT", 5432),
			User:           getEnv("POSTGRES_USER", "postgres"),
			Password:       getEnv("POSTGRES_PASSWORD", "postgres"),
			DBName:         getEnv("POSTGRES_DB", "postgres"),
			MaxConnections: getEnvAsInt("POSTGRES_MAX_CONNECTIONS", 20),
			SSLMode:        getEnv("POSTGRES_SSL_MODE", "disable"),
		},
	}
}

func (c *RedisConfig) GetRedisURL() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func (c *PostgresConfig) GetPostgresURL() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}

func (c *PostgresConfig) GetPostgresDSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.DBName, c.SSLMode,
	)
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	strValue := getEnv(key, "")
	if value, err := strconv.Atoi(strValue); err == nil {
		return value
	}
	return defaultValue
}
