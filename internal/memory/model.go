package memory

import (
	"context"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/llm"
)

// Service 是 assistant 主流程依赖的最小记忆接口。
//
// 这里只约束两件事：
// 1. Recall：按 memoryKey 返回当前完整记忆文本；
// 2. UpdateFromSession：接收一整段刚结束的 session history，由具体实现决定怎么更新记忆。
//
// 像“手动编辑记忆文件”“查看更新日志”“展示 diff”这类管理能力，
// 不属于这个抽象接口的职责范围，由具体记忆实现额外提供。
type Service interface {
	Recall(ctx context.Context, memoryKey string) (string, error)
	UpdateFromSession(ctx context.Context, memoryKey string, history []llm.Message) error
}
