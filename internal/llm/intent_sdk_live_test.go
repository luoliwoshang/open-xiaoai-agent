package llm_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/amap"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/config"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/llm"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins/continuetask"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// TestIntentSDKChatCompletionsLiveProbe 用官方 Go SDK 直接探测当前 intent 提示词和工具定义，
// 用来观察 OpenAI-compatible provider 在“正式 intent 提示词 + 完整工具集”下，
// 是否稳定返回原生 tool_calls。
//
// 这个测试只用于本地调试，不是 CI 里的稳定断言：
//   - 如果仓库根目录没有 config.yaml，会直接 skip；
//   - 如果本地 config.yaml 缺少运行所需字段（例如 soul_path），也会 skip；
//   - 如果 intent 配置还是 placeholder，也会 skip；
//   - 探测文本固定为“帮我做一个文本文件”，用于稳定观察当前 provider
//     在同一条样本下到底是否返回 native tool_call。
//
// 之所以单独用 SDK 再跑一遍，是为了把“模型真实返回了什么”直接暴露出来，
// 便于判断问题到底在 provider 的原生 tool calling 支持，还是提示词 / 工具描述本身。
func TestIntentSDKChatCompletionsLiveProbe(t *testing.T) {
	if testing.Short() {
		t.Skip("skip live sdk intent probe in short mode")
	}

	rootDir, err := findRepoRoot()
	if err != nil {
		t.Skipf("skip live sdk intent probe: %v", err)
	}
	if _, err := os.Stat(rootDir + "/config.yaml"); err != nil {
		t.Skip("skip live sdk intent probe: config.yaml not found")
	}

	appConfig, err := config.Load(rootDir)
	if err != nil {
		t.Skipf("skip live sdk intent probe: local config is not ready: %v", err)
	}
	if shouldSkipLiveIntentConfig(appConfig.Intent) {
		t.Skip("skip live sdk intent probe: intent model config is empty or still placeholder")
	}

	toolDefinitions, err := buildSDKProbeToolDefinitions()
	if err != nil {
		t.Fatalf("build sdk probe tools: %v", err)
	}
	t.Logf("sdk probe registered tools: %s", strings.Join(toolNames(toolDefinitions), ", "))

	probeText := "帮我做一个文本文件"

	client := openai.NewClient(
		option.WithAPIKey(appConfig.Intent.APIKey),
		option.WithBaseURL(strings.TrimRight(appConfig.Intent.BaseURL, "/")),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:       openai.ChatModel(appConfig.Intent.Model),
		Messages:    buildSDKProbeMessages(probeText),
		Temperature: openai.Float(0),
		Tools:       makeSDKTools(toolDefinitions),
		ToolChoice: openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.String("required"),
		},
	})
	if err != nil {
		t.Fatalf("sdk chat completion error: %v", err)
	}

	t.Logf("sdk raw response: %s", resp.RawJSON())
	if len(resp.Choices) == 0 {
		t.Fatalf("sdk response has no choices")
	}

	choice := resp.Choices[0]
	t.Logf("sdk finish_reason: %s", choice.FinishReason)
	t.Logf("sdk content: %q", choice.Message.Content)
	t.Logf("sdk tool_calls len: %d", len(choice.Message.ToolCalls))

	if len(choice.Message.ToolCalls) > 0 {
		for index, call := range choice.Message.ToolCalls {
			switch variant := call.AsAny().(type) {
			case openai.ChatCompletionMessageFunctionToolCall:
				t.Logf(
					"sdk native tool_call[%d]: id=%s name=%s arguments=%s",
					index,
					variant.ID,
					variant.Function.Name,
					variant.Function.Arguments,
				)
			default:
				t.Logf("sdk tool_call[%d]: raw=%s", index, call.RawJSON())
			}
		}
		return
	}

	t.Logf("sdk returned no native tool_call; plain content=%q", choice.Message.Content)
}

