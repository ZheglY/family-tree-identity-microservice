package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultServiceName             = "identity-service"
	defaultEnvironment             = "development"
	defaultGRPCAddress             = ":50051"
	defaultShutdownTimeout         = 10 * time.Second
	defaultLogLevel                = "INFO"
	defaultGRPCReflection          = false
	defaultPostgresURL             = "postgres://identity:identity@localhost:5433/identity?sslmode=disable"
	defaultPostgresMaxConnections  = 10
	defaultPostgresMinConnections  = 1
	defaultPostgresMaxConnLifetime = 30 * time.Minute
	defaultPostgresMaxConnIdleTime = 5 * time.Minute
	defaultPostgresHealthCheck     = 15 * time.Second
	defaultPostgresConnectTimeout  = 5 * time.Second
	defaultReadinessCheckInterval  = 5 * time.Second
	defaultReadinessCheckTimeout   = 2 * time.Second
)

type Config struct {
	App      AppConfig
	GRPC     GRPCConfig
	Postgres PostgresConfig
	Logger   LoggerConfig
}

func NewConfig(
	environment string,
	grpcAddress string,
	shutdownTimeout time.Duration,
	reflection bool,
	postgresConfig PostgresConfig,
	level string,
) *Config {
	return &Config{
		App: AppConfig{
			Name:        defaultServiceName,
			Environment: environment,
		},
		GRPC: GRPCConfig{
			Address:                grpcAddress,
			ShutdownTimeout:        shutdownTimeout,
			Reflection:             reflection,
			ReadinessCheckInterval: defaultReadinessCheckInterval,
			ReadinessCheckTimeout:  defaultReadinessCheckTimeout,
		},
		Postgres: postgresConfig,
		Logger: LoggerConfig{
			Level: level,
		},
	}
}

type AppConfig struct {
	Name        string
	Environment string
}

type GRPCConfig struct {
	Address                string
	ShutdownTimeout        time.Duration
	Reflection             bool
	ReadinessCheckInterval time.Duration
	ReadinessCheckTimeout  time.Duration
}

type PostgresConfig struct {
	URL               string
	MaxConnections    int32
	MinConnections    int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	ConnectTimeout    time.Duration
}

type LoggerConfig struct {
	Level string
}

func Load() (Config, error) {
	environment := stringFromEnv(
		"IDENTITY_ENVIRONMENT",
		defaultEnvironment,
	)

	shutdownTimeout, err := durationFromEnv(
		"IDENTITY_SHUTDOWN_TIMEOUT",
		defaultShutdownTimeout,
	)

	if err != nil {
		return Config{}, err
	}

	readinessCheckInterval, err := durationFromEnv(
		"IDENTITY_GRPC_READINESS_CHECK_INTERVAL",
		defaultReadinessCheckInterval,
	)
	if err != nil {
		return Config{}, err
	}

	readinessCheckTimeout, err := durationFromEnv(
		"IDENTITY_GRPC_READINESS_CHECK_TIMEOUT",
		defaultReadinessCheckTimeout,
	)
	if err != nil {
		return Config{}, err
	}

	grpcReflection, err := boolFromEnv(
		"IDENTITY_GRPC_REFLECTION",
		defaultGRPCReflection,
	)

	if err != nil {
		return Config{}, err
	}

	grpcAddress := stringFromEnv(
		"IDENTITY_GRPC_ADDR",
		defaultGRPCAddress,
	)

	loggingLevel := stringFromEnv(
		"IDENTITY_LOG_LEVEL",
		defaultLogLevel,
	)

	postgresConfig, err := loadPostgresConfig(environment)
	if err != nil {
		return Config{}, err
	}

	config := NewConfig(
		environment,
		grpcAddress,
		shutdownTimeout,
		grpcReflection,
		postgresConfig,
		loggingLevel,
	)
	config.GRPC.ReadinessCheckInterval = readinessCheckInterval
	config.GRPC.ReadinessCheckTimeout = readinessCheckTimeout

	return *config, nil
}

func loadPostgresConfig(environment string) (PostgresConfig, error) {
	postgresURL := strings.TrimSpace(os.Getenv("IDENTITY_POSTGRES_URL"))
	if postgresURL == "" {
		if environment == "production" {
			return PostgresConfig{}, fmt.Errorf(
				"IDENTITY_POSTGRES_URL is required in production",
			)
		}
		postgresURL = defaultPostgresURL
	}

	maxConnections, err := int32FromEnv(
		"IDENTITY_POSTGRES_MAX_CONNECTIONS",
		defaultPostgresMaxConnections,
	)
	if err != nil {
		return PostgresConfig{}, err
	}

	minConnections, err := int32FromEnv(
		"IDENTITY_POSTGRES_MIN_CONNECTIONS",
		defaultPostgresMinConnections,
	)
	if err != nil {
		return PostgresConfig{}, err
	}

	if minConnections > maxConnections {
		return PostgresConfig{}, fmt.Errorf(
			"IDENTITY_POSTGRES_MIN_CONNECTIONS must not exceed IDENTITY_POSTGRES_MAX_CONNECTIONS",
		)
	}

	maxConnLifetime, err := durationFromEnv(
		"IDENTITY_POSTGRES_MAX_CONN_LIFETIME",
		defaultPostgresMaxConnLifetime,
	)
	if err != nil {
		return PostgresConfig{}, err
	}

	maxConnIdleTime, err := durationFromEnv(
		"IDENTITY_POSTGRES_MAX_CONN_IDLE_TIME",
		defaultPostgresMaxConnIdleTime,
	)
	if err != nil {
		return PostgresConfig{}, err
	}

	healthCheckPeriod, err := durationFromEnv(
		"IDENTITY_POSTGRES_HEALTH_CHECK_PERIOD",
		defaultPostgresHealthCheck,
	)
	if err != nil {
		return PostgresConfig{}, err
	}

	connectTimeout, err := durationFromEnv(
		"IDENTITY_POSTGRES_CONNECT_TIMEOUT",
		defaultPostgresConnectTimeout,
	)
	if err != nil {
		return PostgresConfig{}, err
	}

	return PostgresConfig{
		URL:               postgresURL,
		MaxConnections:    maxConnections,
		MinConnections:    minConnections,
		MaxConnLifetime:   maxConnLifetime,
		MaxConnIdleTime:   maxConnIdleTime,
		HealthCheckPeriod: healthCheckPeriod,
		ConnectTimeout:    connectTimeout,
	}, nil
}

// Функция получает значение из env файла и при отсутсвии
// возвращает дефолтное значение const
func durationFromEnv(
	key string, // Ключ "IDENTITY_SHUTDOWN_TIMEOUT" в .env
	defaultValue time.Duration, // дефолтная константа выше
) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}

	// Время 10s парсится в тип time.Duration
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf(
			"parse %s: %w",
			key,
			err,
		)
	}

	if duration <= 0 {
		return 0, fmt.Errorf(
			"%s must be positive",
			key,
		)
	}

	return duration, nil
}

func boolFromEnv(
	key string,
	defaultValue bool,
) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}

	result, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf(
			"parse %s: %w",
			key,
			err,
		)
	}

	return result, nil
}

func int32FromEnv(
	key string,
	defaultValue int32,
) (int32, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}

	result, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}

	if result <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}

	return int32(result), nil
}

func stringFromEnv(
	key string,
	defaultValue string,
) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}

	return value
}
