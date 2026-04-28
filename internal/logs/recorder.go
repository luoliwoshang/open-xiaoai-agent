package logs

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const standardLogTimeLayout = "2006/01/02 15:04:05.000000"

type Recorder struct {
	mu        sync.Mutex
	out       io.Writer
	store     *Store
	remainder []byte
	now       func() time.Time
}

func NewRecorder(store *Store, out io.Writer) *Recorder {
	return &Recorder{
		out:   out,
		store: store,
		now:   time.Now,
	}
}

func (r *Recorder) Write(p []byte) (int, error) {
	if r == nil {
		return len(p), nil
	}
	if r.out != nil {
		if _, err := r.out.Write(p); err != nil {
			return 0, err
		}
	}

	lines := r.collectLines(p)
	for _, line := range lines {
		entry := r.parseLine(line)
		if entry.Raw == "" {
			continue
		}
		if r.store == nil {
			continue
		}
		if err := r.store.Append(entry); err != nil && r.out != nil {
			_, _ = fmt.Fprintf(r.out, "runtime log persist failed: %v\n", err)
		}
	}
	return len(p), nil
}

func (r *Recorder) collectLines(p []byte) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.remainder = append(r.remainder, p...)
	var lines []string
	for {
		index := bytes.IndexByte(r.remainder, '\n')
		if index < 0 {
			break
		}
		line := strings.TrimRight(string(r.remainder[:index]), "\r")
		r.remainder = append([]byte(nil), r.remainder[index+1:]...)
		lines = append(lines, line)
	}
	return lines
}

func (r *Recorder) parseLine(line string) Entry {
	line = strings.TrimSpace(line)
	if line == "" {
		return Entry{}
	}

	entry := Entry{
		Level:     inferLevel(line),
		Raw:       line,
		Message:   line,
		CreatedAt: r.now(),
	}
	if len(line) < len(standardLogTimeLayout) {
		return entry
	}

	if createdAt, err := time.ParseInLocation(standardLogTimeLayout, line[:len(standardLogTimeLayout)], time.Local); err == nil {
		entry.CreatedAt = createdAt
		rest := strings.TrimSpace(line[len(standardLogTimeLayout):])
		if rest != "" {
			if index := strings.Index(rest, ": "); index >= 0 {
				entry.Source = strings.TrimSpace(rest[:index])
				entry.Message = strings.TrimSpace(rest[index+2:])
				entry.Level = inferLevel(entry.Message)
				return entry
			}
			entry.Message = rest
			entry.Level = inferLevel(rest)
		}
	}
	return entry
}

func inferLevel(text string) string {
	value := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.Contains(value, "panic"), strings.Contains(value, "fatal"):
		return "fatal"
	case strings.Contains(value, "error"), strings.Contains(value, "failed"), strings.Contains(value, "invalid"):
		return "error"
	case strings.Contains(value, "warn"), strings.Contains(value, "timeout"), strings.Contains(value, "dropped"):
		return "warn"
	case strings.Contains(value, "debug"):
		return "debug"
	default:
		return "info"
	}
}
