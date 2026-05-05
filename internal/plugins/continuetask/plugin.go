package continuetask

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/tasks"
)

type TaskLookup interface {
	GetTask(taskID string) (*tasks.Task, bool)
	InterruptTask(taskID string) error
	MarkTaskSuperseded(taskID string, summary string) (bool, error)
}

// Resumer 表示“这个 plugin 知道如何继续自己的旧任务”。
//
// 当前 continuation 仍然是 plugin-owned：
// - continue_task 先根据主任务表找到 plugin；
// - 再由具体 plugin 自己中断旧执行、读取私有状态，并在新 child task 上继续。
type Resumer interface {
	InterruptTask(ctx context.Context, taskID string) error
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
	resumer, err := r.lookup(pluginName)
	if err != nil {
		return "", err
	}
	return resumer.ResumeTask(ctx, taskID, request, reporter)
}

func (r *ResumeRegistry) Interrupt(pluginName string, ctx context.Context, taskID string) error {
	resumer, err := r.lookup(pluginName)
	if err != nil {
		return err
	}
	return resumer.InterruptTask(ctx, taskID)
}

func (r *ResumeRegistry) lookup(pluginName string) (Resumer, error) {
	if r == nil {
		return nil, fmt.Errorf("resume registry is not configured")
	}
	resumer, ok := r.items[strings.TrimSpace(pluginName)]
	if !ok || resumer == nil {
		return nil, fmt.Errorf("resume plugin %q is not registered", pluginName)
	}
	return resumer, nil
}

func Register(registry *plugin.Registry, manager TaskLookup, resumes *ResumeRegistry) error {
	return registry.Register(plugin.Tool{
		Definition: plugin.Definition{
			Name:        "continue_task",
			Summary:     "续任务",
			Description: "接续一条之前已经创建过的异步任务链。当用户是在补充、修改、继续刚才那个网页、文档、文件或电脑任务时调用，不管那条任务现在是执行中还是已经完成。必须提供 task_id 和新的补充要求 request。task_id 应该指向那条任务链当前最新的可继续节点。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{
						"type":        "string",
						"description": "要接续的任务链当前最新可继续任务 ID，可能是执行中，也可能是已完成。",
					},
					"request": map[string]any{
						"type":        "string",
						"description": "用户新的补充要求或修改要求。",
					},
				},
				"required": []string{"task_id", "request"},
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
				TaskID  string `json:"task_id"`
				Request string `json:"request"`
			}
			if err := json.Unmarshal(arguments, &args); err != nil {
				return plugin.Result{}, fmt.Errorf("decode continue task arguments: %w", err)
			}

			args.TaskID = strings.TrimSpace(args.TaskID)
			args.Request = strings.TrimSpace(args.Request)
			log.Printf("continue_task request received: task_id=%s request=%q", args.TaskID, args.Request)
			if args.TaskID == "" || args.Request == "" {
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
			if taskPlugin == "" {
				return plugin.Result{Text: "这个任务没有可用的执行插件，我先不继续处理。"}, nil
			}
			log.Printf(
				"continue_task resolved task: task_id=%s plugin=%s title=%q parent_task_id=%s input=%q summary=%q result=%q",
				task.ID,
				taskPlugin,
				strings.TrimSpace(task.Title),
				strings.TrimSpace(task.ParentTaskID),
				strings.TrimSpace(task.Input),
				strings.TrimSpace(task.Summary),
				strings.TrimSpace(task.Result),
			)

			title := strings.TrimSpace(task.Title)
			if title == "" {
				title = "之前那个任务"
			}
			memoryCtx, hasMemory := plugin.MemoryFromContext(ctx)

			return plugin.Result{
				Text:       fmt.Sprintf("好，我就在“%s”这个任务基础上继续处理。", title),
				OutputMode: plugin.OutputModeAsyncAccept,
				AsyncTask: &plugin.AsyncTask{
					Plugin:       taskPlugin,
					Kind:         strings.TrimSpace(task.Kind),
					Title:        args.Request,
					Input:        args.Request,
					ParentTaskID: task.ID,
					Run: func(ctx context.Context, reporter plugin.AsyncReporter) (string, error) {
						if hasMemory {
							ctx = plugin.WithMemoryContext(ctx, memoryCtx.Key, memoryCtx.Text)
						}
						return runContinuation(ctx, manager, resumes, task.ID, taskPlugin, args.Request, reporter)
					},
				},
			}, nil
		},
	})
}

// runContinuation 是 continue_task 的最小编排层。
//
// 它故意不放进 ResumeRegistry 里，避免 registry 既负责“分发”，又负责“任务状态机”。
// 当前这里统一处理三件事：
// 1. 读取 source task 当前状态；
// 2. 如果 source 仍在执行，先中断旧执行，并把旧任务标成 superseded；
// 3. 再交给具体 plugin 的 ResumeTask，在新 child task 上继续。
func runContinuation(ctx context.Context, manager TaskLookup, resumes *ResumeRegistry, taskID string, pluginName string, request string, reporter plugin.AsyncReporter) (string, error) {
	sourceTask, ok := manager.GetTask(taskID)
	if !ok {
		return "", fmt.Errorf("source task %q not found", taskID)
	}

	switch sourceTask.State {
	case tasks.StateAccepted, tasks.StateRunning:
		log.Printf("continue_task interrupt running source task: task_id=%s plugin=%s state=%s", taskID, pluginName, sourceTask.State)
		if err := manager.InterruptTask(taskID); err != nil {
			return "", err
		}
		if err := resumes.Interrupt(pluginName, ctx, taskID); err != nil {
			return "", err
		}

		latestTask, ok := manager.GetTask(taskID)
		if !ok {
			return "", fmt.Errorf("source task %q disappeared during continuation", taskID)
		}
		switch latestTask.State {
		case tasks.StateAccepted, tasks.StateRunning:
			applied, err := manager.MarkTaskSuperseded(taskID, "任务已被新的补充要求接续")
			if err != nil {
				return "", err
			}
			if applied {
				log.Printf("continue_task source task superseded: task_id=%s plugin=%s", taskID, pluginName)
			}
		case tasks.StateCompleted:
			log.Printf("continue_task source task completed during interrupt race: task_id=%s plugin=%s", taskID, pluginName)
		case tasks.StateFailed, tasks.StateCanceled, tasks.StateSuperseded:
			return "", fmt.Errorf("source task %q moved to state %s during continuation", taskID, latestTask.State)
		default:
			return "", fmt.Errorf("source task %q moved to unknown state %s during continuation", taskID, latestTask.State)
		}
	case tasks.StateCompleted:
		log.Printf("continue_task reuse completed source task: task_id=%s plugin=%s", taskID, pluginName)
	case tasks.StateFailed, tasks.StateCanceled, tasks.StateSuperseded:
		return "", fmt.Errorf("source task %q in state %s cannot be continued", taskID, sourceTask.State)
	default:
		return "", fmt.Errorf("source task %q in unknown state %s cannot be continued", taskID, sourceTask.State)
	}

	return resumes.Resume(pluginName, ctx, taskID, request, reporter)
}
