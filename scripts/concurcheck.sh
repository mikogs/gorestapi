#!/usr/bin/env bash
# Runs the tests of gorestapi with the race detector, which needs cgo, and checks that no goroutines are still running
# after the tests of each package, see CheckLeaks of gocrud/pkg/gocrudtest. The tests start PostgreSQL in docker.
set -euo pipefail

export LEAKCHECK=1 CGO_ENABLED=1

cd "$(dirname "$0")/.."
go test -race -count=1 -timeout 20m ./...
