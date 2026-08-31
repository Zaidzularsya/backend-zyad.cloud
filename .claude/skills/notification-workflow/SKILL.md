---
description: Reading-order checklist untuk kerja di core notification (dispatch email/WhatsApp, outbox pattern, worker). Pakai kalau task menyentuh internal/core/notification atau cmd/worker.
---

# Notification Workflow

Baca dokumen berikut secara berurutan sebelum mengerjakan core notification:

1. `README.md` — konteks repo.
2. `docs/reference-notification.md` — source requirement.
3. `docs/notification-development-tasks.md` — breakdown pekerjaan.
4. `docs/notification-traceability-index.md` — kaitan requirement, config, API, migration, dan status.
5. `docs/migration-guide.md` — sebelum membuat migration notification.

Catatan implementasi: worker (`cmd/worker`) default hanya mendaftarkan email dispatcher — WhatsApp belum
diproses worker sampai provider WhatsApp tersedia. Email pakai SMTP kalau `MAIL_HOST` terisi, otomatis
fallback ke `noop` kalau kosong (untuk local test).
