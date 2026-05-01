package voice

import (
	"strings"
	"time"
	"unicode/utf8"
)

// StreamSpeaker 把上游流式增量文本整理成多段完整短句，再交给 Channel 逐段播报。
//
// 这样主流程仍然可以一边拿 reply delta，一边把内容持续说出来，
// 而具体的分句、缓冲和段间停顿策略保持在 voice 层通用实现里。
type StreamSpeaker struct {
	channel Channel
	timeout time.Duration
	gap     time.Duration
	buffer  strings.Builder
}

func NewStreamSpeaker(channel Channel, timeout time.Duration, gap time.Duration) *StreamSpeaker {
	return &StreamSpeaker{
		channel: channel,
		timeout: timeout,
		gap:     gap,
	}
}

func (s *StreamSpeaker) Push(delta string) error {
	if delta == "" {
		return nil
	}

	s.buffer.WriteString(delta)
	return s.flushReady(false)
}

func (s *StreamSpeaker) Close() error {
	return s.flushReady(true)
}

func (s *StreamSpeaker) flushReady(force bool) error {
	text := strings.TrimSpace(s.buffer.String())
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

		if err := s.channel.SpeakText(chunk, s.timeout); err != nil {
			return err
		}
		if s.gap > 0 && text != "" {
			time.Sleep(s.gap)
		}
	}

	s.buffer.Reset()
	s.buffer.WriteString(text)
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
