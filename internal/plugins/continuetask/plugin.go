package continuetask

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/tasks"
)

type TaskLookup interface {
	GetTask(taskID string) (*tasks.Task, bool)
}

type Resumer interface {
	ResumeTask(ctx context.Context, taskID string, request string, reporter plugin.AsyncReporter) (string, error)
}

type ResumeRegistry struct {
	items map[string]Resumer
}

func NewResumeRegistry() *ResumeRegistry {
	return &ResumeRegistry{items: make(map[string]Resumer)}
}

func (r *ResumeRegistry) Register(pluginName string, resumer Resumer) {
	if r == nil || resumer == nil {
		return
	}
	pluginName = strings.TrimSpace(pluginName)
	if pluginName == "" {
		return
	}
	r.items[pluginName] = resumer
}

func (r *ResumeRegistry) Resume(pluginName string, ctx context.Context, taskID string, request string, reporter plugin.AsyncReporter) (string, error) {
	if r == nil {
		return "", fmt.Errorf("resume registry is not configured")
	}
	resumer, ok := r.items[strings.TrimSpace(pluginName)]
	if !ok || resumer == nil {
		return "", fmt.Errorf("resume plugin %q is not registered", pluginName)
	}
	return resumer.ResumeTask(ctx, taskID, request, reporter)
}

func Register(registry *plugin.Registry, manager TaskLookup, resumes *ResumeRegistry) error {
	return registry.Register(plugin.Tool{
		Definition: plugin.Definition{
			Name:        "continue_task",
			Summary:     "续任务",
			Description: "接续一个之前已经完成的异步任务。当用户是在补充、修改、继续之前做完的网页、文档、文件或电脑任务时调用。必须提供 plugin_name、task_id 和新的补充要求 request。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"plugin_name": map[string]any{
						"type":        "string",
						"description": "要接续的任务所属插件名，例如 complex_task。",
					},
					"task_id": map[string]any{
						"type":        "string",
						"description": "要接续的已完成任务 ID。",
					},
					"request": map[string]any{
						"type":        "string",
						"description": "用户新的补充要求或修改要求。",
					},
				},
				"required": []string{"plugin_name", "task_id", "request"},
			},
		},
		Handler: func(ctx context.Context, callCtx plugin.CallContext, arguments json.RawMessage) (plugin.Result, error) {
			_ = callCtx

			if manager == nil {
				return plugin.Result{}, fmt.Errorf("task manager is not configured")
			}
			if resumes == nil {
				return plugin.Result{}, fmt.Errorf("resume registry is not configured")
			}

			var args struct {
				PluginName string `json:"plugin_name"`
				TaskID     string `json:"task_id"`
				Request    string `json:"request"`
			}
			if err := json.Unmarshal(arguments, &args); err != nil {
				return plugin.Result{}, fmt.Errorf("decode continue task arguments: %w", err)
			}

			args.PluginName = strings.TrimSpace(args.PluginName)
			args.TaskID = strings.TrimSpace(args.TaskID)
			args.Request = strings.TrimSpace(args.Request)
			if args.PluginName == "" || args.TaskID == "" || args.Request == "" {
				return plugin.Result{Text: "你想在刚刚哪个任务基础上继续补充什么要求？"}, nil
			}

			task, ok := manager.GetTask(args.TaskID)
			if !ok {
				return plugin.Result{Text: "我没找到你要继续的那个任务。"}, nil
			}
			taskPlugin := strings.TrimSpace(task.Plugin)
			if taskPlugin == "" {
				taskPlugin = strings.TrimSpace(task.Kind)
			}
			if taskPlugin != args.PluginName {
				return plugin.Result{Text: "这个任务和指定插件对不上，我先不继续处理。"}, nil
			}

			title := strings.TrimSpace(task.Title)
			if title == "" {
				title = "之前那个任务"
			}

			return plugin.Result{
				Text:       fmt.Sprintf("好，我就在“%s”这个任务基础上继续处理。", title),
				OutputMode: plugin.OutputModeAsyncAccept,
				AsyncTask: &plugin.AsyncTask{
					Plugin:       taskPlugin,
					Kind:         strings.TrimSpace(task.Kind),
					Title:        "接续：" + title,
					Input:        args.Request,
					ParentTaskID: task.ID,
					Run: func(ctx context.Context, reporter plugin.AsyncReporter) (string, error) {
						return resumes.Resume(taskPlugin, ctx, task.ID, args.Request, reporter)
					},
				},
			}, nil
		},
	})
}
