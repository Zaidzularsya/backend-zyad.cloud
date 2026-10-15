package service

import (
	"errors"
	"strings"
	"testing"
)

func TestAccountValidation(t *testing.T) {
	svc := NewAccountService(fakeAccounts{})
	for _, in := range []AccountInput{
		{Name: " "},
		{Name: strings.Repeat("x", 201)},
		{Name: "Budi", Email: "bukan-email"},
		{Name: "Budi", CompanyName: strings.Repeat("x", 201)},
		{Name: "Budi", Phone: strings.Repeat("1", 51)},
		{Name: "Budi", SourceType: "crm_contact"}, // sumber tanpa id
		{Name: "Budi", SourceID: "c1"},            // id tanpa tipe
		{Name: "Budi", SourceType: "bogus", SourceID: "c1"},
	} {
		if _, err := svc.Create(ctx, scope, in, "u1"); !errors.Is(err, ErrInvalidAccount) {
			t.Errorf("%+v: err = %v, want ErrInvalidAccount", in, err)
		}
	}
	acc, err := svc.Create(ctx, scope, AccountInput{Name: "  Budi  ", Email: "budi@example.com"}, "u1")
	if err != nil || acc.Name != "Budi" {
		t.Fatalf("acc = %+v err=%v", acc, err)
	}
	if _, err := svc.Update(ctx, scope, "a1", AccountInput{Name: ""}); !errors.Is(err, ErrInvalidAccount) {
		t.Fatalf("update err = %v", err)
	}
}
