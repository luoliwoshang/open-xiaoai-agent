package im

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	weChatUploadMediaTypeImage = 1
	weChatUploadMediaTypeVideo = 2
	weChatUploadMediaTypeFile  = 3
)

type weChatGetUploadURLResponse struct {
	UploadParam      string `json:"upload_param"`
	ThumbUploadParam string `json:"thumb_upload_param"`
	UploadFullURL    string `json:"upload_full_url"`
}

type weChatUploadedMedia struct {
	DownloadEncryptedParam string
	CiphertextBytes        int64
}

type weChatUploadedAsset struct {
	FileKey        string
	AESKeyHex      string
	PlaintextBytes int64
	Original       weChatUploadedMedia
}

type weChatMediaUploadRequest struct {
	FilePath  string
	FileName  string
	MediaType int
	LogLabel  string
}

func (a *WeChatAdapter) uploadMediaAsset(ctx context.Context, account Account, target Target, req weChatMediaUploadRequest) (weChatUploadedAsset, error) {
	content, err := os.ReadFile(req.FilePath)
	if err != nil {
		return weChatUploadedAsset{}, fmt.Errorf("read %s file: %w", req.LogLabel, err)
	}
	fileKey, err := randomWeChatToken(16)
	if err != nil {
		return weChatUploadedAsset{}, err
	}
	aesKeyRaw := make([]byte, 16)
	if _, err := rand.Read(aesKeyRaw); err != nil {
		return weChatUploadedAsset{}, fmt.Errorf("generate %s aes key: %w", req.LogLabel, err)
	}

	rawMD5 := md5.Sum(content)
	ciphertext, err := encryptWeChatAESECB(content, aesKeyRaw)
	if err != nil {
		return weChatUploadedAsset{}, err
	}

	log.Printf("im wechat %s upload start: account=%s target=%s file=%q raw_bytes=%d", req.LogLabel, account.ID, target.ID, req.FileName, len(content))

	aesKeyHex := hex.EncodeToString(aesKeyRaw)
	uploadMeta, err := a.getUploadURL(ctx, account, target, fileKey, req.MediaType, len(content), hex.EncodeToString(rawMD5[:]), len(ciphertext), aesKeyHex, req.LogLabel)
	if err != nil {
		return weChatUploadedAsset{}, err
	}
	log.Printf("im wechat %s upload url ready: account=%s target=%s file_key=%s official_flow=true", req.LogLabel, account.ID, target.ID, fileKey)

	downloadParam, err := a.uploadMediaCiphertext(ctx, uploadMeta.UploadFullURL, uploadMeta.UploadParam, fileKey, ciphertext, req.LogLabel)
	if err != nil {
		return weChatUploadedAsset{}, err
	}
	log.Printf("im wechat %s upload succeeded: account=%s target=%s file=%q file_key=%s encrypted_bytes=%d official_flow=true", req.LogLabel, account.ID, target.ID, req.FileName, fileKey, len(ciphertext))

	return weChatUploadedAsset{
		FileKey:        fileKey,
		AESKeyHex:      aesKeyHex,
		PlaintextBytes: int64(len(content)),
		Original: weChatUploadedMedia{
			DownloadEncryptedParam: downloadParam,
			CiphertextBytes:        int64(len(ciphertext)),
		},
	}, nil
}

func (a *WeChatAdapter) getUploadURL(ctx context.Context, account Account, target Target, fileKey string, mediaType int, rawSize int, rawMD5 string, cipherSize int, aesKeyHex string, logLabel string) (weChatGetUploadURLResponse, error) {
	log.Printf("im wechat %s upload url request: account=%s target=%s file_key=%s raw_bytes=%d encrypted_bytes=%d official_flow=true", logLabel, account.ID, target.ID, fileKey, rawSize, cipherSize)

	body, err := json.Marshal(map[string]any{
		"filekey":       fileKey,
		"media_type":    mediaType,
		"to_user_id":    target.TargetUserID,
		"rawsize":       rawSize,
		"rawfilemd5":    rawMD5,
		"filesize":      cipherSize,
		"no_need_thumb": true,
		"aeskey":        aesKeyHex,
		"base_info": map[string]any{
			"channel_version": weChatDefaultChannelLabel,
		},
	})
	if err != nil {
		return weChatGetUploadURLResponse{}, fmt.Errorf("encode wechat upload url request: %w", err)
	}

	respBody, err := a.postJSON(ctx, account.BaseURL, "ilink/bot/getuploadurl", account.Token, body)
	if err != nil {
		return weChatGetUploadURLResponse{}, err
	}

	var resp weChatGetUploadURLResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return weChatGetUploadURLResponse{}, fmt.Errorf("decode wechat upload url response: %w", err)
	}
	if strings.TrimSpace(resp.UploadFullURL) == "" && strings.TrimSpace(resp.UploadParam) == "" {
		return weChatGetUploadURLResponse{}, fmt.Errorf("wechat upload url response is missing upload_full_url and upload_param")
	}
	return resp, nil
}

