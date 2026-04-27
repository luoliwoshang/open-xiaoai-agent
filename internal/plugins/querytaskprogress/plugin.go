package querytaskprogress

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/tasks"
)

func Register(registry *plugin.Registry, manager *tasks.Manager) error {
	return registry.Register(plugin.Tool{
		Definition: plugin.Definition{
			Name:        "query_task_progress",
			Summary:     "查进度",
			Description: "查询当前异步任务进度。当用户问刚刚那个任务做到哪了、任务进展如何、现在处理到哪一步时调用。",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		Handler: func(ctx context.Context, callCtx plugin.CallContext, arguments json.RawMessage) (plugin.Result, error) {
			_ = ctx
			_ = callCtx
			_ = arguments
			if manager == nil {
				return plugin.Result{}, fmt.Errorf("task manager is not configured")
			}
			return plugin.Result{Text: manager.SummarizeProgress(3)}, nil
		},
	})
}
