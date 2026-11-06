package transaction

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type txKey struct{}

type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Manager struct {
	pool     *pgxpool.Pool
	isoLevel pgx.TxIsoLevel
	timeout  time.Duration
}

func NewManager(pool *pgxpool.Pool, isoLevel pgx.TxIsoLevel, timeout time.Duration) *Manager {
	return &Manager{pool: pool, isoLevel: isoLevel, timeout: timeout}
}

func (m *Manager) Querier(ctx context.Context) Querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}

	return m.pool
}

func (m *Manager) Do(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}

	tx, err := m.begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = m.rollback(ctx, tx)

			panic(p)
		}
	}()

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		if rbErr := m.rollback(ctx, tx); rbErr != nil {
			return errors.Join(err, rbErr)
		}

		return err
	}

	if err := m.commit(ctx, tx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

func (m *Manager) begin(ctx context.Context) (pgx.Tx, error) {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	return m.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: m.isoLevel})
}

func (m *Manager) commit(ctx context.Context, tx pgx.Tx) error {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	return tx.Commit(ctx)
}

func (m *Manager) rollback(ctx context.Context, tx pgx.Tx) error {
	connClosed := tx.Conn().IsClosed()

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.timeout)
	defer cancel()

	err := tx.Rollback(ctx)
	if err == nil || errors.Is(err, pgx.ErrTxClosed) || connClosed {
		return nil
	}

	return fmt.Errorf("rollback tx: %w", err)
}
