package debug

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/voice"
)

// Channel 是一个仅用于服务端调试的语音通道实现。
//
// 它不会真的把文本播到设备上，而是把“准备播放”和“播放文本”记录到日志里，
// 这样 dashboard 手动注入 ASR 时，也能完整走 assistant 主流程，
// 同时又不依赖真实 XiaoAI 连接。
type Channel struct {
	label string
}

func NewChannel(label string) *Channel {
	label = strings.TrimSpace(label)
	if label == "" {
		label = "debug"
	}
	return &Channel{label: label}
}

func (c *Channel) PreparePlayback(opts voice.PlaybackOptions) error {
	if c == nil {
		return fmt.Errorf("debug voice channel is not configured")
	}
	log.Printf(
		"debug voice prepare: channel=%s interrupt_native=%t already_interrupted=%t post_delay=%s",
		c.label,
		opts.InterruptNativeFlow,
		opts.NativeFlowInterrupted,
		opts.PostInterruptDelay,
	)
	return nil
}

func (c *Channel) SpeakText(text string, timeout time.Duration) error {
	if c == nil {
		return fmt.Errorf("debug voice channel is not configured")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("empty text")
	}

	log.Printf("debug voice speak: channel=%s timeout=%s text=%q", c.label, timeout, text)
	return nil
}
