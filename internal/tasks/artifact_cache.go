package tasks

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
)

type artifactCache struct {
	dir string
}

func newArtifactCache(dir string) (*artifactCache, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("task artifact cache dir is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create task artifact cache dir: %w", err)
	}
	return &artifactCache{dir: dir}, nil
}

func (c *artifactCache) put(taskID string, req plugin.PutArtifactRequest, artifactID string, nextID func(string) string) (Artifact, error) {
	if c == nil {
		return Artifact{}, fmt.Errorf("task artifact cache is not configured")
	}
	if req.Reader == nil {
		return Artifact{}, fmt.Errorf("artifact reader is required")
	}
	if closer, ok := req.Reader.(io.Closer); ok {
		defer closer.Close()
	}

	fileName := filepath.Base(strings.TrimSpace(req.Name))
	if fileName == "." || fileName == string(filepath.Separator) {
		fileName = ""
	}
	ext := strings.TrimSpace(filepath.Ext(fileName))
	if ext == "" && strings.TrimSpace(req.MIMEType) != "" {
		if exts, _ := mime.ExtensionsByType(strings.TrimSpace(req.MIMEType)); len(exts) > 0 {
			ext = exts[0]
		}
	}
	if ext == "" {
		ext = ".bin"
	}

	baseName := sanitizeArtifactBase(strings.TrimSuffix(fileName, filepath.Ext(fileName)))
	if baseName == "" {
		baseName = "task-artifact"
	}

	taskDir := filepath.Join(c.dir, sanitizeArtifactBase(taskID))
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return Artifact{}, fmt.Errorf("create task artifact dir: %w", err)
	}

	storedName := fmt.Sprintf("%s-%s%s", baseName, sanitizeArtifactBase(nextID("blob")), ext)
	storagePath := filepath.Join(taskDir, storedName)

	file, err := os.Create(storagePath)
	if err != nil {
		return Artifact{}, fmt.Errorf("create artifact file: %w", err)
	}
	defer file.Close()

	sniffer := &contentSniffer{}
	sizeBytes, err := io.Copy(io.MultiWriter(file, sniffer), req.Reader)
	if err != nil {
		_ = os.Remove(storagePath)
		return Artifact{}, fmt.Errorf("write artifact file: %w", err)
	}
	if req.Size > 0 && req.Size != sizeBytes {
		_ = os.Remove(storagePath)
		return Artifact{}, fmt.Errorf("artifact size mismatch: declared=%d actual=%d", req.Size, sizeBytes)
	}

	mimeType := strings.TrimSpace(req.MIMEType)
	if mimeType == "" {
		mimeType = http.DetectContentType(sniffer.Bytes())
	}

	return Artifact{
		ID:          artifactID,
		TaskID:      taskID,
		Kind:        strings.TrimSpace(req.Kind),
		FileName:    chooseArtifactFileName(fileName, storedName),
		MIMEType:    mimeType,
		StoragePath: storagePath,
		SizeBytes:   sizeBytes,
		Deliver:     false,
	}, nil
}

func (c *artifactCache) reset() error {
	if c == nil || strings.TrimSpace(c.dir) == "" {
		return nil
	}
	if err := os.RemoveAll(c.dir); err != nil {
		return fmt.Errorf("remove task artifact cache dir: %w", err)
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return fmt.Errorf("recreate task artifact cache dir: %w", err)
	}
	return nil
}

type contentSniffer struct {
	buf []byte
}

func (s *contentSniffer) Write(p []byte) (int, error) {
	if len(s.buf) < 512 {
		remaining := 512 - len(s.buf)
		if remaining > len(p) {
			remaining = len(p)
		}
		s.buf = append(s.buf, p[:remaining]...)
	}
	return len(p), nil
}

func (s *contentSniffer) Bytes() []byte {
	return append([]byte(nil), s.buf...)
}

func chooseArtifactFileName(original string, fallback string) string {
	original = strings.TrimSpace(original)
	if original != "" {
		return original
	}
	return fallback
}

func sanitizeArtifactBase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
