package plugin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRegistryRegisterAndCall(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	err := registry.Register(Tool{
		Definition: Definition{
			Name:        " Ask Weather ",
			Summary:     "查天气",
			Description: "查询天气",
			InputSchema: map[string]any{
				"properties": map[string]any{
					"city": map[string]any{"type": "string"},
				},
			},
		},
		Handler: func(ctx context.Context, callCtx CallContext, arguments json.RawMessage) (Result, error) {
			if callCtx.Registry == nil {
				t.Fatal("callCtx.Registry = nil")
			}
			if callCtx.Tool.Name != "ask_weather" {
				t.Fatalf("callCtx.Tool.Name = %q", callCtx.Tool.Name)
			}
			return Result{Text: string(arguments)}, nil
		},
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	definitions := registry.Definitions()
	if len(definitions) != 1 {
		t.Fatalf("len(definitions) = %d, want 1", len(definitions))
	}
	if definitions[0].Name != "ask_weather" {
		t.Fatalf("definitions[0].Name = %q", definitions[0].Name)
	}
	if definitions[0].InputSchema["type"] != "object" {
		t.Fatalf("definitions[0].InputSchema[type] = %v", definitions[0].InputSchema["type"])
	}

	result, err := registry.Call(context.Background(), "ask_weather", json.RawMessage(`{"city":"上海"}`))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if result.Text != `{"city":"上海"}` {
		t.Fatalf("result.Text = %q", result.Text)
	}
}

func TestRegistryRejectsDuplicateTool(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	tool := Tool{
		Definition: Definition{
			Name:        "ask_stock",
			Summary:     "查股票",
			Description: "查询股票",
		},
		Handler: func(ctx context.Context, callCtx CallContext, arguments json.RawMessage) (Result, error) {
			_ = callCtx
			return Result{Text: "ok"}, nil
		},
	}

	if err := registry.Register(tool); err != nil {
		t.Fatalf("Register() first error = %v", err)
	}
	if err := registry.Register(tool); err == nil {
		t.Fatal("Register() second error = nil, want non-nil")
	}
}

func TestRegistryRejectsMissingOrLongSummary(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	tool := Tool{
		Definition: Definition{
			Name:        "ask_anything",
			Description: "查询任意内容",
		},
		Handler: func(ctx context.Context, callCtx CallContext, arguments json.RawMessage) (Result, error) {
			return Result{}, nil
		},
	}

	if err := registry.Register(tool); err == nil {
		t.Fatal("Register() missing summary error = nil, want non-nil")
	}

	tool.Summary = "这个简介真的超过十个字"
	if err := registry.Register(tool); err == nil {
		t.Fatal("Register() long summary error = nil, want non-nil")
	}
}

func TestRegistryMetadataSorted(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	register := func(name, summary string) {
		t.Helper()
		if err := registry.Register(Tool{
			Definition: Definition{
				Name:        name,
				Summary:     summary,
				Description: summary,
			},
			Handler: func(ctx context.Context, callCtx CallContext, arguments json.RawMessage) (Result, error) {
				return Result{}, nil
			},
		}); err != nil {
			t.Fatalf("Register(%q) error = %v", name, err)
		}
	}

	register("z_tool", "末尾")
	register("a_tool", "开头")

	items := registry.Metadata()
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Name != "a_tool" || items[1].Name != "z_tool" {
		t.Fatalf("items = %#v", items)
	}
}

func TestListToolsStyleHandlerCanReadRegistry(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	register := func(tool Tool) {
		t.Helper()
		if err := registry.Register(tool); err != nil {
			t.Fatalf("Register(%q) error = %v", tool.Name, err)
		}
	}

	register(Tool{
		Definition: Definition{
			Name:        "ask_weather",
			Summary:     "查天气",
			Description: "查询天气",
		},
		Handler: func(ctx context.Context, callCtx CallContext, arguments json.RawMessage) (Result, error) {
			return Result{Text: "天气不错！"}, nil
		},
	})
	register(Tool{
		Definition: Definition{
			Name:        "ask_stock",
			Summary:     "查股票",
			Description: "查询股票",
		},
		Handler: func(ctx context.Context, callCtx CallContext, arguments json.RawMessage) (Result, error) {
			return Result{Text: "股票不错！"}, nil
		},
	})
	register(Tool{
		Definition: Definition{
			Name:        "list_tools",
			Summary:     "看能力",
			Description: "查看能力列表",
		},
		Handler: func(ctx context.Context, callCtx CallContext, arguments json.RawMessage) (Result, error) {
			items := callCtx.Registry.Metadata()
			summaries := make([]string, 0, len(items))
			for _, item := range items {
				if item.Name == callCtx.Tool.Name {
					continue
				}
				summaries = append(summaries, item.Summary)
			}
			return Result{Text: strings.Join(summaries, "、")}, nil
		},
	})

	result, err := registry.Call(context.Background(), "list_tools", nil)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if result.Text != "查股票、查天气" {
		t.Fatalf("result.Text = %q", result.Text)
	}
}
