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
		buildClaudePrompt(taskID, prompt),
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
		buildClaudeResumePrompt(taskID, prompt),
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

func buildClaudePrompt(taskID string, task string) string {
	task = strings.TrimSpace(task)
	manifestPath := artifactManifestRelativePath(taskID)
	return strings.TrimSpace(fmt.Sprintf(
		`执行以下任务：%s

输出要求：
1. 执行中请持续汇报阶段性进度，但进度汇报要相对简短。
2. 进度汇报只用自然中文短句，不要使用特殊符号、emoji、Markdown 列表、代码块或其他不利于 TTS 识别的格式。
3. 如果任务还没有真正结束，不要提前说已经完成。
4. 最终总结也要简短精炼，默认按可以直接播报给用户的口语化结果来写。
5. 如果本次任务产出了需要交付给系统的文件，请在工作目录里写入 JSON 索引文件：%s
6. 这个 JSON 文件只负责声明交付产物位置和元数据，不要把文件内容写进 JSON。
7. JSON 结构固定为 {"deliver":[{"path":"相对工作目录的文件路径","name":"展示文件名","kind":"file","mime_type":"可选 MIME 类型"}]}。
8. manifest 里的 path 必须指向这次任务真实生成的文件，并且使用相对工作目录的路径。
9. 如果没有需要交付的文件，就不要创建这个 JSON 索引文件。
10. 最终总结优先说明两件事：你完成了什么、用户接下来可以怎么用；尽量控制在 2 到 4 句，不要长篇展开。
11. 真实用户并不坐在你所在的电脑前，所以面向用户的进度汇报和最终总结里，不要提工作目录、相对路径、绝对路径、manifest、终端命令或其他内部工程细节。
12. 面向用户的表达默认按普通人能直接听懂的方式来写，避免过于专业、冗余或工程化的描述。`,
		task,
		manifestPath,
	))
}

func buildClaudeResumePrompt(taskID string, task string) string {
	task = strings.TrimSpace(task)
	manifestPath := artifactManifestRelativePath(taskID)
	return strings.TrimSpace(fmt.Sprintf(
		`继续基于刚才已经完成的同一个任务接着处理。补充要求如下：%s

输出要求：
1. 把这次输入视为对上一个任务的补充、修改或追加要求，不要丢掉之前已经完成的上下文。
2. 执行中请持续汇报阶段性进度，但进度汇报要相对简短。
3. 进度汇报只用自然中文短句，不要使用特殊符号、emoji、Markdown 列表、代码块或其他不利于 TTS 识别的格式。
4. 如果任务还没有真正结束，不要提前说已经完成。
5. 最终总结也要简短精炼，默认按可以直接播报给用户的口语化结果来写。
6. 如果本次续做任务产出了需要交付给系统的文件，请在工作目录里写入 JSON 索引文件：%s
7. 这个 JSON 文件只负责声明交付产物位置和元数据，不要把文件内容写进 JSON。
8. JSON 结构固定为 {"deliver":[{"path":"相对工作目录的文件路径","name":"展示文件名","kind":"file","mime_type":"可选 MIME 类型"}]}。
9. manifest 里的 path 必须指向这次续做真实生成或更新过的文件，并且使用相对工作目录的路径。
10. 如果这次续做没有新的交付文件，就不要创建这个 JSON 索引文件。
11. 最终总结优先说明两件事：这次补充后你完成了什么、用户接下来可以怎么用；尽量控制在 2 到 4 句，不要长篇展开。
12. 真实用户并不坐在你所在的电脑前，所以面向用户的进度汇报和最终总结里，不要提工作目录、相对路径、绝对路径、manifest、终端命令或其他内部工程细节。
13. 面向用户的表达默认按普通人能直接听懂的方式来写，避免过于专业、冗余或工程化的描述。`,
		task,
		manifestPath,
	))
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
		req, err := r.buildArtifactRequest(item)
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

func (r *ClaudeRunner) buildArtifactRequest(item artifactManifestEntry) (plugin.PutArtifactRequest, error) {
	resolvedPath, err := resolveArtifactPath(r.cwd, item.Path)
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

func resolveArtifactPath(cwd string, rawPath string) (string, error) {
	cwd = strings.TrimSpace(cwd)
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
