package service

import (
	"errors"
	"testing"
	"time"

	"zyad.cloud/internal/modules/receivable/domain"
)

func TestContractServiceValidation(t *testing.T) {
	repo := newFakeContracts()
	c, _ := repo.Create(ctx, scope, contractParamsForTest())
	svc := NewContractService(repo, func() time.Time { return nowWIB })

	yesterday := date("2026-10-04")
	if _, err := svc.SetEndDate(ctx, scope, c.ID, &yesterday, "u1"); !errors.Is(err, ErrInvalidContractInput) {
		t.Fatalf("SetEndDate yesterday err = %v", err)
	}
	today := date("2026-10-05")
	if got, err := svc.SetEndDate(ctx, scope, c.ID, &today, "u1"); err != nil || got.EndDate == nil {
		t.Fatalf("SetEndDate today = %+v err=%v", got, err)
	}
	if _, err := svc.End(ctx, scope, c.ID, today, "  ", "u1"); !errors.Is(err, ErrInvalidContractInput) {
		t.Fatalf("End without reason err = %v", err)
	}
	ended, err := svc.End(ctx, scope, c.ID, today, "Pelanggan berhenti", "u1")
	if err != nil || ended.Status != domain.ContractEnded {
		t.Fatalf("End = %+v err=%v", ended, err)
	}
	if _, err := svc.End(ctx, scope, c.ID, today, "lagi", "u1"); !errors.Is(err, domain.ErrContractNotActive) {
		t.Fatalf("second End err = %v", err)
	}
}
