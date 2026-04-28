package im

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"
)

const (
	weChatFixedBaseURL        = "https://ilinkai.weixin.qq.com"
	weChatDefaultBotType      = "3"
	weChatILinkAppID          = "bot"
	weChatILinkClientVersion  = 131335
	weChatDefaultLoginTTL     = 5 * time.Minute
	weChatDefaultRequestTTL   = 15 * time.Second
	weChatDefaultChannelLabel = "open-xiaoai-agent"
)

type WeChatAdapter struct {
	client *http.Client

	mu     sync.Mutex
	logins map[string]*activeWeChatLogin
}

type activeWeChatLogin struct {
	SessionKey     string
	QRCode         string
	CurrentBaseURL string
	ExpiresAt      time.Time
}

type weChatQRCodeResponse struct {
	QRCode           string `json:"qrcode"`
	QRCodeImgContent string `json:"qrcode_img_content"`
}

type weChatQRStatusResponse struct {
	Status       string `json:"status"`
	BotToken     string `json:"bot_token"`
	ILinkBotID   string `json:"ilink_bot_id"`
	BaseURL      string `json:"baseurl"`
	ILinkUserID  string `json:"ilink_user_id"`
	RedirectHost string `json:"redirect_host"`
	ErrorMessage string `json:"errmsg"`
}

func NewWeChatAdapter() *WeChatAdapter {
	return &WeChatAdapter{
		client: &http.Client{Timeout: weChatDefaultRequestTTL},
		logins: make(map[string]*activeWeChatLogin),
	}
}

func (a *WeChatAdapter) Platform() string {
	return PlatformWeChat
}

func (a *WeChatAdapter) StartLogin(ctx context.Context) (WeChatLoginStart, error) {
	log.Printf("im wechat adapter login start: base_url=%s", logSafeBaseURL(weChatFixedBaseURL))
	body, err := a.get(ctx, weChatFixedBaseURL, "ilink/bot/get_bot_qrcode?bot_type="+weChatDefaultBotType)
	if err != nil {
		log.Printf("im wechat adapter login start failed: err=%v", err)
		return WeChatLoginStart{}, err
	}

	var payload weChatQRCodeResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return WeChatLoginStart{}, fmt.Errorf("decode wechat qr code response: %w", err)
	}
	if strings.TrimSpace(payload.QRCode) == "" || strings.TrimSpace(payload.QRCodeImgContent) == "" {
		return WeChatLoginStart{}, fmt.Errorf("wechat qr code response is incomplete")
	}

	sessionKey, err := randomWeChatToken(12)
	if err != nil {
		return WeChatLoginStart{}, err
	}
	dataURL, err := buildWeChatQRCodeDataURL(payload.QRCodeImgContent)
	if err != nil {
		return WeChatLoginStart{}, err
	}

	login := &activeWeChatLogin{
		SessionKey:     sessionKey,
		QRCode:         payload.QRCode,
		CurrentBaseURL: weChatFixedBaseURL,
		ExpiresAt:      time.Now().Add(weChatDefaultLoginTTL),
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.purgeExpiredLoginsLocked()
	a.logins[sessionKey] = login

	start := WeChatLoginStart{
		SessionKey:    sessionKey,
		QRRawText:     payload.QRCodeImgContent,
		QRCodeDataURL: dataURL,
		ExpiresAt:     login.ExpiresAt,
	}
	log.Printf("im wechat adapter login qr ready: session=%s expires_at=%s", logSafeID(sessionKey), start.ExpiresAt.Format(time.RFC3339))
	return start, nil
}

