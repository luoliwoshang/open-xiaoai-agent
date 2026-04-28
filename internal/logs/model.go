package logs

import (
	"fmt"
	"strings"
	"time"
)

const (
	DefaultPageSize = 50
	MaxPageSize     = 200
)

type Entry struct {
	ID        string    `json:"id"`
	Level     string    `json:"level"`
	Source    string    `json:"source"`
	Message   string    `json:"message"`
	Raw       string    `json:"raw"`
	CreatedAt time.Time `json:"created_at"`
}

type ListQuery struct {
	Page     int
	PageSize int
}

type ListPage struct {
	Items    []Entry `json:"items"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
	Total    int     `json:"total"`
	HasMore  bool    `json:"has_more"`
}

func NormalizeQuery(query ListQuery) (ListQuery, error) {
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = DefaultPageSize
	}
	switch {
	case query.Page < 1:
		return ListQuery{}, fmt.Errorf("page must be at least 1")
	case query.PageSize < 1:
		return ListQuery{}, fmt.Errorf("page size must be at least 1")
	case query.PageSize > MaxPageSize:
		return ListQuery{}, fmt.Errorf("page size must be at most %d", MaxPageSize)
	default:
		return query, nil
	}
}

func normalizeLevel(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "debug", "info", "warn", "error", "fatal":
		return value
	default:
		return "info"
	}
}
