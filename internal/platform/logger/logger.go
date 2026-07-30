// Package logger provides GuardPipe's one structured logger: JSON to
// stdout via log/slog, with a redaction hook that strips secret-shaped
// values before they're ever written (documentation/03-architecture-overview.md
// §7.3).
package logger

import (
	"io"
	"log/slog"
)

// New returns a slog.Logger that writes JSON to w, filtered through the
// redaction hook. Production code calls New(level, os.Stdout) — w is a
// parameter (rather than hardcoded) so tests can inspect real output
// instead of re-implementing the redaction rules to check them.
//
// level is one of "debug", "info", "warn", "error" (GUARDPIPE_LOG_LEVEL);
// an unrecognised value falls back to "info" rather than failing, since a
// logger is the wrong place to abort startup over a typo —
// platform/config is where fail-fast validation belongs.
func New(level string, w io.Writer) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       parseLevel(level),
		ReplaceAttr: redactAttr,
	})
	return slog.New(handler)
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// redactAttr is the slog.HandlerOptions.ReplaceAttr hook. It runs on every
// attribute, at every nesting level, before the handler serialises it.
func redactAttr(groups []string, a slog.Attr) slog.Attr {
	if isRedactedKey(a.Key) {
		a.Value = slog.StringValue(redactedPlaceholder)
	}
	return a
}

// WithRequestID returns a logger that annotates every subsequent log line
// with the given request ID, so a user-reported problem can be traced back
// to its exact log lines (documentation/07-api-specification.md §1.3,
// `request_id`). Transport middleware calls this once per request.
func WithRequestID(l *slog.Logger, requestID string) *slog.Logger {
	return l.With("request_id", requestID)
}
