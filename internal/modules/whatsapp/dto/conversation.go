package dto

import (
	"time"

	"zyad.cloud/internal/modules/whatsapp/domain"
)

type ConversationListQuery struct {
	RelatedEntityType string `form:"related_entity_type"`
	RelatedEntityID   string `form:"related_entity_id" binding:"omitempty,uuid"`
	Assignee          string `form:"assignee" binding:"omitempty,uuid"`
	Status            string `form:"status" binding:"omitempty,oneof=open closed"`
	Search            string `form:"search"`
	Page              int    `form:"page"`
	PerPage           int    `form:"per_page"`
}

type MessageListQuery struct {
	Before string `form:"before" binding:"omitempty,uuid"`
	Limit  int    `form:"limit"`
}

type StartConversationRequest struct {
	SessionID         string `json:"session_id" binding:"omitempty,uuid"`
	RelatedEntityType string `json:"related_entity_type" binding:"required,oneof=lead contact"`
	RelatedEntityID   string `json:"related_entity_id" binding:"required,uuid"`
}

type SendMessageRequest struct {
	Text string `json:"text" binding:"required"`
}

// UpdateConversationRequest: assignee_user_id "" unassigns.
type UpdateConversationRequest struct {
	AssigneeUserID *string `json:"assignee_user_id" binding:"omitempty"`
	Status         *string `json:"status" binding:"omitempty,oneof=open closed"`
}

type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

func BuildMeta(page, perPage int, total int64) PaginationMeta {
	totalPages := 0
	if perPage > 0 {
		totalPages = int((total + int64(perPage) - 1) / int64(perPage))
	}
	return PaginationMeta{Page: page, PerPage: perPage, Total: total, TotalPages: totalPages}
}

type ConversationResponse struct {
	ID                 string     `json:"id"`
	SessionID          string     `json:"session_id"`
	Phone              string     `json:"phone,omitempty"`
	ContactName        string     `json:"contact_name,omitempty"`
	RelatedEntityType  string     `json:"related_entity_type,omitempty"`
	RelatedEntityID    string     `json:"related_entity_id,omitempty"`
	AssigneeUserID     string     `json:"assignee_user_id,omitempty"`
	LastMessageAt      *time.Time `json:"last_message_at,omitempty"`
	LastMessagePreview string     `json:"last_message_preview,omitempty"`
	UnreadCount        int        `json:"unread_count"`
	Status             string     `json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// ConversationFromDomain omits the WAHA chat id; clients use phone.
func ConversationFromDomain(c domain.Conversation) ConversationResponse {
	return ConversationResponse{
		ID:                 c.ID,
		SessionID:          c.SessionID,
		Phone:              c.PhoneNormalized,
		ContactName:        c.ContactName,
		RelatedEntityType:  string(c.RelatedEntityType),
		RelatedEntityID:    c.RelatedEntityID,
		AssigneeUserID:     c.AssigneeUserID,
		LastMessageAt:      c.LastMessageAt,
		LastMessagePreview: c.LastMessagePreview,
		UnreadCount:        c.UnreadCount,
		Status:             string(c.Status),
		CreatedAt:          c.CreatedAt,
		UpdatedAt:          c.UpdatedAt,
	}
}

func ConversationListFromDomain(items []domain.Conversation) []ConversationResponse {
	out := make([]ConversationResponse, 0, len(items))
	for _, item := range items {
		out = append(out, ConversationFromDomain(item))
	}
	return out
}

type MessageResponse struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Direction      string    `json:"direction"`
	Body           string    `json:"body"`
	HasMedia       bool      `json:"has_media"`
	Status         string    `json:"status"`
	Error          string    `json:"error,omitempty"`
	SentByUserID   string    `json:"sent_by_user_id,omitempty"`
	SentAt         time.Time `json:"sent_at"`
}

func MessageFromDomain(m domain.Message) MessageResponse {
	hasMedia, _ := m.Raw["hasMedia"].(bool)
	return MessageResponse{
		ID:             m.ID,
		ConversationID: m.ConversationID,
		Direction:      string(m.Direction),
		Body:           m.Body,
		HasMedia:       hasMedia,
		Status:         string(m.Status),
		Error:          m.Error,
		SentByUserID:   m.SentByUserID,
		SentAt:         m.SentAt,
	}
}

type MessagePageResponse struct {
	Messages   []MessageResponse `json:"messages"`
	NextBefore string            `json:"next_before,omitempty"`
}

func MessagePageFromDomain(messages []domain.Message, nextBefore string) MessagePageResponse {
	out := make([]MessageResponse, 0, len(messages))
	for _, message := range messages {
		out = append(out, MessageFromDomain(message))
	}
	return MessagePageResponse{Messages: out, NextBefore: nextBefore}
}
