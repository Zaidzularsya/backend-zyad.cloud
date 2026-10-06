package repository

import (
	"context"
	"time"

	"zyad.cloud/internal/platform/database"
)

// AdvisoryLocker mengunci satu kunci string lintas proses/instance lewat advisory lock Postgres.
// Lock berbasis sesi sehingga memerlukan koneksi khusus selama dipegang; koneksi yang putus
// melepas lock otomatis.
type AdvisoryLocker struct{ db *database.Pool }

func NewAdvisoryLocker(db *database.Pool) *AdvisoryLocker { return &AdvisoryLocker{db: db} }

// TryLock mencoba mengambil lock tanpa menunggu. ok=false bila lock dipegang pihak lain.
// Bila ok, pemanggil wajib memanggil unlock (aman dipanggil dari defer).
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
		// Context pemanggil bisa sudah dibatalkan saat unlock; lepas lock dengan context baru.
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(releaseCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, key); err != nil {
			// Lock tidak bisa dipastikan lepas: buang koneksi supaya sesi (dan lock-nya) berakhir.
			_ = conn.Conn().Close(releaseCtx)
		}
		conn.Release()
	}, true, nil
}
