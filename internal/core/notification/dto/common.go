package dto

import "zyad.cloud/internal/core/notification/domain"

type VariableItem struct {
	Key         string `json:"key" binding:"required"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Example     any    `json:"example,omitempty"`
}

type NotificationRecipient struct {
	Type        string `json:"type" binding:"required,max=50"`
	UserID      string `json:"user_id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Destination string `json:"destination" binding:"required"`
}

func NewVariableItem(variable domain.NotificationVariable) VariableItem {
	return VariableItem{
		Key:         variable.Key,
		Description: variable.Description,
		Required:    variable.Required,
		Example:     variable.Example,
	}
}

func NewVariableItems(variables []domain.NotificationVariable) []VariableItem {
	items := make([]VariableItem, 0, len(variables))
	for _, variable := range variables {
		items = append(items, NewVariableItem(variable))
	}
	return items
}
