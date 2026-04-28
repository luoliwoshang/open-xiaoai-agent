package im

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type MediaCache struct {
	dir string
}

func NewMediaCache(dir string) (*MediaCache, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("media cache dir is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create media cache dir: %w", err)
	}
	return &MediaCache{dir: dir}, nil
}

func (c *MediaCache) Dir() string {
	if c == nil {
		return ""
	}
	return c.dir
}

func (c *MediaCache) StoreImage(req ImageSendRequest) (PreparedImage, error) {
	if c == nil {
		return PreparedImage{}, fmt.Errorf("media cache is not configured")
	}

	content := req.Content
	if len(content) == 0 {
		return PreparedImage{}, fmt.Errorf("image content is required")
	}

	fileName := filepath.Base(strings.TrimSpace(req.FileName))
	if fileName == "." || fileName == string(filepath.Separator) {
		fileName = ""
	}
	mimeType := strings.TrimSpace(req.MimeType)
	if mimeType == "" {
		mimeType = http.DetectContentType(content)
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return PreparedImage{}, fmt.Errorf("only image uploads are supported, got %q", mimeType)
	}

	ext := strings.TrimSpace(filepath.Ext(fileName))
	if ext == "" {
		if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
			ext = exts[0]
		}
	}
	if ext == "" {
		ext = ".img"
	}

	prefix := sanitizeMediaBaseName(strings.TrimSuffix(fileName, filepath.Ext(fileName)))
	if prefix == "" {
		prefix = "weixin-image"
	}
	token, err := randomMediaToken(8)
	if err != nil {
		return PreparedImage{}, err
	}
	storedName := fmt.Sprintf("%s-%s%s", prefix, token, ext)
	filePath := filepath.Join(c.dir, storedName)
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		return PreparedImage{}, fmt.Errorf("write cached image: %w", err)
	}

	return PreparedImage{
		FilePath: filePath,
		FileName: chooseMediaFileName(fileName, storedName),
		MimeType: mimeType,
		Size:     int64(len(content)),
	}, nil
}

func chooseMediaFileName(original string, fallback string) string {
	original = strings.TrimSpace(original)
	if original != "" {
		return original
	}
	return fallback
}

func sanitizeMediaBaseName(value string) string {
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

func randomMediaToken(bytesLen int) (string, error) {
	raw := make([]byte, bytesLen)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate media token: %w", err)
	}
	return hex.EncodeToString(raw), nil
}
