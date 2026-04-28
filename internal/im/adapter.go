package im

import "context"

type TextSendResult struct {
	MessageID string
}

type WeChatLoginResult struct {
	Status          string
	Message         string
	RemoteAccountID string
	OwnerUserID     string
	BaseURL         string
	Token           string
}

type Adapter interface {
	Platform() string
	StartLogin(ctx context.Context) (WeChatLoginStart, error)
	PollLogin(ctx context.Context, sessionKey string) (WeChatLoginResult, error)
	SendText(ctx context.Context, account Account, target Target, text string) (TextSendResult, error)
	SendImage(ctx context.Context, account Account, target Target, image PreparedImage, caption string) (ImageSendResult, error)
	SendFile(ctx context.Context, account Account, target Target, file PreparedFile, caption string) (FileSendResult, error)
}
