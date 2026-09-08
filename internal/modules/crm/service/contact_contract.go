package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type ContactService interface {
	Create(context.Context, coretenant.Scope, repository.CreateContactParams) (domain.Contact, error)
	Get(context.Context, coretenant.Scope, string) (domain.Contact, error)
	List(context.Context, coretenant.Scope, repository.ContactListFilter) ([]domain.Contact, int64, error)
	Update(context.Context, coretenant.Scope, string, repository.UpdateContactParams) (domain.Contact, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Restore(context.Context, coretenant.Scope, string, string) error
}
