package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

const (
	ProtocolHTTP       = "http/protobuf"
	ProtocolGRPC       = "grpc"
	DefaultServiceName = "trip-service"
)

type HTTPTimeouts struct {
	Read       time.Duration
	ReadHeader time.Duration
	Write      time.Duration
	Idle       time.Duration
}

type Config struct {
	HTTPAddr        string
	HTTPTimeouts    HTTPTimeouts
	LogLevel        slog.Level
	ShutdownTimeout time.Duration
	Database        DatabaseConfig
	Telemetry       TelemetryConfig
}

type TelemetryConfig struct {
	ServiceName string
	Endpoint    string
	Protocol    string
}

type DatabaseConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	ConnectTimeout  time.Duration
	QueryTimeout    time.Duration
}

func Load() (Config, error) {
	database, err := loadDatabase()
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := requiredDuration("SHUTDOWN_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	httpAddr, err := requiredString("HTTP_ADDR")
	if err != nil {
		return Config{}, err
	}

	httpTimeouts, err := loadHTTPTimeouts()
	if err != nil {
		return Config{}, err
	}

	logLevel, err := loadLogLevel()
	if err != nil {
		return Config{}, err
	}

	telemetry, err := loadTelemetry()
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTPAddr:        httpAddr,
		HTTPTimeouts:    httpTimeouts,
		LogLevel:        logLevel,
		ShutdownTimeout: shutdownTimeout,
		Database:        database,
		Telemetry:       telemetry,
	}, nil
}

func loadLogLevel() (slog.Level, error) {
	value, err := requiredString("LOG_LEVEL")
	if err != nil {
		return 0, err
	}

	var level slog.Level

	if err := level.UnmarshalText([]byte(value)); err != nil {
		return 0, fmt.Errorf("environment variable LOG_LEVEL: invalid level %q: %w", value, err)
	}

	return level, nil
}

func loadTelemetry() (TelemetryConfig, error) {
	endpoint, err := requiredString("OTEL_EXPORTER_OTLP_ENDPOINT")
	if err != nil {
		return TelemetryConfig{}, err
	}

	protocol := optionalString("OTEL_EXPORTER_OTLP_PROTOCOL", ProtocolHTTP)
	if protocol != ProtocolHTTP && protocol != ProtocolGRPC {
		return TelemetryConfig{}, fmt.Errorf(
			"environment variable OTEL_EXPORTER_OTLP_PROTOCOL must be %q or %q, got %q",
			ProtocolHTTP, ProtocolGRPC, protocol,
		)
	}

	return TelemetryConfig{
		ServiceName: optionalString("OTEL_SERVICE_NAME", DefaultServiceName),
		Endpoint:    endpoint,
		Protocol:    protocol,
	}, nil
}

func loadHTTPTimeouts() (HTTPTimeouts, error) {
	read, err := optionalDuration("HTTP_READ_TIMEOUT", 10*time.Second)
	if err != nil {
		return HTTPTimeouts{}, err
	}

	readHeader, err := optionalDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second)
	if err != nil {
		return HTTPTimeouts{}, err
	}

	write, err := optionalDuration("HTTP_WRITE_TIMEOUT", 10*time.Second)
	if err != nil {
		return HTTPTimeouts{}, err
	}

	idle, err := optionalDuration("HTTP_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		return HTTPTimeouts{}, err
	}

	return HTTPTimeouts{Read: read, ReadHeader: readHeader, Write: write, Idle: idle}, nil
}

func loadDatabase() (DatabaseConfig, error) {
	url, err := requiredString("DATABASE_URL")
	if err != nil {
		return DatabaseConfig{}, err
	}

	maxConns, err := requiredInt32("DATABASE_MAX_CONNS")
	if err != nil {
		return DatabaseConfig{}, err
	}

	if maxConns <= 0 {
		return DatabaseConfig{}, fmt.Errorf("environment variable DATABASE_MAX_CONNS must be greater than zero")
	}

	minConns, err := requiredInt32("DATABASE_MIN_CONNS")
	if err != nil {
		return DatabaseConfig{}, err
	}

	if minConns < 0 {
		return DatabaseConfig{}, fmt.Errorf("environment variable DATABASE_MIN_CONNS must not be negative")
	}

	if minConns > maxConns {
		return DatabaseConfig{}, fmt.Errorf(
			"DATABASE_MIN_CONNS (%d) must not be greater than DATABASE_MAX_CONNS (%d)",
			minConns, maxConns,
		)
	}

	maxConnLifetime, err := requiredDuration("DATABASE_MAX_CONN_LIFETIME")
	if err != nil {
		return DatabaseConfig{}, err
	}

	connectTimeout, err := requiredDuration("DATABASE_CONNECT_TIMEOUT")
	if err != nil {
		return DatabaseConfig{}, err
	}

	queryTimeout, err := requiredDuration("DATABASE_QUERY_TIMEOUT")
	if err != nil {
		return DatabaseConfig{}, err
	}

	return DatabaseConfig{
		URL:             url,
		MaxConns:        maxConns,
		MinConns:        minConns,
		MaxConnLifetime: maxConnLifetime,
		ConnectTimeout:  connectTimeout,
		QueryTimeout:    queryTimeout,
	}, nil
}

func optionalString(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}

func requiredString(name string) (string, error) {
	value, ok := os.LookupEnv(name)
	if !ok || value == "" {
		return "", fmt.Errorf("environment variable %s is required", name)
	}

	return value, nil
}

func requiredDuration(name string) (time.Duration, error) {
	value, err := requiredString(name)
	if err != nil {
		return 0, err
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("environment variable %s: invalid duration %q: %w", name, value, err)
	}

	if duration <= 0 {
		return 0, fmt.Errorf("environment variable %s must be greater than zero", name)
	}

	return duration, nil
}

func optionalDuration(name string, fallback time.Duration) (time.Duration, error) {
	if os.Getenv(name) == "" {
		return fallback, nil
	}

	return requiredDuration(name)
}

func requiredInt32(name string) (int32, error) {
	value, err := requiredString(name)
	if err != nil {
		return 0, err
	}

	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("environment variable %s: invalid integer %q: %w", name, value, err)
	}

	return int32(parsed), nil
}
