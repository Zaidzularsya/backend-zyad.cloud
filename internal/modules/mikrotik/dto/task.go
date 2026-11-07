package dto

import (
	"time"

	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/service"
)

// TaskListQuery filters the isolation tasks. Status and router_id are
// validated by the handler (400).
type TaskListQuery struct {
	Status   string `form:"status"`
	RouterID string `form:"router_id"`
	Limit    int    `form:"limit"`
	Offset   int    `form:"offset"`
}

type NamedRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TaskResponse is the dashboard row. Router is id and name only: never the
// host, port or credentials. LastError is already scrubbed when stored.
type TaskResponse struct {
	ID            string     `json:"id"`
	Action        string     `json:"action"`
	Status        string     `json:"status"`
	Priority      int        `json:"priority"`
	Attempts      int        `json:"attempts"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	LastErrorCode string     `json:"last_error_code"`
	LastError     string     `json:"last_error"`
	Router        NamedRef   `json:"router"`
	Secret        NamedRef   `json:"secret"`
	Account       NamedRef   `json:"account"`
	CreatedAt     time.Time  `json:"created_at"`
	FinishedAt    *time.Time `json:"finished_at"`
}

func TaskFromView(v domain.TaskView) TaskResponse {
	return TaskResponse{
		ID: v.ID, Action: v.Action, Status: v.Status, Priority: v.Priority, Attempts: v.Attempts,
		NextAttemptAt: v.NextAttemptAt, LastErrorCode: v.LastErrorCode, LastError: v.LastError,
		Router:    NamedRef{ID: v.RouterID, Name: v.RouterName},
		Secret:    NamedRef{ID: v.SecretID, Name: v.SecretName},
		Account:   NamedRef{ID: v.AccountID, Name: v.AccountName},
		CreatedAt: v.CreatedAt, FinishedAt: v.FinishedAt,
	}
}

func TaskListFromView(views []domain.TaskView) []TaskResponse {
	out := make([]TaskResponse, 0, len(views))
	for _, v := range views {
		out = append(out, TaskFromView(v))
	}
	return out
}

type TaskSummaryResponse struct {
	Counts                 map[string]int `json:"counts"`
	OldestPendingRestoreAt *time.Time     `json:"oldest_pending_restore_at"`
}

func TaskSummaryFromService(s service.TaskSummary) TaskSummaryResponse {
	return TaskSummaryResponse{Counts: s.Counts, OldestPendingRestoreAt: s.OldestPendingRestoreAt}
}
