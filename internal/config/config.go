package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultServiceName     = "identity-service"
	defaultEnvironment     = "development"
	defaultGRPCAddress     = ":50051"
	defaultShutdownTimeout = 10 * time.Second
	defaultLogLevel        = "INFO"
	defaultGRPCReflection  = false
)

type Config struct {
	App    AppConfig
	GRPC   GRPCConfig
	Logger LoggerConfig
}

func NewConfig(
	environment string,
	grpcAddress string,
	shutdownTimeout time.Duration,
	reflection bool,
	level string,
) *Config {
	return &Config{
		App: AppConfig{
			Name:        defaultServiceName,
			Environment: environment,
		},
		GRPC: GRPCConfig{
			Address:         grpcAddress,
			ShutdownTimeout: shutdownTimeout,
			Reflection:      reflection,
		},
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
	Address         string
	ShutdownTimeout time.Duration
	Reflection      bool
}

type LoggerConfig struct {
	Level string
}

func Load() (Config, error) {
	shutdownTimeout, err := durationFromEnv(
		"IDENTITY_SHUTDOWN_TIMEOUT",
		defaultShutdownTimeout,
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

	environment := stringFromEnv(
		"IDENTITY_ENVIRONMENT",
		defaultEnvironment,
	)

	config := NewConfig(
		environment,
		grpcAddress,
		shutdownTimeout,
		grpcReflection,
		loggingLevel,
	)

	return *config, nil
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
