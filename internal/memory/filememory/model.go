package filememory

import (
	"fmt"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/llm"
)

const (
	DefaultPageSize       = 20
	MaxPageSize           = 100
	DashboardManualSource = "dashboard_manual"
	SessionSummarySource  = "session_summary"
)

type ManagedFile struct {
	MemoryKey string    `json:"memory_key"`
	Path      string    `json:"path"`
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UpdateLog struct {
	ID        string        `json:"id"`
	MemoryKey string        `json:"memory_key"`
	Source    string        `json:"source"`
	Messages  []llm.Message `json:"messages"`
	Before    string        `json:"before"`
	After     string        `json:"after"`
	CreatedAt time.Time     `json:"created_at"`
}

type ListQuery struct {
	Page      int
	PageSize  int
	MemoryKey string
}

type ListPage struct {
	Items    []UpdateLog `json:"items"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Total    int         `json:"total"`
	HasMore  bool        `json:"has_more"`
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
