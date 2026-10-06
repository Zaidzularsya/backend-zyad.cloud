//go:build integration

package repository_test

import (
	"context"
	"testing"

	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestAdvisoryLockExclusive(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	locker := repository.NewAdvisoryLocker(db)

	unlock, ok, err := locker.TryLock(ctx, "self_serve:ws-test")
	if err != nil || !ok {
		t.Fatalf("first lock: ok=%v err=%v", ok, err)
	}
	if _, ok2, err := locker.TryLock(ctx, "self_serve:ws-test"); err != nil || ok2 {
		t.Fatalf("second lock same key: ok=%v err=%v, want not acquired", ok2, err)
	}
	// kunci lain tidak terpengaruh
	otherUnlock, okOther, err := locker.TryLock(ctx, "self_serve:ws-other")
	if err != nil || !okOther {
		t.Fatalf("other key: ok=%v err=%v", okOther, err)
	}
	otherUnlock()

	unlock()
	again, ok3, err := locker.TryLock(ctx, "self_serve:ws-test")
	if err != nil || !ok3 {
		t.Fatalf("after unlock: ok=%v err=%v", ok3, err)
	}
	again()
}
