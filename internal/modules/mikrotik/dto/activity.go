package dto

import (
	"time"

	"zyad.cloud/internal/modules/mikrotik/service"
)

// ActivityQuery pages the control center log (limit capped at 100).
type ActivityQuery struct {
	Limit  int `form:"limit"`
	Offset int `form:"offset"`
}

type ActivityResponse struct {
	ID          string    `json:"id"`
	At          time.Time `json:"at"`
	Event       string    `json:"event"`
	ActorUserID string    `json:"actor_user_id"`
	AccountID   string    `json:"account_id"`
	AccountName string    `json:"account_name"`
	SecretName  string    `json:"secret_name"`
	Message     string    `json:"message"`
}

type ActivityMeta struct {
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
	Total  int64 `json:"total"`
}

func ActivityListFromService(items []service.ActivityItem) []ActivityResponse {
	out := make([]ActivityResponse, 0, len(items))
	for _, it := range items {
		out = append(out, ActivityResponse{
			ID: it.ID, At: it.At, Event: it.Event, ActorUserID: it.ActorUserID, AccountID: it.AccountID,
			AccountName: it.AccountName, SecretName: it.SecretName, Message: it.Message,
		})
	}
	return out
}
