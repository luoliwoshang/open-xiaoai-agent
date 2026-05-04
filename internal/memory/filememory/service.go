package filememory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/llm"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/memory"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/settings"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/storage"
)

type runtimeSettings interface {
	Snapshot() settings.Snapshot
	MemoryStorageDir() string
}

// SessionUpdater 是 filememory 这个默认实现内部依赖的 session 级记忆总结器。
//
// assistant 不需要知道这里是否调了 LLM、是否重写了整个文件；
// 它只负责在 session 结束时把完整 history 交给 memory.Service。
// 而 filememory 则通过这个内部协作者，把“已有长期记忆 + 本次已结束 session”
// 整理成新的完整记忆文本。
type SessionUpdater interface {
	UpdateFromSession(ctx context.Context, memoryKey string, currentMemory string, history []llm.Message) (string, error)
}

// Service 是 Phase 1 的默认文件型记忆实现：
// - 记忆正文保存在本地文件；
// - settings 表里只存目录配置；
// - 记忆更新日志落到 MySQL，供 dashboard 分页查看。
//
// 注意：
// 这里除了实现 memory.Service 之外，还额外承载了
// “记忆文件管理”和“更新日志”的实现者语义。
// 这些能力不属于抽象接口，而是当前本地实现自己的附加能力。
type Service struct {
	db       *sql.DB
	settings runtimeSettings
	updater  SessionUpdater
	seq      uint64
}

func New(dsn string, runtimeSettings runtimeSettings, updater SessionUpdater) (*Service, error) {
	db, err := storage.OpenRuntimeDB(dsn)
	if err != nil {
		return nil, err
	}
	return &Service{
		db:       db,
		settings: runtimeSettings,
		updater:  updater,
	}, nil
}

var _ memory.Service = (*Service)(nil)

func (s *Service) Recall(ctx context.Context, memoryKey string) (string, error) {
	_ = ctx
	file, err := s.GetFile(memoryKey)
	if err != nil {
		return "", err
	}
	return file.Content, nil
}

func (s *Service) UpdateFromSession(ctx context.Context, memoryKey string, history []llm.Message) error {
	memoryKey = strings.TrimSpace(memoryKey)
	if memoryKey == "" {
		return fmt.Errorf("memory key is required")
	}
	if s == nil || s.updater == nil {
		return fmt.Errorf("memory session updater is not configured")
	}

	messages := normalizeMessages(history)
	if len(messages) == 0 {
		return nil
	}
	now := time.Now()

	file, err := s.GetFile(memoryKey)
	if err != nil {
		return err
	}
	before := file.Content
	after, err := s.updater.UpdateFromSession(ctx, memoryKey, before, messages)
	if err != nil {
		return fmt.Errorf("update memory from session: %w", err)
	}
	after = normalizeSavedContent(after)
	if after == before {
		return nil
	}
	if err := os.WriteFile(file.Path, []byte(after), 0o644); err != nil {
		return fmt.Errorf("write memory file: %w", err)
	}
	return s.appendUpdateLog(UpdateLog{
		MemoryKey: memoryKey,
		Source:    SessionSummarySource,
		Messages:  messages,
		Before:    before,
		After:     after,
		CreatedAt: now,
	})
}

func (s *Service) GetFile(memoryKey string) (ManagedFile, error) {
	path, err := s.ensureMemoryFile(memoryKey)
	if err != nil {
		return ManagedFile{}, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return ManagedFile{}, fmt.Errorf("stat memory file: %w", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ManagedFile{}, fmt.Errorf("read memory file: %w", err)
	}
	return ManagedFile{
		MemoryKey: strings.TrimSpace(memoryKey),
		Path:      path,
		Content:   string(raw),
		UpdatedAt: info.ModTime(),
	}, nil
}

// SaveFile 直接覆盖记忆正文文件。
//
// 这是当前默认文件型实现暴露给 dashboard 的管理能力，
// 不属于 memory.Service 抽象接口的一部分。
func (s *Service) SaveFile(memoryKey string, content string, source string) (ManagedFile, error) {
	memoryKey = strings.TrimSpace(memoryKey)
	if memoryKey == "" {
		return ManagedFile{}, fmt.Errorf("memory key is required")
	}
	if strings.TrimSpace(source) == "" {
		source = DashboardManualSource
	}

	file, err := s.GetFile(memoryKey)
	if err != nil {
		return ManagedFile{}, err
	}
	before := file.Content
	after := normalizeSavedContent(content)
	if after == before {
		return file, nil
	}

	if err := os.WriteFile(file.Path, []byte(after), 0o644); err != nil {
		return ManagedFile{}, fmt.Errorf("write memory file: %w", err)
	}
	now := time.Now()
	if err := s.appendUpdateLog(UpdateLog{
		MemoryKey: memoryKey,
		Source:    source,
		Messages:  nil,
		Before:    before,
		After:     after,
		CreatedAt: now,
	}); err != nil {
		return ManagedFile{}, err
	}
	info, err := os.Stat(file.Path)
	if err != nil {
		return ManagedFile{}, fmt.Errorf("stat memory file: %w", err)
	}
	return ManagedFile{
		MemoryKey: memoryKey,
		Path:      file.Path,
		Content:   after,
		UpdatedAt: info.ModTime(),
	}, nil
}

func (s *Service) ListLogs(query ListQuery) (ListPage, error) {
	query, err := NormalizeQuery(query)
	if err != nil {
		return ListPage{}, err
	}
	if s == nil || s.db == nil {
		return ListPage{Page: query.Page, PageSize: query.PageSize}, nil
	}

	whereClause := ""
	args := []any(nil)
	if memoryKey := strings.TrimSpace(query.MemoryKey); memoryKey != "" {
		whereClause = " WHERE memory_key = ?"
		args = append(args, memoryKey)
	}

	var total int
	countQuery := `SELECT COUNT(*) FROM memory_update_logs` + whereClause
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return ListPage{}, fmt.Errorf("count memory update logs: %w", err)
	}

	listArgs := append([]any(nil), args...)
	listArgs = append(listArgs, query.PageSize, (query.Page-1)*query.PageSize)
	rows, err := s.db.Query(
		`SELECT id, memory_key, source, messages_json, before_text, after_text, created_at
		FROM memory_update_logs`+whereClause+`
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?`,
		listArgs...,
	)
	if err != nil {
		return ListPage{}, fmt.Errorf("query memory update logs: %w", err)
	}
	defer rows.Close()

	page := ListPage{
		Page:     query.Page,
		PageSize: query.PageSize,
		Total:    total,
	}
	for rows.Next() {
		var item UpdateLog
		var rawMessages string
		var createdAt int64
		if err := rows.Scan(&item.ID, &item.MemoryKey, &item.Source, &rawMessages, &item.Before, &item.After, &createdAt); err != nil {
			return ListPage{}, fmt.Errorf("scan memory update log row: %w", err)
		}
		item.CreatedAt = storage.TimeFromUnixMillis(createdAt)
		if raw := strings.TrimSpace(rawMessages); raw != "" {
			if err := json.Unmarshal([]byte(raw), &item.Messages); err != nil {
				return ListPage{}, fmt.Errorf("decode memory update messages: %w", err)
			}
		}
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return ListPage{}, fmt.Errorf("iterate memory update logs: %w", err)
	}
	page.HasMore = query.Page*query.PageSize < total
	return page, nil
}

