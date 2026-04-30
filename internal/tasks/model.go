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
	Deliver     bool      `json:"deliver"`
	CreatedAt   time.Time `json:"created_at"`
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
	Version   int        `json:"version"`
	Tasks     []Task     `json:"tasks"`
	Events    []Event    `json:"events"`
	Artifacts []Artifact `json:"artifacts"`
}
