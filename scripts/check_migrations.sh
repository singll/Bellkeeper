#!/usr/bin/env sh
set -eu

for up in migrations/*.up.sql migrations/*_*.up.sql; do
  [ -e "$up" ] || continue
  down=$(printf '%s' "$up" | sed 's/\.up\.sql$/.down.sql/')
  if [ ! -f "$down" ]; then
    echo "missing down migration for $up" >&2
    exit 1
  fi
done

for down in migrations/*.down.sql migrations/*_*.down.sql; do
  [ -e "$down" ] || continue
  up=$(printf '%s' "$down" | sed 's/\.down\.sql$/.up.sql/')
  if [ ! -f "$up" ]; then
    echo "missing up migration for $down" >&2
    exit 1
  fi
done
