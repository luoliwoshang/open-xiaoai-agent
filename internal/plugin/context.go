package plugin

import "context"

type memoryContextKey struct{}

type MemoryContext struct {
	Key  string
	Text string
}

// WithMemoryContext 把当前主流程已经召回好的记忆挂到工具调用上下文里。
//
// 这不是通用 ToolDefinition 的一部分，而是运行时调用上下文：
// 某些真正要执行任务的工具（例如 complex_task / continue_task）
// 可以从 ctx 里读取这段记忆，再决定是否传给具体执行器。
func WithMemoryContext(ctx context.Context, memoryKey string, memoryText string) context.Context {
	return context.WithValue(ctx, memoryContextKey{}, MemoryContext{
		Key:  memoryKey,
		Text: memoryText,
	})
}

func MemoryFromContext(ctx context.Context) (MemoryContext, bool) {
	if ctx == nil {
		return MemoryContext{}, false
	}
	value, ok := ctx.Value(memoryContextKey{}).(MemoryContext)
	if !ok {
		return MemoryContext{}, false
	}
	return value, true
}
