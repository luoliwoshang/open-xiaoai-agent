package assistant

import (
	"context"
	"strings"
	"sync"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/llm"
)

type speculativeReply struct {
	cancel context.CancelFunc

	mu     sync.Mutex
	notify chan struct{}
	deltas []string
	done   bool
	err    error
}

func startSpeculativeReply(parent context.Context, reply ReplyStreamer, history []llm.Message, text string) *speculativeReply {
	ctx, cancel := context.WithCancel(parent)
	s := &speculativeReply{
		cancel: cancel,
		notify: make(chan struct{}, 1),
	}

	go func() {
		err := reply.Stream(ctx, history, text, func(delta string) error {
			s.append(delta)
			return nil
		})
		s.finish(err)
	}()

	return s
}

func (s *speculativeReply) Cancel() {
	if s == nil || s.cancel == nil {
		return
	}
	s.cancel()
}

func (s *speculativeReply) Play(onDelta func(string) error) (string, error) {
	index := 0
	for {
		s.mu.Lock()
		for index >= len(s.deltas) && !s.done {
			notify := s.notify
			s.mu.Unlock()
			<-notify
			s.mu.Lock()
		}

		if index < len(s.deltas) {
			delta := s.deltas[index]
			index++
			s.mu.Unlock()
			if err := onDelta(delta); err != nil {
				return "", err
			}
			continue
		}

		text := strings.TrimSpace(strings.Join(s.deltas, ""))
		err := s.err
		s.mu.Unlock()
		if err == context.Canceled {
			err = nil
		}
		return text, err
	}
}

func (s *speculativeReply) append(delta string) {
	s.mu.Lock()
	s.deltas = append(s.deltas, delta)
	s.mu.Unlock()
	s.signal()
}

func (s *speculativeReply) finish(err error) {
	s.mu.Lock()
	s.done = true
	s.err = err
	s.mu.Unlock()
	s.signal()
}

func (s *speculativeReply) signal() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}