func (a *WeChatAdapter) PollLogin(ctx context.Context, sessionKey string) (WeChatLoginResult, error) {
	a.mu.Lock()
	a.purgeExpiredLoginsLocked()
	login, ok := a.logins[strings.TrimSpace(sessionKey)]
	if !ok {
		a.mu.Unlock()
		return WeChatLoginResult{Status: "expired", Message: "当前登录二维码已失效，请重新发起扫码。"}, nil
	}
	if time.Now().After(login.ExpiresAt) {
		delete(a.logins, sessionKey)
		a.mu.Unlock()
		return WeChatLoginResult{Status: "expired", Message: "当前登录二维码已过期，请重新发起扫码。"}, nil
	}
	currentBaseURL := login.CurrentBaseURL
	qrCode := login.QRCode
	a.mu.Unlock()

	body, err := a.get(ctx, currentBaseURL, "ilink/bot/get_qrcode_status?qrcode="+url.QueryEscape(qrCode))
	if err != nil {
		log.Printf("im wechat adapter login poll request failed: session=%s base_url=%s err=%v", logSafeID(sessionKey), logSafeBaseURL(currentBaseURL), err)
		return WeChatLoginResult{}, err
	}

	var payload weChatQRStatusResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return WeChatLoginResult{}, fmt.Errorf("decode wechat qr status response: %w", err)
	}

	switch payload.Status {
	case "wait":
		log.Printf("im wechat adapter login poll status: session=%s status=wait", logSafeID(sessionKey))
		return WeChatLoginResult{Status: "pending", Message: "等待扫码。"}, nil
	case "scaned":
		log.Printf("im wechat adapter login poll status: session=%s status=scaned", logSafeID(sessionKey))
		return WeChatLoginResult{Status: "scanned", Message: "已扫码，请在微信里确认授权。"}, nil
	case "scaned_but_redirect":
		if host := strings.TrimSpace(payload.RedirectHost); host != "" {
			a.mu.Lock()
			if current, ok := a.logins[strings.TrimSpace(sessionKey)]; ok {
				current.CurrentBaseURL = "https://" + host
			}
			a.mu.Unlock()
		}
		log.Printf("im wechat adapter login poll status: session=%s status=redirect redirect_host=%s", logSafeID(sessionKey), strings.TrimSpace(payload.RedirectHost))
		return WeChatLoginResult{Status: "scanned", Message: "已扫码，正在切换微信侧节点，请继续确认。"}, nil
	case "expired":
		a.mu.Lock()
		delete(a.logins, strings.TrimSpace(sessionKey))
		a.mu.Unlock()
		log.Printf("im wechat adapter login poll status: session=%s status=expired", logSafeID(sessionKey))
		return WeChatLoginResult{Status: "expired", Message: "二维码已过期，请重新发起扫码。"}, nil
	case "confirmed":
		if strings.TrimSpace(payload.BotToken) == "" || strings.TrimSpace(payload.ILinkBotID) == "" {
			log.Printf("im wechat adapter login poll failed: session=%s reason=incomplete_account_payload", logSafeID(sessionKey))
			return WeChatLoginResult{Status: "failed", Message: "微信登录成功，但返回的账号信息不完整。"}, nil
		}

		a.mu.Lock()
		delete(a.logins, strings.TrimSpace(sessionKey))
		a.mu.Unlock()
		baseURL := strings.TrimSpace(payload.BaseURL)
		if baseURL == "" {
			baseURL = currentBaseURL
		}
		result := WeChatLoginResult{
			Status:          "confirmed",
			Message:         "微信账号登录成功。",
			RemoteAccountID: strings.TrimSpace(payload.ILinkBotID),
			OwnerUserID:     strings.TrimSpace(payload.ILinkUserID),
			BaseURL:         baseURL,
			Token:           strings.TrimSpace(payload.BotToken),
		}
		log.Printf("im wechat adapter login poll status: session=%s status=confirmed account=%s owner=%s base_url=%s", logSafeID(sessionKey), logSafeID(result.RemoteAccountID), logSafeID(result.OwnerUserID), logSafeBaseURL(result.BaseURL))
		return result, nil
	default:
		message := strings.TrimSpace(payload.ErrorMessage)
		if message == "" {
			message = "微信登录状态异常。"
		}
		log.Printf("im wechat adapter login poll failed: session=%s status=%s message=%q", logSafeID(sessionKey), strings.TrimSpace(payload.Status), message)
		return WeChatLoginResult{Status: "failed", Message: message}, nil
	}
}

