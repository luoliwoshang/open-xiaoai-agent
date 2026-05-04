package plugins

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/amap"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
)

type fakeWeatherService struct {
	configured bool
	gotCity    string
	live       amap.LiveWeather
	err        error
}

func (f *fakeWeatherService) APIKeyConfigured() bool {
	return f.configured
}

func (f *fakeWeatherService) LiveWeather(ctx context.Context, city string) (amap.LiveWeather, error) {
	_ = ctx
	f.gotCity = city
	return f.live, f.err
}

func TestRegisterAll_ListTools(t *testing.T) {
	t.Parallel()

	registry := plugin.NewRegistry()
	if err := RegisterAll(registry, &fakeWeatherService{}, nil, nil, nil); err != nil {
		t.Fatalf("RegisterAll() error = %v", err)
	}

	result, err := registry.Call(context.Background(), "list_tools", nil)
	if err != nil {
		t.Fatalf("Call(list_tools) error = %v", err)
	}
	if result.Text != "我现在可以帮你：查天气、停任务、做任务、继续聊、续任务、查进度。" {
		t.Fatalf("result.Text = %q", result.Text)
	}
}

func TestRegisterAll_WeatherTool(t *testing.T) {
	t.Parallel()

	weather := &fakeWeatherService{
		configured: true,
		live: amap.LiveWeather{
			City:          "上海市",
			Weather:       "晴",
			Temperature:   "26",
			Humidity:      "61",
			WindDirection: "东南",
			WindPower:     "3",
			ReportTime:    "2026-04-24 18:00:00",
		},
	}
	registry := plugin.NewRegistry()
	if err := RegisterAll(registry, weather, nil, nil, nil); err != nil {
		t.Fatalf("RegisterAll() error = %v", err)
	}

	result, err := registry.Call(context.Background(), "ask_weather", json.RawMessage(`{"city":"上海"}`))
	if err != nil {
		t.Fatalf("Call(ask_weather) error = %v", err)
	}
	if weather.gotCity != "310000" {
		t.Fatalf("weather.gotCity = %q, want 310000", weather.gotCity)
	}
	for _, want := range []string{"上海市", "晴", "26", "61", "东南风3级"} {
		if !strings.Contains(result.Text, want) {
			t.Fatalf("result.Text = %q, want contains %q", result.Text, want)
		}
	}
}

func TestRegisterAll_WeatherToolWithoutAPIKey(t *testing.T) {
	t.Parallel()

	registry := plugin.NewRegistry()
	if err := RegisterAll(registry, &fakeWeatherService{configured: false}, nil, nil, nil); err != nil {
		t.Fatalf("RegisterAll() error = %v", err)
	}

	result, err := registry.Call(context.Background(), "ask_weather", json.RawMessage(`{"city":"上海"}`))
	if err != nil {
		t.Fatalf("Call(ask_weather) error = %v", err)
	}
	if result.Text != "天气服务还没有配置好。" {
		t.Fatalf("result.Text = %q", result.Text)
	}
}
