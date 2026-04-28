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

const weChatDefaultCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"

type weChatGetUploadURLResponse struct {
	UploadParam    string `json:"upload_param"`
	ThumbUploadURL string `json:"thumb_upload_param"`
	UploadFullURL  string `json:"upload_full_url"`
}

type weChatUploadedImage struct {
	FileKey                 string
	DownloadEncryptedParam  string
	AESKeyRaw               []byte
	FileSizeCiphertextBytes int64
}

func (a *WeChatAdapter) SendImage(ctx context.Context, account Account, target Target, image PreparedImage, caption string) (ImageSendResult, error) {
	log.Printf(
		"im wechat adapter send image start: account=%s target=%s file=%q mime=%s size=%d caption_len=%d base_url=%s",
		logSafeID(account.RemoteAccountID),
		logSafeID(target.TargetUserID),
		image.FileName,
		image.MimeType,
		image.Size,
		logTextLen(caption),
		logSafeBaseURL(account.BaseURL),
	)
	if strings.TrimSpace(caption) != "" {
		if _, err := a.SendText(ctx, account, target, caption); err != nil {
			log.Printf("im wechat adapter send image failed: account=%s target=%s stage=caption_text err=%v", logSafeID(account.RemoteAccountID), logSafeID(target.TargetUserID), err)
			return ImageSendResult{}, err
		}
	}

	uploaded, err := a.uploadImage(ctx, account, target, image)
	if err != nil {
		log.Printf("im wechat adapter send image failed: account=%s target=%s stage=upload err=%v", logSafeID(account.RemoteAccountID), logSafeID(target.TargetUserID), err)
		return ImageSendResult{}, err
	}

	clientID, err := randomWeChatToken(12)
	if err != nil {
		log.Printf("im wechat adapter send image failed: account=%s target=%s stage=message_id err=%v", logSafeID(account.RemoteAccountID), logSafeID(target.TargetUserID), err)
		return ImageSendResult{}, err
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
					"type": 2,
					"image_item": map[string]any{
						"media": map[string]any{
							"encrypt_query_param": uploaded.DownloadEncryptedParam,
							"aes_key":             base64.StdEncoding.EncodeToString(uploaded.AESKeyRaw),
							"encrypt_type":        1,
						},
						"mid_size": uploaded.FileSizeCiphertextBytes,
					},
				},
			},
		},
		"base_info": map[string]any{
			"channel_version": weChatDefaultChannelLabel,
		},
	})
	if err != nil {
		log.Printf("im wechat adapter send image failed: account=%s target=%s stage=encode err=%v", logSafeID(account.RemoteAccountID), logSafeID(target.TargetUserID), err)
		return ImageSendResult{}, fmt.Errorf("encode wechat image message body: %w", err)
	}

	if _, err := a.postJSON(ctx, account.BaseURL, "ilink/bot/sendmessage", account.Token, body); err != nil {
		log.Printf("im wechat adapter send image failed: account=%s target=%s stage=sendmessage err=%v", logSafeID(account.RemoteAccountID), logSafeID(target.TargetUserID), err)
		return ImageSendResult{}, err
	}
	log.Printf("im wechat adapter send image succeeded: account=%s target=%s file=%q message_id=%s", logSafeID(account.RemoteAccountID), logSafeID(target.TargetUserID), image.FileName, logSafeID(clientID))
	return ImageSendResult{MessageID: clientID}, nil
}

