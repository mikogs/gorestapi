# Demo

The `cmd/poc` directory contains a self-contained runnable server that demonstrates the most common gorestapi patterns: two resources related by a foreign key, logins with JWTs, ownership guards, server-side filters, rate limits and CORS, with JSON logs, traces and metrics. It is a Go module of its own, so that the libraries of its telemetry, e.g. the client of Prometheus, are not dependencies of gorestapi. The walkthrough below follows that code.

## Resources

Two structs are defined — `User` and `Note`. A note belongs to a user through a foreign key. Users have numeric IDs, and notes have UUIDs, so that the URLs of notes do not reveal how many there are.

```go
type User struct {
    ID         uint64
    Name       string `validate:"required,max=100"`
    Email      string `validate:"required,email" crud:"uniq"`
    Password   string `validate:"required,min=8,max=72" crud:"pass"`
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
* `crud:"pass"` stores the password as a bcrypt hash, and never returns it, see [Password fields](handler.md#password-fields).
* `fk:user on_delete:cascade` makes `UserID` reference a user, and deleting a user deletes their notes. The server sets it to the user of the token, see [Hooks](#hooks).
* `ID string` makes gocrud generate a UUID for every new note, see [The ID Field](https://github.com/mikogs/gocrud/blob/v1.0.0/docs/struct.md#1-the-id-field). The URL of a note is e.g. `/api/notes/0190f5c3-6d1e-7b3a-9c4d-2e5f6a7b8c9d`.
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

`Migrate` creates the tables that do not exist yet, the `user` table before the `note` table that references it, and adds the columns of new fields to existing tables. The registry maps URL path segments to their constructors. In production, keep the changes of the tables in migration files instead, see [Versioned migrations](https://github.com/mikogs/gocrud/blob/v1.0.0/docs/crud/versioned-migrations.md).

## Logins

`POST /token`, in `cmd/poc/auth.go`, checks the e-mail and the password of a user with `VerifyPasswords` of the registry, and responds with a JWT of [jwtauth](jwt.md), whose subject is the ID of the user, in the shape of a token of OAuth 2:

```go
obj, ok, err := reg.VerifyPasswords(r.Context(), "users",
    registry.Filters{Vals: map[string]string{"Email": credentials.Email}},
    map[string]string{"Password": credentials.Password})
// ...
signed, _, _, expiresAt, err := provider.Issue(audience, strconv.FormatUint(obj.(*User).ID, 10), "member")
```

```json
{"access_token": "eyJhbGciOiJSUzI1NiIs...", "token_type": "Bearer", "expires_in": 3599}
```

* **An unknown e-mail and a wrong password** are the same `401` problem `INVALID_CREDENTIALS`, so that the response does not tell which users exist. `VerifyPasswords` takes as long for an unknown e-mail, so that the time of the response does not tell it either.
* **The logins of each client are limited** to 1 a second, with bursts of 5, by a `RateLimiter` of their own, against guessing the passwords, with the `429` problem `TOO_MANY_REQUESTS` over it.
* **The key of the tokens** is generated at every start, so that the demo needs no secrets, and its tokens are valid until it stops. A real application loads it, e.g. with `jwtauth.ParsePrivateKeyPEM`, or uses the tokens of an identity provider instead, see [Authentication](handler.md#authentication).

The handler reads the token of a request with `Options.Authenticate`, and `userID` returns the user of its claims, for the hooks:

```go
const audience = "gorestapi-poc"

func authenticate(provider *jwtauth.Provider) func(r *http.Request) (any, error) {
    return gorestapi.Bearer(func(_ context.Context, token string) (any, error) {
        claims, ok := provider.Parse(token, audience)
        if !ok {
            return nil, errors.New("invalid token")
        }
        return claims, nil
    })
}

