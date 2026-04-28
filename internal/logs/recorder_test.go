package logs

import (
	"bytes"
	"fmt"
	stdlog "log"
	"strings"
	"testing"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/testmysql"
)

func TestRecorderPersistsStandardLogLines(t *testing.T) {
	t.Parallel()

	store, err := NewStore(testmysql.NewDSN(t))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	var sink bytes.Buffer
	recorder := NewRecorder(store, &sink)

	logger := stdlog.New(recorder, "", stdlog.LstdFlags|stdlog.Lmicroseconds|stdlog.Lshortfile)
	logger.Printf("test recorder message %d", 42)

	page, err := store.List(ListQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(page.Items))
	}
	if !strings.Contains(page.Items[0].Message, "test recorder message 42") {
		t.Fatalf("Message = %q", page.Items[0].Message)
	}
	if !strings.Contains(page.Items[0].Source, "recorder_test.go:") {
		t.Fatalf("Source = %q, want recorder_test.go:*", page.Items[0].Source)
	}
	if !strings.Contains(sink.String(), "test recorder message 42") {
		t.Fatalf("stdout sink missing message: %q", sink.String())
	}
}

func TestStoreListPaginatesNewestFirst(t *testing.T) {
	t.Parallel()

	store, err := NewStore(testmysql.NewDSN(t))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	base := time.Date(2026, 4, 28, 13, 0, 0, 0, time.Local)
	for index := 0; index < 3; index++ {
		err := store.Append(Entry{
			ID:        fmt.Sprintf("log_manual_%d", index),
			Level:     "info",
			Source:    fmt.Sprintf("source-%d", index),
			Message:   fmt.Sprintf("message-%d", index),
			Raw:       fmt.Sprintf("raw-%d", index),
			CreatedAt: base.Add(time.Duration(index) * time.Second),
		})
		if err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}

	page, err := store.List(ListQuery{Page: 2, PageSize: 2})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if page.Total != 3 {
		t.Fatalf("Total = %d, want 3", page.Total)
	}
	if len(page.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(page.Items))
	}
	if page.Items[0].Message != "message-0" {
		t.Fatalf("Message = %q, want message-0", page.Items[0].Message)
	}
	if page.HasMore {
		t.Fatal("HasMore = true, want false")
	}
}
