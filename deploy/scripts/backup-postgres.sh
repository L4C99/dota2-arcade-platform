#!/usr/bin/env bash
set -euo pipefail

: "${PGSERVICE:?set a private libpq service name for the Platform database}"
# Require a private, current-user-owned service file; no credential in argv.
service_file="${PGSERVICEFILE:-${HOME}/.pg_service.conf}"
for private_file in "$service_file" "${PGPASSFILE:-$service_file}"; do
  if [[ ! -f "$private_file" || -L "$private_file" || ! -O "$private_file" ]] ||
     (( (8#$(stat -c %a -- "$private_file") & 077) != 0 )); then
    echo 'libpq service/password file must be private and owned by the current user' >&2
    exit 1
  fi
done
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
temporary="$(mktemp -- "${output}.partial.XXXXXXXX")"
trap 'rm -f -- "$temporary"' EXIT
pg_dump --format=custom --no-owner --file="$temporary"
pg_restore --list "$temporary" >/dev/null
ln -- "$temporary" "$output"
rm -- "$temporary"
trap - EXIT
echo "backup verified: $output"