type sdkProbeWeatherService struct{}

func (sdkProbeWeatherService) APIKeyConfigured() bool { return false }

func (sdkProbeWeatherService) LiveWeather(ctx context.Context, city string) (amap.LiveWeather, error) {
	_ = ctx
	_ = city
	return amap.LiveWeather{}, nil
}

func buildSDKProbeToolDefinitions() ([]llm.ToolDefinition, error) {
	registry := plugin.NewRegistry()
	if err := plugins.RegisterAll(registry, sdkProbeWeatherService{}, nil, nil, continuetask.NewResumeRegistry()); err != nil {
		return nil, err
	}
	return registry.Definitions(), nil
}

func toolNames(defs []llm.ToolDefinition) []string {
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, strings.TrimSpace(def.Name))
	}
	return names
}

func makeSDKTools(defs []llm.ToolDefinition) []openai.ChatCompletionToolUnionParam {
	tools := make([]openai.ChatCompletionToolUnionParam, 0, len(defs))
	for _, def := range defs {
		tools = append(tools, openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        strings.TrimSpace(def.Name),
			Description: openai.String(strings.TrimSpace(def.Description)),
			Parameters:  openai.FunctionParameters(def.InputSchema),
		}))
	}
	return tools
}

func buildSDKProbeMessages(text string) []openai.ChatCompletionMessageParamUnion {
	return []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(strings.TrimSpace(`
你是一个小爱音箱外部接管器的工具路由器。

当前系统策略是：拿到 ASR 结果后，外部助手始终接管并负责回复，不再回退给原生小爱。

规则：
1. 当用户只是普通聊天、解释、建议、总结、延伸问答、不需要任何外部动作或取数时，调用 continue_chat。
2. 如果用户输入混乱、断裂、像 ASR 纠错残片、语义不完整，或者当前信息不足以稳定判断具体工具、任务对象或参数，也调用 continue_chat，让主回复模型先请用户澄清、重说或补充。
3. 工具只负责取数或执行明确动作，不负责基于已有上下文做建议、解释或延伸聊天。
4. 如果用户明确要求你在当前电脑上实际做事，例如创建文件、修改文件、整理桌面、生成网页、写文档、执行命令、完成一个需要落地产出的多步骤任务，优先调用 complex_task，而不是 continue_chat。
5. 如果用户是在要求你代为执行一个泛化的现实任务，而当前没有更专门的已注册工具，但你可以尝试借助长期记忆、联网服务、家庭自动化系统、网页后台或其它可操作环境去完成，也优先调用 complex_task。例如“打开家里的灯”“把客厅灯关掉”“帮我开一下家里的空调”“去 Home Assistant 里把某个设备打开”。
6. 对“操作电脑”“帮我在桌面放一个文件”“帮我做个网页并保存下来”“帮我整理一个文档”这类请求，只要需要本机执行和产出物，就优先视为 complex_task。
7. 如果用户是在补充、修改、继续刚才那条任务链，不管那条任务现在是执行中还是已经完成，例如“刚刚那个网页再加一个按钮”“把上次那个文件改一下”“在刚才那个任务基础上继续做”，优先调用 continue_task。
8. 调用 continue_task 时，只需要提供 task_id 和 request 两个字段。
9. 如果任务链摘要里给出了 latest_task_id，那么 task_id 必须填写对应摘要里的 latest_task_id，不要编造，也不要回退到更早的任务 ID。
`)),
		openai.UserMessage("ASR文本：" + strings.TrimSpace(text)),
	}
}

func findRepoRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := current
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found while searching repo root")
		}
		dir = parent
	}
}

func shouldSkipLiveIntentConfig(cfg config.ModelConfig) bool {
	if strings.TrimSpace(cfg.Model) == "" {
		return true
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return true
	}
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return true
	}
	if strings.Contains(strings.ToLower(apiKey), "placeholder") {
		return true
	}
	return false
}
