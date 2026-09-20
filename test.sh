#!/usr/bin/env bash
#
# test.sh — manual unit-test runner for the headless crawler refactor
# (Command-pattern action executors + middleware crawl pipeline).
#
# Usage:
#   ./test.sh            # run the refactored-package unit tests
#   ./test.sh -v         # verbose
#   ./test.sh -race      # run with the race detector
#   ./test.sh -run R     # forward a -run regexp to go test
#   ./test.sh parser     # also run pkg/engine/parser unit tests
#   ./test.sh all        # run the full repository test suite (./...)
#
# Note: the refactored unit tests are browser-free; other packages may
# require a locally installed Chromium when run via "./test.sh all".
set -euo pipefail

cd "$(dirname "$0")"

# Keep build/test caches inside a writable location on sandboxed machines.
: "${GOCACHE:=${TMPDIR:-/tmp}/gocache-katana}"
export GOCACHE
mkdir -p "$GOCACHE"

RACE=""
VERBOSE=""
RUN=""
SCOPE="refactor"

while (($#)); do
	case "$1" in
	-v)
		VERBOSE="-v"
		;;
	-race)
		RACE="-race"
		;;
	-run)
		shift
		RUN="$1"
		;;
	all)
		SCOPE="all"
		;;
	parser)
		SCOPE="parser"
		;;
	-h | --help)
		grep '^#' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "unknown argument: $1" >&2
		exit 2
		;;
	esac
	shift
done

PACKAGES=(
	./pkg/engine/headless/crawler/...
	./pkg/engine/headless/
)
if [[ "$SCOPE" == "parser" ]]; then
	PACKAGES+=(./pkg/engine/parser/)
fi
if [[ "$SCOPE" == "all" ]]; then
	PACKAGES=(./...)
fi

echo "==> go build"
go build ./...

echo "==> go vet"
go vet "${PACKAGES[@]}"

echo "==> go test (scope: $SCOPE, race: ${RACE:-off})"
# shellcheck disable=SC2086
go test -timeout 120s $VERBOSE $RACE ${RUN:+-run "$RUN"} "${PACKAGES[@]}"

echo "==> all unit tests passed"
