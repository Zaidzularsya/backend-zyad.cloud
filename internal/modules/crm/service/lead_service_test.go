package service

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/repository"
)

type fakeOwnerValidator struct {
	members map[string]bool
	checked []string
}

func (f *fakeOwnerValidator) IsActiveMember(_ context.Context, _ coretenant.Scope, userID string) (bool, error) {
	f.checked = append(f.checked, userID)
	return f.members[userID], nil
}

func newLeadServiceFixture() (*leadService, *fakeLeadRepo, *fakeOwnerValidator) {
	repo := &fakeLeadRepo{}
	validator := &fakeOwnerValidator{members: map[string]bool{"member-1": true}}
	svc := NewLeadService(repo, nil, nil, WithLeadOwnerValidator(validator)).(*leadService)
	return svc, repo, validator
}

func TestLeadCreateRejectsOwnerOutsideOrganization(t *testing.T) {
	svc, repo, _ := newLeadServiceFixture()

	_, err := svc.Create(context.Background(), testScope(t), repository.CreateLeadParams{
		ContactName: "Budi", OwnerUserID: "stranger",
	})
	if !errors.Is(err, ErrLeadOwnerNotMember) {
		t.Fatalf("Create() error = %v, want ErrLeadOwnerNotMember", err)
	}
	if repo.lastCreate.ContactName != "" {
		t.Error("repository Create called despite invalid owner")
	}
}

func TestLeadCreateAllowsMemberOwnerAndEmptyOwner(t *testing.T) {
	svc, _, validator := newLeadServiceFixture()

	for _, owner := range []string{"member-1", ""} {
		if _, err := svc.Create(context.Background(), testScope(t), repository.CreateLeadParams{
			ContactName: "Budi", OwnerUserID: owner,
		}); err != nil {
			t.Fatalf("Create(owner=%q) error = %v", owner, err)
		}
	}
	// Owner kosong tidak perlu dicek ke database.
	if len(validator.checked) != 1 {
		t.Errorf("validator checked %v, want only [member-1]", validator.checked)
	}
}

func TestLeadAssignRejectsOwnerOutsideOrganization(t *testing.T) {
	svc, repo, _ := newLeadServiceFixture()

	_, err := svc.Assign(context.Background(), testScope(t), "lead-1", "stranger", "actor")
	if !errors.Is(err, ErrLeadOwnerNotMember) {
		t.Fatalf("Assign() error = %v, want ErrLeadOwnerNotMember", err)
	}
	if repo.assignCalls != 0 {
		t.Error("repository Assign called despite invalid owner")
	}
}

func TestLeadUpdateValidatesAnnualRevenue(t *testing.T) {
	svc, _, _ := newLeadServiceFixture()

	cases := []struct {
		value   string
		wantErr bool
	}{
		{"", false}, // kosongkan nilai
		{"5000", false},
		{"5000.5", false},
		{"5000.55", false},
		{"-1", true},
		{"5000.555", true},
		{"1e6", true},
		{"12345678901234567", true}, // melebihi numeric(18,2)
	}
	for _, tc := range cases {
		value := tc.value
		_, err := svc.Update(context.Background(), testScope(t), "lead-1", repository.UpdateLeadParams{
			AnnualRevenue: &value,
		})
		if gotErr := errors.Is(err, ErrInvalidAnnualRevenue); gotErr != tc.wantErr {
			t.Errorf("Update(annual_revenue=%q) error = %v, wantErr %v", tc.value, err, tc.wantErr)
		}
	}
}
