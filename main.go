package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/amap"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/assistant"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/config"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/dashboard"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/im"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/llm"
	runtimelogs "github.com/luoliwoshang/open-xiaoai-agent/internal/logs"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins/complextask"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins/continuetask"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins/weather"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/server"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/settings"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/tasks"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/voice/xiaoai"
)

func main() {
	addr := flag.String("addr", ":4399", "websocket listen address")
	dashboardAddr := flag.String("dashboard-addr", ":8090", "dashboard listen address")
	claudeCwd := flag.String("claude-cwd", "", "working directory for claude complex tasks")
	debug := flag.Bool("debug", false, "print raw events for debugging")
	abortAfterASR := flag.Bool("abort-after-asr", true, "abort original XiaoAI immediately before intent stage")
	postAbortDelay := flag.Duration("post-abort-delay", 0, "delay after aborting original XiaoAI before starting playback, for example 500ms or 2s")
	useParallelIntentChat := flag.Bool("parallel-intent-chat", true, "run intent and main chat reply in parallel, and reuse speculative reply when no tool is selected")
	flag.Parse()

	cfg := server.Config{
		Addr:  *addr,
		Debug: *debug,
	}

	appConfig, err := config.Load(".")
	if err != nil {
		log.Fatal(err)
	}
	dsn := appConfig.Database.DSN
	logStore, err := runtimelogs.NewStore(dsn)
	if err != nil {
		log.Fatal(err)
	}
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	log.SetOutput(runtimelogs.NewRecorder(logStore, os.Stderr))
	log.Printf("loaded SOUL.md (%d chars)", len(appConfig.Soul))
	log.Printf("loaded models: intent=%s reply=%s", appConfig.Intent.Model, appConfig.Reply.Model)
	llmClient := llm.NewClient()
	weatherClient := amap.NewClient(appConfig.AMap.APIKey)
	settingsStore, err := settings.NewStore(dsn)
	if err != nil {
		log.Fatal(err)
	}
	imService, err := im.NewService(dsn, settingsStore, appConfig.IM.MediaCacheDir)
	if err != nil {
		log.Fatal(err)
	}
	taskManager, err := tasks.NewManager(dsn, appConfig.Task.ArtifactCacheDir)
	if err != nil {
		log.Fatal(err)
	}
	rootCWD, err := resolveClaudeCWD(*claudeCwd)
	if err != nil {
		log.Fatal(err)
	}
	claudeStore, err := complextask.NewStore(dsn)
	if err != nil {
		log.Fatal(err)
	}
	complexTaskService := complextask.NewService(claudeStore, complextask.NewClaudeRunner(claudeStore, rootCWD))
	resumeRegistry := continuetask.NewResumeRegistry()
	resumeRegistry.Register("complex_task", complexTaskService)
	plugins, err := buildPlugins(weatherClient, taskManager, complexTaskService, resumeRegistry)
	if err != nil {
		log.Fatal(err)
	}
	asrService, err := assistant.New(
		assistant.Config{
			AbortAfterASR:         *abortAfterASR,
			PostAbortDelay:        *postAbortDelay,
			UseParallelIntentChat: *useParallelIntentChat,
			StateDSN:              dsn,
		},
		settingsStore,
		llm.NewIntentRecognizer(llmClient, appConfig.Intent, plugins, taskManager),
		llm.NewReplyGenerator(llmClient, appConfig.Reply, appConfig.Soul),
		plugins,
		taskManager,
		imService,
	)
	if err != nil {
		log.Fatal(err)
	}
	taskManager.SetResultReportHook(asrService.TryDeliverTaskResultReports)

	srv := server.New(cfg, func(session *server.Session, text string) {
		asrService.HandleUserText(assistant.MainVoiceHistoryKey, xiaoai.NewChannel(session), text)
	})
	go func() {
		if err := dashboard.New(*dashboardAddr, taskManager, complexTaskService, asrService, srv, settingsStore, imService, logStore).ListenAndServe(); err != nil {
			log.Printf("dashboard stopped: %v", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func buildPlugins(weatherClient weather.Service, taskManager *tasks.Manager, complexTaskService *complextask.Service, resumeRegistry *continuetask.ResumeRegistry) (*plugin.Registry, error) {
	registry := plugin.NewRegistry()
	if err := plugins.RegisterAll(registry, weatherClient, taskManager, complexTaskService, resumeRegistry); err != nil {
		return nil, err
	}
	return registry, nil
}

func resolveClaudeCWD(value string) (string, error) {
	if strings.TrimSpace(value) != "" {
		return filepath.Abs(value)
	}

	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(current, "..", ".."))
}
