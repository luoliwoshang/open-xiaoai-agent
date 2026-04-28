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

type preparedCachedMedia struct {
	FilePath string
	FileName string
	MimeType string
	Size     int64
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
	prepared, err := c.storeMedia(req.FileName, req.MimeType, req.Content, "weixin-image", ".img", func(mimeType string) error {
		if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
			return fmt.Errorf("only image uploads are supported, got %q", mimeType)
		}
		return nil
	})
	if err != nil {
		return PreparedImage{}, err
	}
	return PreparedImage(prepared), nil
}

func (c *MediaCache) StoreFile(req FileSendRequest) (PreparedFile, error) {
	if c == nil {
		return PreparedFile{}, fmt.Errorf("media cache is not configured")
	}
	prepared, err := c.storeMedia(req.FileName, req.MimeType, req.Content, "weixin-file", ".bin", nil)
	if err != nil {
		return PreparedFile{}, err
	}
	return PreparedFile(prepared), nil
}

func (c *MediaCache) storeMedia(fileName string, mimeType string, content []byte, defaultPrefix string, defaultExt string, validate func(string) error) (preparedCachedMedia, error) {
	if len(content) == 0 {
		return preparedCachedMedia{}, fmt.Errorf("media content is required")
	}

	fileName = filepath.Base(strings.TrimSpace(fileName))
	if fileName == "." || fileName == string(filepath.Separator) {
		fileName = ""
	}
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		mimeType = http.DetectContentType(content)
	}
	if validate != nil {
		if err := validate(mimeType); err != nil {
			return preparedCachedMedia{}, err
		}
	}

	ext := strings.TrimSpace(filepath.Ext(fileName))
	if ext == "" {
		if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
			ext = exts[0]
		}
	}
	if ext == "" {
		ext = defaultExt
	}

	prefix := sanitizeMediaBaseName(strings.TrimSuffix(fileName, filepath.Ext(fileName)))
	if prefix == "" {
		prefix = defaultPrefix
	}
	token, err := randomMediaToken(8)
	if err != nil {
		return preparedCachedMedia{}, err
	}
	storedName := fmt.Sprintf("%s-%s%s", prefix, token, ext)
	filePath := filepath.Join(c.dir, storedName)
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		return preparedCachedMedia{}, fmt.Errorf("write cached media: %w", err)
	}

	return preparedCachedMedia{
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
