package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/amap"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
)

type Service interface {
	APIKeyConfigured() bool
	LiveWeather(ctx context.Context, city string) (amap.LiveWeather, error)
}

func Register(registry *plugin.Registry, service Service) error {
	resolver := NewResolver()

	return registry.Register(plugin.Tool{
		Definition: plugin.Definition{
			Name:        "ask_weather",
			Summary:     "查天气",
			Description: "查询某个城市或地区的实时天气数据。只有当用户明确要求查询、获取、确认或刷新天气信息时调用。如果用户是在已有天气上下文基础上继续追问穿衣建议、是否带伞、是否适合出门等，不要调用这个工具，交给主回复模型回答。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city": map[string]any{
						"type":        "string",
						"description": "用户想查询天气的城市或地区名称，例如上海、北京、杭州。",
					},
				},
				"required": []string{"city"},
			},
		},
		Handler: func(ctx context.Context, callCtx plugin.CallContext, arguments json.RawMessage) (plugin.Result, error) {
			_ = callCtx

			var args struct {
				City string `json:"city"`
			}
			if len(arguments) > 0 {
				if err := json.Unmarshal(arguments, &args); err != nil {
					return plugin.Result{}, fmt.Errorf("decode weather arguments: %w", err)
				}
			}

			args.City = strings.TrimSpace(args.City)
			if args.City == "" {
				return plugin.Result{Text: "请告诉我你想查询哪个城市的天气。"}, nil
			}
			if service == nil || !service.APIKeyConfigured() {
				return plugin.Result{Text: "天气服务还没有配置好。"}, nil
			}

			resolved, ok := resolver.Resolve(args.City)
			if !ok {
				return plugin.Result{Text: fmt.Sprintf("我还没识别出“%s”对应的城市编码。", args.City)}, nil
			}

			live, err := service.LiveWeather(ctx, resolved.Adcode)
			if err != nil {
				return plugin.Result{Text: "天气查询暂时失败了，请稍后再试。"}, nil
			}

			location := strings.TrimSpace(live.City)
			if location == "" {
				location = resolved.Name
			}
			return plugin.Result{
				Text: fmt.Sprintf(
					"%s现在%s，气温%s度，湿度%s，%s风%s级。更新时间%s。",
					location,
					live.Weather,
					live.Temperature,
					live.Humidity,
					live.WindDirection,
					live.WindPower,
					live.ReportTime,
				),
			}, nil
		},
	})
}
