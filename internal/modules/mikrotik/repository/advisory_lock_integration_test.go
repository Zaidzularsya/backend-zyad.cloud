//go:build integration

package repository_test

import (
	"context"
	"sync"
	"testing"

	"zyad.cloud/internal/modules/mikrotik/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestAdvisoryLockerSerializesSameKeyOnly(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	locker := repository.NewAdvisoryLocker(db)
	const keyA = "mikrotik.setup:00000000-0000-0000-0000-00000000000a"
	const keyB = "mikrotik.setup:00000000-0000-0000-0000-00000000000b"

	// Two concurrent attempts on the same key: exactly one wins.
	var wg sync.WaitGroup
	results := make([]bool, 2)
	unlocks := make([]func(), 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			unlock, ok, err := locker.TryLock(ctx, keyA)
			if err != nil {
				t.Errorf("TryLock: %v", err)
				return
			}
			results[i], unlocks[i] = ok, unlock
		}(i)
	}
	wg.Wait()
	if results[0] == results[1] {
		t.Fatalf("concurrent attempts on one key: ok = %v, want exactly one true", results)
	}
	holder := 0
	if results[1] {
		holder = 1
	}

	// While A is held a second attempt reports "held"; a different router is not blocked.
	if _, ok, err := locker.TryLock(ctx, keyA); err != nil || ok {
		t.Fatalf("second attempt on held key: ok=%v err=%v, want held", ok, err)
	}
	unlockB, ok, err := locker.TryLock(ctx, keyB)
	if err != nil || !ok {
		t.Fatalf("different key: ok=%v err=%v, want acquired", ok, err)
	}
	unlockB()

	// After release the key can be taken again, also with a canceled caller ctx on unlock.
	unlocks[holder]()
	cctx, cancel := context.WithCancel(ctx)
	unlock, ok, err := locker.TryLock(cctx, keyA)
	if err != nil || !ok {
		t.Fatalf("after unlock: ok=%v err=%v, want acquired", ok, err)
	}
	cancel()
	unlock()
	if u, ok, err := locker.TryLock(ctx, keyA); err != nil || !ok {
		t.Fatalf("unlock with canceled ctx did not release: ok=%v err=%v", ok, err)
	} else {
		u()
	}
}
