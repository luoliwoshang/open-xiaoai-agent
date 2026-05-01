package voice

import (
	"errors"
	"testing"
	"time"
)

type fakeChannel struct {
	texts []string
	errAt int
	calls int
}

func (f *fakeChannel) PreparePlayback(opts PlaybackOptions) error {
	_ = opts
	return nil
}

func (f *fakeChannel) SpeakText(text string, timeout time.Duration) error {
	_ = timeout
	f.calls++
	if f.errAt > 0 && f.calls == f.errAt {
		return errors.New("boom")
	}
	f.texts = append(f.texts, text)
	return nil
}

func TestStreamSpeakerFlushesWhenPunctuationArrives(t *testing.T) {
	t.Parallel()

	channel := &fakeChannel{}
	player := NewStreamSpeaker(channel, 3*time.Second, 0)

	if err := player.Push("你好"); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if len(channel.texts) != 0 {
		t.Fatalf("len(texts) = %d, want 0", len(channel.texts))
	}

	if err := player.Push("，我是小智。"); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	if len(channel.texts) != 1 || channel.texts[0] != "你好，我是小智。" {
		t.Fatalf("texts = %#v, want merged spoken sentence", channel.texts)
	}
}

func TestStreamSpeakerFlushesRemainingTextOnClose(t *testing.T) {
	t.Parallel()

	channel := &fakeChannel{}
	player := NewStreamSpeaker(channel, 3*time.Second, 0)

	if err := player.Push("这是一个没有句号的长回复"); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if err := player.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if len(channel.texts) != 1 || channel.texts[0] != "这是一个没有句号的长回复" {
		t.Fatalf("texts = %#v, want single flushed chunk", channel.texts)
	}
}

func TestStreamSpeakerStopsOnFirstFailedChunk(t *testing.T) {
	t.Parallel()

	channel := &fakeChannel{errAt: 2}
	player := NewStreamSpeaker(channel, time.Second, 0)

	if err := player.Push("第一句。"); err != nil {
		t.Fatalf("Push(first) error = %v", err)
	}
	err := player.Push("第二句。")
	if err == nil {
		t.Fatal("Push(second) error = nil, want non-nil")
	}
	if len(channel.texts) != 1 || channel.texts[0] != "第一句。" {
		t.Fatalf("texts = %#v, want only first chunk spoken", channel.texts)
	}
}
