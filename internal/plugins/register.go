package plugins

import (
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins/canceltask"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins/complextask"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins/continuetask"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins/listtools"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins/querytaskprogress"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins/stock"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins/weather"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/tasks"
)

func RegisterAll(registry *plugin.Registry, weatherService weather.Service, taskManager *tasks.Manager, complexTaskService *complextask.Service, resumeRegistry *continuetask.ResumeRegistry) error {
	if err := weather.Register(registry, weatherService); err != nil {
		return err
	}
	if err := stock.Register(registry); err != nil {
		return err
	}
	if err := complextask.Register(registry, complexTaskService); err != nil {
		return err
	}
	if err := continuetask.Register(registry, taskManager, resumeRegistry); err != nil {
		return err
	}
	if err := querytaskprogress.Register(registry, taskManager); err != nil {
		return err
	}
	if err := canceltask.Register(registry, taskManager); err != nil {
		return err
	}
	if err := listtools.Register(registry); err != nil {
		return err
	}
	return nil
}
