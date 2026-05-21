#!/usr/bin/env bash
#
# check-coverage.sh fails when the total statement coverage in cover.out is
# below the given threshold (default 80).
set -euo pipefail

threshold="${1:-80}"
profile="${2:-cover.out}"

if [ ! -f "$profile" ]; then
  echo "coverage profile ${profile} not found" >&2
  exit 1
fi

total=$(go tool cover -func="$profile" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')
echo "Total coverage: ${total}% (threshold: ${threshold}%)"

awk -v total="$total" -v threshold="$threshold" 'BEGIN {
  if (total + 0 < threshold + 0) {
    print "coverage below threshold"
    exit 1
  }
}'
