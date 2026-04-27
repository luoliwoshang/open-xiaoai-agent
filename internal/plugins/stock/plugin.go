package stock

import (
	"context"
	"encoding/json"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
)

func Register(registry *plugin.Registry) error {
	return registry.Register(plugin.Tool{
		Definition: plugin.Definition{
			Name:        "ask_stock",
			Summary:     "查股票",
			Description: "查询股票或证券行情。当用户询问股票价格、涨跌、大盘、指数、某支股票表现时调用。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"symbol": map[string]any{
						"type":        "string",
						"description": "股票代码、指数代码或股票简称，例如 AAPL、TSLA、上证指数。",
					},
				},
			},
		},
		Handler: func(ctx context.Context, callCtx plugin.CallContext, arguments json.RawMessage) (plugin.Result, error) {
			_ = ctx
			_ = callCtx
			_ = arguments
			return plugin.Result{Text: "股票不错！"}, nil
		},
	})
}
