# Handler

## Initialization

```go
import (
    "codeberg.org/mikolajgasior/gorestapi"
    svccrud "codeberg.org/mikolajgasior/gocrud/pkg/service"
)

svc := svccrud.New(registry, dbConn, gocrud.DialectPostgres)

handler := gorestapi.New(svc, gorestapi.Options{})
```

`New` takes two arguments:

| Argument | Type | Description |
|---|---|---|
| `svc` | `*service.CRUD` | An initialised service instance |
| `options` | `gorestapi.Options` | Configuration for the handler (zero value is valid) |

### Options

```go
type Options struct {
    CORS       cors.CORS
    Routes     map[string]Route
    UserIDFunc func(r *http.Request) uint64
}
```

| Field | Type | Description |
|---|---|---|
| `CORS` | `cors.CORS` | CORS headers written on every response. Zero value emits no CORS headers. |
| `Routes` | `map[string]Route` | Per-route configuration keyed by URL path segment. Routes absent from the map use the zero value (all operations enabled, all filters allowed). |
| `UserIDFunc` | `func(*http.Request) uint64` | Called on every create and update request to obtain the current user's ID, passed to the service as `ModifiedBy`. When `nil`, `ModifiedBy` is always `0`. |

### Route

`Route` maps a URL path segment to a service registry key and configures per-operation behavior.

```go
type Route struct {
    RegistryKey    string
    Flags          int64
    AllowedFilters []string

    AllowCreate func(obj interface{}, r *http.Request) error
    AllowUpdate func(obj interface{}, r *http.Request) error
    AllowRead   func(obj interface{}, r *http.Request) error
    AllowDelete func(obj interface{}, r *http.Request) error

    PreCreate func(obj interface{}, r *http.Request) error
    PreUpdate func(obj interface{}, r *http.Request) error

    PostRead     func(obj interface{}, r *http.Request) error
    PostListItem func(obj interface{}, r *http.Request) error

    FilterList func(r *http.Request) FilterSet
    FilterRead func(r *http.Request) FilterSet

    CreateConstructor func() interface{}
    UpdateConstructor func() interface{}
    ReadConstructor   func() interface{}
    ListConstructor   func() interface{}
}
```

**RegistryKey**

When `RegistryKey` is empty the URL path segment is used as the service registry key. Set it explicitly when the URL path and the registry key should differ:

```go
Routes: map[string]gorestapi.Route{
    "v1/users": {RegistryKey: "users"},
}
```

**Flags**

`Flags` is a bitmask that controls which operations and features are disabled for a route. Combine multiple flags with `|`. The zero value enables everything.

| Constant | Disables |
|---|---|
| `DisableCreate` | `PUT /{path}/` (create) — responds `405` |
| `DisableUpdate` | `PUT /{path}/{id}` (update) — responds `405` |
| `DisableDelete` | `DELETE /{path}/{id}` — responds `405` |
| `DisableRead` | `GET /{path}/{id}` — responds `405` |
| `DisableList` | `GET /{path}/` — responds `405` |
| `DisableFilters` | All `filter_val_*` / `filter_op_*` query parameters are ignored |

**AllowedFilters**

