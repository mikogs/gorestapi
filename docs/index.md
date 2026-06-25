# Overview

**gorestapi** is a Go package that mounts a ready-made JSON REST API on top of a [gocrud](https://codeberg.org/mikolajgasior/gocrud) service. A single handler covers list, read, create, update, and delete for every route you register — with no code generation and no framework lock-in.

## How it works

1. Define your structs and register them with a gocrud service.
2. Configure one `Route` per URL path segment.
3. Mount the handler on any `http.ServeMux`.

```go
import (
    "codeberg.org/mikolajgasior/gorestapi"
    svccrud "codeberg.org/mikolajgasior/gocrud/pkg/service"
)

svc := svccrud.New(registry, db, gocrud.DialectPostgres)

handler := gorestapi.New(svc, gorestapi.Options{
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
| PostgreSQL | `github.com/lib/pq` |
| SQLite | `modernc.org/sqlite` |

## Installation

```
go get codeberg.org/mikolajgasior/gorestapi
```

## Demo

See [Demo](demo.md) for a complete runnable example covering two resources, ownership hooks, and server-side filters.

## Handler reference

See [Handler](handler.md) for the full `Route`, `Options`, and response format reference.
