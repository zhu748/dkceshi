#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
TARGETS_FILE="${1:-$ROOT_DIR/plans/refactor-line-gate-targets.txt}"
BASELINE_FILE="$ROOT_DIR/tests/refactor-line-baseline.tsv"

DEFAULT_MAX=300
FRONTEND_MAX=500
ENTRY_MAX=120

is_entry_file() {
  case "$1" in
    api/chat-stream.js|\
    internal/js/helpers/stream-tool-sieve.js|\
    webui/src/App.jsx)
      return 0
      ;;
  esac
  return 1
}

is_frontend_file() {
  [[ "$1" == webui/* ]]
}

is_test_file() {
  local file="$1"
  local base
  base="$(basename "$file")"

  [[ "$file" == tests/* ]] && return 0
  [[ "$file" == */tests/* ]] && return 0
  [[ "$file" == */__tests__/* ]] && return 0
  [[ "$base" == *_test.go ]] && return 0
  [[ "$base" == *.test.js ]] && return 0
  [[ "$base" == *.test.jsx ]] && return 0
  [[ "$base" == *.test.ts ]] && return 0
  [[ "$base" == *.test.tsx ]] && return 0
  [[ "$base" == *.test.mjs ]] && return 0
  [[ "$base" == *.test.cjs ]] && return 0

  return 1
}

if [[ -f "$TARGETS_FILE" ]]; then
  targets="$(cat "$TARGETS_FILE")"
elif [[ $# -gt 0 ]]; then
  echo "target list not found: $TARGETS_FILE" >&2
  exit 1
else
  # Include new files without relying on a historical refactor plan. Deleted
  # tracked paths are intentionally absent from the current source inventory.
  targets="$(git -C "$ROOT_DIR" -c core.quotePath=false ls-files --cached --others --exclude-standard -- '*.go' '*.js' '*.mjs' '*.cjs' '*.jsx' '*.ts' '*.tsx' |
    while IFS= read -r file; do
      if [[ -f "$ROOT_DIR/$file" ]]; then
        printf '%s\n' "$file"
      fi
    done)"
fi

missing=0
over=0
checked=0

while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  [[ "${file:0:1}" == "#" ]] && continue

  if is_test_file "$file"; then
    continue
  fi

  checked=$((checked + 1))
  abs="$ROOT_DIR/$file"
  if [[ ! -f "$abs" ]]; then
    echo "MISSING $file"
    missing=$((missing + 1))
    continue
  fi

  lines="$(wc -l < "$abs" | tr -d ' ')"
  limit="$DEFAULT_MAX"
  if is_entry_file "$file"; then
    limit="$ENTRY_MAX"
  elif is_frontend_file "$file"; then
    limit="$FRONTEND_MAX"
  fi

  # Existing large implementations have a frozen ceiling. Splitting a module
  # should lower/remove its baseline; new code must fit the default limits.
  if [[ -f "$BASELINE_FILE" ]]; then
    baseline="$(awk -F '\t' -v path="$file" '$1 == path { print $2; exit }' "$BASELINE_FILE")"
    if [[ -n "$baseline" ]]; then
      if [[ ! "$baseline" =~ ^[0-9]+$ ]]; then
        echo "invalid line baseline for $file: $baseline" >&2
        exit 1
      fi
      limit="$baseline"
    fi
  fi

  if (( lines > limit )); then
    echo "OVER $file lines=$lines limit=$limit"
    over=$((over + 1))
  fi
done <<< "$targets"

echo "checked=$checked missing=$missing over_limit=$over"

if (( checked == 0 || missing > 0 || over > 0 )); then
  exit 1
fi
