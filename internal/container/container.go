package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/config"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/gateway/push"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/health"
	httplist "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/position/list"
	httpsave "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/position/save"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/router"
	httpcreate "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/trip/create"
	httpfinish "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/trip/finish"
	httpget "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/trip/get"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/infra/clock"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/infra/postgres"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/infra/postgres/transaction"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/infra/telemetry"
	positionrepo "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/repository/position"
	triprepo "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/repository/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/service/idempotency"
	usecaselist "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/usecase/position/list"
	usecasesave "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/usecase/position/save"
	usecasecreate "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/usecase/trip/create"
	usecasefinish "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/usecase/trip/finish"
	usecaseget "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/usecase/trip/get"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/worker/positions"
)

type Container struct {
	log       *slog.Logger
	telemetry *telemetry.Telemetry
	pool      *pgxpool.Pool
	push      *push.Client
	positions *positions.Pool
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

	c = &Container{
		log:       log,
		telemetry: tel,
		pool:      pool,
		push:      push.New(cfg.Push.HTTPURL, cfg.Push.RequestTimeout, tel.TracerProvider, tel.Propagator),
	}

	handler, err := c.wire(cfg)
	if err != nil {
		return nil, err
	}

	c.server = &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           handler,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}

	return c, nil
}

func (c *Container) wire(cfg config.Config) (http.Handler, error) {
	txManager := transaction.NewManager(c.pool, pgx.ReadCommitted, cfg.DB.QueryTimeout)
	trips := triprepo.New(txManager, cfg.DB.QueryTimeout)
	tripPositions := positionrepo.New(txManager, cfg.DB.QueryTimeout)

	metrics, err := telemetry.NewTripMetrics(c.telemetry.MeterProvider)
	if err != nil {
		return nil, err
	}

	if err := telemetry.RegisterActiveTrips(c.telemetry.MeterProvider, trips.CountActive, c.log); err != nil {
		return nil, err
	}

	c.positions, err = positions.New(
		positions.Config{
			CheckInterval: cfg.Positions.CheckInterval,
			StaleAfter:    cfg.Positions.StaleAfter,
			Workers:       cfg.Positions.Workers,
			QueueSize:     cfg.Positions.QueueSize,
			PushTimeout:   cfg.Push.RequestTimeout,
		},
		trips,
		c.push,
		clock.Real{},
		c.telemetry.TracerProvider,
		c.telemetry.MeterProvider,
		c.log,
	)
	if err != nil {
		return nil, err
	}

	log := c.log

	return router.New(
		router.Handlers{
			Create:        httpcreate.New(usecasecreate.New(trips, idempotency.New(trips, cfg.IdempotencyKeyTTL), txManager, metrics, log, now, uuid.NewV7), log),
			Get:           httpget.New(usecaseget.New(trips), log),
			Finish:        httpfinish.New(usecasefinish.New(trips, txManager, metrics, log, now), log),
			Health:        health.New(c.pool, cfg.DB.QueryTimeout, log),
			SavePosition:  httpsave.New(usecasesave.New(trips, tripPositions, txManager, c.positions, metrics, now), log),
			ListPositions: httplist.New(usecaselist.New(trips, tripPositions), log),
		},
		router.Telemetry{
			Log:            log,
			TracerProvider: c.telemetry.TracerProvider,
			MeterProvider:  c.telemetry.MeterProvider,
			Propagator:     c.telemetry.Propagator,
		},
	)
}

func (c *Container) Logger() *slog.Logger {
	return c.log
}

func (c *Container) Start(ctx context.Context) <-chan error {
	failed := make(chan error, 1)

	c.positions.Start(ctx)

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

	c.positions.StopScanner(ctx)

	if err := c.positions.StopWorkers(ctx); err != nil {
		c.log.ErrorContext(ctx, "position workers stopped forcibly", slog.Any("error", err))
	}

	if err := c.telemetry.Shutdown(ctx); err != nil {
		c.log.WarnContext(ctx, "telemetry shutdown failed", slog.Any("error", err))
	}

	c.push.Close()
	c.pool.Close()
}

func now() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}
