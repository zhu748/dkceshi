#!/usr/bin/env bash

# goos goarch goarm package-label
DS2API_RELEASE_TARGETS=(
  "linux amd64 - linux_amd64"
  "linux arm64 - linux_arm64"
  "linux arm 7 linux_armv7"
  "darwin amd64 - darwin_amd64"
  "darwin arm64 - darwin_arm64"
  "windows amd64 - windows_amd64"
  "windows arm64 - windows_arm64"
)

if [[ "${BASH_SOURCE[0]}" == "$0" && "${1:-}" == "--json" ]]; then
  printf '%s\n' "${DS2API_RELEASE_TARGETS[@]}" | python3 -c 'import json,sys; print(json.dumps([line.split()[3] for line in sys.stdin if line.strip()]))'
fi
