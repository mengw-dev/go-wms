package log

import (
	"context"
	"log/slog"
	"os"
)

type ctxKey string

const (
	ctxRequestID ctxKey = "request_id"
	ctxUserID    ctxKey = "user_id"
)

var logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

// Init 初始化全局 slog。
func Init(level string) {
	var lv slog.Level
	switch level {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lv}))
}

// L returns the process-wide structured logger initialized by Init.
func L() *slog.Logger { return logger }

// WithContext 从 ctx 提取 request_id / user_id 一并输出。
func WithContext(ctx context.Context) *slog.Logger {
	l := logger
	if ctx == nil {
		return l
	}
	if v, ok := ctx.Value(ctxRequestID).(string); ok && v != "" {
		l = l.With(string(ctxRequestID), v)
	}
	if v, ok := ctx.Value(ctxUserID).(int64); ok && v > 0 {
		l = l.With(string(ctxUserID), v)
	}
	return l
}

// WithRequestID attaches a request ID to the context for structured logs.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, ctxRequestID, requestID)
}

// WithUserID attaches an authenticated user ID to the context for structured logs.
func WithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, ctxUserID, userID)
}
