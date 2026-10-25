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

See [Demo](demo.md) for a complete runnable example covering two resources, ownership hooks, and server-side filters.

## Handler reference

See [Handler](handler.md) for the full `Route`, `Options`, and response format reference.
