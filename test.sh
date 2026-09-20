#!/usr/bin/env bash
# test.sh — runs all unit tests for the headless crawler refactor.
#
# Usage:
#   ./test.sh           # build + vet + all unit tests
#   ./test.sh crawler   # only the refactored crawler package (verbose)
#
# Note: headless browser tests require a Chromium/Chrome binary; tests that
# need one are skipped automatically when it is not available.
set -euo pipefail

cd "$(dirname "$0")"

echo "==> go build ./..."
go build ./...

echo "==> go vet ./..."
go vet ./...

if [[ "${1:-}" == "crawler" ]]; then
	echo "==> go test -v ./pkg/engine/headless/crawler/..."
	go test -v -count=1 ./pkg/engine/headless/crawler/...
	exit 0
fi

echo "==> go test ./pkg/engine/headless/... (refactored packages)"
go test -count=1 ./pkg/engine/headless/...

echo "==> go test ./... (full unit test suite)"
go test -count=1 ./...

echo "==> all tests passed"
