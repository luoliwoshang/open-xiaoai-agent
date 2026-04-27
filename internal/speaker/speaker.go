package speaker

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/server"
)

type ShellRunner interface {
	RunShell(script string, timeout time.Duration) (server.CommandResult, error)
}

type Speaker struct{}

type StreamPlayer struct {
	speaker *Speaker
	runner  ShellRunner
	timeout time.Duration
	gap     time.Duration
	buffer  strings.Builder
}

func New() *Speaker {
	return &Speaker{}
}

func NewStreamPlayer(speaker *Speaker, runner ShellRunner, timeout time.Duration, gap time.Duration) *StreamPlayer {
	return &StreamPlayer{
		speaker: speaker,
		runner:  runner,
		timeout: timeout,
		gap:     gap,
	}
}

func (s *Speaker) PlayText(runner ShellRunner, text string, timeout time.Duration) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("empty text")
	}

	result, err := runner.RunShell("/usr/sbin/tts_play.sh "+shellQuote(text), timeout)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("exit_code=%d stderr=%s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}

	return nil
}

func (s *Speaker) PlayTextStream(runner ShellRunner, chunks []string, timeout time.Duration, gap time.Duration) error {
	player := NewStreamPlayer(s, runner, timeout, gap)
	for _, chunk := range chunks {
		if err := player.Push(chunk); err != nil {
			return err
		}
	}
	return player.Close()
}

func shellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'"'"'`) + "'"
}

func (p *StreamPlayer) Push(delta string) error {
	if delta == "" {
		return nil
	}

	p.buffer.WriteString(delta)
	return p.flushReady(false)
}

func (p *StreamPlayer) Close() error {
	return p.flushReady(true)
}

func (p *StreamPlayer) flushReady(force bool) error {
	text := strings.TrimSpace(p.buffer.String())
	for {
		idx := nextBoundary(text, force)
		if idx == 0 {
			break
		}

		chunk := strings.TrimSpace(text[:idx])
		text = strings.TrimSpace(text[idx:])
		if chunk == "" {
			continue
		}

		if err := p.speaker.PlayText(p.runner, chunk, p.timeout); err != nil {
			return err
		}
		if p.gap > 0 && text != "" {
			time.Sleep(p.gap)
		}
	}

	p.buffer.Reset()
	p.buffer.WriteString(text)
	return nil
}

func nextBoundary(text string, force bool) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}

	if idx := firstBoundaryIndex(text, "。！？!?；;\n"); idx > 0 {
		return idx
	}
	if utf8.RuneCountInString(text) >= 18 {
		if idx := lastBoundaryIndex(text, "，,、：:"); idx > 0 {
			return idx
		}
	}
	if utf8.RuneCountInString(text) >= 36 {
		if idx := lastBoundaryIndex(text, " "); idx > 0 {
			return idx
		}
	}
	if force {
		return len(text)
	}

	return 0
}

func firstBoundaryIndex(text string, punctuations string) int {
	for i, r := range text {
		if strings.ContainsRune(punctuations, r) {
			return i + utf8.RuneLen(r)
		}
	}
	return 0
}

func lastBoundaryIndex(text string, punctuations string) int {
	last := 0
	for i, r := range text {
		if strings.ContainsRune(punctuations, r) {
			last = i + utf8.RuneLen(r)
		}
	}
	return last
}
