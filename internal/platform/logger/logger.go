package logger

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	environmentDevelopment = "development"
	environmentTest        = "test"
	environmentProduction  = "production"
)

type Config struct {
	ServiceName string
	Environment string
	Level       string
}

func New(config Config) (*zap.Logger, error) {
	serviceName := strings.TrimSpace(config.ServiceName)
	if serviceName == "" {
		return nil, fmt.Errorf(
			"logger service name must not be empty",
		)
	}

	environment := strings.ToLower(
		strings.TrimSpace(config.Environment),
	)

	level, err := parseLevel(config.Level)
	if err != nil {
		return nil, err
	}

	zapConfig, err := buildConfig(
		environment,
		level,
	)
	if err != nil {
		return nil, err
	}

	log, err := zapConfig.Build()
	if err != nil {
		return nil, fmt.Errorf(
			"build zap logger: %w",
			err,
		)
	}

	return log.With(
		zap.String("service", serviceName),
		zap.String("environment", environment),
	), nil
}

func parseLevel(value string) (zapcore.Level, error) {
	var level zapcore.Level

	if err := level.UnmarshalText(
		[]byte(strings.TrimSpace(value)),
	); err != nil {
		return zapcore.InfoLevel, fmt.Errorf(
			"parse logger level %q: %w",
			value,
			err,
		)
	}

	return level, nil
}

func buildConfig(
	environment string,
	level zapcore.Level,
) (zap.Config, error) {
	var config zap.Config

	switch environment {
	case environmentDevelopment:
		config = zap.NewDevelopmentConfig()

	case environmentTest:
		config = zap.NewProductionConfig()
		config.Sampling = nil

	case environmentProduction:
		config = zap.NewProductionConfig()

	default:
		return zap.Config{}, fmt.Errorf(
			"unsupported environment %q",
			environment,
		)
	}

	config.Level = zap.NewAtomicLevelAt(level)

	config.OutputPaths = []string{
		"stdout",
	}

	config.ErrorOutputPaths = []string{
		"stderr",
	}

	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.LevelKey = "level"
	config.EncoderConfig.NameKey = "logger"
	config.EncoderConfig.CallerKey = "caller"
	config.EncoderConfig.MessageKey = "message"
	config.EncoderConfig.StacktraceKey = "stacktrace"

	config.EncoderConfig.EncodeTime =
		zapcore.ISO8601TimeEncoder

	config.EncoderConfig.EncodeLevel =
		zapcore.LowercaseLevelEncoder

	config.EncoderConfig.EncodeDuration =
		zapcore.StringDurationEncoder

	config.EncoderConfig.EncodeCaller =
		zapcore.ShortCallerEncoder

	return config, nil
}
