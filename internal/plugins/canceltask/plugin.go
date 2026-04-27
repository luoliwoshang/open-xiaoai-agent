package canceltask

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
			Name:        "cancel_task",
			Summary:     "停任务",
			Description: "取消最近一个正在执行的异步任务。当用户说把刚刚那个任务停掉、先别做了、取消这个任务时调用。",
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
			task, err := manager.CancelLatest()
			if err != nil {
				return plugin.Result{}, err
			}
			if task == nil {
				return plugin.Result{Text: "现在没有可以取消的任务。"}, nil
			}
			return plugin.Result{Text: fmt.Sprintf("好，我已经把“%s”停掉了。", task.Title)}, nil
		},
	})
}
