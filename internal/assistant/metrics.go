package assistant

import (
	"log"
	"sync"
	"time"
)

type turnMetrics struct {
	startedAt time.Time

	mu            sync.Mutex
	firstOutputAt time.Time
}

func newTurnMetrics(startedAt time.Time, text string, historyCount int) *turnMetrics {
	log.Printf("turn started: text=%q history=%d", text, historyCount)
	return &turnMetrics{startedAt: startedAt}
}

func (m *turnMetrics) MarkOutputStart(source string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.firstOutputAt.IsZero() {
		return
	}

	m.firstOutputAt = time.Now()
	log.Printf(
		"output started: source=%s first_output=%s",
		source,
		m.firstOutputAt.Sub(m.startedAt).Round(time.Millisecond),
	)
}

func (m *turnMetrics) LogCompleted(label string) {
	m.mu.Lock()
	firstOutputAt := m.firstOutputAt
	m.mu.Unlock()

	total := time.Since(m.startedAt).Round(time.Millisecond)
	if firstOutputAt.IsZero() {
		log.Printf("%s completed: total=%s first_output=not_started", label, total)
		return
	}

	log.Printf(
		"%s completed: first_output=%s total=%s",
		label,
		firstOutputAt.Sub(m.startedAt).Round(time.Millisecond),
		total,
	)
}
