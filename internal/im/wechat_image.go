package im

import (
	"context"
	"log"
	"strings"
)

const weChatDefaultCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"

type weChatUploadedImage struct {
	FileKey   string
	AESKeyHex string
	Original  weChatUploadedMedia
}

func (a *WeChatAdapter) SendImage(ctx context.Context, account Account, target Target, image PreparedImage, caption string) (ImageSendResult, error) {
	log.Printf("im wechat send image start: account=%s target=%s file=%q mime=%s size=%d caption_len=%d", account.ID, target.ID, image.FileName, image.MimeType, image.Size, len(strings.TrimSpace(caption)))

	if strings.TrimSpace(caption) != "" {
		if _, err := a.SendText(ctx, account, target, caption); err != nil {
			return ImageSendResult{}, err
		}
	}

	uploaded, err := a.uploadImage(ctx, account, target, image)
	if err != nil {
		return ImageSendResult{}, err
	}

	imageItem := map[string]any{
		"media": map[string]any{
			"encrypt_query_param": uploaded.Original.DownloadEncryptedParam,
			"aes_key":             encodeWeChatMediaAESKey(uploaded.AESKeyHex),
			"encrypt_type":        1,
		},
		"mid_size": uploaded.Original.CiphertextBytes,
	}

	clientID, err := a.sendMediaItem(ctx, account, target, 2, "image_item", imageItem)
	if err != nil {
		return ImageSendResult{}, err
	}
	log.Printf("im wechat send image succeeded: account=%s target=%s file=%q message_id=%s official_payload=true", account.ID, target.ID, image.FileName, clientID)
	return ImageSendResult{MessageID: clientID}, nil
}

func (a *WeChatAdapter) uploadImage(ctx context.Context, account Account, target Target, image PreparedImage) (weChatUploadedImage, error) {
	uploaded, err := a.uploadMediaAsset(ctx, account, target, weChatMediaUploadRequest{
		FilePath:  image.FilePath,
		FileName:  image.FileName,
		MediaType: weChatUploadMediaTypeImage,
		LogLabel:  "image",
	})
	if err != nil {
		return weChatUploadedImage{}, err
	}

	return weChatUploadedImage{
		FileKey:   uploaded.FileKey,
		AESKeyHex: uploaded.AESKeyHex,
		Original:  uploaded.Original,
	}, nil
}
