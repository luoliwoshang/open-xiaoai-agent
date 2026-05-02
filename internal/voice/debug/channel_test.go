package debug

import (
	"testing"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/voice"
)

func TestChannelPreparePlayback(t *testing.T) {
	t.Parallel()

	channel := NewChannel("dashboard")
	if err := channel.PreparePlayback(voice.PlaybackOptions{
		InterruptNativeFlow:   true,
		NativeFlowInterrupted: false,
		PostInterruptDelay:    200 * time.Millisecond,
	}); err != nil {
		t.Fatalf("PreparePlayback() error = %v", err)
	}
}

func TestChannelSpeakTextRejectsEmptyText(t *testing.T) {
	t.Parallel()

	channel := NewChannel("dashboard")
	if err := channel.SpeakText("   ", time.Second); err == nil {
		t.Fatal("SpeakText() error = nil, want non-nil")
	}
}
