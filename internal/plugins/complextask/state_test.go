package complextask

import (
	"testing"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/testmysql"
)

func TestStoreStartAndSnapshot(t *testing.T) {
	t.Parallel()

	store, err := NewStore(testmysql.NewDSN(t))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	if err := store.Start("task_1", "做一个网页", "/tmp/work"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := store.SetSession("task_1", "session_abc"); err != nil {
		t.Fatalf("SetSession() error = %v", err)
	}
	if err := store.UpdateSummary("task_1", "正在生成第一版", "第一版已经有了"); err != nil {
		t.Fatalf("UpdateSummary() error = %v", err)
	}
	if err := store.Complete("task_1", "网页已经生成完成"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	records := store.Snapshot()
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	record := records[0]
	if record.TaskID != "task_1" {
		t.Fatalf("record.TaskID = %q", record.TaskID)
	}
	if record.SessionID != "session_abc" {
		t.Fatalf("record.SessionID = %q", record.SessionID)
	}
	if record.Status != StatusCompleted {
		t.Fatalf("record.Status = %q", record.Status)
	}
	if record.Result != "网页已经生成完成" {
		t.Fatalf("record.Result = %q", record.Result)
	}

	found, ok := store.Get("task_1")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if found.SessionID != "session_abc" {
		t.Fatalf("found.SessionID = %q", found.SessionID)
	}
}

func TestStoreResetClearsRecords(t *testing.T) {
	t.Parallel()

	store, err := NewStore(testmysql.NewDSN(t))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	if err := store.Start("task_1", "做一个网页", "/tmp/work"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := store.Reset(); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	if records := store.Snapshot(); len(records) != 0 {
		t.Fatalf("len(records) = %d, want 0", len(records))
	}
}

func TestStoreResetBlocksStaleUpdates(t *testing.T) {
	t.Parallel()

	store, err := NewStore(testmysql.NewDSN(t))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	if err := store.Start("task_1", "做一个网页", "/tmp/work"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := store.Reset(); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	if err := store.MarkRunning("task_1"); err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}
	if err := store.SetSession("task_1", "session_abc"); err != nil {
		t.Fatalf("SetSession() error = %v", err)
	}
	if err := store.UpdateSummary("task_1", "正在生成第一版", "第一版已经有了"); err != nil {
		t.Fatalf("UpdateSummary() error = %v", err)
	}
	if err := store.Complete("task_1", "网页已经生成完成"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if err := store.Fail("task_1", "命令失败"); err != nil {
		t.Fatalf("Fail() error = %v", err)
	}
	if records := store.Snapshot(); len(records) != 0 {
		t.Fatalf("len(records) = %d, want 0", len(records))
	}
}
