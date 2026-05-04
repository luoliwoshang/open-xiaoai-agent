package complextask

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
)

type Runner interface {
	Run(ctx context.Context, prompt string, reporter plugin.AsyncReporter) (string, error)
	Resume(ctx context.Context, sourceTaskID string, prompt string, reporter plugin.AsyncReporter) (string, error)
}

type ClaudeRunner struct {
	store *Store
	cwd   string
}

func NewClaudeRunner(store *Store, cwd string) *ClaudeRunner {
	return &ClaudeRunner{store: store, cwd: cwd}
}

func (r *ClaudeRunner) Run(ctx context.Context, prompt string, reporter plugin.AsyncReporter) (string, error) {
	taskID := reporter.TaskID()
	if err := r.store.Start(taskID, prompt, r.cwd); err != nil {
		return "", err
	}
	if err := r.store.MarkRunning(taskID); err != nil {
		return "", err
	}

	return r.runCommand(ctx, taskID, exec.CommandContext(
		ctx,
		"claude",
		"--dangerously-skip-permissions",
		"--print",
		"--output-format",
		"stream-json",
		"--verbose",
		buildClaudePrompt(taskID, prompt, buildMemoryPrompt(ctx)),
	), reporter)
}

func (r *ClaudeRunner) Resume(ctx context.Context, sourceTaskID string, prompt string, reporter plugin.AsyncReporter) (string, error) {
	source, ok := r.store.Get(sourceTaskID)
	if !ok {
		return "", fmt.Errorf("source claude task %q not found", sourceTaskID)
	}
	if strings.TrimSpace(source.SessionID) == "" {
		return "", fmt.Errorf("source claude task %q has no session id", sourceTaskID)
	}

	taskID := reporter.TaskID()
	if err := r.store.Start(taskID, prompt, r.cwd); err != nil {
		return "", err
	}
	if err := r.store.SetSession(taskID, source.SessionID); err != nil {
		return "", err
	}
	if err := r.store.MarkRunning(taskID); err != nil {
		return "", err
	}

	return r.runCommand(ctx, taskID, exec.CommandContext(
		ctx,
		"claude",
		"--dangerously-skip-permissions",
		"--resume",
		source.SessionID,
		"--print",
		"--output-format",
		"stream-json",
		"--verbose",
		buildClaudeResumePrompt(taskID, prompt, buildMemoryPrompt(ctx)),
	), reporter)
}

func (r *ClaudeRunner) runCommand(ctx context.Context, taskID string, command *exec.Cmd, reporter plugin.AsyncReporter) (string, error) {
	command.Dir = r.cwd

	stdout, err := command.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("open claude stdout: %w", err)
	}

	var stderr bytes.Buffer
	command.Stderr = &stderr

	if err := command.Start(); err != nil {
		return "", fmt.Errorf("start claude: %w", err)
	}

	parser := newStreamParser(taskID, r.store, reporter)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if err := parser.HandleLine(scanner.Bytes()); err != nil {
			_ = r.store.Fail(taskID, err.Error())
			return "", err
		}
	}
	if err := scanner.Err(); err != nil {
		_ = r.store.Fail(taskID, err.Error())
		return "", fmt.Errorf("read claude output: %w", err)
	}

	if err := command.Wait(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		_ = r.store.Fail(taskID, message)
		return "", fmt.Errorf("claude command failed: %s", message)
	}

	if parser.result != "" {
		if err := r.importArtifacts(taskID, reporter); err != nil {
			_ = r.store.Fail(taskID, err.Error())
			return "", err
		}
		if err := r.store.Complete(taskID, parser.result); err != nil {
			return "", err
		}
		return parser.result, nil
	}

	message := "claude command finished without final result"
	_ = r.store.Fail(taskID, message)
	return "", errors.New(message)
}

type streamEnvelope struct {
	Type      string         `json:"type"`
	Subtype   string         `json:"subtype"`
	SessionID string         `json:"session_id"`
	Result    string         `json:"result"`
	Errors    []string       `json:"errors"`
	Message   *streamMessage `json:"message"`
}

type streamMessage struct {
	Content []streamContent `json:"content"`
}

type streamContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type streamParser struct {
	taskID   string
	store    *Store
	reporter plugin.AsyncReporter
	result   string
}

func newStreamParser(taskID string, store *Store, reporter plugin.AsyncReporter) *streamParser {
	return &streamParser{taskID: taskID, store: store, reporter: reporter}
}

func (p *streamParser) HandleLine(line []byte) error {
	var envelope streamEnvelope
	if err := json.Unmarshal(line, &envelope); err != nil {
		return fmt.Errorf("decode claude stream line: %w", err)
	}

	switch envelope.Type {
	case "system":
		if envelope.Subtype == "init" && envelope.SessionID != "" {
			if err := p.store.SetSession(p.taskID, envelope.SessionID); err != nil {
				return err
			}
			return p.reporter.Event("claude_init", "Claude 会话已建立")
		}
	case "assistant":
		if envelope.Message == nil {
			return nil
		}
		for _, content := range envelope.Message.Content {
			if content.Type != "text" {
				continue
			}
			text := strings.TrimSpace(content.Text)
			if text == "" {
				continue
			}
			summary := summarizeProgressText(text)
			if err := p.store.UpdateSummary(p.taskID, summary, text); err != nil {
				return err
			}
			if err := p.reporter.Update(summary); err != nil {
				return err
			}
		}
	case "result":
		if len(envelope.Errors) > 0 {
			message := strings.Join(envelope.Errors, "; ")
			if err := p.store.Fail(p.taskID, message); err != nil {
				return err
			}
			return errors.New(message)
		}
		result := strings.TrimSpace(envelope.Result)
		if result != "" {
			p.result = result
		}
	}

	return nil
}

func summarizeText(text string) string {
	return strings.TrimSpace(text)
}

type artifactManifest struct {
	Deliver []artifactManifestEntry `json:"deliver"`
}

type artifactManifestEntry struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	MIMEType string `json:"mime_type"`
}

func buildClaudePrompt(taskID string, task string, memoryPrompt string) string {
	task = strings.TrimSpace(task)
	manifestPath := artifactManifestRelativePath(taskID)
	artifactDir := artifactOutputDirRelativePath(taskID)
	prompt := strings.TrimSpace(fmt.Sprintf(
		`执行以下任务：%s

输出要求：
1. 执行中请持续汇报阶段性进度，但进度汇报要相对简短。
2. 进度汇报只用自然中文短句，不要使用特殊符号、emoji、Markdown 列表、代码块或其他不利于 TTS 识别的格式。
3. 如果任务还没有真正结束，不要提前说已经完成。
4. 最终总结也要简短精炼，默认按可以直接播报给用户的口语化结果来写。
5. 如果本次任务产出了需要交付给系统的文件，必须把这些最终交付文件统一放在目录：%s
6. 不要把需要交付的最终文件直接留在工作目录根目录、桌面路径描述或其他任意位置；只要是要交付给系统的产物，就必须放进这个任务专属目录。
7. 然后在工作目录里写入 JSON 索引文件：%s
8. 这个 JSON 文件只负责声明交付产物位置和元数据，不要把文件内容写进 JSON。
9. JSON 结构固定为 {"deliver":[{"path":"相对工作目录的文件路径","name":"展示文件名","kind":"file","mime_type":"可选 MIME 类型"}]}。
10. manifest 里的 path 必须指向这次任务真实生成的文件，并且只能引用上面那个任务专属目录里的文件。
11. 如果你在 manifest 里填写 name，name 必须优先带上和真实文件一致的后缀；如果你不确定展示文件名，就直接省略 name，让系统回退到 path 对应的文件名。
12. 如果没有需要交付的文件，就不要创建这个 JSON 索引文件。
13. 最终总结优先说明两件事：你完成了什么、用户接下来可以怎么用；尽量控制在 2 到 4 句，不要长篇展开。
14. 真实用户并不坐在你所在的电脑前，所以面向用户的进度汇报和最终总结里，不要提工作目录、相对路径、绝对路径、manifest、终端命令、文件保存位置或内部工程细节。
15. 如果已经产出了文件，也不要直接说“保存为 xxx.png / xxx.html / xxx.txt”；只需要说明你已经把结果准备好了，系统会按产物交付流程处理。
16. 面向用户的表达默认按普通人能直接听懂的方式来写，避免过于专业、冗余或工程化的描述。`,
		task,
		artifactDir,
		manifestPath,
	))
	if strings.TrimSpace(memoryPrompt) != "" {
		prompt += "\n\n" + memoryPrompt
	}
	return prompt
}

