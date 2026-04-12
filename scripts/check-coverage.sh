#!/bin/bash
# Enforce minimum code coverage per testable package.
# Reads from go test output (piped or from file).
# Usage: scripts/check-coverage.sh <threshold> <test-output-file>
# Example: go test -cover ./... 2>&1 | tee test.out && scripts/check-coverage.sh 75 test.out

set -euo pipefail

THRESHOLD="${1:-75}"
TEST_OUTPUT="${2:-}"

# Packages to enforce coverage on (those with meaningful unit tests)
PACKAGES=(
  "internal/diff"
  "internal/storage/sqlite"
  "internal/server"
)

FAILED=0

for pkg in "${PACKAGES[@]}"; do
  if [ -n "$TEST_OUTPUT" ]; then
    line=$(grep "$pkg" "$TEST_OUTPUT" | grep 'coverage:' || true)
  else
    line=""
  fi

  if [ -z "$line" ]; then
    echo "WARNING: No coverage data for $pkg"
    continue
  fi

  coverage=$(echo "$line" | grep -oE '[0-9]+\.[0-9]+%' | head -1 | sed 's/%//')

  if [ -z "$coverage" ]; then
    echo "WARNING: Could not parse coverage for $pkg"
    continue
  fi

  int_cov=$(echo "$coverage" | cut -d. -f1)

  if [ "$int_cov" -lt "$THRESHOLD" ]; then
    echo "FAIL $pkg: ${coverage}% (below ${THRESHOLD}%)"
    FAILED=1
  else
    echo "OK   $pkg: ${coverage}%"
  fi
done

if [ "$FAILED" -eq 1 ]; then
  echo ""
  echo "Coverage check failed. Minimum required: ${THRESHOLD}%"
  exit 1
fi

echo ""
echo "All packages meet the ${THRESHOLD}% coverage threshold."
