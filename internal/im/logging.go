package im

import (
	"net/url"
	"strings"
)

func logSafeID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	if len(value) <= 12 {
		return value
	}
	return value[:8] + "..." + value[len(value)-4:]
}

func logSafeBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "-"
	}
	parsed, err := url.Parse(raw)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return raw
	}
	return parsed.Host
}

func logTextLen(value string) int {
	return len([]rune(strings.TrimSpace(value)))
}
