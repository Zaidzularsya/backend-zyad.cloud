#!/usr/bin/env bash
# Dijalankan di VPS oleh GitHub Actions (lihat .github/workflows/deploy.yml).
# Artifact sudah diupload ke ~/deploy-tmp-backend/<release>/{dist/,deploy/deploy.sh}
# sebelum script ini dipanggil: bash deploy.sh <release>
set -euo pipefail

RELEASE="$1"
ROOT=/var/www/zyad-cloud-backend
TMP="$HOME/deploy-tmp-backend/$RELEASE"
RELEASE_DIR="$ROOT/releases/$RELEASE"

PREV=""
if [ -L "$ROOT/current" ]; then
  PREV=$(readlink -f "$ROOT/current")
fi

mkdir -p "$RELEASE_DIR"
mv "$TMP"/dist/* "$RELEASE_DIR"/
chmod +x "$RELEASE_DIR"/api "$RELEASE_DIR"/worker "$RELEASE_DIR"/migrate
ln -sf "$ROOT/shared/.env" "$RELEASE_DIR/.env"

cd "$RELEASE_DIR"
./migrate -direction up -dir migrations

ln -sfn "$RELEASE_DIR" "$ROOT/current"
pm2 startOrReload "$ROOT/ecosystem.config.js" --update-env

sleep 3
if ! curl -fsS http://127.0.0.1:8080/healthz >/dev/null; then
  echo "Healthcheck gagal, rollback ke release sebelumnya: $PREV"
  if [ -n "$PREV" ]; then
    ln -sfn "$PREV" "$ROOT/current"
    pm2 startOrReload "$ROOT/ecosystem.config.js" --update-env
  fi
  rm -rf "$TMP"
  exit 1
fi

cd "$ROOT/releases"
ls -1dt */ 2>/dev/null | tail -n +6 | xargs -r rm -rf

rm -rf "$TMP"
echo "Deploy backend $RELEASE OK"
