package postgres

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

type executor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func executorFrom(ctx context.Context, pool *pgxpool.Pool) executor {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}

	return pool
}

type TxManager struct {
	pool            *pgxpool.Pool
	isoLevel        pgx.TxIsoLevel
	rollbackTimeout time.Duration
}

func NewTxManager(pool *pgxpool.Pool, isoLevel pgx.TxIsoLevel, rollbackTimeout time.Duration) *TxManager {
	return &TxManager{pool: pool, isoLevel: isoLevel, rollbackTimeout: rollbackTimeout}
}

func (m *TxManager) Do(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}

	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: m.isoLevel})
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

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

func (m *TxManager) rollback(ctx context.Context, tx pgx.Tx) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.rollbackTimeout)
	defer cancel()

	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return fmt.Errorf("rollback tx: %w", err)
	}

	return nil
}
