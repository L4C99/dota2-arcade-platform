#!/usr/bin/env bash
set -euo pipefail

: "${PGSERVICE:?set a private libpq service name for the Platform database}"
if (( $# != 1 )); then
  echo 'usage: backup-postgres.sh ABSOLUTE_OUTPUT.dump' >&2
  exit 2
fi
output="$1"
if [[ "$output" != /* ]]; then
  echo 'backup output path must be absolute' >&2
  exit 2
fi
if [[ -e "$output" ]]; then
  echo 'backup output already exists; refusing overwrite' >&2
  exit 1
fi
umask 077
mkdir -p -- "$(dirname -- "$output")"
temporary="${output}.partial.$$"
trap 'rm -f -- "$temporary"' EXIT
pg_dump --format=custom --no-owner --file="$temporary"
pg_restore --list "$temporary" >/dev/null
ln -- "$temporary" "$output"
rm -- "$temporary"
trap - EXIT
echo "backup verified: $output"
