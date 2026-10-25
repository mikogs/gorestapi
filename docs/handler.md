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
| `reg` | `gorestapi.Registry` | A registry of the structs, usually `*registry.CRUD` of gocrud, see [Registry](https://github.com/mikogs/gocrud/blob/v1.0.0/docs/pkgs/registry.md), or a layer of your application in front of it, see [Business logic](#business-logic) |
| `options` | `gorestapi.Options` | Configuration for the handler (zero value is valid) |

### Business logic

The handler maps HTTP requests to the records of the registry, which are the structs of the tables. The rules of your application go in one of two places:

* **The hooks of a route**, e.g. `AllowUpdate` for who can change a record, or `PreCreate` for the fields that the server sets, see [Route](#route). They are enough for most rules of a CRUD API.
* **A layer in front of the registry**, for rules that are not about a single request, e.g. of the state of an order, or that other code than the handler uses too, e.g. a background job. `New` takes a `gorestapi.Registry`, the interface of the methods of the registry that the handler calls, so the layer can be passed instead of the registry, e.g. a struct that embeds `*registry.CRUD` and replaces `Save`:

```go
type OrderRules struct {
    *registry.CRUD
}

func (o *OrderRules) Save(ctx context.Context, obj any, userID any) error {
    if order, ok := obj.(*Order); ok && order.Status == "shipped" && order.CustomerID == 0 {
        return &registry.ModelValidationError{
            Err:        errors.New("a shipped order needs a customer"),
            Violations: map[string]string{"CustomerID": "required"},
        }
    }
    return o.CRUD.Save(ctx, obj, userID)
}

handler := gorestapi.New(&OrderRules{CRUD: reg}, gorestapi.Options{Routes: routes})
```

The handler maps the errors of the registry to its responses, so the layer returns the same errors, e.g. a `*registry.ModelValidationError` for the `400` problem `VALIDATION_FAILED`, or an error that matches `gocrud.ErrNotFound` for `404`. The same interface is also the port of a hexagonal architecture: the handler is an adapter of HTTP, and the layer is the application, with the registry, or a repository of its own, behind it.

### Options

```go
type Options struct {
    Routes      map[string]Route
    UserIDFunc  func(r *http.Request) any
    MaxFilters  int
    MaxInValues int
    Timeout     time.Duration
    ProblemTypeBaseURL string
    Logger      *slog.Logger
    AccessLog   bool
    TracerProvider trace.TracerProvider
    MeterProvider  metric.MeterProvider
    Authenticate   func(r *http.Request) (any, error)
    RateLimiter    RateLimiter
    RateLimitKey   func(r *http.Request) string
    TrustedProxies []netip.Prefix
    DisableSecurityHeaders bool
    CacheControl   string
    HSTSMaxAge     time.Duration
    MaxBodyBytes   int64
    AllowUnknownFields bool
}
```

| Field | Type | Description |
|---|---|---|
| `Routes` | `map[string]Route` | Per-route configuration keyed by URL path segment. Only the paths in the map are served, other paths are the `404` problem `NOT_FOUND`. A route with the zero value has all operations enabled and all filters allowed, e.g. `"notes": {}`. |
| `UserIDFunc` | `func(*http.Request) any` | Called on every create and update request to obtain the current user's ID, passed to the registry as `ModifiedBy`: a `uint64`, or a `string` for `CreatedBy` and `ModifiedBy` fields of type `string`, e.g. the subject of a JWT. When `nil`, or when it returns `nil`, `ModifiedBy` is `0` or `""`. When the struct has the `CreatedAt`, `CreatedBy`, `ModifiedAt` and `ModifiedBy` fields, gocrud sets them from this ID and the time of the request, ignores the values sent in the request body, and never changes `CreatedAt` and `CreatedBy` on an update. |
| `MaxFilters` | `int` | The largest number of `filter_val_*` parameters of a list request, otherwise the response is the `400` problem `VALIDATION_FAILED` with `{"filters": "max"}` in `errors`. `0` is `DefaultMaxFilters` (20), and a negative value is no limit. |
| `MaxInValues` | `int` | The largest number of the values of an `in` filter, otherwise the response is the `400` problem `VALIDATION_FAILED` with e.g. `{"Status": "max"}` in `errors`. `0` is `DefaultMaxInValues` (100), and a negative value is no limit. |
| `Timeout` | `time.Duration` | The longest time of a request: its queries are cancelled after it, also in the database, and the response is the `500` problem `SERVICE_ERROR`. `0` is `DefaultTimeout` (30 seconds), and a negative value is no timeout. |
| `ProblemTypeBaseURL` | `string` | The beginning of the type URIs of the problems of the errors, e.g. `https://api.example.com/problems/`. When it is empty, the type is `about:blank`, see [Errors](#errors). |
| `Logger` | `*slog.Logger` | The logger of the handler, `slog.Default()` when `nil`, see [Logging](#logging). |
| `AccessLog` | `bool` | Logs every request with `Logger`, at the info level, see [Logging](#logging). |
| `TracerProvider`, `MeterProvider` | `trace.TracerProvider`, `metric.MeterProvider` | The providers of OpenTelemetry of the spans and the metrics of the requests, or the global ones when `nil`, see [Traces and metrics](#traces-and-metrics). |
| `Authenticate` | `func(*http.Request) (any, error)` | Returns the principal of a request, e.g. the claims of its token, or an error, which is the `401` problem `UNAUTHORIZED`, see [Authentication](#authentication). When `nil`, the requests are not authenticated. |
| `RateLimiter` | `gorestapi.RateLimiter` | Limits the requests of each client, e.g. `gorestapi.NewRateLimiter(10, 20)`, with the `429` problem `TOO_MANY_REQUESTS` over the limit, see [Rate limits](#rate-limits). When `nil`, there is no limit. |
| `RateLimitKey` | `func(*http.Request) string` | The key of the limit of a request, e.g. its user. When `nil`, the IP of the client, see [Rate limits](#rate-limits). |
| `TrustedProxies` | `[]netip.Prefix` | The networks of the proxies in front of the API, e.g. of its load balancer, whose `X-Forwarded-For` header has the IP of the client, see [Client IPs](#client-ips). |
| `DisableSecurityHeaders` | `bool` | Leaves out the headers of security of the responses, e.g. when a proxy sets them, see [Security headers](#security-headers). |
| `CacheControl` | `string` | The `Cache-Control` header of the responses, `no-store` when it is empty, see [Security headers](#security-headers). |
| `HSTSMaxAge` | `time.Duration` | Sends `Strict-Transport-Security` for this long, e.g. a year, see [Security headers](#security-headers). It is not sent when it is `0`. |
| `MaxBodyBytes` | `int64` | The largest body of a create or an update, otherwise the response is the `413` problem `REQUEST_TOO_LARGE`. `0` is `jsonreq.DefaultMaxBytes` (1 MiB), and a negative value is no limit, see [Request bodies](#request-bodies). |
| `AllowUnknownFields` | `bool` | Ignores the fields of a body that the struct does not have, instead of the `400` problem `VALIDATION_FAILED`, see [Request bodies](#request-bodies). |

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
| `DisableCreate` | `PUT /{path}/` (create) — responds with the `405` problem `NOT_ALLOWED`, and the allowed methods in the `Allow` header |
| `DisableUpdate` | `PUT /{path}/{id}` (update) — responds with the `405` problem `NOT_ALLOWED`, and the allowed methods in the `Allow` header |
| `DisableDelete` | `DELETE /{path}/{id}` — responds with the `405` problem `NOT_ALLOWED`, and the allowed methods in the `Allow` header |
| `DisableRead` | `GET /{path}/{id}` — responds with the `405` problem `NOT_ALLOWED`, and the allowed methods in the `Allow` header |
| `DisableList` | `GET /{path}/` — responds with the `405` problem `NOT_ALLOWED`, and the allowed methods in the `Allow` header |
| `DisableFilters` | All `filter_val_*` / `filter_op_*` query parameters are ignored |
| `EnableMatchFilters` | Allows the `match` operator, a regular expression from the client, which can take the database a long time. Without the flag, `match` is the `400` problem `VALIDATION_FAILED` with e.g. `{"Name": "type"}` in `errors` |

**AllowedFilters**

`AllowedFilters` is a whitelist of field names that clients may pass as filters. When it is empty, the fields that List returns are allowed (unless `DisableFilters` is set): not the [password fields](#password-fields), and not the fields with a `json:"-"` tag. A client cannot filter by the values that it does not get, as a filter, e.g. with `like`, would reveal them. Fields that are not allowed are silently dropped from the request, also in [filter groups](#list--get-path), e.g. `filter_val_Status~q` is allowed when `Status` is. To allow a field that is not returned, list it. Password fields cannot be used in filters even when they are listed: the response is the `400` problem `VALIDATION_FAILED` with `{"Password": "password"}` in `errors`.

`order` can be a field that List returns, or one of `AllowedFilters`. Another `order` is a `400 VALIDATION_FAILED` problem with `{"<order>": "type"}` in `errors`, the same as for a field that does not exist, so that clients cannot find out which fields are not returned.

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

A listed path that cannot be used is treated like a field that is not allowed: its filters are ignored, and an `order` by it is the `400` problem `VALIDATION_FAILED`. The handler logs a warning with the reason once, e.g. `reason="route customers has FilterList"`. For a path to records that are only for some clients, e.g. the owner of a note, filter by the key field instead, e.g. `UserID`, or add a route of the related struct for the records that every client can read.

```go
Routes: map[string]gorestapi.Route{
    "notes": {AllowedFilters: []string{"UserID", "Status"}},
}
```

**Authorization hooks — Allow\***

Each `Allow*` hook is called with the loaded object and the request. Return a non-nil error to reject the operation with the `403` problem `FORBIDDEN`. The error is not sent to the client, so it can name the reason for the logs of your application.

| Field | When called | Object passed |
|---|---|---|
| `AllowCreate` | After the request body is unmarshalled, before saving | The new object populated from the request |
| `AllowUpdate` | After the existing record is loaded, **before** the request body is applied | The **stored** record (original state) |
| `AllowRead` | After the record is loaded | The loaded record |
| `AllowDelete` | After the record is loaded, before deletion | The loaded record |

`AllowUpdate` receives the record as stored in the database so it reflects the original owner or state — the incoming request values have not been applied yet.

`userID` returns the user of the request from its [principal](#authentication), e.g. the subject of the claims of its JWT:

```go
func userID(r *http.Request) uint64 {
    claims, ok := gorestapi.Principal(r.Context()).(*jwtauth.CustomClaims)
    if !ok {
        return 0
    }
    id, _ := strconv.ParseUint(claims.Subject, 10, 64)
    return id
}

noteOwner := func(obj any, r *http.Request) error {
    if obj.(*Note).UserID != userID(r) {
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
                Vals: map[string]string{"UserID": strconv.FormatUint(userID(r), 10)},
            }
        },
        FilterRead: func(r *http.Request) gorestapi.FilterSet {
            return gorestapi.FilterSet{
                Vals: map[string]string{"UserID": strconv.FormatUint(userID(r), 10)},
            }
        },
    },
}
```

**Pre hooks — PreCreate / PreUpdate**

Called with the fully-prepared object just before it is written to the database. The request body has already been applied, and the ID is the one from the URL for an update, and `0` for a create. Use these to stamp server-controlled fields that should not be settable by the client, e.g. the owner of a record, `obj.(*Note).UserID = userID(r)`, so that a client cannot create a record of another user, or give its record to one.

Return a non-nil error to abort the operation with the `500` problem `SERVICE_ERROR`. The error is logged, see [Logging](#logging), and not sent to the client.

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

Return a non-nil error to abort with the `500` problem `SERVICE_ERROR`, which is logged, and not sent to the client. For `PostListItem` an error aborts the entire list response.

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

## Request bodies

The body of a create or an update is read strictly, so that a client finds its mistakes, and cannot make the API read more than it needs:

* **The content type** has to be `application/json`, or a JSON type of its own, e.g. `application/merge-patch+json`, with any parameters, e.g. `; charset=utf-8`. Another one, or none, is the `415` problem `UNSUPPORTED_MEDIA_TYPE`. This also keeps a page of another site from sending a create of a form to the API, as a browser sends a JSON content type only from the origins that [CORS](#cors) allows.
* **The size** is at most `MaxBodyBytes`, 1 MiB by default. A larger body is the `413` problem `REQUEST_TOO_LARGE`, and the rest of it is not read.
* **A field that the struct does not have**, e.g. `Titel` instead of `Title`, is the `400` problem `VALIDATION_FAILED` with `{"Titel": "unknown"}` in `errors`, as it would be silently lost otherwise. Set `AllowUnknownFields` to ignore such fields, e.g. for clients that send the fields of a newer version of the API. The names of the fields are matched in any letter case, as with `encoding/json`.
* **One JSON value**: a body with more after the object, e.g. two objects, is the `400` problem `UNMARSHAL_REQUEST`.

The handlers of your application read bodies the same way with `jsonreq.Decode`:

```go
var credentials struct{ Email, Password string }
err := jsonreq.Decode(w, r, &credentials, jsonreq.Options{MaxBytes: 4096})
// errors.Is(err, jsonreq.ErrContentType), errors.Is(err, jsonreq.ErrTooLarge), or a *jsonreq.UnknownFieldError
```

## Authentication

The handler authenticates the requests with `Options.Authenticate`, which returns the principal of a request, e.g. the claims of its token, or an error. It does not decide who can do what: the hooks of the routes do, with the principal, see [Route](#route). `gorestapi.Bearer` reads the token of the `Authorization: Bearer` header, and passes it to a function that verifies it, e.g. of a JWT of [jwtauth](jwt.md):

```go
provider, err := jwtauth.NewProvider("https://api.example.com", signingKey)

handler := gorestapi.New(reg, gorestapi.Options{
    Authenticate: gorestapi.Bearer(func(ctx context.Context, token string) (any, error) {
        claims, ok := provider.Parse(token, "notes")
        if !ok {
            return nil, errors.New("invalid token")
        }
        return claims, nil
    }),
    UserIDFunc: func(r *http.Request) any { return userID(r) },
    Routes:     routes,
})
```

* **Every request** of a route, except `OPTIONS`, is authenticated before its operation. `gorestapi.Principal(r.Context())` returns the principal in the hooks and in `UserIDFunc`, e.g. the `*jwtauth.CustomClaims` above.
* **A request without credentials**, for which `Authenticate` returns `gorestapi.ErrNoCredentials`, as `Bearer` does for a request without a token, is the `401` problem `UNAUTHORIZED`, with the header `WWW-Authenticate: Bearer`. Another error, e.g. of an expired token, is the same problem, with `WWW-Authenticate: Bearer error="invalid_token"`, as RFC 6750 says. The error is logged at the debug level, and not sent, so that clients cannot learn which part of a token is wrong.
* **A route with `Anonymous`** serves the requests without credentials, e.g. a sign-up, or public records, with no principal. A request with a token that is not valid is still the `401` problem, so that a client finds out that its token expired. The hooks of the route decide what an anonymous request can do, e.g. `AllowUpdate` rejects a request without a principal.
* **Tokens are read only from the header**, not from the URL, e.g. `?access_token=`, which is kept in the logs of proxies and in the history of browsers.
* **The handlers of logins**, e.g. `POST /token`, are of your application, as in the [demo](demo.md#logins): check the password with `VerifyPasswords` of the [registry](https://github.com/mikogs/gocrud/blob/v1.0.0/docs/pkgs/registry.md), respond to an unknown user and a wrong password with the same problem, and limit the logins of each client with a [rate limiter](#rate-limits) of their own, against guessing the passwords.

**Identity providers.** The tokens of an identity provider of OpenID Connect, e.g. Keycloak, Auth0, Okta or Microsoft Entra ID, are JWTs signed with the keys of its JWKS, which `jwtauth.JWKSValidator` fetches and keeps, see [Validating tokens from a remote issuer](jwt.md#validating-tokens-from-a-remote-issuer). The users log in at the provider, e.g. with the flow of OAuth 2 of a web app, and the API only verifies their tokens:

```go
validator, err := jwtauth.NewJWKSValidator(
    "https://auth.example.com/realms/app/protocol/openid-connect/certs", // the jwks_uri of the provider
    "https://auth.example.com/realms/app",                               // its issuer
    "notes",                                                              // the audience of the API
)

Authenticate: gorestapi.Bearer(func(ctx context.Context, token string) (any, error) {
    return validator.Validate(ctx, token)
}),
```

**Other credentials.** `verify` of `Bearer` can verify any token, e.g. a PASETO, or an API key, by looking it up, and `Authenticate` can read other credentials than a bearer token, e.g. the session cookie of a web app, or a client certificate of mutual TLS, from `r.TLS.PeerCertificates`. A cookie is sent by the browser also with the requests of other sites, so an API with cookies needs a protection against CSRF too, e.g. the `SameSite=Strict` attribute of the cookie, while the JSON content type of the [request bodies](#request-bodies) keeps the forms of other sites out. Bearer tokens are not sent by browsers on their own, so they need none.

The [OpenAPI](#openapi) document of a handler with `Authenticate` declares the bearer tokens as its security.

## Rate limits

`Options.RateLimiter` limits the requests of each client, after they are authenticated, so that one client cannot use up the API, e.g. its connections of the database:

```go
handler := gorestapi.New(reg, gorestapi.Options{
    // 10 requests a second of each client, with bursts of 20
    RateLimiter: gorestapi.NewRateLimiter(10, 20),
    Routes:      routes,
})
```

* **A request over the limit** is the `429` problem `TOO_MANY_REQUESTS`, with the seconds until the next request in the `Retry-After` header, and is logged at the debug level.
* **`NewRateLimiter(perSecond, burst)`** keeps a token bucket of each key in the memory of the process: it serves `perSecond` requests a second, and up to `burst` at once. The buckets that are not used are dropped, so the memory is of the clients of the last minutes.
* **The key** is the IP of the client, see [Client IPs](#client-ips), or its network `/64` for IPv6, as a client usually has a whole `/64`, and could change its IP in it for every request. Set `RateLimitKey` for another key, e.g. the user of the principal, so that the users behind one IP, e.g. of an office, have limits of their own:

```go
RateLimitKey: func(r *http.Request) string {
    if claims, ok := gorestapi.Principal(r.Context()).(*jwtauth.CustomClaims); ok {
        return "user:" + claims.Subject
    }
    return gorestapi.ClientIPKey(gorestapi.ClientIP(r, trustedProxies))
},
```

* **Several instances** of the API have limits of their own with `NewRateLimiter`, so a client can send the limit to each of them. To share the limits, implement `gorestapi.RateLimiter` with a store of all the instances, e.g. Redis:

```go
type RateLimiter interface {
    // Allow returns whether a request of key can be served now, or how long until the next one can be.
    Allow(ctx context.Context, key string) (bool, time.Duration, error)
}
```

A limiter that returns an error, e.g. when Redis is down, lets the request through, and the error is logged at the error level, so that the API works without it.

* **The limits of the handler** are of its routes. Limit the other handlers too, e.g. of logins, with a limiter of their own, and a lower limit, e.g. `NewRateLimiter(1, 5)`, as in the [demo](demo.md#logins).
* **A limit is not a protection against DDoS**: the requests over it are cheap, but they still reach the process. A flood of requests is stopped in front of the API, e.g. by a CDN or a load balancer, which can also limit the requests before they reach any instance.

### Client IPs

`gorestapi.ClientIP(r, trustedProxies)` returns the IP of the client of a request, for the rate limits and the [access log](#logging): the IP of its connection, or, when the connection is of a trusted proxy, e.g. of a load balancer, the last IP of its `X-Forwarded-For` header that is not of a trusted proxy. The header of other clients is not read, as a client can send any IP in it, e.g. a new one for every request, to get around the limits:

```go
TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, // the network of the load balancer
```

Without `TrustedProxies`, the IP behind a proxy is the IP of the proxy, so all the clients share one limit. List only the networks of your proxies, not e.g. `0.0.0.0/0`, which would trust the header of every client.

## Security headers

Every response of the handler has the headers that keep browsers from using it in other ways than as the data of a client:

| Header | Value | Why |
|---|---|---|
| `X-Content-Type-Options` | `nosniff` | A browser does not read a response as another type than its content type, e.g. as a script or a page |
| `X-Frame-Options` | `DENY` | A page of another site cannot show the response in a frame |
| `Content-Security-Policy` | `default-src 'none'; frame-ancestors 'none'` | A response that a browser shows as a page, e.g. a record with HTML in it, cannot load or run anything |
| `Referrer-Policy` | `no-referrer` | The URL of the API, e.g. with the filters of a list, is not sent to other sites |
| `Cache-Control` | `no-store`, or `CacheControl` | Browsers and proxies do not keep the records of a user, e.g. on a shared computer. Set `CacheControl`, e.g. to `public, max-age=60`, for an API of public data |
| `Strict-Transport-Security` | `max-age=…; includeSubDomains`, with `HSTSMaxAge` | Browsers use only HTTPS for the domain. It is not sent by default, as it applies to the whole domain, and should be sent over HTTPS only, e.g. by the proxy that ends TLS |

Set `DisableSecurityHeaders` when a proxy in front of the API sets them, or to set others in a middleware. The handlers that are not of the routes, e.g. of the [OpenAPI document](#openapi), need headers of their own.

## CORS

A web app of another origin, e.g. `https://app.example.com` with the API at `https://api.example.com`, can call the API from a browser only when the API allows it with the headers of CORS. The handler does not write them, so that the API can use any middleware of CORS, e.g. [rs/cors](https://github.com/rs/cors), in front of it:

```go
import "github.com/rs/cors"

api := cors.New(cors.Options{
    AllowedOrigins: []string{"https://app.example.com"},
    AllowedMethods: []string{http.MethodGet, http.MethodPut, http.MethodDelete},
    AllowedHeaders: []string{"Authorization", "Content-Type", gorestapi.RequestIDHeader},
    ExposedHeaders: []string{gorestapi.RequestIDHeader, "Allow", "Retry-After"},
    MaxAge:         600, // seconds a browser keeps the preflight
}).Handler(http.StripPrefix("/api", http.HandlerFunc(handler.Serve)))

mux.Handle("/api/", api)
```

* **The origins** are the ones of your web apps. `*`, any origin, lets any site read the responses of the users of its pages, which is safe only for an API of public data without credentials.
* **`AllowCredentials`** lets the browser send cookies to the API, and is needed only for an API with cookies, see [Authentication](#authentication). Bearer tokens are sent by the web app in the `Authorization` header, which is in `AllowedHeaders`.
* **The preflight** of a browser, an `OPTIONS` request, is answered by the middleware. An `OPTIONS` request that reaches the handler, e.g. without the headers of a preflight, is answered with `204 No Content` and the methods of the URL in the `Allow` header, without authentication.

## Logging

The handler logs with `log/slog`, with the `Logger` of the options, or `slog.Default()`, and with the context of the request, so that a handler of `slog` can add e.g. the ID of its trace:

* **Errors of the hooks** of the routes, e.g. of `PreCreate` or `PostListItem`, which are `500` responses, at the error level.
* **A relation path** of `AllowedFilters` that cannot be used, once, as a warning, see [Route](#route).
* **An invalid ID** in a URL, a request that is not [authenticated](#authentication), and a request over the [rate limit](#rate-limits), at the debug level.
* **An error of the rate limiter**, e.g. of its Redis, at the error level.
* **Every request**, with `AccessLog`, at the info level: the `method`, the `route`, e.g. `notes`, the `path`, the `client`, its IP, see [Client IPs](#client-ips), the `status` and the `size` of the response, and the `duration`.

The errors of the queries are logged by the [registry](https://github.com/mikogs/gocrud/blob/v1.0.0/docs/pkgs/registry.md#logging): at the error level for the errors of the database, which are `500` responses, and at the debug level for the errors of a request, e.g. a validation error, which are `400`, `404` and `409` responses. They are not logged again by the handler. Set the logger of the registry with `WithLogger`:

```go
logger := slog.New(gorestapi.LogHandler(slog.NewJSONHandler(os.Stderr, nil)))
reg := registry.New(structs, db, gocrud.Options{Dialect: gocrud.DialectPostgres}).WithLogger(logger)
handler := gorestapi.New(reg, gorestapi.Options{Routes: routes, Logger: logger, AccessLog: true})
```

**Request IDs.** Every request has an ID, from its `X-Request-ID` header, e.g. of a proxy or of the client, or a new one when it has none, or one of other characters than letters, digits, `.`, `_` and `-`, or longer than 128 characters, which could change the lines of the logs. The response has it in its `X-Request-ID` header, and `gorestapi.RequestID(ctx)` returns it in the hooks, e.g. for the logs of your application. `gorestapi.LogHandler` wraps a handler of `slog`, and adds the `request_id` attribute to all the records that are logged with the context of a request, by the handler, by the registry and by your code, as above, and the `trace_id` and `span_id` of its span, when OpenTelemetry records it, see [Traces and metrics](#traces-and-metrics):

```json
{"time":"...","level":"INFO","msg":"request","hand":"github.com/mikogs/gorestapi.Handler","method":"GET","route":"notes","path":"/api/notes/42","client":"203.0.113.7","status":404,"size":164,"duration":461633,"request_id":"req-1","trace_id":"e78a1928ca73364072ba62df3c5d508f","span_id":"a6983d5ea2195040"}
```

## Traces and metrics

The handler is instrumented with [OpenTelemetry](https://opentelemetry.io/docs/languages/go/), by its [semantic conventions of HTTP servers](https://opentelemetry.io/docs/specs/semconv/http/http-spans/). It depends only on the API of OpenTelemetry, which records nothing, at almost no cost, until the application sets an SDK, and the SDK chooses where the spans and the metrics go, e.g. Prometheus, an OTLP collector, Jaeger or a vendor:

* **A span of each request**, of the kind server, named by its method and its route, e.g. `GET /notes/{id}`, as the path has the IDs. A method that is not of HTTP is named `HTTP`, so that clients cannot add names. The span has the attributes `http.request.method`, `http.route`, `url.path`, `url.scheme`, `user_agent.original` and `http.response.status_code`. A response of `5xx` has the status `Error`, and the attribute `error.type`, e.g. `500`. The spans of the queries of the [gocrud hook](https://github.com/mikogs/gocrud/blob/v1.0.0/docs/crud/init.md#opentelemetry) are its children.
* **The trace context** of a request, e.g. the `traceparent` header of a gateway, makes its span a child of the span of the caller, with the global propagator of `otel.SetTextMapPropagator`, e.g. `propagation.TraceContext{}`. Without one, the propagator of OpenTelemetry reads nothing.
* **The histogram `http.server.request.duration`**, in seconds, by `http.request.method`, `http.route`, `http.response.status_code`, `url.scheme` and `error.type`. The route is `/{path}/` or `/{path}/{id}`, without the prefix of the mount, e.g. `/notes/{id}`.
* **The providers** are the global ones, of `otel.SetTracerProvider` and `otel.SetMeterProvider`, or the `TracerProvider` and `MeterProvider` of the options.
* **The logs** of a request have the IDs of its trace and span with `gorestapi.LogHandler`, see [Logging](#logging), so that a log can be found from a trace, and the other way round.

### Prometheus

The exporter of OpenTelemetry for Prometheus serves the metrics at `/metrics`, together with the statistics of the connections of the database, which are often the first to run out:

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/collectors"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "go.opentelemetry.io/otel"
    otelprometheus "go.opentelemetry.io/otel/exporters/prometheus"
    sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

registry := prometheus.NewRegistry()
registry.MustRegister(collectors.NewDBStatsCollector(db, "app"))
exporter, err := otelprometheus.New(otelprometheus.WithRegisterer(registry))
otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter)))

mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
```

```
http_server_request_duration_seconds_count{http_request_method="GET",http_response_status_code="200",http_route="/notes/",url_scheme="http",...} 12
db_client_operation_duration_seconds_count{db_collection_name="note",db_operation_name="SELECT",db_system_name="postgresql",...} 14
go_sql_in_use_connections{db_name="app"} 1
```

The [demo](demo.md#traces-metrics-and-logs) sets up the metrics, and the traces with OTLP, in `cmd/poc/telemetry.go`. `handler.RoutePath(r)` returns the path of the route of a request, e.g. `notes`, or an empty string for a path that is not a route, e.g. for the labels of other metrics: not the path of the request, which has the IDs, and any path that a client sends, so each of them would be a new time series.

## Health checks

```go
mux.HandleFunc("/healthz", gorestapi.Liveness)
mux.Handle("/readyz", handler.Readiness(db.PingContext))
```

* **`Liveness`** responds `200` while the process serves requests, e.g. for the liveness probe of Kubernetes, which restarts a process that does not respond. It does not check the database, as a restart would not fix it.
* **`Readiness(check)`** responds `200` when `check` returns `nil`, e.g. the ping of the database, and the `503` problem `SERVICE_UNAVAILABLE` otherwise, which is logged as a warning, e.g. for the readiness probe of Kubernetes, which sends no requests to the process until it is ready. The check gets the context of the request, with the `Timeout` of the options.

## OpenAPI

`handler.OpenAPI(info)` returns the [OpenAPI](https://spec.openapis.org/oas/v3.0.3) 3.0 document of the routes, as JSON, from the same configuration as the handler, so that it is always the API that the handler serves:

```go
spec, err := handler.OpenAPI(gorestapi.OpenAPIInfo{
    Title:   "Notes",
    Version: "1.0.0",
    Servers: []string{"/api"}, // the prefix of the mount, or the URL of the API
})

mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, _ *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    _, _ = w.Write(spec)
})
```

* **The paths** are `/{path}/` and `/{path}/{id}` of each route, with the operations that its `Flags` allow, e.g. no `delete` with `DisableDelete`.
* **The parameters of a list** are `limit`, `offset`, `after`, `order`, with the fields that it can be ordered by, `order_direction`, and `filter_val_{Field}` and `filter_op_{Field}` of the fields that clients can use in filters, as in [AllowedFilters](#route), without them with `DisableFilters`.
* **The schemas** of the structs of the routes are their JSON, with the names of their `json` tags, and with the structs of `CreateConstructor`, `UpdateConstructor`, `ReadConstructor` and `ListConstructor`:
  * the types of the fields, e.g. `date-time` for `time.Time`, `uuid` for a UUID ID, and `nullable` for a pointer,
  * the rules of the `validate` tags, e.g. `required`, `min` and `max`, `email`, and `oneof` as an `enum`, and the `crud_regexp` tag as a `pattern`,
  * the password fields as `writeOnly`, as they are never returned,
  * the `ID`, and the [creation and modification fields](https://github.com/mikogs/gocrud/blob/v1.0.0/docs/struct.md#4-creation-and-modification-fields), as `readOnly`, as the server sets them,
  * the relation fields as `readOnly` references to the schemas of the related structs, as they are not saved with a record.
* **The responses** are the successful responses, and the [problems](#errors) of each operation, by status, with their codes, also `413` and `415` of the bodies, `429` with `RateLimiter`, and `401` with `Authenticate`.
* **The security**, with `Authenticate`, is the scheme `bearerAuth` of the bearer tokens, of every operation, and optional for the routes with `Anonymous`. `BearerFormat` of `OpenAPIInfo`, e.g. `JWT`, tells the clients the format of the tokens. For other credentials, e.g. cookies, change the `securitySchemes` of the document, e.g. by decoding and encoding it again, when you publish it.

A page of [Swagger UI](https://github.com/swagger-api/swagger-ui) shows the document, and sends requests to the API, e.g. `/docs` of the [demo](demo.md).

## Versions

The handler serves the routes of its options, so a version of the API is a handler, mounted at its prefix, e.g. `/api/v1/` and `/api/v2/`:

```go
v1 := gorestapi.New(reg, gorestapi.Options{Routes: routesV1})
v2 := gorestapi.New(reg, gorestapi.Options{Routes: routesV2})

mux.Handle("/api/v1/", http.StripPrefix("/api/v1", http.HandlerFunc(v1.Serve)))
mux.Handle("/api/v2/", http.StripPrefix("/api/v2", http.HandlerFunc(v2.Serve)))
```

The routes of a new version can use other structs of the same tables, with `ReadConstructor`, `ListConstructor`, `CreateConstructor` and `UpdateConstructor`, e.g. a projection without a field that is removed, and other hooks and filters, while the old version serves the old structs, until its clients move. Each version has its own OpenAPI document, of its handler, with its prefix in `Servers`.

A version in the URL is visible in the logs, the traces, the caches and the documents, and works with any client, e.g. curl and a browser. A version in a header, e.g. `Accept: application/vnd.example.v2+json`, is not supported by the handler, as it is harder to cache and to test, but a middleware can choose the handler by the header, when clients need it.

## Mounting

`handler.Serve` is a standard `http.HandlerFunc`. Mount it under a prefix using `http.StripPrefix` so the handler only sees the path-relative URL:

```go
mux := http.NewServeMux()
mux.Handle("/api/", http.StripPrefix("/api", http.HandlerFunc(handler.Serve)))
// the timeouts keep slow clients from holding connections open, e.g. by sending the headers byte by byte
srv := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 10 * time.Second}
log.Fatal(srv.ListenAndServe())
```

After stripping the mount prefix the handler expects URLs of the form `/{path}/{id}` where `path` is a key from `Options.Routes` and `id` is an optional record ID: a number, or a UUID for structs with a `string` ID, see [The ID Field](https://github.com/mikogs/gocrud/blob/v1.0.0/docs/struct.md#1-the-id-field).

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

An update (`PUT /{path}/{id}`) keeps the stored password when the request has no password, or an empty one, e.g. `{"Email": "new@example.com"}` changes only the e-mail. A request with a password, e.g. `{"Password": "new secret"}`, changes it. The password is hashed with bcrypt, also when it looks like a hash. A password longer than 72 bytes, the most that bcrypt hashes, is the `400` problem `VALIDATION_FAILED` with `{"Password": "max"}` in `errors`.

## Endpoints

### List — `GET /{path}/`

Returns a paginated, filtered list of records.

**Query parameters:**

| Parameter | Type | Default | Description |
|---|---|---|---|
| `limit` | int | `10` | Maximum number of records to return |
| `offset` | int | `0` | Number of records to skip |
| `after` | string | — | The cursor of the next page, `meta.next` of the previous page, see below. It cannot be combined with `offset` |
| `order` | string | `ID` | Field name to sort by, of a field that List returns or one listed in `AllowedFilters`, or a path through belongs-to relations listed in `AllowedFilters`, e.g. `Customer.Name`. The records with the same value are ordered by `ID`, in the same direction, so that the pages neither miss nor repeat them |
| `order_direction` | string | `asc` | `asc` or `desc`, in any letter case |
| `filter_val_{Field}` | string | — | Filter value for the named field |
| `filter_op_{Field}` | string | `eq` | Filter operator for the named field: `eq`, `ne`, `lt`, `le`, `gt`, `ge`, `like`, `match` (PostgreSQL only, with the flag `EnableMatchFilters`), `bit` or `in` |

Values are converted to the type of the field. For `in`, the values are separated with commas, e.g. `filter_val_Status=paid,shipped&filter_op_Status=in`, up to [`MaxInValues`](#options). Times can be written as e.g. `2024-05-01` or `2024-05-01T10:30:00Z`. For a nullable (pointer) field, an empty value is `NULL`: `filter_val_ShippedAt=&filter_op_ShippedAt=eq` lists the records without a value. See the [gocrud registry](https://github.com/mikogs/gocrud/blob/v1.0.0/docs/pkgs/registry.md) for the details.

**Filters on related records.** A field can be a path through relation fields, e.g. `filter_val_Customer.Country=PL` for the orders of customers from Poland, or `filter_val_Items.Product=Mouse` for the orders with such an item, when the path is listed in [`AllowedFilters`](#route), and clients can read its value through a route of the related records. See [Conditions on related records](https://github.com/mikogs/gocrud/blob/v1.0.0/docs/crud/ops.md#conditions-on-related-records) for the details.

**Filter groups.** The filters are joined with AND. To join filters with OR, put them in a group, by adding the same name of letters, digits or underscores after the field name and a tilde, e.g. `q` in `filter_val_Title~q` and `filter_op_Title~q`. The filters of a group are joined with OR, and the group with AND with the other filters and groups.

| Query | Lists the records |
|---|---|
| `filter_val_Title~q=%25go%25&filter_op_Title~q=like&filter_val_Body~q=%25go%25&filter_op_Body~q=like` | with `go` in the title or the body |
| `filter_val_Status=paid&filter_val_Total~a=100&filter_op_Total~a=gt&filter_val_Express~a=true` | that are paid, and either cost more than 100 or are express |
| `filter_val_Total~min=100&filter_op_Total~min=ge&filter_val_Total~max=500&filter_op_Total~max=lt` | that cost from 100 to 500, as a group with one filter is joined with AND |

`%25` is the URL encoding of `%`, the wildcard of `like`.

An invalid filter is a `400 VALIDATION_FAILED` problem, with the failed rules in `errors`, e.g. `{"Status": "oneof"}`, and so is an `order` that is not allowed, e.g. `{"Unknown": "type"}`, see [AllowedFilters](#route), and a request over the [limits](#options), e.g. `{"filters": "max"}`.

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

**Pages with cursors.** A full page, with `limit` records, has the cursor of the next page in `meta.next`. Pass it as `after`, with the same `limit`, `order`, `order_direction` and filters, to get the records after the last one of the page, e.g. `GET /notes/?limit=20&order=CreatedAt&order_direction=desc&after=eyJv...`. Unlike `offset`, the database reads only the records of the page, also of the last pages of a large table, and no record is missed or repeated when records are added or deleted while the client reads the pages. The page after the last one is empty. See [Pages with cursors](https://github.com/mikogs/gocrud/blob/v1.0.0/docs/crud/ops.md#pages-with-cursors) for the details.

* A cursor of another order, or a string that is not a cursor, is a `400 VALIDATION_FAILED` problem with `{"after": "type"}` in `errors`, and so is `after` with `offset`.
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

**Errors:** the `404` problem `NOT_FOUND`, also for an `id` that is not valid for the route, or a record of [FilterRead](#route) that the client cannot read, the `403` problem `FORBIDDEN` of `AllowRead`, and the `500` problem `SERVICE_ERROR`, e.g. of `PostRead`. See [Errors](#errors).

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

**Response `400 Bad Request`** (validation failure). `errors` maps the fields that failed to the names of the failed [validation rules](https://github.com/mikogs/gocrud/blob/v1.0.0/docs/struct.md#validation-errors):
```json
{
  "type": "about:blank",
  "title": "Bad Request",
  "status": 400,
  "detail": "The record has invalid fields, see errors.",
  "instance": "/api/users/",
  "code": "VALIDATION_FAILED",
  "errors": { "Email": "email" }
}
```

**Response `403 Forbidden`:** the problem `FORBIDDEN` of `AllowCreate`. **Response `400 Bad Request`:** also the problem `UNMARSHAL_REQUEST`, of a body that is not a JSON object of the record, and `VALIDATION_FAILED` with e.g. `{"Titel": "unknown"}` in `errors`, of a field that the record does not have. **Responses `413` and `415`:** the problems `REQUEST_TOO_LARGE` and `UNSUPPORTED_MEDIA_TYPE`, see [Request bodies](#request-bodies).

**Response `409 Conflict`**: `UNIQUE_VIOLATION` when a unique field already has the value, `FOREIGN_KEY_VIOLATION` when the record references a record that does not exist, and `VERSION_CONFLICT` when the record has a [version field](https://github.com/mikogs/gocrud/blob/v1.0.0/docs/struct.md#10-version) and the body has a version other than the stored one, as the record was changed since the client read it. A body without the version is saved with the version that was read, so only a client that sends the version it read is protected from overwriting another change.

---

### Update — `PUT /{path}/{id}`

Updates an existing record. The request body replaces the record's fields. Returns `404` if the record does not exist. The record is the one of the `id` in the URL: an `ID` in the body is ignored, so an update cannot write to another record.

**Response `200 OK`:**
```json
{ "ok": true, "code": "SUCCESS", "data": { "id": 42 } }
```

**Errors:** the same as of a create, `400`, `403` of `AllowUpdate`, `409` and `500`, and the `404` problem `NOT_FOUND`.

---

### Delete — `DELETE /{path}/{id}`

Deletes a record by ID.

**Response `200 OK`:**
```json
{ "ok": true, "code": "SUCCESS" }
```

**Errors:** the `404` problem `NOT_FOUND`, the `403` problem `FORBIDDEN` of `AllowDelete`, and the `409` problem `FOREIGN_KEY_VIOLATION` when other records reference the record with a foreign key that does not allow deleting it. Records referenced with `on_delete:cascade` are deleted along with it.

---

## Response format

A successful response is a JSON object, of the content type `application/json`:

```json
{
  "ok":   true,
  "code": "SUCCESS",
  "data": { "id": 42 }
}
```

| Field | Type | Description |
|---|---|---|
| `ok` | bool | Always `true` |
| `code` | string | `SUCCESS` (200), or `CREATED` (201) for a create |
| `data` | any? | The record, the list of records, or the `id` of a record that is created or updated |
| `meta` | object? | Present on a full page of a list: `next`, the cursor of the next page, see [Pages with cursors](#list--get-path) |

## Errors

An error is a problem of [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457), Problem Details for HTTP APIs, the successor of RFC 7807, of the content type `application/problem+json`:

```json
{
  "type": "about:blank",
  "title": "Conflict",
  "status": 409,
  "detail": "A unique field already has the value in another record.",
  "instance": "/api/users/",
  "code": "UNIQUE_VIOLATION"
}
```

| Field | Type | Description |
|---|---|---|
| `type` | string | The type of the problem: `about:blank`, or a URI of `ProblemTypeBaseURL`, see below |
| `title` | string | The title of the type: the phrase of the status for `about:blank`, e.g. `Conflict`, or of the problem, e.g. `Unique violation` |
| `status` | number | The HTTP status, the same as of the response |
| `detail` | string | What went wrong in this request, for a developer. It does not name internal fields, and the details of an error of the server are only in its logs |
| `instance` | string | The path of the request, as the client sent it, also when the handler is mounted with `http.StripPrefix` |
| `code` | string | The code of the problem, see below. Clients switch on it, as it is the same with every `ProblemTypeBaseURL` |
| `errors` | object? | Present on `VALIDATION_FAILED`: the names of the failed [validation rules](https://github.com/mikogs/gocrud/blob/v1.0.0/docs/struct.md#validation-errors) of the fields, e.g. `{"Email": "email"}`, or of the parameters of a list, e.g. `{"filters": "max"}` |

**Types.** With `ProblemTypeBaseURL`, e.g. `https://api.example.com/problems/`, each problem has a type of its own, the base URL followed by the code in lower case, with `-` instead of `_`, e.g. `https://api.example.com/problems/unique-violation`, and the title of the problem:

```json
{
  "type": "https://api.example.com/problems/unique-violation",
  "title": "Unique violation",
  "status": 409,
  "detail": "A unique field already has the value in another record.",
  "instance": "/api/users/",
  "code": "UNIQUE_VIOLATION"
}
```

The URIs identify the problems, so keep them the same once clients use them. They can also be the addresses of pages that describe the problems to the developers of the clients, e.g. with the table below. Without `ProblemTypeBaseURL`, the type is `about:blank`, which RFC 9457 defines for problems that are as specific as their status, and `code` tells them apart, e.g. the three problems of `409`.

**Problems:**

| Code | Status | Title | Type | When |
|---|---|---|---|---|
| `URL_PATH_ID` | 400 | Invalid ID | `url-path-id` | The part of the URL after the route is not a single segment of letters, digits, `-` and `_` |
| `UNMARSHAL_REQUEST` | 400 | Invalid request body | `unmarshal-request` | The body of a create or an update is not a JSON object of the record |
| `VALIDATION_FAILED` | 400 | Validation failed | `validation-failed` | The record, a filter, the order or the cursor of a list is invalid, or the body has a field that the record does not have, see `errors` |
| `UNAUTHORIZED` | 401 | Unauthorized | `unauthorized` | The request has no valid credentials, see [Authentication](#authentication). The `WWW-Authenticate` header has the scheme |
| `FORBIDDEN` | 403 | Forbidden | `forbidden` | An `Allow*` hook rejected the request |
| `NOT_FOUND` | 404 | Not found | `not-found` | No record with the ID, or no route for the URL |
| `NOT_ALLOWED` | 405 | Method not allowed | `not-allowed` | The method is not supported for the URL, e.g. `PATCH`, or `DELETE` of a list, or it is disabled for the route with `Flags`. The `Allow` header has the methods that are allowed |
| `UNIQUE_VIOLATION` | 409 | Unique violation | `unique-violation` | A unique field already has the value |
| `FOREIGN_KEY_VIOLATION` | 409 | Foreign key violation | `foreign-key-violation` | The record references a record that does not exist, or a deleted record is referenced by other records |
| `VERSION_CONFLICT` | 409 | Version conflict | `version-conflict` | The record was changed since the client read it, see its version field |
| `REQUEST_TOO_LARGE` | 413 | Request too large | `request-too-large` | The body is larger than `MaxBodyBytes`, see [Request bodies](#request-bodies) |
| `UNSUPPORTED_MEDIA_TYPE` | 415 | Unsupported media type | `unsupported-media-type` | The body is not of a JSON content type |
| `TOO_MANY_REQUESTS` | 429 | Too many requests | `too-many-requests` | The client is over its [rate limit](#rate-limits). The `Retry-After` header has the seconds until the next request |
| `SERVICE_ERROR` | 500 | Service error | `service-error` | An error of the database, or of a `Pre*` or `Post*` hook, which is logged |
| `SERVICE_UNAVAILABLE` | 503 | Service unavailable | `service-unavailable` | The check of a [readiness probe](#health-checks) failed |

The codes are constants in Go, e.g. `jsonresp.CodeValidationFailed`, `jsonresp.CodeNotFound`, `gorestapi.CodeForbidden` and `gorestapi.CodeUnauthorized`, for clients and tests in Go. The handlers of your application, e.g. of a login, can respond with the same problems with `jsonresp.WriteProblem`:

```go
jsonresp.WriteProblem(w, &jsonresp.Problem{
    Type:     "about:blank",
    Title:    http.StatusText(http.StatusUnauthorized),
    Status:   http.StatusUnauthorized,
    Detail:   "The e-mail or the password is not valid.",
    Instance: r.URL.Path,
    Code:     "INVALID_CREDENTIALS",
})
```

