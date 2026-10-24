# Handler

## Initialization

```go
import (
    "github.com/mikogs/gorestapi"
    "github.com/mikogs/gocrud/pkg/registry"
)

reg := registry.New(structs, dbConn, gocrud.Options{Dialect: gocrud.DialectPostgres})

handler := gorestapi.New(reg, gorestapi.Options{})
```

`New` takes two arguments:

| Argument | Type | Description |
|---|---|---|
| `reg` | `*registry.CRUD` | A registry of the structs, see [Registry](https://github.com/mikogs/gocrud/blob/main/docs/pkgs/registry.md) |
| `options` | `gorestapi.Options` | Configuration for the handler (zero value is valid) |

### Options

```go
type Options struct {
    CORS        cors.CORS
    Routes      map[string]Route
    UserIDFunc  func(r *http.Request) any
    MaxFilters  int
    MaxInValues int
    Timeout     time.Duration
    Logger      *slog.Logger
}
```

| Field | Type | Description |
|---|---|---|
| `CORS` | `cors.CORS` | CORS headers written on every response. Zero value emits no CORS headers. |
| `Routes` | `map[string]Route` | Per-route configuration keyed by URL path segment. Only the paths in the map are served, other paths return `404`. A route with the zero value has all operations enabled and all filters allowed, e.g. `"notes": {}`. |
| `UserIDFunc` | `func(*http.Request) any` | Called on every create and update request to obtain the current user's ID, passed to the registry as `ModifiedBy`: a `uint64`, or a `string` for `CreatedBy` and `ModifiedBy` fields of type `string`, e.g. the subject of a JWT. When `nil`, or when it returns `nil`, `ModifiedBy` is `0` or `""`. When the struct has the `CreatedAt`, `CreatedBy`, `ModifiedAt` and `ModifiedBy` fields, gocrud sets them from this ID and the time of the request, ignores the values sent in the request body, and never changes `CreatedAt` and `CreatedBy` on an update. |
| `MaxFilters` | `int` | The largest number of `filter_val_*` parameters of a list request, otherwise the response is `400` with `{"filters": "max"}`. `0` is `DefaultMaxFilters` (20), and a negative value is no limit. |
| `MaxInValues` | `int` | The largest number of the values of an `in` filter, otherwise the response is `400` with e.g. `{"Status": "max"}`. `0` is `DefaultMaxInValues` (100), and a negative value is no limit. |
| `Timeout` | `time.Duration` | The longest time of a request: its queries are cancelled after it, also in the database, and the response is `500 SERVICE_ERROR`. `0` is `DefaultTimeout` (30 seconds), and a negative value is no timeout. |
| `Logger` | `*slog.Logger` | The logger of the handler, `slog.Default()` when `nil`, see [Logging](#logging). |

The limits keep a request cheap for the database, whatever its filters. Also set a timeout of the queries in PostgreSQL, which applies to all of them, e.g. with `statement_timeout=10000` (milliseconds) in the connection string, which works with both lib/pq and pgx: `postgres://user:pass@host/db?statement_timeout=10000`.

### Route

`Route` maps a URL path segment to a registry key and configures per-operation behavior.

```go
type Route struct {
    RegistryKey    string
    Flags          int64
    AllowedFilters []string

    AllowCreate func(obj any, r *http.Request) error
    AllowUpdate func(obj any, r *http.Request) error
    AllowRead   func(obj any, r *http.Request) error
    AllowDelete func(obj any, r *http.Request) error

    PreCreate func(obj any, r *http.Request) error
    PreUpdate func(obj any, r *http.Request) error

    PostRead     func(obj any, r *http.Request) error
    PostListItem func(obj any, r *http.Request) error

    FilterList func(r *http.Request) FilterSet
    FilterRead func(r *http.Request) FilterSet

    CreateConstructor func() any
    UpdateConstructor func() any
    ReadConstructor   func() any
    ListConstructor   func() any
}
```

**RegistryKey**

When `RegistryKey` is empty the URL path segment is used as the registry key. Set it explicitly when the URL path and the registry key should differ:

```go
Routes: map[string]gorestapi.Route{
    "v1/users": {RegistryKey: "users"},
}
```

**Flags**

`Flags` is a bitmask that controls which operations and features are disabled for a route. Combine multiple flags with `|`. The zero value enables everything, except the `match` operator.

| Constant | Effect |
|---|---|
| `DisableCreate` | `PUT /{path}/` (create) — responds `405` |
| `DisableUpdate` | `PUT /{path}/{id}` (update) — responds `405` |
| `DisableDelete` | `DELETE /{path}/{id}` — responds `405` |
| `DisableRead` | `GET /{path}/{id}` — responds `405` |
| `DisableList` | `GET /{path}/` — responds `405` |
| `DisableFilters` | All `filter_val_*` / `filter_op_*` query parameters are ignored |
| `EnableMatchFilters` | Allows the `match` operator, a regular expression from the client, which can take the database a long time. Without the flag, `match` is a `400` response with e.g. `{"Name": "type"}` |

**AllowedFilters**

`AllowedFilters` is a whitelist of field names that clients may pass as filters. When it is empty, the fields that List returns are allowed (unless `DisableFilters` is set): not the [password fields](#password-fields), and not the fields with a `json:"-"` tag. A client cannot filter by the values that it does not get, as a filter, e.g. with `like`, would reveal them. Fields that are not allowed are silently dropped from the request, also in [filter groups](#list--get-path), e.g. `filter_val_Status~q` is allowed when `Status` is. To allow a field that is not returned, list it. Password fields cannot be used in filters even when they are listed: the response is `400` with `{"Password": "password"}`.

`order` can be a field that List returns, or one of `AllowedFilters`. Another `order` is a `400 VALIDATION_FAILED` response with `{"<order>": "type"}`, the same as for a field that does not exist, so that clients cannot find out which fields are not returned.

Paths through relation fields, e.g. `Customer.Country`, query the tables of related records, so they are allowed only when they are listed, also when `AllowedFilters` is empty. The same paths can be used in `order`:

```go
Routes: map[string]gorestapi.Route{
    "orders":    {AllowedFilters: []string{"Status", "CreatedAt", "Customer.Country", "Customer.Name"}},
    "customers": {},
}
// GET /orders/?filter_val_Customer.Country=PL&order=Customer.Name&order_direction=asc
```

A path must not let clients find records by values that they cannot read, e.g. by the email of a customer whose record they cannot see. So a listed path can be used only when clients can read its value through a route of the related records, here `customers`. For each relation field of the path, there has to be a route of the related struct, found by its registered struct, that:

* returns the next field of the path, or lists it in its own `AllowedFilters`, e.g. `Customer.Email` needs a route of customers that returns `Email`, or lists it, and
* shows the same records to every client: it has no `FilterList`, `FilterRead` and `AllowRead`, which a path cannot apply to the related records, and it does not have the flags `DisableList` or `DisableFilters`.

A listed path that cannot be used is treated like a field that is not allowed: its filters are ignored, and an `order` by it is a `400` response. The handler logs a warning with the reason once, e.g. `reason="route customers has FilterList"`. For a path to records that are only for some clients, e.g. the owner of a note, filter by the key field instead, e.g. `UserID`, or add a route of the related struct for the records that every client can read.

```go
Routes: map[string]gorestapi.Route{
    "notes": {AllowedFilters: []string{"UserID", "Status"}},
}
```

**Authorization hooks — Allow\***

Each `Allow*` hook is called with the loaded object and the request. Return a non-nil error to reject the operation with `403 Forbidden`.

| Field | When called | Object passed |
|---|---|---|
| `AllowCreate` | After the request body is unmarshalled, before saving | The new object populated from the request |
| `AllowUpdate` | After the existing record is loaded, **before** the request body is applied | The **stored** record (original state) |
| `AllowRead` | After the record is loaded | The loaded record |
| `AllowDelete` | After the record is loaded, before deletion | The loaded record |

`AllowUpdate` receives the record as stored in the database so it reflects the original owner or state — the incoming request values have not been applied yet.

```go
noteOwner := func(obj any, r *http.Request) error {
    note := obj.(*Note)
    headerUserID, _ := strconv.ParseUint(r.Header.Get("X-User-ID"), 10, 64)
    if note.UserID != headerUserID {
        return errors.New("not the note owner")
    }
    return nil
}

Routes: map[string]gorestapi.Route{
    "notes": {
        AllowUpdate: noteOwner,
        AllowDelete: noteOwner,
    },
}
```

**Server-side filters — FilterList / FilterRead**

These hooks add server-controlled filters to queries, e.g. that a record belongs to the signed-in user. They are joined with AND around all the filters of the client, so the client cannot get other records: a client filter on the same field only narrows the result, e.g. to nothing, and a [filter group](#list--get-path) joined with OR stays inside them.

`FilterList` is applied to every list request (`GET /{path}/`), also with `DisableFilters`. `FilterRead` is applied when reading a single record by ID (`GET /{path}/{id}`); when set, the handler uses `List(limit=1)` with these filters and the ID, so mismatches return `404` rather than revealing record existence.

```go
type FilterSet struct {
    Vals map[string]string // field → value
    Ops  map[string]string // field → operator (defaults to "eq" when absent)
}
```

```go
Routes: map[string]gorestapi.Route{
    "notes": {
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
}
```

**Pre hooks — PreCreate / PreUpdate**

Called with the fully-prepared object just before it is written to the database. The request body has already been applied, and the ID is the one from the URL for an update, and `0` for a create. Use these to stamp server-controlled fields that should not be settable by the client.

Return a non-nil error to abort the operation with `500 Internal Server Error`.

```go
Routes: map[string]gorestapi.Route{
    "notes": {
        PreCreate: func(obj any, _ *http.Request) error {
            obj.(*Note).Status = "pending"
            return nil
        },
    },
}
```

**Post hooks — PostRead / PostListItem**

Called on loaded objects just before they are serialised to JSON. Use these to set or transform fields in the response that should differ from the stored values.

| Field | Fires on |
|---|---|
| `PostRead` | A single-record read (`GET /{path}/{id}`) |
| `PostListItem` | Each item in a list response (`GET /{path}/`) |

Return a non-nil error to abort with `500 Internal Server Error`. For `PostListItem` an error aborts the entire list response.

```go
Routes: map[string]gorestapi.Route{
    "notes": {
        PostRead: func(obj any, _ *http.Request) error {
            obj.(*Note).Comment = "Returned from API"
            return nil
        },
        PostListItem: func(obj any, _ *http.Request) error {
            obj.(*Note).Comment = "Returned from API"
            return nil
        },
    },
}
```

**Constructor overrides**

By default each operation uses the constructor registered for the key in the registry. Setting a constructor here overrides that for the specific operation, allowing a different struct type — for example a create-only struct with fewer fields, so that clients cannot set the other fields when they create a record. The audit fields do not need it, as gocrud ignores the values sent for them.

| Field | Type | Description |
|---|---|---|
| `CreateConstructor` | `func() any` | Constructor for `PUT /{path}/` (create) |
| `UpdateConstructor` | `func() any` | Constructor for `PUT /{path}/{id}` (update). The existing record is loaded into this struct, and the request body is applied to it. |
| `ReadConstructor` | `func() any` | Constructor for `GET /{path}/{id}`. Only fields present on the override struct are SELECTed. |
| `ListConstructor` | `func() any` | Constructor for `GET /{path}/`. Only fields present on the override struct are SELECTed. |

```go
// "_" suffix is stripped when deriving the table name:
//   strings.Split("Note_Draft", "_")[0] = "Note" → table "note"
type Note_Draft struct {
    ID      uint64
    Title   string `validate:"required,max=200"`
    Content string
    UserID  uint64 `validate:"required"`
}

Routes: map[string]gorestapi.Route{
    "notes": {
        CreateConstructor: func() any { return &Note_Draft{} },
    },
}
```

## Logging

The handler logs with `log/slog`, with the `Logger` of the options, or `slog.Default()`, and with the context of the request, so that a handler of `slog` can add e.g. the ID of its trace:

* **Errors of the hooks** of the routes, e.g. of `PreCreate` or `PostListItem`, which are `500` responses, at the error level.
* **A relation path** of `AllowedFilters` that cannot be used, once, as a warning, see [Route](#route).
* **An invalid ID** in a URL, at the debug level.

The errors of the queries are logged by the [registry](https://github.com/mikogs/gocrud/blob/main/docs/pkgs/registry.md#logging): at the error level for the errors of the database, which are `500` responses, and at the debug level for the errors of a request, e.g. a validation error, which are `400`, `404` and `409` responses. They are not logged again by the handler. Set the logger of the registry with `WithLogger`:

```go
logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
reg := registry.New(structs, db, gocrud.Options{Dialect: gocrud.DialectPostgres}).WithLogger(logger)
handler := gorestapi.New(reg, gorestapi.Options{Routes: routes, Logger: logger})
```

## Metrics

gorestapi does not depend on a library of metrics. The metrics of the requests are measured around `handler.Serve`, in a middleware, and the metrics of the queries with the [query hook](https://github.com/mikogs/gocrud/blob/main/docs/crud/init.md#metrics) of gocrud, in `gocrud.Options` of the registry.

Label the requests with `handler.RoutePath(r)`, the path of the route, e.g. `notes` for `/notes/42`, or an empty string for a path that is not a route. Not with the path of the request: it has the IDs, and any path that a client sends, so each of them would be a new time series. E.g. with [Prometheus](https://github.com/prometheus/client_golang):

```go
requestDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
    Name: "api_request_duration_seconds",
    Help: "The durations of the requests of the API.",
}, []string{"route", "method", "code"})
prometheus.MustRegister(requestDuration)

type statusRecorder struct {
    http.ResponseWriter
    code int
}

func (r *statusRecorder) WriteHeader(code int) {
    r.code = code
    r.ResponseWriter.WriteHeader(code)
}

instrumented := func(w http.ResponseWriter, r *http.Request) {
    recorder := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
    start := time.Now()
    handler.Serve(recorder, r)
    requestDuration.WithLabelValues(handler.RoutePath(r), r.Method, strconv.Itoa(recorder.code)).
        Observe(time.Since(start).Seconds())
}
mux.Handle("/api/", http.StripPrefix("/api", http.HandlerFunc(instrumented)))
```

## Mounting

`handler.Serve` is a standard `http.HandlerFunc`. Mount it under a prefix using `http.StripPrefix` so the handler only sees the path-relative URL:

```go
mux := http.NewServeMux()
mux.Handle("/api/", http.StripPrefix("/api", http.HandlerFunc(handler.Serve)))
// the timeouts keep slow clients from holding connections open, e.g. by sending the headers byte by byte
srv := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 10 * time.Second}
log.Fatal(srv.ListenAndServe())
```

After stripping the mount prefix the handler expects URLs of the form `/{path}/{id}` where `path` is a key from `Options.Routes` and `id` is an optional record ID: a number, or a UUID for structs with a `string` ID, see [The ID Field](https://github.com/mikogs/gocrud/blob/main/docs/struct.md#1-the-id-field).

## URL scheme

```
/{path}/           — list / create
/{path}/{id}       — read / update / delete
```

`path` must match a key in `Options.Routes`. `id` must be a single path segment of letters, digits, `-` and `_`, or absent, otherwise the response is `400 URL_PATH_ID`.

An `id` that is not valid for the records of the route, e.g. `abc` for a numeric ID, or `42` for a UUID, is answered like an `id` that does not exist, with `404 NOT_FOUND`, so that the response does not tell which IDs could exist. A UUID can be written in any letter case.

## Password fields

Fields tagged `crud:"pass"` are **never included in responses** from the Read and List endpoints. The handler strips them before serialising the object to JSON, regardless of whether the struct field has a `json` tag or not.

```go
type User struct {
    ID       uint64
    Email    string `validate:"required,email"`
    Password string `crud:"pass"` // omitted from GET /users/ and GET /users/{id}
}
```

An update (`PUT /{path}/{id}`) keeps the stored password when the request has no password, or an empty one, e.g. `{"Email": "new@example.com"}` changes only the e-mail. A request with a password, e.g. `{"Password": "new secret"}`, changes it. The password is hashed with bcrypt, also when it looks like a hash. A password longer than 72 bytes, the most that bcrypt hashes, is a `400` response with `{"Password": "max"}`.

## Endpoints

### List — `GET /{path}/`

Returns a paginated, filtered list of records.

**Query parameters:**

| Parameter | Type | Default | Description |
|---|---|---|---|
| `limit` | int | `10` | Maximum number of records to return |
| `offset` | int | `0` | Number of records to skip |
| `after` | string | — | The cursor of the next page, `meta.next` of the previous page, see below. It cannot be combined with `offset` |
| `order` | string | — | Field name to sort by, of a field that List returns or one listed in `AllowedFilters`, or a path through belongs-to relations listed in `AllowedFilters`, e.g. `Customer.Name` |
| `order_direction` | string | — | `asc` or `desc`, in any letter case |
| `filter_val_{Field}` | string | — | Filter value for the named field |
| `filter_op_{Field}` | string | `eq` | Filter operator for the named field: `eq`, `ne`, `lt`, `le`, `gt`, `ge`, `like`, `match` (PostgreSQL only, with the flag `EnableMatchFilters`), `bit` or `in` |

Values are converted to the type of the field. For `in`, the values are separated with commas, e.g. `filter_val_Status=paid,shipped&filter_op_Status=in`, up to [`MaxInValues`](#options). Times can be written as e.g. `2024-05-01` or `2024-05-01T10:30:00Z`. For a nullable (pointer) field, an empty value is `NULL`: `filter_val_ShippedAt=&filter_op_ShippedAt=eq` lists the records without a value. See the [gocrud registry](https://github.com/mikogs/gocrud/blob/main/docs/pkgs/registry.md) for the details.

**Filters on related records.** A field can be a path through relation fields, e.g. `filter_val_Customer.Country=PL` for the orders of customers from Poland, or `filter_val_Items.Product=Mouse` for the orders with such an item, when the path is listed in [`AllowedFilters`](#route), and clients can read its value through a route of the related records. See [Conditions on related records](https://github.com/mikogs/gocrud/blob/main/docs/crud/ops.md#conditions-on-related-records) for the details.

**Filter groups.** The filters are joined with AND. To join filters with OR, put them in a group, by adding the same name of letters, digits or underscores after the field name and a tilde, e.g. `q` in `filter_val_Title~q` and `filter_op_Title~q`. The filters of a group are joined with OR, and the group with AND with the other filters and groups.

| Query | Lists the records |
|---|---|
| `filter_val_Title~q=%25go%25&filter_op_Title~q=like&filter_val_Body~q=%25go%25&filter_op_Body~q=like` | with `go` in the title or the body |
| `filter_val_Status=paid&filter_val_Total~a=100&filter_op_Total~a=gt&filter_val_Express~a=true` | that are paid, and either cost more than 100 or are express |
| `filter_val_Total~min=100&filter_op_Total~min=ge&filter_val_Total~max=500&filter_op_Total~max=lt` | that cost from 100 to 500, as a group with one filter is joined with AND |

`%25` is the URL encoding of `%`, the wildcard of `like`.

An invalid filter is a `400 VALIDATION_FAILED` response, with the failed rules in `data`, e.g. `{"Status": "oneof"}`, and so is an `order` that is not allowed, e.g. `{"Unknown": "type"}`, see [AllowedFilters](#route), and a request over the [limits](#options), e.g. `{"filters": "max"}`.

**Response `200 OK`:**
```json
{
  "ok": true,
  "code": "SUCCESS",
  "data": [ { "ID": 1, "Title": "Hello", "..." : "..." } ],
  "meta": { "next": "eyJvIjpbIklEIiwiYXNjIl0sInYiOlsx..." }
}
```

When no records match, `data` is an empty list, `[]`.

**Pages with cursors.** A full page, with `limit` records, has the cursor of the next page in `meta.next`. Pass it as `after`, with the same `limit`, `order`, `order_direction` and filters, to get the records after the last one of the page, e.g. `GET /notes/?limit=20&order=CreatedAt&order_direction=desc&after=eyJv...`. Unlike `offset`, the database reads only the records of the page, also of the last pages of a large table, and no record is missed or repeated when records are added or deleted while the client reads the pages. The page after the last one is empty. See [Pages with cursors](https://github.com/mikogs/gocrud/blob/main/docs/crud/ops.md#pages-with-cursors) for the details.

* A cursor of another order, or a string that is not a cursor, is a `400 VALIDATION_FAILED` response with the message `invalid cursor` and `{"after": "type"}`, and so is `after` with `offset`.
* An `order` of a nullable field cannot be in a cursor, so its pages have no `meta.next`, and are read with `offset`.
* The cursor has the values of the `order` field and the `ID` of the last record, encoded but not encrypted. They are values that the client can get anyway, as the order is a field that List returns, or one of `AllowedFilters`, by which it can filter.

---

### Read — `GET /{path}/{id}`

Returns a single record by ID.

**Response `200 OK`:**
```json
{
  "ok": true,
  "code": "SUCCESS",
  "data": { "ID": 42, "Title": "Hello", "..." : "..." }
}
```

**Response `404 Not Found`:**
```json
{ "ok": true, "code": "NOT_FOUND" }
```

---

### Create — `PUT /{path}/`

Creates a new record. The request body must be a JSON object whose keys match the struct's field names (or `json` tags when set). An `ID` in the body is ignored, so a create cannot overwrite an existing record. The ID is assigned by the database, or a UUID is generated for a struct with a `string` ID, and returned in `data.id`.

**Request body:**
```json
{ "Title": "Hello", "Content": "World" }
```

**Response `201 Created`:**
```json
{
  "ok": true,
  "code": "CREATED",
  "data": { "id": 42 }
}
```

For a struct with a `string` ID, `id` is the UUID, e.g. `"0190f5c3-6d1e-7b3a-9c4d-2e5f6a7b8c9d"`.

**Response `400 Bad Request`** (validation failure). `data` maps the fields that failed to the names of the failed [validation rules](https://github.com/mikogs/gocrud/blob/main/docs/struct.md#validation-errors):
```json
{
  "ok": true,
  "code": "VALIDATION_FAILED",
  "message": "invalid object",
  "data": { "Email": "email" }
}
```

**Response `409 Conflict`**: `UNIQUE_VIOLATION` when a unique field already has the value, `FOREIGN_KEY_VIOLATION` when the record references a record that does not exist, and `VERSION_CONFLICT` when the record has a [version field](https://github.com/mikogs/gocrud/blob/main/docs/struct.md#10-version) and the body has a version other than the stored one, as the record was changed since the client read it. A body without the version is saved with the version that was read, so only a client that sends the version it read is protected from overwriting another change.

---

### Update — `PUT /{path}/{id}`

Updates an existing record. The request body replaces the record's fields. Returns `404` if the record does not exist. The record is the one of the `id` in the URL: an `ID` in the body is ignored, so an update cannot write to another record.

**Response `200 OK`:**
```json
{ "ok": true, "code": "SUCCESS", "data": { "id": 42 } }
```

---

### Delete — `DELETE /{path}/{id}`

Deletes a record by ID.

**Response `200 OK`:**
```json
{ "ok": true, "code": "SUCCESS" }
```

**Response `404 Not Found`:**
```json
{ "ok": true, "code": "NOT_FOUND" }
```

**Response `409 Conflict`**: `FOREIGN_KEY_VIOLATION` when other records reference the record with a foreign key that does not allow deleting it. Records referenced with `on_delete:cascade` are deleted along with it.

---

## Response format

Every response is a JSON object with the following shape:

```json
{
  "ok":      true,
  "code":    "SUCCESS",
  "message": "optional human-readable detail",
  "data":    null
}
```

| Field | Type | Description |
|---|---|---|
| `ok` | bool | Always `true` — use `code` to distinguish outcomes |
| `code` | string | Machine-readable result code (see below) |
| `message` | string? | Present only on validation failures: `invalid object`, `invalid filters`, `invalid order` or `invalid cursor`. It does not name internal fields: the violations are in `data`, and the details are in the log of the registry, at the debug level |
| `data` | any? | Present on successful read / list / create / update, and on validation failures |
| `meta` | object? | Present on a full page of a list: `next`, the cursor of the next page, see [Pages with cursors](#list--get-path) |

**Response codes:**

| Code | HTTP status | Meaning |
|---|---|---|
| `SUCCESS` | 200 | Operation completed successfully |
| `CREATED` | 201 | Record created |
| `NOT_FOUND` | 404 | No record with the given ID |
| `VALIDATION_FAILED` | 400 | Request body or a filter failed struct validation |
| `UNIQUE_VIOLATION` | 409 | A unique field already has the value |
| `FOREIGN_KEY_VIOLATION` | 409 | The record references a record that does not exist, or is referenced by other records |
| `VERSION_CONFLICT` | 409 | The record was changed since the client read it, see its version field |
| `BAD_REQUEST` | 400 | Malformed URL, unparseable request body, or a method that is not supported for the URL, e.g. `PATCH`, or `DELETE` without an id |
| `URL_PATH_ID` | 400 | The part of the URL after the route is not a single segment of letters, digits, `-` and `_` |
| `NOT_ALLOWED` | 405 | Operation is disabled for this route via `Flags` |
| `FORBIDDEN` | 403 | An `Allow*` hook rejected the request |
| `SERVICE_ERROR` | 500 | Internal error from the registry or a `Pre*` / `Post*` hook |

## CORS

Populate `Options.CORS` to emit CORS headers on every response:

```go
import (
    "github.com/mikogs/gorestapi"
    "github.com/mikogs/gorestapi/pkg/cors"
)

handler := gorestapi.New(reg, gorestapi.Options{
    CORS: cors.CORS{
        AllowOrigin:  "https://app.example.com",
        AllowHeaders: "Content-Type, Authorization",
        AllowMethods: "GET, PUT, DELETE",
        MaxAge:       3600,
    },
})
```

Each header is written only when its field is set. Set `AllowCredentials: true` to emit `Access-Control-Allow-Credentials: true`. Leave `CORS` as its zero value to emit no CORS headers. `OPTIONS` requests to any path are answered with `204 No Content` and the CORS headers.
