package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/shared/phone"
)

// dedupeContactRepo is an in-memory ContactRepository for the duplicate guard.
// FindDuplicate mirrors the SQL: lower(trim(email)) and phone_normalized.
type dedupeContactRepo struct {
	repository.ContactRepository
	contacts []domain.Contact
	created  int
	updated  int
}

func (r *dedupeContactRepo) FindDuplicate(_ context.Context, _ coretenant.Scope, email, phoneNormalized, excludeID string) (domain.Contact, bool, error) {
	for _, c := range r.contacts {
		if c.ID == excludeID || c.DeletedAt != nil {
			continue
		}
		if email != "" && strings.ToLower(strings.TrimSpace(c.Email)) == email {
			return c, true, nil
		}
		if phoneNormalized != "" {
			if p, ok := phone.NormalizeID(c.Phone); ok && p == phoneNormalized {
				return c, true, nil
			}
		}
	}
	return domain.Contact{}, false, nil
}

func (r *dedupeContactRepo) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.Contact, error) {
	for _, c := range r.contacts {
		if c.ID == id {
			return c, nil
		}
	}
	return domain.Contact{}, pgx.ErrNoRows
}

func (r *dedupeContactRepo) Create(_ context.Context, _ coretenant.Scope, p repository.CreateContactParams) (domain.Contact, error) {
	r.created++
	c := domain.Contact{ID: "new", FirstName: p.FirstName, Email: p.Email, Phone: p.Phone}
	r.contacts = append(r.contacts, c)
	return c, nil
}

func (r *dedupeContactRepo) Update(_ context.Context, _ coretenant.Scope, id string, _ repository.UpdateContactParams) (domain.Contact, error) {
	r.updated++
	return domain.Contact{ID: id}, nil
}


func TestNormalizePhone(t *testing.T) {
	for _, raw := range []string{"0812-3456-7890", "+62 812 3456 7890", "6281234567890"} {
		if got := NormalizePhone(raw); got != "6281234567890" {
			t.Errorf("NormalizePhone(%q) = %q, want 6281234567890", raw, got)
		}
	}
	if got := NormalizePhone(""); got != "" {
		t.Errorf("NormalizePhone(\"\") = %q, want empty", got)
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := NormalizeEmail("  A@B.Com "); got != "a@b.com" {
		t.Errorf("NormalizeEmail = %q", got)
	}
}

func TestContactCreate_DuplicateByEmailCaseInsensitive(t *testing.T) {
	repo := &dedupeContactRepo{contacts: []domain.Contact{{ID: "old", FirstName: "Budi", Email: "A@B.com"}}}
	svc := NewContactService(repo)

	_, err := svc.Create(context.Background(), testScope(t), repository.CreateContactParams{FirstName: "X", Email: " a@b.com "})
	var dup *DuplicateContactError
	if !errors.As(err, &dup) {
		t.Fatalf("Create() error = %v, want DuplicateContactError", err)
	}
	if dup.Existing.ID != "old" || dup.Field != "email" {
		t.Fatalf("Existing = %q field %q, want old/email", dup.Existing.ID, dup.Field)
	}
	if repo.created != 0 {
		t.Fatal("duplicate contact must not be created")
	}
}

func TestContactCreate_DuplicateByPhoneFormats(t *testing.T) {
	repo := &dedupeContactRepo{contacts: []domain.Contact{{ID: "old", FirstName: "Budi", Phone: "0812-3456-7890"}}}
	svc := NewContactService(repo)

	_, err := svc.Create(context.Background(), testScope(t), repository.CreateContactParams{FirstName: "X", Phone: "+62 812 3456 7890"})
	var dup *DuplicateContactError
	if !errors.As(err, &dup) || dup.Existing.ID != "old" || dup.Field != "phone" {
		t.Fatalf("Create() error = %v, want phone duplicate of old", err)
	}

	if _, err := svc.Create(context.Background(), testScope(t), repository.CreateContactParams{FirstName: "Y", Phone: "0899 1111 2222", Email: "y@b.com"}); err != nil {
		t.Fatalf("distinct contact rejected: %v", err)
	}
}

func TestContactUpdate_OnlyChecksWhenEmailOrPhoneChanges(t *testing.T) {
	// c1 and c2 are already duplicates (legacy data); c3 is distinct.
	repo := &dedupeContactRepo{contacts: []domain.Contact{
		{ID: "c1", Email: "dup@b.com", Phone: "081234567890"},
		{ID: "c2", Email: "dup@b.com", Phone: "081234567890"},
		{ID: "c3", Email: "other@b.com"},
	}}
	svc := NewContactService(repo)
	ctx := context.Background()

	// Unrelated edit on a legacy duplicate passes.
	if _, err := svc.Update(ctx, testScope(t), "c1", repository.UpdateContactParams{FirstName: strPtr("Budi")}); err != nil {
		t.Fatalf("unrelated edit rejected: %v", err)
	}
	// Re-sending the same email (formatted differently) is not a change.
	if _, err := svc.Update(ctx, testScope(t), "c1", repository.UpdateContactParams{Email: strPtr(" DUP@b.com"), Phone: strPtr("+62 812-3456-7890")}); err != nil {
		t.Fatalf("unchanged email/phone rejected: %v", err)
	}
	// Changing c3's email onto an existing one is rejected.
	_, err := svc.Update(ctx, testScope(t), "c3", repository.UpdateContactParams{Email: strPtr("dup@b.com")})
	var dup *DuplicateContactError
	if !errors.As(err, &dup) {
		t.Fatalf("conflicting edit error = %v, want DuplicateContactError", err)
	}
	if repo.updated != 2 {
		t.Fatalf("updated = %d, want 2", repo.updated)
	}
}
