package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/Ruhanyat-994/GuardPipe/internal/platform/logger"
)

func TestWithRequestID_AddsRequestIDToEveryLine(t *testing.T) {
	var buf bytes.Buffer
	base := logger.New("info", &buf)
	withReq := logger.WithRequestID(base, "req-123")

	withReq.Info("handled request")

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log output is not valid JSON: %v\n%s", err, buf.String())
	}
	if line["request_id"] != "req-123" {
		t.Errorf("request_id = %v, want %q", line["request_id"], "req-123")
	}
}

func TestNew_UnknownLevelFallsBackToInfo(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New("not-a-real-level", &buf)

	if l.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("an unrecognised level enabled debug logging; want the info fallback")
	}
	if !l.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("an unrecognised level should still fall back to info, not disable logging entirely")
	}
}

func TestNew_DebugLevelEnablesDebugLogging(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New("debug", &buf)
	if !l.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("New(\"debug\", ...) does not have debug level enabled")
	}
}

// TestRedaction_StripsKnownSecretKeys is the core security-relevant test for
// this package, exercising the real ReplaceAttr hook inside logger.New (not
// a re-implemented copy of the redaction rules): a log line built with a
// secret-shaped key must never carry the real value.
func TestRedaction_StripsKnownSecretKeys(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New("info", &buf)

	l.Info("user login", "email", "nadia@example.com", "password", "hunter2")

	out := buf.String()
	if strings.Contains(out, "hunter2") {
		t.Fatalf("log line leaked a redacted-key value: %s", out)
	}
	if !strings.Contains(out, `"[REDACTED]"`) {
		t.Errorf("expected the redaction placeholder in the output: %s", out)
	}
	// Near-miss: a key that isn't secret-shaped must pass through untouched.
	if !strings.Contains(out, "nadia@example.com") {
		t.Errorf("a non-secret key was redacted or dropped: %s", out)
	}
}

// TestRedaction_IsCaseInsensitiveAndCoversKnownAliases checks a spread of
// the documented secret-shaped keys, not just "password" — a redaction hook
// that only catches the one example it was tested against is not a
// redaction hook.
func TestRedaction_IsCaseInsensitiveAndCoversKnownAliases(t *testing.T) {
	keys := []string{"Authorization", "JWT_SECRET", "api_key", "refresh_token", "encryption_key"}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			var buf bytes.Buffer
			l := logger.New("info", &buf)
			l.Info("event", key, "super-secret-value")

			if strings.Contains(buf.String(), "super-secret-value") {
				t.Errorf("key %q was not redacted: %s", key, buf.String())
			}
		})
	}
}

func TestNew_WritesOneValidJSONObjectPerLine(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New("info", &buf)

	l.Info("first")
	l.Info("second")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", len(lines), buf.String())
	}
	for _, line := range lines {
		var decoded map[string]any
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Errorf("line is not valid JSON: %v: %s", err, line)
		}
	}
}