func buildClaudeResumePrompt(taskID string, task string, memoryPrompt string) string {
	task = strings.TrimSpace(task)
	manifestPath := artifactManifestRelativePath(taskID)
	artifactDir := artifactOutputDirRelativePath(taskID)
	prompt := strings.TrimSpace(fmt.Sprintf(
		`继续基于刚才已经完成的同一个任务接着处理。补充要求如下：%s

输出要求：
1. 把这次输入视为对上一个任务的补充、修改或追加要求，不要丢掉之前已经完成的上下文。
2. 执行中请持续汇报阶段性进度，但进度汇报要相对简短。
3. 进度汇报只用自然中文短句，不要使用特殊符号、emoji、Markdown 列表、代码块或其他不利于 TTS 识别的格式。
4. 如果任务还没有真正结束，不要提前说已经完成。
5. 最终总结也要简短精炼，默认按可以直接播报给用户的口语化结果来写。
6. 如果本次续做任务产出了需要交付给系统的文件，必须把这些最终交付文件统一放在目录：%s
7. 不要把需要交付的最终文件直接留在工作目录根目录、桌面路径描述或其他任意位置；只要是要交付给系统的产物，就必须放进这个任务专属目录。
8. 然后在工作目录里写入 JSON 索引文件：%s
9. 这个 JSON 文件只负责声明交付产物位置和元数据，不要把文件内容写进 JSON。
10. JSON 结构固定为 {"deliver":[{"path":"相对工作目录的文件路径","name":"展示文件名","kind":"file","mime_type":"可选 MIME 类型"}]}。
11. manifest 里的 path 必须指向这次续做真实生成或更新过的文件，并且只能引用上面那个任务专属目录里的文件。
12. 如果你在 manifest 里填写 name，name 必须优先带上和真实文件一致的后缀；如果你不确定展示文件名，就直接省略 name，让系统回退到 path 对应的文件名。
13. 如果这次续做没有新的交付文件，就不要创建这个 JSON 索引文件。
14. 最终总结优先说明两件事：这次补充后你完成了什么、用户接下来可以怎么用；尽量控制在 2 到 4 句，不要长篇展开。
15. 真实用户并不坐在你所在的电脑前，所以面向用户的进度汇报和最终总结里，不要提工作目录、相对路径、绝对路径、manifest、终端命令、文件保存位置或其他内部工程细节。
16. 如果已经产出了文件，也不要直接说“保存为 xxx.png / xxx.html / xxx.txt”；只需要说明你已经把结果准备好了，系统会按产物交付流程处理。
17. 面向用户的表达默认按普通人能直接听懂的方式来写，避免过于专业、冗余或工程化的描述。`,
		task,
		artifactDir,
		manifestPath,
	))
	if strings.TrimSpace(memoryPrompt) != "" {
		prompt += "\n\n" + memoryPrompt
	}
	return prompt
}

func buildMemoryPrompt(ctx context.Context) string {
	memoryCtx, ok := plugin.MemoryFromContext(ctx)
	if !ok {
		return ""
	}
	memoryText := strings.TrimSpace(memoryCtx.Text)
	if memoryText == "" {
		return ""
	}
	memoryKey := strings.TrimSpace(memoryCtx.Key)
	if memoryKey == "" {
		memoryKey = "memory"
	}
	return strings.TrimSpace(fmt.Sprintf(`
下面是当前用户可供参考的长期记忆，请只在确实相关时使用：
1. 不要机械复述这段记忆。
2. 不要把它伪装成这轮用户刚刚输入的新要求。
3. 如果其中包含 URL、Token、密钥或其他敏感信息，只有在任务确实需要时才内部使用；不要在面向用户的进度汇报、最终总结或交付说明里主动泄露。
4. 如果记忆和当前任务无关，就忽略它，不要硬套。

记忆键：%s
-----
%s
-----`, memoryKey, memoryText))
}

