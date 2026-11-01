package config_test

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/Mentigen/go-labs/internal/config"
)

const (
	envLogLevel        = "LOG_LEVEL"
	envShutdownTimeout = "SHUTDOWN_TIMEOUT"
	envDatabaseURL     = "DATABASE_URL"
	envMinConns        = "DATABASE_MIN_CONNS"
	envEndpoint        = "OTEL_EXPORTER_OTLP_ENDPOINT"
	envProtocol        = "OTEL_EXPORTER_OTLP_PROTOCOL"
)

func setValidEnvironment(t *testing.T) {
	t.Helper()

	for name, value := range map[string]string{
		"HTTP_ADDR":                  ":8080",
		envLogLevel:                  "debug",
		envShutdownTimeout:           "10s",
		envDatabaseURL:               "postgres://localhost:5432/db",
		"DATABASE_MAX_CONNS":         "10",
		envMinConns:                  "2",
		"DATABASE_MAX_CONN_LIFETIME": "30m",
		"DATABASE_CONNECT_TIMEOUT":   "5s",
		"DATABASE_QUERY_TIMEOUT":     "3s",
		envEndpoint:                  "http://localhost:4318",
		"OTEL_SERVICE_NAME":          "",
		envProtocol:                  "",
	} {
		t.Setenv(name, value)
	}
}

func TestLoadReadsTelemetrySettings(t *testing.T) {
	setValidEnvironment(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("log level = %v, want debug", cfg.LogLevel)
	}

	want := config.TelemetryConfig{
		ServiceName: config.DefaultServiceName,
		Endpoint:    "http://localhost:4318",
		Protocol:    config.ProtocolHTTP,
	}

	if cfg.Telemetry != want {
		t.Errorf("telemetry = %+v, want %+v", cfg.Telemetry, want)
	}
}

func TestLoadFailsFast(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"missing collector endpoint", map[string]string{envEndpoint: ""}, envEndpoint},
		{"unknown protocol", map[string]string{envProtocol: "carrier-pigeon"}, envProtocol},
		{"unknown log level", map[string]string{envLogLevel: "loud"}, envLogLevel},
		{"missing log level", map[string]string{envLogLevel: ""}, envLogLevel},
		{"missing database url", map[string]string{envDatabaseURL: ""}, envDatabaseURL},
		{"min conns above max", map[string]string{envMinConns: "20"}, envMinConns},
		{"zero shutdown timeout", map[string]string{envShutdownTimeout: "0s"}, envShutdownTimeout},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setValidEnvironment(t)

			for name, value := range test.env {
				t.Setenv(name, value)
			}

			_, err := config.Load()
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want mention of %s", err, test.wantErr)
			}
		})
	}
}
