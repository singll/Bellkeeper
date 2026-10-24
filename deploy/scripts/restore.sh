#!/bin/sh
# 产品恢复入口。由 spool restore 调用，SPOOL_RESTORE_REF 指向备份产物目录。
set -eu

: "${SPOOL_RESTORE_REF:?SPOOL_RESTORE_REF is required}"

if [ -f "$SPOOL_RESTORE_REF/bellkeeper.dump" ]; then
  PGPASSWORD="${BELLKEEPER_DATABASE_PASSWORD:-}" docker run --rm -i \
    --network host \
    -e PGPASSWORD \
    postgres:16-alpine \
    pg_restore \
      -h "${BELLKEEPER_DATABASE_HOST}" \
      -p "${BELLKEEPER_DATABASE_PORT:-5432}" \
      -U "${BELLKEEPER_DATABASE_USER:-bellkeeper}" \
      -d "${BELLKEEPER_DATABASE_NAME:-bellkeeper}" \
      --clean --if-exists < "$SPOOL_RESTORE_REF/bellkeeper.dump"
fi

if [ -f "$SPOOL_RESTORE_REF/bellkeeper-config.tar.gz" ]; then
  tar xzf "$SPOOL_RESTORE_REF/bellkeeper-config.tar.gz" -C .
fi

echo "[restore] ref=$SPOOL_RESTORE_REF"