func (s *Service) ensureMemoryFile(memoryKey string) (string, error) {
	memoryKey = strings.TrimSpace(memoryKey)
	if memoryKey == "" {
		return "", fmt.Errorf("memory key is required")
	}
	path, err := s.memoryFilePath(memoryKey)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create memory dir: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat memory file: %w", err)
	}
	if err := os.WriteFile(path, []byte(defaultMemoryContent(memoryKey)), 0o644); err != nil {
		return "", fmt.Errorf("create memory file: %w", err)
	}
	return path, nil
}

func (s *Service) memoryFilePath(memoryKey string) (string, error) {
	dir := storage.DefaultMemoryStorageDir
	if s != nil && s.settings != nil {
		if value := strings.TrimSpace(s.settings.MemoryStorageDir()); value != "" {
			dir = value
		}
	}
	if strings.TrimSpace(dir) == "" {
		dir = storage.DefaultMemoryStorageDir
	}
	if !filepath.IsAbs(dir) {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", fmt.Errorf("resolve memory dir: %w", err)
		}
		dir = abs
	}
	name := sanitizeMemoryKey(memoryKey)
	if !strings.HasSuffix(strings.ToLower(name), ".md") {
		name += ".md"
	}
	return filepath.Join(dir, name), nil
}

func (s *Service) appendUpdateLog(entry UpdateLog) error {
	if s == nil || s.db == nil {
		return nil
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	if entry.ID == "" {
		entry.ID = s.nextID()
	}
	rawMessages := "[]"
	if payload, err := json.Marshal(entry.Messages); err == nil {
		rawMessages = string(payload)
	} else {
		return fmt.Errorf("encode memory update messages: %w", err)
	}

	if _, err := s.db.Exec(
		`INSERT INTO memory_update_logs (id, memory_key, source, messages_json, before_text, after_text, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.ID,
		strings.TrimSpace(entry.MemoryKey),
		strings.TrimSpace(entry.Source),
		rawMessages,
		entry.Before,
		entry.After,
		storage.UnixMillis(entry.CreatedAt),
	); err != nil {
		return fmt.Errorf("insert memory update log: %w", err)
	}
	return nil
}

func (s *Service) nextID() string {
	return fmt.Sprintf("memlog_%d_%d", time.Now().UnixMilli(), atomic.AddUint64(&s.seq, 1))
}

func sanitizeMemoryKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "memory"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	name := strings.Trim(b.String(), "._")
	if name == "" {
		return "memory"
	}
	return name
}

func normalizeMessages(messages []llm.Message) []llm.Message {
	items := make([]llm.Message, 0, len(messages))
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		content := strings.TrimSpace(message.Content)
		if role == "" || content == "" {
			continue
		}
		items = append(items, llm.Message{
			Role:    role,
			Content: content,
		})
	}
	return items
}

func defaultMemoryContent(memoryKey string) string {
	return strings.TrimSpace(fmt.Sprintf(`
# XiaoAiAgent Memory

这是一份可直接编辑的长期记忆文件。

- 记忆键：%s
- 你可以在“长期记忆”部分维护希望系统长期记住的信息。
- 系统会在 session 结束后整理一次会话重点，并按需要更新长期记忆。

## 长期记忆

请在这里补充需要长期保留的背景信息、偏好、固定环境说明、常用服务地址、口令备注（如果你确认这样存储是安全的）等内容。

## 最近一次会话整理

当前还没有整理记录。
`, strings.TrimSpace(memoryKey))) + "\n"
}

func normalizeSavedContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	return content + "\n"
}
