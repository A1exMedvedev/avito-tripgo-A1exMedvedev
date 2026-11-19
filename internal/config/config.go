package config

import (
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/go-playground/validator/v10"
)

type Config struct {
	LogLevel          slog.Level    `env:"LOG_LEVEL,required,notEmpty"`
	ShutdownTimeout   time.Duration `env:"SHUTDOWN_TIMEOUT,required,notEmpty" validate:"gt=0"`
	IdempotencyKeyTTL time.Duration `env:"IDEMPOTENCY_KEY_TTL" envDefault:"24h" validate:"gt=0"`
	HTTP              HTTP
	DB                DB
	Telemetry         Telemetry
}

type HTTP struct {
	Addr              string        `env:"HTTP_ADDR,required,notEmpty"`
	ReadTimeout       time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"10s" validate:"gt=0"`
	ReadHeaderTimeout time.Duration `env:"HTTP_READ_HEADER_TIMEOUT" envDefault:"5s" validate:"gt=0"`
	WriteTimeout      time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"15s" validate:"gt=0"`
	IdleTimeout       time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"60s" validate:"gt=0"`
}

type DB struct {
	URL             string        `env:"DATABASE_URL,required,notEmpty"`
	MaxConns        int32         `env:"DATABASE_MAX_CONNS,required,notEmpty" validate:"gte=0"`
	MinConns        int32         `env:"DATABASE_MIN_CONNS,required,notEmpty" validate:"gte=0,ltefield=MaxConns"`
	MaxConnLifetime time.Duration `env:"DATABASE_MAX_CONN_LIFETIME,required,notEmpty" validate:"gt=0"`
	ConnectTimeout  time.Duration `env:"DATABASE_CONNECT_TIMEOUT,required,notEmpty" validate:"gt=0"`
	QueryTimeout    time.Duration `env:"DATABASE_QUERY_TIMEOUT,required,notEmpty" validate:"gt=0"`
}

type Telemetry struct {
	ServiceName string `env:"OTEL_SERVICE_NAME" envDefault:"trip-service"`
	Endpoint    string `env:"OTEL_EXPORTER_OTLP_ENDPOINT,required,notEmpty"`
	Protocol    string `env:"OTEL_EXPORTER_OTLP_PROTOCOL" envDefault:"http/protobuf" validate:"oneof=http/protobuf"`
}

func Load() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, fmt.Errorf("invalid config: %w", err)
	}

	if err := validate(cfg); err != nil {
		return Config{}, fmt.Errorf("invalid config: %w", err)
	}

	cfg.Telemetry.Endpoint = strings.TrimRight(cfg.Telemetry.Endpoint, "/")

	return cfg, nil
}

func validate(cfg Config) error {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get("env"), ",")
		return name
	})

	err := v.Struct(cfg)

	fieldErrs, ok := errors.AsType[validator.ValidationErrors](err)
	if !ok {
		return err
	}

	errs := make([]error, 0, len(fieldErrs))
	for _, fe := range fieldErrs {
		errs = append(errs, fmt.Errorf("%s must satisfy %s %s", fe.Field(), fe.Tag(), fe.Param()))
	}

	return errors.Join(errs...)
}
