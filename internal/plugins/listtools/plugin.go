package listtools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
)

func Register(registry *plugin.Registry) error {
	return registry.Register(plugin.Tool{
		Definition: plugin.Definition{
			Name:        "list_tools",
			Summary:     "看能力",
			Description: "查看当前助手可以做什么。当用户询问你能做什么、会什么、可以帮什么、有哪些功能、能干啥时调用。",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		Handler: func(ctx context.Context, callCtx plugin.CallContext, arguments json.RawMessage) (plugin.Result, error) {
			_ = ctx
			_ = arguments

			items := callCtx.Registry.Metadata()
			summaries := make([]string, 0, len(items))
			for _, item := range items {
				if item.Name == callCtx.Tool.Name {
					continue
				}
				summaries = append(summaries, item.Summary)
			}
			if len(summaries) == 0 {
				return plugin.Result{Text: "当前还没有可用能力。"}, nil
			}
			return plugin.Result{
				Text: "我现在可以帮你：" + strings.Join(summaries, "、") + "。",
			}, nil
		},
	})
}
