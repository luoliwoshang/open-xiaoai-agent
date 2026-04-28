package im

import (
	"context"
	"fmt"
	"log"
	"strings"
)

func (a *WeChatAdapter) SendFile(ctx context.Context, account Account, target Target, file PreparedFile, caption string) (FileSendResult, error) {
	log.Printf("im wechat send file start: account=%s target=%s file=%q mime=%s size=%d caption_len=%d", account.ID, target.ID, file.FileName, file.MimeType, file.Size, len(strings.TrimSpace(caption)))

	if strings.TrimSpace(caption) != "" {
		if _, err := a.SendText(ctx, account, target, caption); err != nil {
			return FileSendResult{}, err
		}
	}

	uploaded, err := a.uploadMediaAsset(ctx, account, target, weChatMediaUploadRequest{
		FilePath:  file.FilePath,
		FileName:  file.FileName,
		MediaType: weChatUploadMediaTypeFile,
		LogLabel:  "file",
	})
	if err != nil {
		return FileSendResult{}, err
	}

	fileItem := map[string]any{
		"media": map[string]any{
			"encrypt_query_param": uploaded.Original.DownloadEncryptedParam,
			"aes_key":             encodeWeChatMediaAESKey(uploaded.AESKeyHex),
			"encrypt_type":        1,
		},
		"file_name": file.FileName,
		"len":       fmt.Sprintf("%d", uploaded.PlaintextBytes),
	}

	clientID, err := a.sendMediaItem(ctx, account, target, 4, "file_item", fileItem)
	if err != nil {
		return FileSendResult{}, err
	}
	log.Printf("im wechat send file succeeded: account=%s target=%s file=%q message_id=%s official_payload=true", account.ID, target.ID, file.FileName, clientID)
	return FileSendResult{MessageID: clientID}, nil
}