func userID(r *http.Request) uint64 {
    claims, ok := gorestapi.Principal(r.Context()).(*jwtauth.CustomClaims)
    if !ok {
        return 0
    }
    id, _ := strconv.ParseUint(claims.Subject, 10, 64)
    return id
}
```

## Hooks

**`noteSave`** stamps every create and update with a server-controlled `Comment`, and with the user of the token as the `UserID`, so that a client can neither set the comment nor give a note to another user:

```go
noteSave := func(obj any, r *http.Request) error {
    note := obj.(*Note)
    note.Comment = "Added with API"
    note.UserID = userID(r)
    return nil
}
```

**`noteOwner`** rejects the updates and the deletes of other users than the owner of the note, and **`self`** the updates and the deletes of other users than the user:

```go
noteOwner := func(obj any, r *http.Request) error {
    if obj.(*Note).UserID != userID(r) {
        return errors.New("not the note owner")
    }
    return nil
}
```

`AllowUpdate` receives the **stored** record before the request body is applied, so `noteOwner` always checks the original owner rather than a value the client could forge.

## Handler

```go
handler := gorestapi.New(reg, gorestapi.Options{
    UserIDFunc:   func(r *http.Request) any { return userID(r) },
    Authenticate: authenticate(provider),
    // 10 requests a second of each client, with bursts of 20
    RateLimiter: gorestapi.NewRateLimiter(10, 20),
    Routes: map[string]gorestapi.Route{
        // anyone can sign up, without a token
        "users": {Anonymous: true, AllowUpdate: self, AllowDelete: self, FilterList: selfOnly, FilterRead: selfOnly},
        "notes": {
            AllowedFilters: []string{"UserID"},
            AllowUpdate:    noteOwner,
            AllowDelete:    noteOwner,
            PreCreate:      noteSave,
            PreUpdate:      noteSave,
            PostRead: func(obj any, _ *http.Request) error {
                obj.(*Note).Comment = "Returned from gocrud"
                return nil
            },
            PostListItem: func(obj any, _ *http.Request) error {
                obj.(*Note).Comment = "Returned from gocrud"
                return nil
            },
            FilterList: ownNotes,
            FilterRead: ownNotes,
        },
    },
})
```

* **The notes need a token**: a request without one is the `401` problem `UNAUTHORIZED`. The users are `Anonymous`, so that anyone can sign up, but a user can read, change and delete only themselves, with `selfOnly` and `self`, so that the e-mails are not public.
* **`ownNotes`** is the filter of the `UserID` of the user of the token, so a user can only list and read their own notes. `FilterList` and `FilterRead` are joined with AND around any client-supplied `filter_val_*` parameters, so a client cannot widen them.
* **Every client** can send 10 requests a second, with bursts of 20, by its IP, and more are the `429` problem `TOO_MANY_REQUESTS`.

## Mounting

```go
api := cors.New(cors.Options{
    AllowedOrigins: []string{corsOrigin}, // CORS_ORIGIN, http://localhost:3000 by default
    AllowedMethods: []string{http.MethodGet, http.MethodPut, http.MethodDelete},
    AllowedHeaders: []string{"Authorization", "Content-Type", gorestapi.RequestIDHeader},
    ExposedHeaders: []string{gorestapi.RequestIDHeader, "Allow", "Retry-After"},
    MaxAge:         600,
}).Handler(http.StripPrefix("/api", http.HandlerFunc(handler.Serve)))

mux := http.NewServeMux()
mux.Handle("/api/", api)
mux.Handle("/token", tokenHandler(reg, provider, gorestapi.NewRateLimiter(1, 5)))
mux.HandleFunc("/healthz", gorestapi.Liveness)
mux.Handle("/readyz", handler.Readiness(db.PingContext))
mux.Handle("/metrics", metrics)
mux.HandleFunc("/openapi.json", ...) // handler.OpenAPI
mux.HandleFunc("/docs", ...)         // Swagger UI of /openapi.json

srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
log.Fatal(srv.ListenAndServe())
```

[rs/cors](https://github.com/rs/cors) lets a web app of the origin of `CORS_ORIGIN` call the API from a browser, with its token in the `Authorization` header, see [CORS](handler.md#cors).

The [OpenAPI](handler.md#openapi) document of the API is at `/openapi.json`, with the bearer tokens of JWT as its security, and [Swagger UI](https://github.com/swagger-api/swagger-ui) shows it at `http://localhost:8080/docs`, where the requests can be tried out: get a token from `POST /token`, e.g. with curl, as below, and paste it in **Authorize**.

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
| `CORS_ORIGIN` | `http://localhost:3000` | The origin of the web app whose browsers can call the API, see [Mounting](#mounting) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | The OTLP endpoint of the traces, e.g. `http://localhost:4318`, without which they are not exported. The other `OTEL_*` variables of OpenTelemetry are read too, e.g. `OTEL_SERVICE_NAME` |

With Jaeger in docker, the traces of the requests are at `http://localhost:16686`:

```
docker run --rm -p 16686:16686 -p 4318:4318 jaegertracing/jaeger:2.22.0
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 go run .
```

## Trying it out

```bash
# Sign up, without a token
curl -s -X PUT http://localhost:8080/api/users/ \
  -H 'Content-Type: application/json' \
  -d '{"Name":"Alice","Email":"alice@example.com","Password":"correct horse"}' | jq

# Log in, and keep the token
TOKEN=$(curl -s -X POST http://localhost:8080/token \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"correct horse"}' | jq -r .access_token)

# Create a note of the user of the token, and keep its UUID
NOTE_ID=$(curl -s -X PUT http://localhost:8080/api/notes/ \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"Title":"Hello","Content":"World"}' | jq -r .data.id)

# List notes — only the caller's notes are returned
curl -s http://localhost:8080/api/notes/ \
  -H "Authorization: Bearer $TOKEN" | jq

# Read a single note (404 if it is of another user)
curl -s http://localhost:8080/api/notes/$NOTE_ID \
  -H "Authorization: Bearer $TOKEN" | jq

# Update a note (403 if not the owner)
curl -s -X PUT http://localhost:8080/api/notes/$NOTE_ID \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"Title":"Updated"}' | jq

# Delete a note (403 if not the owner)
curl -s -X DELETE http://localhost:8080/api/notes/$NOTE_ID \
  -H "Authorization: Bearer $TOKEN" | jq
```

| Request | Behaviour |
|---|---|
| `GET /api/notes/` without a token | Returns the 401 problem `UNAUTHORIZED`, with `WWW-Authenticate: Bearer` |
| `GET /api/notes/` with an expired or forged token | Returns the 401 problem `UNAUTHORIZED`, with `WWW-Authenticate: Bearer error="invalid_token"` |
| `GET /api/notes/` | Returns only the caller's notes (UserID from the token) |
| `GET /api/notes/{id}` | Returns 404 if the note is of another user, or if `id` is not a UUID |
| `PUT /api/notes/` | Creates a note of the caller with a new UUID, returned in `data.id`; `Comment` and `UserID` are stamped by `PreCreate` |
| `PUT /api/notes/{id}` | Updates a note; returns 403 if the caller is not the owner |
| `DELETE /api/notes/{id}` | Deletes a note; returns 403 if the caller is not the owner |
| `PUT /api/notes/` with a field that `Note` does not have | Returns the 400 problem `VALIDATION_FAILED`, with e.g. `{"Titel": "unknown"}` in `errors` |
| `PUT /api/notes/` without `Content-Type: application/json` | Returns the 415 problem `UNSUPPORTED_MEDIA_TYPE` |
| `PUT /api/users/` with an invalid email | Returns the 400 problem `VALIDATION_FAILED`, with `{"Email": "email"}` in `errors` |
| `PUT /api/users/` with an email of another user | Returns 409 `UNIQUE_VIOLATION` |
| `GET /api/users/` | Returns only the caller, or nothing without a token |
| `DELETE /api/users/{id}` of the caller | Deletes the user and, through the foreign key, their notes |
| `POST /token` with a wrong password, more than 5 times at once | Returns the 429 problem `TOO_MANY_REQUESTS`, with the seconds to wait in `Retry-After` |
