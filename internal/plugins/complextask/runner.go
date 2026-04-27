package complextask

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
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
		buildClaudePrompt(prompt),
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
		buildClaudeResumePrompt(prompt),
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

func buildClaudePrompt(task string) string {
	task = strings.TrimSpace(task)
	return strings.TrimSpace(fmt.Sprintf(
		`执行以下任务：%s

输出要求：
1. 执行中请持续汇报阶段性进度，但进度汇报要相对简短。
2. 进度汇报只用自然中文短句，不要使用特殊符号、emoji、Markdown 列表、代码块或其他不利于 TTS 识别的格式。
3. 如果任务还没有真正结束，不要提前说已经完成。
4. 最终总结也要简短精炼，默认按可以直接播报给用户的口语化结果来写。
5. 最终总结优先说明三件事：你完成了什么、产出放在哪里、用户接下来可以怎么用；尽量控制在 2 到 4 句，不要长篇展开。`,
		task,
	))
}

func buildClaudeResumePrompt(task string) string {
	task = strings.TrimSpace(task)
	return strings.TrimSpace(fmt.Sprintf(
		`继续基于刚才已经完成的同一个任务接着处理。补充要求如下：%s

输出要求：
1. 把这次输入视为对上一个任务的补充、修改或追加要求，不要丢掉之前已经完成的上下文。
2. 执行中请持续汇报阶段性进度，但进度汇报要相对简短。
3. 进度汇报只用自然中文短句，不要使用特殊符号、emoji、Markdown 列表、代码块或其他不利于 TTS 识别的格式。
4. 如果任务还没有真正结束，不要提前说已经完成。
5. 最终总结也要简短精炼，默认按可以直接播报给用户的口语化结果来写。
6. 最终总结优先说明三件事：这次补充后你完成了什么、产出放在哪里、用户接下来可以怎么用；尽量控制在 2 到 4 句，不要长篇展开。`,
		task,
	))
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
