# Overview

The pages of the documentation are listed in the [README](../README.md#documentation).

**gorestapi** is a Go package that mounts a ready-made JSON REST API on top of a [gocrud](https://github.com/mikogs/gocrud) registry. A single handler covers list, read, create, update, and delete for every route you register — with no code generation and no framework lock-in.

## How it works

1. Define your structs and register them in a gocrud registry.
2. Configure one `Route` per URL path segment.
3. Mount the handler on any `http.ServeMux`.

```go
import (
    "github.com/mikogs/gorestapi"
    "github.com/mikogs/gocrud/pkg/registry"
)

reg := registry.New(structs, db, gocrud.Options{Dialect: gocrud.DialectPostgres})

handler := gorestapi.New(reg, gorestapi.Options{
    Routes: map[string]gorestapi.Route{
        "notes": {},
    },
})

mux := http.NewServeMux()
mux.Handle("/api/", http.StripPrefix("/api", http.HandlerFunc(handler.Serve)))
```

Each registered route automatically exposes:

| Method | URL | Operation |
|---|---|---|
| `GET` | `/{path}/` | List |
| `GET` | `/{path}/{id}` | Read |
| `PUT` | `/{path}/` | Create |
| `PUT` | `/{path}/{id}` | Update |
| `DELETE` | `/{path}/{id}` | Delete |

On top of the operations, the handler gives:

* **Lists** with filters, filter groups, filters on related records, ordering, and pages with offsets or cursors, see [List](handler.md#list--get-path).
* **Rules of the routes**: who can read and change records, server-side filters, e.g. of the signed-in user, fields that the server sets, and structs of their own for each operation, see [Route](handler.md#route), or a layer of your application in front of the registry, see [Business logic](handler.md#business-logic).
* **Errors** as problems of RFC 9457, `application/problem+json`, with a code of each problem, and the failed validation rules of the fields, see [Errors](handler.md#errors).
* **Optimistic locking** of the records with a version field, which a stale update gets as `409 VERSION_CONFLICT`.
* **An OpenAPI document** of the routes, generated from their configuration and their structs, see [OpenAPI](handler.md#openapi).
* **Observability**: request IDs, JSON logs with the IDs of the requests and their traces, an access log, spans and metrics of OpenTelemetry, e.g. for Prometheus at `/metrics`, and probes of liveness and readiness, see [Logging](handler.md#logging), [Traces and metrics](handler.md#traces-and-metrics) and [Health checks](handler.md#health-checks).
* **Versions** of the API, as handlers mounted at their prefixes, see [Versions](handler.md#versions).

## Supported databases

gorestapi delegates all persistence to gocrud, which supports:

| Database | Driver |
|---|---|
| PostgreSQL | `github.com/lib/pq` or `github.com/jackc/pgx/v5/stdlib` |
| SQLite | `modernc.org/sqlite` |

## Installation

```
go get github.com/mikogs/gorestapi
```

It adds gocrud `v1.0.0-rc.2` too. Import the driver of the database yourself, see [Supported databases](#supported-databases).

## Demo

See [Demo](demo.md) for a complete runnable example covering two resources, ownership hooks, server-side filters, logs, traces, metrics, health checks and the OpenAPI document with Swagger UI.

## Handler reference

See [Handler](handler.md) for the full reference of `Route`, `Options`, the endpoints, the responses and the problems of the errors.
