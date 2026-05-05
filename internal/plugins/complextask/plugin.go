package complextask

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
)

type Service struct {
	store  *Store
	runner Runner
}

func NewService(store *Store, runner Runner) *Service {
	return &Service{store: store, runner: runner}
}

func (s *Service) Snapshot() []Record {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Snapshot()
}

func (s *Service) Reset() error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Reset()
}

func (s *Service) ResumeTask(ctx context.Context, taskID string, request string, reporter plugin.AsyncReporter) (string, error) {
	if s == nil || s.runner == nil {
		return "", fmt.Errorf("claude task service is not configured")
	}
	return s.runner.Resume(ctx, taskID, request, reporter)
}

func (s *Service) InterruptTask(ctx context.Context, taskID string) error {
	if s == nil || s.runner == nil {
		return fmt.Errorf("claude task service is not configured")
	}
	return s.runner.Interrupt(ctx, taskID)
}

func Register(registry *plugin.Registry, service *Service) error {
	return registry.Register(plugin.Tool{
		Definition: plugin.Definition{
			Name:        "complex_task",
			Summary:     "做任务",
			Description: "受理复杂、耗时较长的任务。当用户要求制作网页、整理攻略、生成长文档、收集资料、持续执行多步骤任务，或者明确要求操作电脑、本地创建文件、修改文件、整理桌面、运行命令、在本机完成实际产出时调用。如果用户是在要求你代为执行一个泛化的现实任务，而当前没有更专门的已注册工具，但你可以尝试借助长期记忆、联网服务、家庭自动化系统、网页后台或其它可操作环境去完成，也可以调用它，例如打开家里的灯、关闭客厅空调、去 Home Assistant 操作某个设备。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"request": map[string]any{
						"type":        "string",
						"description": "用户希望异步完成的任务内容。",
					},
				},
				"required": []string{"request"},
			},
		},
		Handler: func(ctx context.Context, callCtx plugin.CallContext, arguments json.RawMessage) (plugin.Result, error) {
			_ = ctx
			_ = callCtx

			if service == nil || service.runner == nil {
				return plugin.Result{}, fmt.Errorf("claude task service is not configured")
			}

			var args struct {
				Request string `json:"request"`
			}
			if len(arguments) > 0 {
				if err := json.Unmarshal(arguments, &args); err != nil {
					return plugin.Result{}, fmt.Errorf("decode complex task arguments: %w", err)
				}
			}
			args.Request = strings.TrimSpace(args.Request)
			if args.Request == "" {
				return plugin.Result{Text: "你想让我异步处理什么任务？"}, nil
			}

			title := summarizeTitle(args.Request)
			memoryCtx, hasMemory := plugin.MemoryFromContext(ctx)
			return plugin.Result{
				Text:       "我这就去做！",
				OutputMode: plugin.OutputModeAsyncAccept,
				AsyncTask: &plugin.AsyncTask{
					Plugin: "complex_task",
					Kind:   "complex_task",
					Title:  title,
					Input:  args.Request,
					Run: func(ctx context.Context, reporter plugin.AsyncReporter) (string, error) {
						if hasMemory {
							ctx = plugin.WithMemoryContext(ctx, memoryCtx.Key, memoryCtx.Text)
						}
						return service.runner.Run(ctx, args.Request, reporter)
					},
				},
			}, nil
		},
	})
}

func summarizeTitle(input string) string {
	return strings.TrimSpace(input)
}
