package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// problems accumulates fail-fast validation failures so Load can report
// every misconfigured variable at once, instead of forcing a developer
// through one restart per typo.
type problems struct {
	messages []string
}

func (p *problems) add(format string, args ...any) {
	p.messages = append(p.messages, fmt.Sprintf(format, args...))
}

func getString(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func requireString(key string, p *problems) string {
	v := os.Getenv(key)
	if strings.TrimSpace(v) == "" {
		p.add("%s is required and was not set", key)
	}
	return v
}

func getBool(key string, def bool, p *problems) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		p.add("%s must be a boolean (true/false), got %q", key, v)
		return def
	}
	return b
}

func getInt(key string, def int, p *problems) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		p.add("%s must be an integer, got %q", key, v)
		return def
	}
	return n
}

func getDuration(key string, def time.Duration, p *problems) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		p.add("%s must be a Go duration (e.g. \"15m\", \"168h\"), got %q", key, v)
		return def
	}
	return d
}

func getCSV(key string, def []string) []string {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
