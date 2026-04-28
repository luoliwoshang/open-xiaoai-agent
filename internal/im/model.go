package im

import "time"

const (
	PlatformWeChat = "weixin"
)

type Account struct {
	ID              string    `json:"id"`
	Platform        string    `json:"platform"`
	RemoteAccountID string    `json:"remote_account_id"`
	OwnerUserID     string    `json:"owner_user_id"`
	DisplayName     string    `json:"display_name"`
	BaseURL         string    `json:"base_url"`
	Token           string    `json:"-"`
	LastError       string    `json:"last_error"`
	LastSentAt      time.Time `json:"last_sent_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Target struct {
	ID           string    `json:"id"`
	AccountID    string    `json:"account_id"`
	Name         string    `json:"name"`
	TargetUserID string    `json:"target_user_id"`
	IsDefault    bool      `json:"is_default"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Event struct {
	ID        string    `json:"id"`
	AccountID string    `json:"account_id"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type Snapshot struct {
	Accounts []Account `json:"accounts"`
	Targets  []Target  `json:"targets"`
	Events   []Event   `json:"events"`
}

type DeliveryConfig struct {
	Enabled           bool   `json:"enabled"`
	SelectedAccountID string `json:"selected_account_id"`
	SelectedTargetID  string `json:"selected_target_id"`
}

type DeliveryReceipt struct {
	Account   Account `json:"account"`
	Target    Target  `json:"target"`
	MessageID string  `json:"message_id"`
	Text      string  `json:"text"`
}

type WeChatLoginStart struct {
	SessionKey    string    `json:"session_key"`
	QRRawText     string    `json:"qr_raw_text"`
	QRCodeDataURL string    `json:"qr_code_data_url"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type WeChatLoginCandidate struct {
	RemoteAccountID string `json:"remote_account_id"`
	OwnerUserID     string `json:"owner_user_id"`
	DisplayName     string `json:"display_name"`
	BaseURL         string `json:"base_url"`
}

type WeChatLoginStatus struct {
	Status    string                `json:"status"`
	Message   string                `json:"message"`
	Candidate *WeChatLoginCandidate `json:"candidate,omitempty"`
}