`AllowedFilters` is a whitelist of field names that clients may pass as filters. When empty all fields are allowed (unless `DisableFilters` is set). Fields not in the list are silently dropped from the request.

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
noteOwner := func(obj interface{}, r *http.Request) error {
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

These hooks inject server-controlled filters into queries, merging with (and taking precedence over) any client-supplied filters so clients cannot override them.

`FilterList` is applied to every list request (`GET /{path}/`). `FilterRead` is applied when reading a single record by ID (`GET /{path}/{id}`); when set, the handler uses `List(limit=1)` with the ID added as an extra constraint so mismatches return `404` rather than revealing record existence.

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

Called with the fully-prepared object just before it is written to the database. The request body has already been applied and the ID is set. Use these to stamp server-controlled fields that should not be settable by the client.

Return a non-nil error to abort the operation with `500 Internal Server Error`.

```go
Routes: map[string]gorestapi.Route{
    "notes": {
        PreCreate: func(obj interface{}, _ *http.Request) error {
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
        PostRead: func(obj interface{}, _ *http.Request) error {
            obj.(*Note).Comment = "Returned from API"
            return nil
        },
        PostListItem: func(obj interface{}, _ *http.Request) error {
            obj.(*Note).Comment = "Returned from API"
            return nil
        },
    },
}
```

**Constructor overrides**

By default each operation uses the constructor registered for the key in the service. Setting a constructor here overrides that for the specific operation, allowing a different struct type — for example a create-only struct with fewer fields.

| Field | Type | Description |
|---|---|---|
| `CreateConstructor` | `func() interface{}` | Constructor for `PUT /{path}/` (create) |
| `UpdateConstructor` | `func() interface{}` | Constructor for `PUT /{path}/{id}` (update). The existing record is **not** pre-loaded; the URL id is stamped onto the object after JSON unmarshalling. |
| `ReadConstructor` | `func() interface{}` | Constructor for `GET /{path}/{id}`. Only fields present on the override struct are SELECTed. |
| `ListConstructor` | `func() interface{}` | Constructor for `GET /{path}/`. Only fields present on the override struct are SELECTed. |

```go
// "_" suffix is stripped when deriving the table name:
//   strings.Split("Note_Draft", "_")[0] = "Note" → table "note"
type Note_Draft struct {
    ID      uint64
    Title   string `crud:"req len:1,200"`
    Content string
    UserID  uint64 `crud:"req"`
}

Routes: map[string]gorestapi.Route{
    "notes": {
        CreateConstructor: func() interface{} { return &Note_Draft{} },
    },
}
```

## Mounting

`handler.Serve` is a standard `http.HandlerFunc`. Mount it under a prefix using `http.StripPrefix` so the handler only sees the path-relative URL:

```go
mux := http.NewServeMux()
mux.Handle("/api/", http.StripPrefix("/api", http.HandlerFunc(handler.Serve)))
http.ListenAndServe(":8080", mux)
```

After stripping the mount prefix the handler expects URLs of the form `/{path}/{id}` where `path` is a key from `Options.Routes` and `id` is an optional numeric record ID.

## URL scheme

```
/{path}/           — list / create
/{path}/{id}       — read / update / delete
```

`path` must match a key in `Options.Routes`. `id` must be a positive integer or absent.

## Password fields

Fields tagged `crud:"pass"` are **never included in responses** from the Read and List endpoints. The handler strips them before serialising the object to JSON, regardless of whether the struct field has a `json` tag or not.

```go
type User struct {
    ID       uint64
    Email    string `crud:"req email"`
    Password string `crud:"pass"` // omitted from GET /users/ and GET /users/{id}
}
```

## Endpoints

### List — `GET /{path}/`

Returns a paginated, filtered list of records.

**Query parameters:**

| Parameter | Type | Default | Description |
|---|---|---|---|
| `limit` | int | `10` | Maximum number of records to return |
| `offset` | int | `0` | Number of records to skip |
| `order` | string | — | Field name to sort by |
| `order_direction` | string | — | `asc` or `desc` |
| `filter_val_{Field}` | string | — | Filter value for the named field |
| `filter_op_{Field}` | string | `eq` | Filter operator for the named field |

**Response `200 OK`:**
```json
{
  "ok": true,
  "code": "SUCCESS",
  "data": [ { "ID": 1, "Title": "Hello", "..." : "..." } ]
}
```

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

Creates a new record. The request body must be a JSON object whose keys match the struct's field names (or `json` tags when set).

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

**Response `400 Bad Request`** (validation failure):
```json
{
  "ok": true,
  "code": "VALIDATION_FAILED",
  "message": "validation failed with violations: ..."
}
```

---

### Update — `PUT /{path}/{id}`

Updates an existing record. The request body replaces the record's fields. Returns `404` if the record does not exist.

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
| `message` | string? | Present only on errors; contains a human-readable detail |
| `data` | any? | Present on successful read / list / create / update |

**Response codes:**

| Code | HTTP status | Meaning |
|---|---|---|
| `SUCCESS` | 200 | Operation completed successfully |
| `CREATED` | 201 | Record created |
| `NOT_FOUND` | 404 | No record with the given ID |
| `VALIDATION_FAILED` | 400 | Request body failed struct validation |
| `BAD_REQUEST` | 400 | Malformed URL or unparseable request body |
| `URL_PATH_ID` | 400 | ID segment in the URL is not a valid number |
| `NOT_ALLOWED` | 405 | Operation is disabled for this route via `Flags` |
| `FORBIDDEN` | 403 | An `Allow*` hook rejected the request |
| `SERVICE_ERROR` | 500 | Internal error from the service or a `Pre*` / `Post*` hook |

## CORS

Populate `Options.CORS` to emit CORS headers on every response:

```go
import (
    "codeberg.org/mikolajgasior/gorestapi"
    "codeberg.org/mikolajgasior/gorestapi/pkg/cors"
)

handler := gorestapi.New(svc, gorestapi.Options{
    CORS: cors.CORS{
        AllowOrigin:  "https://app.example.com",
        AllowHeaders: "Content-Type, Authorization",
        AllowMethods: "GET, PUT, DELETE",
        MaxAge:       3600,
    },
})
```

Leave `CORS` as its zero value to emit no CORS headers.
