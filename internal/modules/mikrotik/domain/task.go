package domain

import "time"

// Actions of an isolation task.
const (
	ActionIsolate = "isolate"
	ActionRestore = "restore"
)

// Statuses of an isolation task.
const (
	TaskPending   = "pending"
	TaskRunning   = "running"
	TaskDone      = "done"
	TaskFailed    = "failed"
	TaskCancelled = "cancelled"
)

// Queue priorities: a restore (customer paid) always goes before an isolate.
const (
	PriorityRestore = 100
	PriorityIsolate = 10
)

// IsolationTask is one outbox entry: isolate or restore a single PPP secret.
type IsolationTask struct {
	ID                 string
	OrganizationID     string
	SecretID           string
	Action             string
	Priority           int
	Status             string
	Attempts           int
	NextAttemptAt      time.Time
	LockedUntil        *time.Time
	LastError          string
	LastErrorCode      string
	TenantNotifiedAt   *time.Time
	CustomerNotifiedAt *time.Time
	CreatedByEvent     string
	CreatedAt          time.Time
	StartedAt          *time.Time
	FinishedAt         *time.Time
}

// TaskView is a task joined with the names shown on the dashboard.
type TaskView struct {
	IsolationTask
	RouterID    string
	RouterName  string
	SecretName  string
	AccountID   string
	AccountName string
}