func (a *WeChatAdapter) uploadMediaCiphertext(ctx context.Context, uploadFullURL string, uploadParam string, fileKey string, ciphertext []byte, label string) (string, error) {
	uploadURL := strings.TrimSpace(uploadFullURL)
	if uploadURL == "" {
		uploadURL = buildWeChatCDNUploadURL(a.cdnBaseURL, strings.TrimSpace(uploadParam), fileKey)
	}
	log.Printf("im wechat cdn upload start: kind=%s file_key=%s bytes=%d host=%s", label, fileKey, len(ciphertext), mustWeChatHost(uploadURL))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(ciphertext))
	if err != nil {
		return "", fmt.Errorf("build wechat cdn upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("wechat cdn upload failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read wechat cdn upload response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(resp.Header.Get("x-error-message"))
		if message == "" {
			message = strings.TrimSpace(string(respBody))
		}
		return "", fmt.Errorf("wechat cdn upload failed: status=%d body=%s", resp.StatusCode, message)
	}

	downloadParam := strings.TrimSpace(resp.Header.Get("x-encrypted-param"))
	if downloadParam == "" {
		return "", fmt.Errorf("wechat cdn upload response is missing x-encrypted-param")
	}
	log.Printf("im wechat cdn upload succeeded: kind=%s file_key=%s", label, fileKey)
	return downloadParam, nil
}

func (a *WeChatAdapter) sendMediaItem(ctx context.Context, account Account, target Target, itemType int, itemKey string, itemPayload map[string]any) (string, error) {
	clientID, err := randomWeChatToken(12)
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(map[string]any{
		"msg": map[string]any{
			"from_user_id":  "",
			"to_user_id":    target.TargetUserID,
			"client_id":     clientID,
			"message_type":  2,
			"message_state": 2,
			"item_list": []map[string]any{
				{
					"type":  itemType,
					itemKey: itemPayload,
				},
			},
		},
		"base_info": map[string]any{
			"channel_version": weChatDefaultChannelLabel,
		},
	})
	if err != nil {
		return "", fmt.Errorf("encode wechat media message body: %w", err)
	}

	if _, err := a.postJSON(ctx, account.BaseURL, "ilink/bot/sendmessage", account.Token, body); err != nil {
		return "", err
	}
	return clientID, nil
}

func encodeWeChatMediaAESKey(aesKeyHex string) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(aesKeyHex)))
}

func buildWeChatCDNUploadURL(baseURL string, uploadParam string, fileKey string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = weChatDefaultCDNBaseURL
	}
	return fmt.Sprintf("%s/upload?encrypted_query_param=%s&filekey=%s", baseURL, url.QueryEscape(uploadParam), url.QueryEscape(fileKey))
}

func mustWeChatHost(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}

func encryptWeChatAESECB(plaintext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}
	padded := pkcs7Pad(plaintext, block.BlockSize())
	ciphertext := make([]byte, len(padded))
	for start := 0; start < len(padded); start += block.BlockSize() {
		block.Encrypt(ciphertext[start:start+block.BlockSize()], padded[start:start+block.BlockSize()])
	}
	return ciphertext, nil
}

func pkcs7Pad(plaintext []byte, blockSize int) []byte {
	padding := blockSize - (len(plaintext) % blockSize)
	if padding == 0 {
		padding = blockSize
	}
	result := make([]byte, len(plaintext)+padding)
	copy(result, plaintext)
	for i := len(plaintext); i < len(result); i++ {
		result[i] = byte(padding)
	}
	return result
}
