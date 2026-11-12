package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/modules/crm/service"
	"zyad.cloud/internal/shared/response"
)

// DuplicateContactRef is the "existing" payload of a CONTACT_DUPLICATE (409)
// response: enough for the UI to offer "use this contact" instead.
type DuplicateContactRef struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

// failContact answers a contact duplicate with 409 + the existing contact;
// every other error goes through corehttp.Fail unchanged.
func failContact(c *gin.Context, err error) {
	var dup *service.DuplicateContactError
	if errors.As(err, &dup) {
		name := dup.Existing.FirstName
		if dup.Existing.LastName != "" {
			name += " " + dup.Existing.LastName
		}
		response.ErrorWithData(c, http.StatusConflict, service.CodeContactDuplicate,
			"Kontak dengan email atau nomor WhatsApp yang sama sudah ada.",
			gin.H{"field": dup.Field, "existing": DuplicateContactRef{
				ID: dup.Existing.ID, Name: name, Email: dup.Existing.Email, Phone: dup.Existing.Phone,
			}})
		return
	}
	corehttp.Fail(c, err)
}
