# Demo

The `cmd/poc` directory contains a self-contained runnable server that demonstrates the most common gorestapi patterns: two resources related by a foreign key, ownership guards, and server-side filters, with JSON logs, traces and metrics. It is a Go module of its own, so that the libraries of its telemetry, e.g. the client of Prometheus, are not dependencies of gorestapi. The walkthrough below follows that code.

## Resources

Two structs are defined — `User` and `Note`. A note belongs to a user through a foreign key. Users have numeric IDs, and notes have UUIDs, so that the URLs of notes do not reveal how many there are.

```go
type User struct {
    ID         uint64
    Name       string `validate:"required,max=100"`
    Email      string `validate:"required,email" crud:"uniq"`
    CreatedAt  time.Time
    CreatedBy  uint64
    ModifiedAt time.Time
    ModifiedBy uint64
}

type Note struct {
    ID         string
    Title      string `validate:"required,max=200"`
    Content    string
    Comment    string
    UserID     uint64 `validate:"required" crud:"idx fk:user on_delete:cascade"`
    CreatedAt  time.Time
    CreatedBy  uint64
    ModifiedAt time.Time
    ModifiedBy uint64
}
```

* The `validate` tags are checked on every create and update, and a failure is returned as `400 VALIDATION_FAILED`.
* `crud:"uniq"` makes the email unique. A second user with the same email is rejected with `409 UNIQUE_VIOLATION`.
* `fk:user on_delete:cascade` makes `UserID` reference a user. A note of a user that does not exist is rejected with `409 FOREIGN_KEY_VIOLATION`, and deleting a user deletes their notes.
* `ID string` makes gocrud generate a UUID for every new note, see [The ID Field](https://github.com/mikogs/gocrud/blob/v1.0.0-rc.2/docs/struct.md#1-the-id-field). The URL of a note is e.g. `/api/notes/0190f5c3-6d1e-7b3a-9c4d-2e5f6a7b8c9d`.
* `CreatedAt`, `CreatedBy`, `ModifiedAt` and `ModifiedBy` are set by gocrud, from the time of the request and the user returned by `UserIDFunc`. Values sent by clients are ignored, and an update never changes `CreatedAt` and `CreatedBy`.

## Database and registry

```go
// SQLite checks foreign keys only when they are enabled for the connection
db, _ := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)")

reg := registry.New(map[string]func() any{
    "users": func() any { return &User{} },
    "notes": func() any { return &Note{} },
}, db, gocrud.Options{Dialect: gocrud.DialectSQLite})

reg.Migrate(context.Background())
```

`Migrate` creates the tables that do not exist yet, the `user` table before the `note` table that references it, and adds the columns of new fields to existing tables. The registry maps URL path segments to their constructors. In production, keep the changes of the tables in migration files instead, see [Versioned migrations](https://github.com/mikogs/gocrud/blob/v1.0.0-rc.2/docs/crud/versioned-migrations.md).

## Hooks

Two hooks shape note behavior.

**`noteComment`** stamps every create and update with a server-controlled `Comment` field that clients cannot override:

```go
noteComment := func(obj any, _ *http.Request) error {
    obj.(*Note).Comment = "Added with API"
    return nil
}
```

**`noteOwner`** rejects update and delete requests from users whose `X-User-ID` header does not match the note's stored `UserID`:

```go
noteOwner := func(obj any, r *http.Request) error {
    note := obj.(*Note)
    headerUserID, _ := strconv.ParseUint(r.Header.Get("X-User-ID"), 10, 64)
    if note.UserID != headerUserID {
        return errors.New("not the note owner")
    }
    return nil
}
```

`AllowUpdate` receives the **stored** record before the request body is applied, so `noteOwner` always checks the original owner rather than a value the client could forge.

## Handler

```go
handler := gorestapi.New(reg, gorestapi.Options{
    UserIDFunc: func(r *http.Request) any {
        id, _ := strconv.ParseUint(r.Header.Get("X-User-ID"), 10, 64)
        return id
    },
    Routes: map[string]gorestapi.Route{
        "users": {},
        "notes": {
            AllowedFilters: []string{"UserID"},
            AllowUpdate:    noteOwner,
            AllowDelete:    noteOwner,
            PreCreate:      noteComment,
            PreUpdate:      noteComment,
            PostRead: func(obj any, _ *http.Request) error {
                obj.(*Note).Comment = "Returned from gocrud"
                return nil
            },
            PostListItem: func(obj any, _ *http.Request) error {
                obj.(*Note).Comment = "Returned from gocrud"
                return nil
            },
            FilterList: func(r *http.Request) gorestapi.FilterSet {
                return gorestapi.FilterSet{
                    Vals: map[string]string{"UserID": r.Header.Get("X-User-ID")},
                }
            },
            FilterRead: func(r *http.Request) gorestapi.FilterSet {
                return gorestapi.FilterSet{
                    Vals: map[string]string{"UserID": r.Header.Get("X-User-ID")},
                }
            },
        },
    },
})
```

`FilterList` and `FilterRead` automatically scope every notes query to the caller's user ID, so a user can only list and read their own notes. They are joined with AND around any client-supplied `filter_val_*` parameters, so a client cannot widen them.

## Mounting

```go
mux := http.NewServeMux()
mux.Handle("/api/", http.StripPrefix("/api", http.HandlerFunc(handler.Serve)))
mux.HandleFunc("/healthz", gorestapi.Liveness)
mux.Handle("/readyz", handler.Readiness(db.PingContext))
mux.Handle("/metrics", metrics)
mux.HandleFunc("/openapi.json", ...) // handler.OpenAPI
mux.HandleFunc("/docs", ...)         // Swagger UI of /openapi.json

srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
log.Fatal(srv.ListenAndServe())
```

The [OpenAPI](handler.md#openapi) document of the API is at `/openapi.json`, and [Swagger UI](https://github.com/swagger-api/swagger-ui) shows it at `http://localhost:8080/docs`, where the requests can be tried out, e.g. of the users. The `X-User-ID` header of the demo, which stands for authentication, is not in the document, so the requests of the notes are better sent with curl, as below.

## Traces, metrics and logs

The logs are JSON, with the IDs of the request, of its trace and of its span in every record of a request, also of the registry, and with a record of every request:

```go
logger := slog.New(gorestapi.LogHandler(slog.NewJSONHandler(os.Stderr, nil)))

queryHook, err := otelhook.New(otelhook.Options{Dialect: gocrud.DialectSQLite})
reg := registry.New(structs, db, gocrud.Options{Dialect: gocrud.DialectSQLite, QueryHook: queryHook}).WithLogger(logger)
handler := gorestapi.New(reg, gorestapi.Options{Logger: logger, AccessLog: true, Routes: routes})
```

`setupTelemetry` in `cmd/poc/telemetry.go` sets the global providers of OpenTelemetry:

* **Metrics** for Prometheus, at `/metrics`: the durations of the requests and of the queries, and the statistics of the connections of the database, see [Traces and metrics](handler.md#traces-and-metrics).
* **Traces**, exported with OTLP over HTTP when `OTEL_EXPORTER_OTLP_ENDPOINT` is set, e.g. to Jaeger, with the spans of the requests, and of their queries as their children. The `traceparent` header of a request makes its span a child of the span of the caller.

## Running

```
cd cmd/poc
DB_PATH=poc.db PORT=8080 go run .
```

| Env var | Default | Description |
|---|---|---|
| `DB_PATH` | `poc.db` | Path to the SQLite database file |
| `PORT` | `8080` | Port the server listens on |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | The OTLP endpoint of the traces, e.g. `http://localhost:4318`, without which they are not exported. The other `OTEL_*` variables of OpenTelemetry are read too, e.g. `OTEL_SERVICE_NAME` |

With Jaeger in docker, the traces of the requests are at `http://localhost:16686`:

```
docker run --rm -p 16686:16686 -p 4318:4318 jaegertracing/jaeger:2.22.0
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 go run .
```

## Trying it out

```bash
# Create a user
curl -s -X PUT http://localhost:8080/api/users/ \
  -H 'Content-Type: application/json' \
  -d '{"Name":"Alice","Email":"alice@example.com"}' | jq

# Create a note owned by user 1, and keep its UUID
NOTE_ID=$(curl -s -X PUT http://localhost:8080/api/notes/ \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 1' \
  -d '{"Title":"Hello","Content":"World","UserID":1}' | jq -r .data.id)

# List notes — only the caller's notes are returned
curl -s http://localhost:8080/api/notes/ \
  -H 'X-User-ID: 1' | jq

# Read a single note (404 if UserID doesn't match)
curl -s http://localhost:8080/api/notes/$NOTE_ID \
  -H 'X-User-ID: 1' | jq

# Update a note (403 if not the owner)
curl -s -X PUT http://localhost:8080/api/notes/$NOTE_ID \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 1' \
  -d '{"Title":"Updated"}' | jq

# Delete a note (403 if not the owner)
curl -s -X DELETE http://localhost:8080/api/notes/$NOTE_ID \
  -H 'X-User-ID: 1' | jq
```

| Request | Behaviour |
|---|---|
| `GET /api/notes/` | Returns only the caller's notes (UserID injected from header) |
| `GET /api/notes/{id}` | Returns 404 if the note's UserID does not match the header, or if `id` is not a UUID |
| `PUT /api/notes/` | Creates a note with a new UUID, returned in `data.id`; `Comment` is stamped by `PreCreate` |
| `PUT /api/notes/{id}` | Updates a note; returns 403 if the caller is not the owner |
| `DELETE /api/notes/{id}` | Deletes a note; returns 403 if the caller is not the owner |
| `PUT /api/users/` with an invalid email | Returns the 400 problem `VALIDATION_FAILED`, with `{"Email": "email"}` in `errors` |
| `PUT /api/users/` with an email of another user | Returns 409 `UNIQUE_VIOLATION` |
| `PUT /api/notes/` with the `UserID` of a user that does not exist | Returns 409 `FOREIGN_KEY_VIOLATION` |
| `DELETE /api/users/1` | Deletes the user and, through the foreign key, their notes |
