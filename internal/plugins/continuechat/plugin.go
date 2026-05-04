package continuechat

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
)

func Register(registry *plugin.Registry) error {
	return registry.Register(plugin.Tool{
		Definition: plugin.Definition{
			Name:        "continue_chat",
			Summary:     "继续聊",
			Description: "用于普通聊天、解释、建议、总结、延伸问答等不需要任何外部取数或本机执行动作的场景。当用户只是想继续对话，而不是查询工具数据或操作电脑时调用。",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		Handler: func(ctx context.Context, callCtx plugin.CallContext, arguments json.RawMessage) (plugin.Result, error) {
			_ = ctx
			_ = callCtx
			_ = arguments
			return plugin.Result{}, fmt.Errorf("continue_chat should be handled by assistant before tool runner")
		},
	})
}
