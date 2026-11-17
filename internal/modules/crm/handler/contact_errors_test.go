package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/modules/crm/service"
)

type duplicateContactService struct {
	service.ContactService
}

func (duplicateContactService) Create(context.Context, coretenant.Scope, repository.CreateContactParams) (domain.Contact, error) {
	return domain.Contact{}, &service.DuplicateContactError{
		Existing: domain.Contact{ID: "c-old", FirstName: "Budi", LastName: "S", Email: "a@b.com", Phone: "0812"},
		Field:    "email",
	}
}

func TestContactCreate_DuplicateReturns409WithExisting(t *testing.T) {
	r := newTestRouter(t)
	h := NewContactHandler(duplicateContactService{})
	r.POST("/contacts", h.Create)

	w := performJSON(r, http.MethodPost, "/contacts", `{"first_name":"X","email":"A@b.com"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var body struct {
		Code string `json:"code"`
		Data struct {
			Field    string              `json:"field"`
			Existing DuplicateContactRef `json:"existing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "CONTACT_DUPLICATE" || body.Data.Existing.ID != "c-old" || body.Data.Existing.Name != "Budi S" || body.Data.Field != "email" {
		t.Fatalf("body = %s", w.Body.String())
	}
}