func (a *WeChatAdapter) SendText(ctx context.Context, account Account, target Target, text string) (TextSendResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return TextSendResult{}, fmt.Errorf("wechat text message is empty")
	}
	log.Printf("im wechat adapter send text start: account=%s target=%s text_len=%d base_url=%s", logSafeID(account.RemoteAccountID), logSafeID(target.TargetUserID), logTextLen(text), logSafeBaseURL(account.BaseURL))
	clientID, err := randomWeChatToken(12)
	if err != nil {
		log.Printf("im wechat adapter send text failed: account=%s target=%s err=%v", logSafeID(account.RemoteAccountID), logSafeID(target.TargetUserID), err)
		return TextSendResult{}, err
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
					"type": 1,
					"text_item": map[string]any{
						"text": text,
					},
				},
			},
		},
		"base_info": map[string]any{
			"channel_version": weChatDefaultChannelLabel,
		},
	})
	if err != nil {
		log.Printf("im wechat adapter send text failed: account=%s target=%s stage=encode err=%v", logSafeID(account.RemoteAccountID), logSafeID(target.TargetUserID), err)
		return TextSendResult{}, fmt.Errorf("encode wechat send message body: %w", err)
	}

	if _, err := a.postJSON(ctx, account.BaseURL, "ilink/bot/sendmessage", account.Token, body); err != nil {
		log.Printf("im wechat adapter send text failed: account=%s target=%s stage=sendmessage err=%v", logSafeID(account.RemoteAccountID), logSafeID(target.TargetUserID), err)
		return TextSendResult{}, err
	}
	log.Printf("im wechat adapter send text succeeded: account=%s target=%s message_id=%s", logSafeID(account.RemoteAccountID), logSafeID(target.TargetUserID), logSafeID(clientID))
	return TextSendResult{MessageID: clientID}, nil
}

func (a *WeChatAdapter) get(ctx context.Context, baseURL string, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinWeChatURL(baseURL, endpoint), nil)
	if err != nil {
		return nil, fmt.Errorf("build wechat get request: %w", err)
	}
	setWeChatCommonHeaders(req.Header)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wechat get request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read wechat get response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("im wechat adapter get failed: endpoint=%s status=%d base_url=%s", endpoint, resp.StatusCode, logSafeBaseURL(baseURL))
		return nil, fmt.Errorf("wechat get request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func (a *WeChatAdapter) postJSON(ctx context.Context, baseURL string, endpoint string, token string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinWeChatURL(baseURL, endpoint), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build wechat post request: %w", err)
	}
	setWeChatCommonHeaders(req.Header)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
	if token = strings.TrimSpace(token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wechat post request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read wechat post response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("im wechat adapter post failed: endpoint=%s status=%d base_url=%s", endpoint, resp.StatusCode, logSafeBaseURL(baseURL))
		return nil, fmt.Errorf("wechat post request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

func setWeChatCommonHeaders(headers http.Header) {
	headers.Set("iLink-App-Id", weChatILinkAppID)
	headers.Set("iLink-App-ClientVersion", fmt.Sprintf("%d", weChatILinkClientVersion))
	headers.Set("X-WECHAT-UIN", randomWeChatUIN())
}

func joinWeChatURL(baseURL string, endpoint string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	endpoint = strings.TrimLeft(strings.TrimSpace(endpoint), "/")
	return baseURL + "/" + endpoint
}

func buildWeChatQRCodeDataURL(rawText string) (string, error) {
	png, err := qrcode.Encode(rawText, qrcode.Medium, 256)
	if err != nil {
		return "", fmt.Errorf("encode wechat qr code image: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

func randomWeChatUIN() string {
	value, err := rand.Int(rand.Reader, big.NewInt(1<<32))
	if err != nil {
		return base64.StdEncoding.EncodeToString([]byte("1"))
	}
	return base64.StdEncoding.EncodeToString([]byte(value.String()))
}

func randomWeChatToken(bytesLen int) (string, error) {
	raw := make([]byte, bytesLen)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func (a *WeChatAdapter) purgeExpiredLoginsLocked() {
	now := time.Now()
	for key, login := range a.logins {
		if now.After(login.ExpiresAt) {
			delete(a.logins, key)
		}
	}
}
