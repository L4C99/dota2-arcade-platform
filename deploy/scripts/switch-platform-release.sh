#!/usr/bin/env bash
# Pointer-only helper. Caller owns backup, migration, restart and health gates.
set -euo pipefail
if (( $# != 2 )) || [[ "$1" != /* || ! "$2" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ || "$2" == *..* ]]; then
  echo 'usage: switch-platform-release.sh ABS_ROOT RELEASE_NAME' >&2
  exit 2
fi
root="$(realpath -e -- "$1")"
target="$(realpath -e -- "$root/releases/$2")"
[[ "$target" == "$root/releases/$2" ]] || { echo 'release must be a real directory inside root/releases' >&2; exit 1; }
test -x "$target/platform-server"
test -f "$target/web/index.html"
test -f "$target/PLATFORM-BUILD.json"
if [[ -e "$root/current" && ! -L "$root/current" ]]; then
  echo 'current must be a symlink' >&2
  exit 1
fi
if [[ -L "$root/current" ]]; then
  old="$(realpath -e -- "$root/current")"
  [[ "$old" == "$root/releases/"* ]] || { echo 'current target is outside releases' >&2; exit 1; }
  if [[ "$old" == "$target" ]]; then exit 0; fi
fi
temporary="$(mktemp -d -- "$root/.switch.XXXXXXXX")"
trap 'rm -f -- "$temporary/current" "$temporary/previous"; rmdir -- "$temporary"' EXIT
if [[ -n "${old:-}" ]]; then
  ln -s -- "$old" "$temporary/previous"
  mv -Tf -- "$temporary/previous" "$root/previous"
fi
ln -s -- "$target" "$temporary/current"
mv -Tf -- "$temporary/current" "$root/current"
echo 'pointer switched; restart Platform and verify local/public health'
