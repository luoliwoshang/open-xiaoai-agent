package xiaoai

import (
	"fmt"
	"strings"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/server"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/voice"
)

type session interface {
	RunShell(script string, timeout time.Duration) (server.CommandResult, error)
	AbortXiaoAI(timeout time.Duration) error
}

type Channel struct {
	session session
}

func NewChannel(session session) *Channel {
	return &Channel{session: session}
}

func (c *Channel) PreparePlayback(opts voice.PlaybackOptions) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("voice channel is not configured")
	}

	interrupted := opts.NativeFlowInterrupted
	if !interrupted && opts.InterruptNativeFlow {
		if err := c.session.AbortXiaoAI(5 * time.Second); err != nil {
			return err
		}
		interrupted = true
	}
	if interrupted && opts.PostInterruptDelay > 0 {
		time.Sleep(opts.PostInterruptDelay)
	}
	return nil
}

func (c *Channel) SpeakText(text string, timeout time.Duration) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("voice channel is not configured")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("empty text")
	}

	result, err := c.session.RunShell("/usr/sbin/tts_play.sh "+shellQuote(text), timeout)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("exit_code=%d stderr=%s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return nil
}

func shellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'"'"'`) + "'"
}
