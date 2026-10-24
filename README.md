# gorestapi

Package `gorestapi` mounts a ready-made JSON REST API on top of a [gocrud](https://github.com/mikogs/gocrud) registry. A single handler covers list, read, create, update, and delete for every route you register — with no code generation and no framework lock-in.

## Documentation

* [Overview](docs/README.md)
* [Demo](docs/demo.md)
* [Handler](docs/handler.md)
* [JWT](docs/jwt.md)

## Development

* `scripts/test.sh` checks the formatting, and runs `go vet` and the tests. The tests start PostgreSQL in docker.
* `scripts/concurcheck.sh` runs the tests with the race detector, and checks that no goroutines are still running after the tests of each package.
* `scripts/lint.sh` runs the static analysis: golangci-lint, with the linters of `.golangci.yml`, which include gosec, and gosec itself, also on the tests.
* `scripts/vulncheck.sh` checks the code and the dependencies for known vulnerabilities with `govulncheck`.

The GitHub workflows run the tests, the static analysis and the vulnerability check on pull requests and on `main`, and Dependabot proposes updates of the dependencies and of the actions every week.
