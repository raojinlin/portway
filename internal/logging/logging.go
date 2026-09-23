package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
)

func New(writer io.Writer, level, format string) (*slog.Logger, error) {
	var min slog.Level
	switch strings.ToLower(level) {
	case "", "info":
		min = slog.LevelInfo
	case "debug":
		min = slog.LevelDebug
	case "warn":
		min = slog.LevelWarn
	case "error":
		min = slog.LevelError
	default:
		return nil, fmt.Errorf("invalid log level %q (use debug, info, warn, error)", level)
	}
	options := &slog.HandlerOptions{Level: min}
	switch strings.ToLower(format) {
	case "", "text":
		return slog.New(slog.NewTextHandler(writer, options)), nil
	case "json":
		return slog.New(slog.NewJSONHandler(writer, options)), nil
	default:
		return nil, fmt.Errorf("invalid log format %q (use text or json)", format)
	}
}

type loggerKey struct{}

func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

var privateKeyBlock = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(-----END [A-Z ]*PRIVATE KEY-----|$)`)

// SafeError never formats configuration structs. Errors with untrusted proxy
// diagnostics supply a safe summary instead; known secrets and PEM are redacted.
func SafeError(err error, secrets ...string) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	var safe interface{ SafeLogError() string }
	if errors.As(err, &safe) {
		message = safe.SafeLogError()
	}
	message = privateKeyBlock.ReplaceAllString(message, "[REDACTED PRIVATE KEY]")
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
		}
	}
	return message
}