func (a *WeChatAdapter) uploadImage(ctx context.Context, account Account, target Target, image PreparedImage) (weChatUploadedImage, error) {
	log.Printf("im wechat adapter image upload start: account=%s target=%s file=%q size=%d", logSafeID(account.RemoteAccountID), logSafeID(target.TargetUserID), image.FileName, image.Size)
	content, err := os.ReadFile(image.FilePath)
	if err != nil {
		return weChatUploadedImage{}, fmt.Errorf("read image file: %w", err)
	}

	fileKey, err := randomWeChatToken(16)
	if err != nil {
		return weChatUploadedImage{}, err
	}
	aesKeyRaw := make([]byte, 16)
	if _, err := rand.Read(aesKeyRaw); err != nil {
		return weChatUploadedImage{}, fmt.Errorf("generate image aes key: %w", err)
	}

	rawSize := len(content)
	rawMD5 := md5.Sum(content)
	ciphertext, err := encryptWeChatAESECB(content, aesKeyRaw)
	if err != nil {
		return weChatUploadedImage{}, err
	}

	uploadMeta, err := a.getUploadURL(ctx, account, target, fileKey, rawSize, hex.EncodeToString(rawMD5[:]), len(ciphertext), hex.EncodeToString(aesKeyRaw))
	if err != nil {
		return weChatUploadedImage{}, err
	}
	downloadParam, err := a.uploadImageCiphertext(ctx, uploadMeta, fileKey, ciphertext)
	if err != nil {
		return weChatUploadedImage{}, err
	}
	log.Printf("im wechat adapter image upload succeeded: account=%s target=%s file=%q file_key=%s encrypted_bytes=%d", logSafeID(account.RemoteAccountID), logSafeID(target.TargetUserID), image.FileName, logSafeID(fileKey), len(ciphertext))

	return weChatUploadedImage{
		FileKey:                 fileKey,
		DownloadEncryptedParam:  downloadParam,
		AESKeyRaw:               aesKeyRaw,
		FileSizeCiphertextBytes: int64(len(ciphertext)),
	}, nil
}

func (a *WeChatAdapter) getUploadURL(ctx context.Context, account Account, target Target, fileKey string, rawSize int, rawMD5 string, cipherSize int, aesKeyHex string) (weChatGetUploadURLResponse, error) {
	log.Printf("im wechat adapter image upload url request: account=%s target=%s file_key=%s raw_bytes=%d encrypted_bytes=%d", logSafeID(account.RemoteAccountID), logSafeID(target.TargetUserID), logSafeID(fileKey), rawSize, cipherSize)
	body, err := json.Marshal(map[string]any{
		"filekey":       fileKey,
		"media_type":    1,
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
	log.Printf("im wechat adapter image upload url ready: account=%s target=%s file_key=%s", logSafeID(account.RemoteAccountID), logSafeID(target.TargetUserID), logSafeID(fileKey))
	return resp, nil
}

func (a *WeChatAdapter) uploadImageCiphertext(ctx context.Context, uploadMeta weChatGetUploadURLResponse, fileKey string, ciphertext []byte) (string, error) {
	uploadURL := strings.TrimSpace(uploadMeta.UploadFullURL)
	if uploadURL == "" {
		uploadURL = buildWeChatCDNUploadURL(strings.TrimSpace(uploadMeta.UploadParam), fileKey)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(ciphertext))
	if err != nil {
		return "", fmt.Errorf("build wechat cdn upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	log.Printf("im wechat adapter cdn upload start: file_key=%s bytes=%d host=%s", logSafeID(fileKey), len(ciphertext), logSafeBaseURL(uploadURL))

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
		log.Printf("im wechat adapter cdn upload failed: file_key=%s status=%d", logSafeID(fileKey), resp.StatusCode)
		return "", fmt.Errorf("wechat cdn upload failed: status=%d body=%s", resp.StatusCode, message)
	}

	downloadParam := strings.TrimSpace(resp.Header.Get("x-encrypted-param"))
	if downloadParam == "" {
		return "", fmt.Errorf("wechat cdn upload response is missing x-encrypted-param")
	}
	log.Printf("im wechat adapter cdn upload succeeded: file_key=%s", logSafeID(fileKey))
	return downloadParam, nil
}

func buildWeChatCDNUploadURL(uploadParam string, fileKey string) string {
	return fmt.Sprintf("%s/upload?encrypted_query_param=%s&filekey=%s", weChatDefaultCDNBaseURL, url.QueryEscape(uploadParam), url.QueryEscape(fileKey))
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
