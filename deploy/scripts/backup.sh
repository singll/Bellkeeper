#!/bin/sh
# 产品一致性备份入口。由 spool 在 backup-then-recreate / 不可逆迁移前调用。
# 环境变量：SPOOL_BACKUP_DIR、SPOOL_GENERATION、wiring 注入的数据库凭据。
set -eu

: "${SPOOL_BACKUP_DIR:?SPOOL_BACKUP_DIR is required}"
mkdir -p "$SPOOL_BACKUP_DIR"

if [ -n "${BELLKEEPER_DATABASE_HOST:-}" ]; then
  PGPASSWORD="${BELLKEEPER_DATABASE_PASSWORD:-}" docker run --rm \
    --network host \
    -e PGPASSWORD \
    postgres:16-alpine \
    pg_dump \
      -h "${BELLKEEPER_DATABASE_HOST}" \
      -p "${BELLKEEPER_DATABASE_PORT:-5432}" \
      -U "${BELLKEEPER_DATABASE_USER:-bellkeeper}" \
      -d "${BELLKEEPER_DATABASE_NAME:-bellkeeper}" \
      -Fc > "$SPOOL_BACKUP_DIR/bellkeeper.dump"
fi

if [ -d config ]; then
  tar czf "$SPOOL_BACKUP_DIR/bellkeeper-config.tar.gz" config
fi

echo "[backup] generation=${SPOOL_GENERATION:-unknown} dir=$SPOOL_BACKUP_DIR"
