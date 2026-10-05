package service

import (
	"errors"
	"testing"

	"zyad.cloud/internal/modules/receivable/domain"
)

func newSettingsService() (SettingsService, *fakeSettings) {
	store := &fakeSettings{domain.Settings{InvoiceLeadDays: 7, PaymentTermsDays: 7, DefaultChannels: []string{"email"}}}
	members := fakeMembers{byID: map[string]Member{"u-1": {UserID: "u-1", Name: "Sari"}}}
	return NewSettingsService(store, members), store
}

func TestSettingsValidation(t *testing.T) {
	svc, _ := newSettingsService()
	bad := []domain.Settings{
		{InvoiceLeadDays: 61, PaymentTermsDays: 7, DefaultChannels: []string{"email"}},
		{InvoiceLeadDays: -1, PaymentTermsDays: 7, DefaultChannels: []string{"email"}},
		{InvoiceLeadDays: 7, PaymentTermsDays: 91, DefaultChannels: []string{"email"}},
		{InvoiceLeadDays: 7, PaymentTermsDays: -1, DefaultChannels: []string{"email"}},
		{InvoiceLeadDays: 7, PaymentTermsDays: 7, DefaultChannels: []string{"sms"}},
		{InvoiceLeadDays: 7, PaymentTermsDays: 7, DefaultChannels: nil},
		{InvoiceLeadDays: 7, PaymentTermsDays: 7, DefaultChannels: []string{"email"}, DefaultSenderUserID: "stranger"},
	}
	for i, s := range bad {
		if _, err := svc.Update(ctx, scope, s, "u-1"); !errors.Is(err, ErrInvalidSettings) {
			t.Errorf("case %d: err = %v, want ErrInvalidSettings", i, err)
		}
	}
}

func TestSettingsUpdateAndGet(t *testing.T) {
	svc, store := newSettingsService()
	got, err := svc.Update(ctx, scope, domain.Settings{InvoiceLeadDays: 0, PaymentTermsDays: 90, DefaultChannels: []string{"whatsapp", "email", "whatsapp"}, DefaultSenderUserID: "u-1"}, "u-1")
	if err != nil || got.InvoiceLeadDays != 0 || got.PaymentTermsDays != 90 || len(got.DefaultChannels) != 2 || got.DefaultSenderUserID != "u-1" {
		t.Fatalf("got = %+v err=%v", got, err)
	}
	if store.s.PaymentTermsDays != 90 {
		t.Fatalf("not persisted: %+v", store.s)
	}
	if read, err := svc.Get(ctx, scope); err != nil || read.PaymentTermsDays != 90 {
		t.Fatalf("get = %+v err=%v", read, err)
	}
	// pengirim default boleh dikosongkan
	if cleared, err := svc.Update(ctx, scope, domain.Settings{InvoiceLeadDays: 7, PaymentTermsDays: 7, DefaultChannels: []string{"email"}}, "u-1"); err != nil || cleared.DefaultSenderUserID != "" {
		t.Fatalf("cleared = %+v err=%v", cleared, err)
	}
}

func TestSettingsListSenders(t *testing.T) {
	svc, _ := newSettingsService()
	members, err := svc.Senders(ctx, scope)
	if err != nil || len(members) != 1 || members[0].UserID != "u-1" {
		t.Fatalf("members = %+v err=%v", members, err)
	}
}
