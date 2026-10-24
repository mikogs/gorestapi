#!/usr/bin/env bash
# Checks gorestapi, with its tests, for known vulnerabilities with govulncheck. It fails when the code calls a
# vulnerable function, of a dependency or of the standard library of the Go version in use.
set -euo pipefail

GOVULNCHECK_VERSION=v1.9.0

cd "$(dirname "$0")/.."
go run "golang.org/x/vuln/cmd/govulncheck@$GOVULNCHECK_VERSION" -test ./...
