package repository

import (
	"context"
	"time"

	"zyad.cloud/internal/platform/database"
)

// AdvisoryLocker takes a Postgres session advisory lock on a string key across
// processes and instances. The lock lives on a dedicated pooled connection for
// as long as it is held; a dropped connection releases it automatically.
// (Same pattern as the crm module's locker; duplicated so mikrotik does not
// import crm.)
type AdvisoryLocker struct{ db *database.Pool }

func NewAdvisoryLocker(db *database.Pool) *AdvisoryLocker { return &AdvisoryLocker{db: db} }

// TryLock attempts the lock without waiting. ok=false means another holder has
// it. When ok, the caller must call unlock exactly once (safe from defer).
func (l *AdvisoryLocker) TryLock(ctx context.Context, key string) (unlock func(), ok bool, err error) {
	conn, err := l.db.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1, 0))`, key).Scan(&ok); err != nil {
		conn.Release()
		return nil, false, err
	}
	if !ok {
		conn.Release()
		return nil, false, nil
	}
	return func() {
		// The caller's ctx may already be canceled: release with a fresh one.
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(releaseCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, key); err != nil {
			// Cannot confirm the release: close the session so the lock ends with it.
			_ = conn.Conn().Close(releaseCtx)
		}
		conn.Release()
	}, true, nil
}
