package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Mentigen/go-labs/internal/config"
)

const (
	envCheckInterval   = "POSITION_CHECK_INTERVAL"
	envStaleAfter      = "POSITION_STALE_AFTER"
	envPositionWorkers = "POSITION_WORKERS"
	envQueueSize       = "POSITION_QUEUE_SIZE"
	envBatchSize       = "POSITION_SCAN_BATCH_SIZE"
	envPushTimeout     = "PUSH_REQUEST_TIMEOUT"
	envPushURL         = "PUSH_HTTP_URL"
)

func TestLoadPositionSettings(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want config.PositionsConfig
	}{
		{
			name: "defaults",
			want: config.PositionsConfig{
				CheckInterval: 30 * time.Second, StaleAfter: time.Minute, Workers: 4,
				QueueSize: 100, BatchSize: 100, PushURL: "http://localhost:8090", RequestTimeout: time.Second,
			},
		},
		{
			name: "read configured values",
			env: map[string]string{
				envCheckInterval: "10s", envStaleAfter: "90s",
				envPositionWorkers: "2", envQueueSize: "20", envBatchSize: "50",
				envPushURL: "https://push.example", envPushTimeout: "3s",
			},
			want: config.PositionsConfig{
				CheckInterval: 10 * time.Second, StaleAfter: 90 * time.Second, Workers: 2,
				QueueSize: 20, BatchSize: 50, PushURL: "https://push.example", RequestTimeout: 3 * time.Second,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setValidEnvironment(t)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			cfg, err := config.Load()
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.Positions)
		})
	}
}

func TestInvalidPositionSettings(t *testing.T) {
	for _, tt := range []struct{ name, value string }{
		{envCheckInterval, "0s"},
		{envStaleAfter, "-1s"},
		{envPushTimeout, "bad"},
		{envPositionWorkers, "0"},
		{envQueueSize, "-1"},
		{envBatchSize, "1.5"},
		{envPushURL, ""},
		{envPushURL, "ftp://localhost"},
		{envPushURL, "http://"},
		{envPushURL, "http://:8090"},
		{envPushURL, "http://user:pass@localhost"},
		{envPushURL, "http://localhost?token=value"},
		{envPushURL, "http://localhost?"},
	} {
		t.Run(tt.name+"="+tt.value, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv(tt.name, tt.value)
			_, err := config.Load()
			require.ErrorContains(t, err, tt.name)
		})
	}
}
