package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/config"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/health"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/router"
	httpcreate "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/trip/create"
	httpfinish "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/trip/finish"
	httpget "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/trip/get"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/infra/postgres"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/infra/postgres/transaction"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/infra/telemetry"
	triprepo "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/repository/trip"
	usecasecreate "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/usecase/trip/create"
	usecasefinish "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/usecase/trip/finish"
	usecaseget "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/usecase/trip/get"
)

type Container struct {
	log       *slog.Logger
	telemetry *telemetry.Telemetry
	pool      *pgxpool.Pool
	server    *http.Server
}

func New(ctx context.Context, cfg config.Config) (c *Container, err error) {
	log := telemetry.NewLogger(os.Stdout, cfg.LogLevel, cfg.Telemetry.ServiceName)

	tel, err := telemetry.New(ctx, cfg.Telemetry, log)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			err = errors.Join(err, tel.Shutdown(context.WithoutCancel(ctx)))
		}
	}()

	pool, err := postgres.NewPool(ctx, cfg.DB, tel.TracerProvider, tel.MeterProvider)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			pool.Close()
		}
	}()

	handler, err := newHandler(cfg, log, tel, pool)
	if err != nil {
		return nil, err
	}

	return &Container{
		log:       log,
		telemetry: tel,
		pool:      pool,
		server: &http.Server{
			Addr:              cfg.HTTP.Addr,
			Handler:           handler,
			ReadTimeout:       cfg.HTTP.ReadTimeout,
			ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
			WriteTimeout:      cfg.HTTP.WriteTimeout,
			IdleTimeout:       cfg.HTTP.IdleTimeout,
			ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
		},
	}, nil
}

func newHandler(cfg config.Config, log *slog.Logger, tel *telemetry.Telemetry, pool *pgxpool.Pool) (http.Handler, error) {
	txManager := transaction.NewManager(pool, pgx.ReadCommitted, cfg.DB.QueryTimeout)
	trips := triprepo.New(txManager, cfg.DB.QueryTimeout)

	metrics, err := telemetry.NewTripMetrics(tel.MeterProvider)
	if err != nil {
		return nil, err
	}

	if err := telemetry.RegisterActiveTrips(tel.MeterProvider, trips.CountActive, log); err != nil {
		return nil, err
	}

	return router.New(
		router.Handlers{
			Create: httpcreate.New(usecasecreate.New(trips, trips, txManager, metrics, log, now, cfg.IdempotencyKeyTTL), log),
			Get:    httpget.New(usecaseget.New(trips), log),
			Finish: httpfinish.New(usecasefinish.New(trips, txManager, metrics, log, now), log),
			Health: health.New(pool, cfg.DB.QueryTimeout, log),
		},
		router.Telemetry{
			Log:            log,
			TracerProvider: tel.TracerProvider,
			MeterProvider:  tel.MeterProvider,
			Propagator:     tel.Propagator,
		},
	)
}

func (c *Container) Logger() *slog.Logger {
	return c.log
}

func (c *Container) Start(ctx context.Context) <-chan error {
	failed := make(chan error, 1)

	go func() {
		c.log.InfoContext(ctx, "http server started", slog.String("addr", c.server.Addr))

		if err := c.server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			failed <- fmt.Errorf("http server: %w", err)
		}
	}()

	return failed
}

func (c *Container) Shutdown(ctx context.Context) {
	if err := c.server.Shutdown(ctx); err != nil {
		c.log.ErrorContext(ctx, "graceful shutdown timed out, closing connections forcibly", slog.Any("error", err))

		_ = c.server.Close()
	}

	if err := c.telemetry.Shutdown(ctx); err != nil {
		c.log.WarnContext(ctx, "telemetry shutdown failed", slog.Any("error", err))
	}

	c.pool.Close()
}

func now() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}
