package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

type Config struct {
	LogLevel          slog.Level
	ShutdownTimeout   time.Duration
	IdempotencyKeyTTL time.Duration
	HTTP              HTTP
	DB                DB
}

type HTTP struct {
	Addr              string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

type DB struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	ConnectTimeout  time.Duration
	QueryTimeout    time.Duration
}

func Load() (Config, error) {
	var (
		p   parser
		cfg Config
	)

	cfg.LogLevel = p.logLevel("LOG_LEVEL")
	cfg.ShutdownTimeout = p.duration("SHUTDOWN_TIMEOUT")
	cfg.IdempotencyKeyTTL = p.durationOr("IDEMPOTENCY_KEY_TTL", 24*time.Hour)

	cfg.HTTP = HTTP{
		Addr:              p.string("HTTP_ADDR"),
		ReadTimeout:       p.durationOr("HTTP_READ_TIMEOUT", 10*time.Second),
		ReadHeaderTimeout: p.durationOr("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		WriteTimeout:      p.durationOr("HTTP_WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:       p.durationOr("HTTP_IDLE_TIMEOUT", 60*time.Second),
	}

	cfg.DB = DB{
		URL:             p.string("DATABASE_URL"),
		MaxConns:        p.int32("DATABASE_MAX_CONNS"),
		MinConns:        p.int32("DATABASE_MIN_CONNS"),
		MaxConnLifetime: p.duration("DATABASE_MAX_CONN_LIFETIME"),
		ConnectTimeout:  p.duration("DATABASE_CONNECT_TIMEOUT"),
		QueryTimeout:    p.duration("DATABASE_QUERY_TIMEOUT"),
	}

	if cfg.DB.MinConns > cfg.DB.MaxConns {
		p.errs = append(p.errs, errors.New("DATABASE_MIN_CONNS must not exceed DATABASE_MAX_CONNS"))
	}

	if err := errors.Join(p.errs...); err != nil {
		return Config{}, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

type parser struct {
	errs []error
}

func (p *parser) lookup(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return "", false
	}

	return v, true
}

func (p *parser) required(key string) (string, bool) {
	v, ok := p.lookup(key)
	if !ok {
		p.errs = append(p.errs, fmt.Errorf("%s is required", key))
	}

	return v, ok
}

func (p *parser) string(key string) string {
	v, _ := p.required(key)
	return v
}

func (p *parser) duration(key string) time.Duration {
	v, ok := p.required(key)
	if !ok {
		return 0
	}

	return p.parseDuration(key, v)
}

func (p *parser) durationOr(key string, def time.Duration) time.Duration {
	v, ok := p.lookup(key)
	if !ok {
		return def
	}

	return p.parseDuration(key, v)
}

func (p *parser) parseDuration(key, v string) time.Duration {
	d, err := time.ParseDuration(v)
	if err != nil {
		p.errs = append(p.errs, fmt.Errorf("%s: %w", key, err))
		return 0
	}

	if d <= 0 {
		p.errs = append(p.errs, fmt.Errorf("%s must be positive, got %s", key, v))
	}

	return d
}

func (p *parser) int32(key string) int32 {
	v, ok := p.required(key)
	if !ok {
		return 0
	}

	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		p.errs = append(p.errs, fmt.Errorf("%s: %w", key, err))
		return 0
	}

	if n < 0 {
		p.errs = append(p.errs, fmt.Errorf("%s must not be negative, got %d", key, n))
	}

	return int32(n)
}

func (p *parser) logLevel(key string) slog.Level {
	v, ok := p.required(key)
	if !ok {
		return slog.LevelInfo
	}

	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(v)); err != nil {
		p.errs = append(p.errs, fmt.Errorf("%s: %w", key, err))
	}

	return lvl
}
