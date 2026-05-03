package tasks

import "time"

type State string

const (
	StateAccepted  State = "accepted"
	StateRunning   State = "running"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateCanceled  State = "canceled"
)

type Task struct {
	ID                  string    `json:"id"`
	Plugin              string    `json:"plugin"`
	Kind                string    `json:"kind"`
	Title               string    `json:"title"`
	Input               string    `json:"input"`
	ParentTaskID        string    `json:"parent_task_id,omitempty"`
	State               State     `json:"state"`
	Summary             string    `json:"summary"`
	Result              string    `json:"result"`
	ResultReportPending bool      `json:"result_report_pending"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type Artifact struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	Kind        string    `json:"kind"`
	FileName    string    `json:"file_name"`
	MIMEType    string    `json:"mime_type"`
	StoragePath string    `json:"-"`
	SizeBytes   int64     `json:"size_bytes"`
	CreatedAt   time.Time `json:"created_at"`
}

type ArtifactDeliveryStatus string

const (
	ArtifactDeliveryPending   ArtifactDeliveryStatus = "pending"
	ArtifactDeliveryNoChannel ArtifactDeliveryStatus = "no_channel"
	ArtifactDeliveryDelivered ArtifactDeliveryStatus = "delivered"
	ArtifactDeliveryFailed    ArtifactDeliveryStatus = "failed"
)

// ArtifactDelivery 保存“某个任务产物的单次交付记录”。
//
// 当前阶段的产品边界比较收敛：
// 1. 一个产物只会对应一条交付记录；
// 2. 这条记录最终只绑定一个账号和一个目标；
// 3. 暂时不处理一份产物多次群发或多次独立投递。
type ArtifactDelivery struct {
	ID                string                 `json:"id"`
	TaskID            string                 `json:"task_id"`
	ArtifactID        string                 `json:"artifact_id"`
	AccountID         string                 `json:"account_id"`
	TargetID          string                 `json:"target_id"`
	ChannelLabel      string                 `json:"channel_label"`
	Status            ArtifactDeliveryStatus `json:"status"`
	ProviderMessageID string                 `json:"provider_message_id"`
	LastError         string                 `json:"last_error"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
	DeliveredAt       time.Time              `json:"delivered_at"`
}

// ArtifactDeliveryItem 是给主流程使用的“任务产物 + 交付状态”联合视图。
//
// 任务结果汇报在决定要不要额外提“产物已经发到微信”时，
// 需要同时看到：
// - 产物本身的名称、类型、缓存路径；
// - 这条产物交付记录的当前状态。
type ArtifactDeliveryItem struct {
	Delivery ArtifactDelivery `json:"delivery"`
	Artifact Artifact         `json:"artifact"`
}

type ResultReportItem struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	State   State  `json:"state"`
	Summary string `json:"summary"`
	Result  string `json:"result"`
}

type Event struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type fileState struct {
	Version    int                `json:"version"`
	Tasks      []Task             `json:"tasks"`
	Events     []Event            `json:"events"`
	Artifacts  []Artifact         `json:"artifacts"`
	Deliveries []ArtifactDelivery `json:"deliveries"`
}