func (r *ClaudeRunner) importArtifacts(taskID string, reporter plugin.AsyncReporter) error {
	manifest, found, err := r.loadArtifactManifest(taskID)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	deliverIDs := make([]string, 0, len(manifest.Deliver))
	for index, item := range manifest.Deliver {
		req, err := r.buildArtifactRequest(taskID, item)
		if err != nil {
			return fmt.Errorf("import manifest deliver[%d]: %w", index, err)
		}
		ref, err := reporter.PutArtifact(req)
		if err != nil {
			return fmt.Errorf("register manifest deliver[%d]: %w", index, err)
		}
		deliverIDs = append(deliverIDs, ref.ID)
	}
	if len(deliverIDs) == 0 {
		return nil
	}
	return reporter.Event("claude_artifacts", fmt.Sprintf("Claude 已登记 %d 个交付产物", len(deliverIDs)))
}

func (r *ClaudeRunner) loadArtifactManifest(taskID string) (artifactManifest, bool, error) {
	path := artifactManifestAbsolutePath(r.cwd, taskID)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return artifactManifest{}, false, nil
		}
		return artifactManifest{}, false, fmt.Errorf("read artifact manifest: %w", err)
	}

	var manifest artifactManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return artifactManifest{}, false, fmt.Errorf("decode artifact manifest: %w", err)
	}
	return manifest, true, nil
}

// buildArtifactRequest 把 manifest 条目转换成任务系统可消费的产物导入请求。
//
// 这里会同时完成三件事：
// 1. 根据 Claude 工作目录和 task 专属产物目录解析、校验真实文件路径。
// 2. 打开真实文件，把内容流交给任务系统导入。
// 3. 规范化展示文件名，避免 manifest.name 漏掉后缀后继续原样流入下载链路。
func (r *ClaudeRunner) buildArtifactRequest(taskID string, item artifactManifestEntry) (plugin.PutArtifactRequest, error) {
	resolvedPath, err := resolveArtifactPath(r.cwd, taskID, item.Path)
	if err != nil {
		return plugin.PutArtifactRequest{}, err
	}
	stat, err := os.Stat(resolvedPath)
	if err != nil {
		return plugin.PutArtifactRequest{}, fmt.Errorf("stat artifact file: %w", err)
	}
	if stat.IsDir() {
		return plugin.PutArtifactRequest{}, fmt.Errorf("artifact path %q is a directory", item.Path)
	}
	file, err := os.Open(resolvedPath)
	if err != nil {
		return plugin.PutArtifactRequest{}, fmt.Errorf("open artifact file: %w", err)
	}

	name := strings.TrimSpace(item.Name)
	if name == "" {
		name = filepath.Base(resolvedPath)
	} else {
		name = ensureArtifactNameHasResolvedExt(name, resolvedPath)
	}
	kind := strings.TrimSpace(item.Kind)
	if kind == "" {
		kind = "file"
	}

	return plugin.PutArtifactRequest{
		Name:     name,
		Kind:     kind,
		MIMEType: strings.TrimSpace(item.MIMEType),
		Reader:   file,
		Size:     stat.Size(),
	}, nil
}

// ensureArtifactNameHasResolvedExt 只处理一种兜底场景：
// manifest 明确给了 name，但这个展示文件名没有后缀。
//
// 此时优先复用真实文件路径上的后缀补齐 name，这样 Dashboard 展示名、
// 下载文件名和移动端打开行为就不会因为缺少后缀而出问题。
//
// 如果 name 本身已经带后缀，或者真实文件路径上也没有可靠后缀，则保持原值。
func ensureArtifactNameHasResolvedExt(name string, resolvedPath string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	if filepath.Ext(name) != "" {
		return name
	}
	resolvedExt := strings.TrimSpace(filepath.Ext(filepath.Base(strings.TrimSpace(resolvedPath))))
	if resolvedExt == "" {
		return name
	}
	return name + resolvedExt
}

func artifactManifestRelativePath(taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		taskID = "task"
	}
	return filepath.ToSlash(filepath.Join(".open-xiaoai-agent", "artifacts", taskID+".json"))
}

