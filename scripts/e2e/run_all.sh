#!/usr/bin/env bash
# Run all phase-XX e2e scripts in order. Bail on first failure.

set -euo pipefail
cd "$(dirname "$0")"

for f in $(ls 0*.sh | sort); do
  printf "\033[1;33m▶▶ %s ▶▶\033[0m\n" "$f"
  bash "./$f"
done

printf "\033[32m✓ all e2e scripts passed\033[0m\n"
