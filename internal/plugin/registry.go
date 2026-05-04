package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/llm"
)

type Result struct {
	Text       string
	OutputMode OutputMode
	AsyncTask  *AsyncTask
}

type OutputMode string

const (
	OutputModeUseReplyModel OutputMode = "use_reply_model"
	OutputModeDirect        OutputMode = "direct"
	OutputModeAsyncAccept   OutputMode = "async_accept"
)

func (r Result) NormalizedOutputMode() OutputMode {
	if r.OutputMode == "" {
		if r.AsyncTask != nil {
			return OutputModeAsyncAccept
		}
		return OutputModeUseReplyModel
	}
	return r.OutputMode
}

type AsyncReporter interface {
	TaskID() string
	Update(summary string) error
	Event(eventType string, message string) error
	PutArtifact(req PutArtifactRequest) (ArtifactRef, error)
}

type AsyncTask struct {
	Plugin       string
	Kind         string
	Title        string
	Input        string
	ParentTaskID string
	Run          func(ctx context.Context, reporter AsyncReporter) (string, error)
}

type PutArtifactRequest struct {
	Name     string
	Kind     string
	MIMEType string
	Reader   io.Reader
	Size     int64
}

type ArtifactRef struct {
	ID       string
	TaskID   string
	Kind     string
	FileName string
	MIMEType string
	Size     int64
}

type CallContext struct {
	Registry *Registry
	Tool     Tool
}

type Handler func(ctx context.Context, callCtx CallContext, arguments json.RawMessage) (Result, error)

type Definition struct {
	Name        string
	Summary     string
	Description string
	InputSchema map[string]any
}

type Tool struct {
	Definition
	Handler Handler
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

func (r *Registry) Register(tool Tool) error {
	name := sanitizeName(tool.Name)
	if name == "" {
		return fmt.Errorf("tool name is required")
	}
	summary := strings.TrimSpace(tool.Summary)
	if summary == "" {
		return fmt.Errorf("tool %q summary is required", name)
	}
	if utf8.RuneCountInString(summary) > 10 {
		return fmt.Errorf("tool %q summary must be 10 chars or fewer", name)
	}
	if strings.TrimSpace(tool.Description) == "" {
		return fmt.Errorf("tool %q description is required", name)
	}
	if tool.Handler == nil {
		return fmt.Errorf("tool %q handler is required", name)
	}

	tool.Name = name
	tool.Summary = summary
	tool.Description = strings.TrimSpace(tool.Description)
	tool.InputSchema = normalizeInputSchema(tool.InputSchema)

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = tool
	return nil
}

// Definitions 返回当前 registry 里所有已注册工具的“LLM 可见定义”。
//
// 这里会把完整的 Tool（包含 Go 侧 handler）裁剪成 llm.ToolDefinition，
// 只保留 name / description / input schema 这类元数据。
// 这些定义会被 IntentRecognizer 传给 intent 模型，用来做工具路由和 tool call 判定。
//
// 因此它返回的不是“函数实现列表”，而是“给模型看的工具说明列表”。
func (r *Registry) Definitions() []llm.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	definitions := make([]llm.ToolDefinition, 0, len(names))
	for _, name := range names {
		tool := r.tools[name]
		definitions = append(definitions, llm.ToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	return definitions
}

type Metadata struct {
	Name        string
	Summary     string
	Description string
}

func (r *Registry) Metadata() []Metadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]Metadata, 0, len(names))
	for _, name := range names {
		tool := r.tools[name]
		items = append(items, Metadata{
			Name:        tool.Name,
			Summary:     tool.Summary,
			Description: tool.Description,
		})
	}
	return items
}

func (r *Registry) Call(ctx context.Context, name string, arguments json.RawMessage) (Result, error) {
	name = sanitizeName(name)

	r.mu.RLock()
	tool, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return Result{}, fmt.Errorf("tool %q not found", name)
	}

	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}

	return tool.Handler(ctx, CallContext{
		Registry: r,
		Tool:     tool,
	}, arguments)
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return ""
	}

	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		case r == '_' || r == '-' || r == ' ':
			if !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}

	result := strings.Trim(b.String(), "_")
	if len(result) > 64 {
		result = result[:64]
	}
	return result
}

func normalizeInputSchema(raw map[string]any) map[string]any {
	if raw == nil {
		return map[string]any{"type": "object"}
	}

	normalized := make(map[string]any, len(raw)+1)
	for k, v := range raw {
		normalized[k] = v
	}

	if schemaType, ok := normalized["type"].(string); !ok || strings.TrimSpace(schemaType) == "" {
		normalized["type"] = "object"
	}
	if props, ok := normalized["properties"]; ok {
		if _, valid := props.(map[string]any); !valid {
			delete(normalized, "properties")
		}
	}

	return normalized
}
