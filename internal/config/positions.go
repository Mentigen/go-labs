package config

import (
	"fmt"
	"net/url"
	"strconv"
	"time"
)

type PositionsConfig struct {
	CheckInterval  time.Duration
	StaleAfter     time.Duration
	Workers        int
	QueueSize      int
	BatchSize      int
	PushURL        string
	RequestTimeout time.Duration
}

func loadPositions() (PositionsConfig, error) {
	var cfg PositionsConfig
	var err error
	cfg.CheckInterval, err = optionalDuration("POSITION_CHECK_INTERVAL", 30*time.Second)
	if err != nil {
		return PositionsConfig{}, err
	}
	cfg.StaleAfter, err = optionalDuration("POSITION_STALE_AFTER", time.Minute)
	if err != nil {
		return PositionsConfig{}, err
	}
	cfg.RequestTimeout, err = optionalDuration("PUSH_REQUEST_TIMEOUT", time.Second)
	if err != nil {
		return PositionsConfig{}, err
	}
	cfg.Workers, err = optionalPositiveInt("POSITION_WORKERS", 4)
	if err != nil {
		return PositionsConfig{}, err
	}
	cfg.QueueSize, err = optionalPositiveInt("POSITION_QUEUE_SIZE", 100)
	if err != nil {
		return PositionsConfig{}, err
	}
	cfg.BatchSize, err = optionalPositiveInt("POSITION_SCAN_BATCH_SIZE", 100)
	if err != nil {
		return PositionsConfig{}, err
	}
	raw, err := requiredString("PUSH_HTTP_URL")
	if err != nil {
		return PositionsConfig{}, err
	}
	parsed, err := url.Parse(raw)
	if err != nil || !validPushURL(parsed) {
		return PositionsConfig{}, fmt.Errorf("environment variable PUSH_HTTP_URL must be an HTTP(S) base URL")
	}
	cfg.PushURL = raw
	return cfg, nil
}

func optionalPositiveInt(name string, fallback int) (int, error) {
	value, err := strconv.Atoi(optionalString(name, strconv.Itoa(fallback)))
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("environment variable %s must be a positive integer", name)
	}
	return value, nil
}

func validPushURL(u *url.URL) bool {
	return (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" &&
		u.User == nil && !u.ForceQuery && u.RawQuery == "" && u.Fragment == ""
}
