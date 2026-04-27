package speaker

import (
	"strings"
	"testing"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/server"
)

type fakeRunner struct {
	script  string
	timeout time.Duration
	scripts []string
	result  server.CommandResult
	err     error
	results []server.CommandResult
	errors  []error
	calls   int
}

func (f *fakeRunner) RunShell(script string, timeout time.Duration) (server.CommandResult, error) {
	f.script = script
	f.timeout = timeout
	f.scripts = append(f.scripts, script)
	index := f.calls
	f.calls++

	if index < len(f.errors) && f.errors[index] != nil {
		return server.CommandResult{}, f.errors[index]
	}
	if index < len(f.results) {
		return f.results[index], nil
	}
	return f.result, f.err
}

func TestPlayText(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		runner := &fakeRunner{
			result: server.CommandResult{ExitCode: 0},
		}

		err := New().PlayText(runner, "你好，很高兴认识你！", 3*time.Second)
		if err != nil {
			t.Fatalf("PlayText() error = %v", err)
		}
		if runner.script != "/usr/sbin/tts_play.sh '你好，很高兴认识你！'" {
			t.Fatalf("script = %q", runner.script)
		}
		if runner.timeout != 3*time.Second {
			t.Fatalf("timeout = %v, want %v", runner.timeout, 3*time.Second)
		}
	})

	t.Run("quotes text", func(t *testing.T) {
		t.Parallel()

		runner := &fakeRunner{
			result: server.CommandResult{ExitCode: 0},
		}

		err := New().PlayText(runner, "他说'你好'", time.Second)
		if err != nil {
			t.Fatalf("PlayText() error = %v", err)
		}
		if !strings.Contains(runner.script, `'他说'"'"'你好'"'"''`) {
			t.Fatalf("script = %q, want single quotes escaped", runner.script)
		}
	})

	t.Run("empty text", func(t *testing.T) {
		t.Parallel()

		runner := &fakeRunner{}
		err := New().PlayText(runner, "   ", time.Second)
		if err == nil {
			t.Fatal("PlayText() error = nil, want non-nil")
		}
	})

	t.Run("non-zero exit code", func(t *testing.T) {
		t.Parallel()

		runner := &fakeRunner{
			result: server.CommandResult{
				ExitCode: 1,
				Stderr:   "boom",
			},
		}

		err := New().PlayText(runner, "你好", time.Second)
		if err == nil {
			t.Fatal("PlayText() error = nil, want non-nil")
		}
		if !strings.Contains(err.Error(), "exit_code=1") {
			t.Fatalf("PlayText() error = %q, want exit code detail", err)
		}
	})
}

func TestPlayTextStream(t *testing.T) {
	t.Parallel()

	t.Run("plays each chunk in order", func(t *testing.T) {
		t.Parallel()

		runner := &fakeRunner{
			result: server.CommandResult{ExitCode: 0},
		}

		err := New().PlayTextStream(
			runner,
			[]string{"第一句。", "   ", "第二句。", "第三句。"},
			3*time.Second,
			0,
		)
		if err != nil {
			t.Fatalf("PlayTextStream() error = %v", err)
		}

		want := []string{
			"/usr/sbin/tts_play.sh '第一句。'",
			"/usr/sbin/tts_play.sh '第二句。'",
			"/usr/sbin/tts_play.sh '第三句。'",
		}
		if len(runner.scripts) != len(want) {
			t.Fatalf("len(scripts) = %d, want %d", len(runner.scripts), len(want))
		}
		for i := range want {
			if runner.scripts[i] != want[i] {
				t.Fatalf("scripts[%d] = %q, want %q", i, runner.scripts[i], want[i])
			}
		}
	})

	t.Run("stops on first failed chunk", func(t *testing.T) {
		t.Parallel()

		runner := &fakeRunner{
			results: []server.CommandResult{
				{ExitCode: 0},
				{ExitCode: 1, Stderr: "boom"},
				{ExitCode: 0},
			},
		}

		err := New().PlayTextStream(
			runner,
			[]string{"第一句。", "第二句。", "第三句。"},
			time.Second,
			0,
		)
		if err == nil {
			t.Fatal("PlayTextStream() error = nil, want non-nil")
		}
		if !strings.Contains(err.Error(), "exit_code=1") {
			t.Fatalf("PlayTextStream() error = %q, want exit code detail", err)
		}
		if len(runner.scripts) != 2 {
			t.Fatalf("len(scripts) = %d, want %d", len(runner.scripts), 2)
		}
	})
}

func TestStreamPlayer(t *testing.T) {
	t.Parallel()

	t.Run("flushes when punctuation arrives", func(t *testing.T) {
		t.Parallel()

		runner := &fakeRunner{
			result: server.CommandResult{ExitCode: 0},
		}
		player := NewStreamPlayer(New(), runner, 3*time.Second, 0)

		if err := player.Push("你好"); err != nil {
			t.Fatalf("Push() error = %v", err)
		}
		if len(runner.scripts) != 0 {
			t.Fatalf("len(scripts) = %d, want 0", len(runner.scripts))
		}

		if err := player.Push("，我是小智。"); err != nil {
			t.Fatalf("Push() error = %v", err)
		}

		want := "/usr/sbin/tts_play.sh '你好，我是小智。'"
		if len(runner.scripts) != 1 || runner.scripts[0] != want {
			t.Fatalf("scripts = %#v, want %q", runner.scripts, want)
		}
	})

	t.Run("flushes remaining text on close", func(t *testing.T) {
		t.Parallel()

		runner := &fakeRunner{
			result: server.CommandResult{ExitCode: 0},
		}
		player := NewStreamPlayer(New(), runner, 3*time.Second, 0)

		if err := player.Push("这是一个没有句号的长回复"); err != nil {
			t.Fatalf("Push() error = %v", err)
		}
		if err := player.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		want := "/usr/sbin/tts_play.sh '这是一个没有句号的长回复'"
		if len(runner.scripts) != 1 || runner.scripts[0] != want {
			t.Fatalf("scripts = %#v, want %q", runner.scripts, want)
		}
	})
}
