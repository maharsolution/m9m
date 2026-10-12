#!/usr/bin/env bash
# check_repo_hygiene.sh — fails the build if stray build artefacts,
# browser downloads, or Python venv leaks are present in the working
# tree. Wired into the GHA harbor-image.yml workflow via the
# `make hygiene` Makefile target (added in the same change that
# landed the 2026-10-12 P5 hygiene bugs).
#
# Origin: BUG-2026-10-12-002 / -003 / -004 — `.exe` artefacts,
# `Unconfirmed 467040.crdownload`, `pyvenv.cfg`, and `,env`
# repeatedly turned up in the working tree. This script makes them
# fail-fast so they can't sneak into a commit.

set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

fail=0
note() { printf '  - %s\n' "$*"; }
error() { printf '✗ %s\n' "$*" >&2; fail=1; }
ok()    { printf '✓ %s\n' "$*"; }

# 1. *.exe at the repo root are platform-specific build artefacts
#    that should be in .gitignore. They were never tracked but keep
#    re-appearing on developer machines.
echo "Checking for .exe binaries at repo root..."
exes=$(find . -maxdepth 1 -name '*.exe' -not -name '.' 2>/dev/null || true)
if [[ -n "$exes" ]]; then
  error "found .exe binaries at repo root (should be in .gitignore or bin/):"
  echo "$exes" | while read -r f; do note "$f"; done
else
  ok "no .exe binaries at repo root"
fi

# 2. *.crdownload — Chrome's incomplete-download marker. Should
#    never live in a source tree.
echo "Checking for .crdownload files..."
cr=$(find . -name '*.crdownload' -not -path './.git/*' 2>/dev/null || true)
if [[ -n "$cr" ]]; then
  error "found .crdownload artefacts (browser leak):"
  echo "$cr" | while read -r f; do note "$f"; done
else
  ok "no .crdownload artefacts"
fi

# 3. pyvenv.cfg at the repo root means someone ran `python -m venv .`
#    by mistake. Per-subdir venvs are fine; root leaks are not.
echo "Checking for repo-root pyvenv.cfg..."
if [[ -f pyvenv.cfg ]]; then
  error "pyvenv.cfg found at repo root — bind the venv to a per-project dir (e.g. sync-service/.venv)"
else
  ok "no repo-root pyvenv.cfg"
fi

# 4. ,env — typo'd sibling of .env. Already in .gitignore but worth
#    catching the on-disk artefact.
echo "Checking for the typo'd ',env' file..."
if [[ -f ',env' ]]; then
  error "found ',env' (typo'd .env) at repo root — rotate any secrets it contains then delete"
else
  ok "no ',env' file"
fi

# 5. supervisord.conf should live under deploy/docker/, not the
#    repo root. BUG-2026-10-12-005.
echo "Checking for stray supervisord.conf..."
if [[ -f supervisord.conf && ! -f deploy/docker/supervisord.conf ]]; then
  error "supervisord.conf is at repo root — move it to deploy/docker/"
elif [[ ! -f supervisord.conf && ! -f deploy/docker/supervisord.conf ]]; then
  error "supervisord.conf is missing entirely (m9m-backend container cannot start without it)"
else
  ok "supervisord.conf is at deploy/docker/supervisord.conf"
fi

if [[ $fail -ne 0 ]]; then
  printf '\nHygiene check FAILED — see BUG-2026-10-12-001..005.\n' >&2
  exit 1
fi
printf '\nHygiene check OK.\n'
