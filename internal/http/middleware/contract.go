package middleware

import (
	"context"
	"log/slog"
)

type (
	errorLogger interface {
		ErrorContext(ctx context.Context, msg string, args ...any)
	}

	attrLogger interface {
		LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr)
	}
)
