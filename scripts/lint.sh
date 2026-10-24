#!/usr/bin/env bash
# Runs the static analysis of gorestapi: golangci-lint, with the linters of .golangci.yml, which include gosec, and
# gosec itself, with all its rules, also on the tests. govulncheck runs in scripts/vulncheck.sh.
set -euo pipefail

GOLANGCI_LINT_VERSION=v2.15.0
GOSEC_VERSION=v2.30.0

cd "$(dirname "$0")/.."

echo "== golangci-lint"
go run "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$GOLANGCI_LINT_VERSION" run ./...

echo "== gosec"
go run "github.com/securego/gosec/v2/cmd/gosec@$GOSEC_VERSION" -quiet -tests ./...
