package xiaoai

import (
	"strings"
	"testing"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/server"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/voice"
)

type fakeSession struct {
	abortCalls int
	scripts    []string
	result     server.CommandResult
	err        error
}

func (f *fakeSession) RunShell(script string, timeout time.Duration) (server.CommandResult, error) {
	_ = timeout
	f.scripts = append(f.scripts, script)
	return f.result, f.err
}

func (f *fakeSession) AbortXiaoAI(timeout time.Duration) error {
	_ = timeout
	f.abortCalls++
	return f.err
}

func TestChannelPreparePlaybackInterruptsNativeFlow(t *testing.T) {
	t.Parallel()

	session := &fakeSession{}
	channel := NewChannel(session)

	err := channel.PreparePlayback(voice.PlaybackOptions{
		InterruptNativeFlow: true,
	})
	if err != nil {
		t.Fatalf("PreparePlayback() error = %v", err)
	}
	if session.abortCalls != 1 {
		t.Fatalf("abortCalls = %d, want 1", session.abortCalls)
	}
}

func TestChannelPreparePlaybackSkipsRepeatedInterrupt(t *testing.T) {
	t.Parallel()

	session := &fakeSession{}
	channel := NewChannel(session)

	err := channel.PreparePlayback(voice.PlaybackOptions{
		InterruptNativeFlow:   true,
		NativeFlowInterrupted: true,
	})
	if err != nil {
		t.Fatalf("PreparePlayback() error = %v", err)
	}
	if session.abortCalls != 0 {
		t.Fatalf("abortCalls = %d, want 0", session.abortCalls)
	}
}

func TestChannelSpeakText(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		session := &fakeSession{result: server.CommandResult{ExitCode: 0}}
		channel := NewChannel(session)

		err := channel.SpeakText("你好，很高兴认识你！", 3*time.Second)
		if err != nil {
			t.Fatalf("SpeakText() error = %v", err)
		}
		if len(session.scripts) != 1 || session.scripts[0] != "/usr/sbin/tts_play.sh '你好，很高兴认识你！'" {
			t.Fatalf("scripts = %#v", session.scripts)
		}
	})

	t.Run("quotes text", func(t *testing.T) {
		t.Parallel()

		session := &fakeSession{result: server.CommandResult{ExitCode: 0}}
		channel := NewChannel(session)

		err := channel.SpeakText("他说'你好'", time.Second)
		if err != nil {
			t.Fatalf("SpeakText() error = %v", err)
		}
		if len(session.scripts) != 1 || !strings.Contains(session.scripts[0], `'他说'"'"'你好'"'"''`) {
			t.Fatalf("script = %#v, want single quotes escaped", session.scripts)
		}
	})

	t.Run("empty text", func(t *testing.T) {
		t.Parallel()

		channel := NewChannel(&fakeSession{})
		if err := channel.SpeakText("   ", time.Second); err == nil {
			t.Fatal("SpeakText() error = nil, want non-nil")
		}
	})

	t.Run("non-zero exit code", func(t *testing.T) {
		t.Parallel()

		session := &fakeSession{
			result: server.CommandResult{
				ExitCode: 1,
				Stderr:   "boom",
			},
		}
		channel := NewChannel(session)

		err := channel.SpeakText("你好", time.Second)
		if err == nil {
			t.Fatal("SpeakText() error = nil, want non-nil")
		}
		if !strings.Contains(err.Error(), "exit_code=1") {
			t.Fatalf("SpeakText() error = %q, want exit code detail", err)
		}
	})
}