func artifactManifestAbsolutePath(cwd string, taskID string) string {
	return filepath.Join(strings.TrimSpace(cwd), filepath.FromSlash(artifactManifestRelativePath(taskID)))
}

func artifactOutputDirRelativePath(taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		taskID = "task"
	}
	return filepath.ToSlash(filepath.Join(".open-xiaoai-agent", "deliverables", taskID))
}

func artifactOutputDirAbsolutePath(cwd string, taskID string) string {
	return filepath.Join(strings.TrimSpace(cwd), filepath.FromSlash(artifactOutputDirRelativePath(taskID)))
}

func resolveArtifactPath(cwd string, taskID string, rawPath string) (string, error) {
	cwd = strings.TrimSpace(cwd)
	taskID = strings.TrimSpace(taskID)
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", fmt.Errorf("artifact path is required")
	}

	root, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve claude cwd: %w", err)
	}

	target := rawPath
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, filepath.FromSlash(rawPath))
	}
	target, err = filepath.Abs(filepath.Clean(target))
	if err != nil {
		return "", fmt.Errorf("resolve artifact path: %w", err)
	}

	relative, err := filepath.Rel(root, target)
	if err != nil {
		return "", fmt.Errorf("rel artifact path: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("artifact path %q escapes claude working directory", rawPath)
	}

	allowedDir, err := filepath.Abs(artifactOutputDirAbsolutePath(root, taskID))
	if err != nil {
		return "", fmt.Errorf("resolve artifact output dir: %w", err)
	}
	allowedRelative, err := filepath.Rel(root, allowedDir)
	if err != nil {
		return "", fmt.Errorf("rel artifact output dir: %w", err)
	}
	if relative != allowedRelative && !strings.HasPrefix(relative, allowedRelative+string(filepath.Separator)) {
		return "", fmt.Errorf(
			"artifact path %q must stay under %q",
			rawPath,
			artifactOutputDirRelativePath(taskID),
		)
	}
	return target, nil
}

func summarizeProgressText(text string) string {
	text = sanitizeProgressText(text)
	if text == "" {
		return ""
	}

	if sentence := firstSentence(text); sentence != "" {
		return sentence
	}
	if clause := firstClause(text); clause != "" {
		return clause
	}
	return text
}

func sanitizeProgressText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	var b strings.Builder
	lastSpace := false
	lastComma := false
	for _, r := range text {
		switch {
		case r == '\r':
			continue
		case r == '\n':
			if !lastComma && b.Len() > 0 {
				b.WriteRune('，')
				lastComma = true
				lastSpace = false
			}
		case unicode.IsSpace(r):
			if !lastSpace && !lastComma && b.Len() > 0 {
				b.WriteRune(' ')
				lastSpace = true
			}
		case isMarkdownRune(r):
			continue
		case unicode.Is(unicode.So, r), unicode.Is(unicode.Sc, r), unicode.Is(unicode.Sk, r), unicode.Is(unicode.Sm, r):
			continue
		default:
			b.WriteRune(r)
			lastSpace = false
			lastComma = r == '，'
		}
	}

	cleaned := strings.TrimSpace(b.String())
	cleaned = strings.Trim(cleaned, "，,、；;：: ")
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return cleaned
}

func firstSentence(text string) string {
	for _, sep := range []string{"。", "！", "？", "!", "?", "；", ";"} {
		if index := strings.Index(text, sep); index >= 0 {
			sentence := strings.TrimSpace(text[:index+len(sep)])
			if sentence != "" {
				return sentence
			}
		}
	}
	return ""
}

func firstClause(text string) string {
	for _, sep := range []string{"，", ",", "：", ":", "、"} {
		if index := strings.Index(text, sep); index >= 0 {
			clause := strings.TrimSpace(text[:index])
			if clause != "" {
				return clause
			}
		}
	}
	return ""
}

func isMarkdownRune(r rune) bool {
	switch r {
	case '*', '_', '`', '#', '>', '|', '~', '[', ']', '{', '}', '<', '\\':
		return true
	default:
		return false
	}
}
