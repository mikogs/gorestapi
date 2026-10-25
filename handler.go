package gorestapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/netip"
	"reflect"
	"regexp"
	"sync"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/mikogs/gocrud/pkg/registry"
)

const (
	CodeServiceError = "SERVICE_ERROR"
	CodeNotAllowed   = "NOT_ALLOWED"
	CodeForbidden    = "FORBIDDEN"
)

const (
	filterValPrefix = "filter_val_"
	filterOpPrefix  = "filter_op_"
)

const (
	DisableCreate = 1 << iota
	DisableUpdate
	DisableDelete
	DisableRead
	DisableList
	DisableFilters
	// EnableMatchFilters allows the match operator, a regular expression, which can take the database a long time.
	EnableMatchFilters
)

// The limits of a request when Options does not set them.
const (
	DefaultMaxFilters  = 20
	DefaultMaxInValues = 100
	DefaultTimeout     = 30 * time.Second
)

var (
	// a field, or a path through relation fields, e.g. Customer.Country, optionally with a group, e.g. Name~q
	filterValRegexp = regexp.MustCompile(`^filter_val_[a-zA-Z0-9_]+(\.[a-zA-Z0-9_]+)*(~[a-zA-Z0-9_]+)?$`)
	filterOpRegexp  = regexp.MustCompile(`^filter_op_[a-zA-Z0-9_]+(\.[a-zA-Z0-9_]+)*(~[a-zA-Z0-9_]+)?$`)
)

// FilterSet holds server-side filters injected independently of client input.
// Ops maps field names to op strings ("eq", "ne", "lt", …); an absent entry
// defaults to "eq".
type FilterSet struct {
	Vals map[string]string
	Ops  map[string]string
}

// Route maps a URL path segment to a registry key and configures
// per-operation behavior. When RegistryKey is empty the URL path segment is
// used as the registry key.
type Route struct {
	RegistryKey string
	Flags       int64
	// AllowedFilters lists the fields and paths that clients may use in filters. When it is empty, the fields that
	// List returns are allowed, i.e. not the password fields and the fields with a json:"-" tag, and no paths. The
	// order can be a field that List returns, or one of AllowedFilters.
	AllowedFilters []string
	// Anonymous serves the requests of the route without credentials, with no Principal, when Options.Authenticate
	// returns ErrNoCredentials, e.g. for the public records of a catalog. Requests with credentials that are not valid
	// are still the problem UNAUTHORIZED.
	Anonymous bool

	// Allow hooks — called with the object and request; return non-nil to reject
	// with 403. AllowUpdate receives the loaded record before the request body is
	// applied, so it reflects the stored state (e.g. the original owner).
	AllowCreate func(obj any, r *http.Request) error
	AllowUpdate func(obj any, r *http.Request) error
	AllowRead   func(obj any, r *http.Request) error
	AllowDelete func(obj any, r *http.Request) error

	// Pre hooks — called with the fully prepared object just before it is saved;
	// the request body has already been applied and the ID is set. Return non-nil
	// to abort the operation with 500.
	PreCreate func(obj any, r *http.Request) error
	PreUpdate func(obj any, r *http.Request) error

	// PostRead is called on a single loaded record before it is serialised to
	// JSON. Return non-nil to abort with 500.
	PostRead func(obj any, r *http.Request) error

	// PostListItem is called on each item in a list response before it is
	// serialised to JSON. Return non-nil to abort the entire list with 500.
	PostListItem func(obj any, r *http.Request) error

	// FilterList returns server-side filters applied to every list request.
	// They are joined with AND around the client's filters, so clients cannot
	// override them or widen them with filter groups.
	FilterList func(r *http.Request) FilterSet

	// FilterRead returns server-side filters applied when reading a single
	// record by ID. The handler calls List(limit=1) with these filters and
	// the ID; a mismatch returns 404 so record existence is not revealed to
	// unauthorised callers.
	FilterRead func(r *http.Request) FilterSet

	// Per-operation constructor overrides. When nil the registry's registered
	// constructor for the key is used. Set to use a different struct type for
	// a specific operation — for example a create-only struct with fewer fields
	// that implements a custom insertQueryBuilder.
	CreateConstructor func() any
	UpdateConstructor func() any
	ReadConstructor   func() any
	ListConstructor   func() any
}

type Options struct {
	// Authenticate returns the principal of a request, e.g. the claims of its token, which the hooks and UserIDFunc
	// get with Principal(r.Context()), or an error, which is the problem UNAUTHORIZED, e.g. ErrNoCredentials for a
	// request without credentials, see Bearer. It is called with every request of a route, except OPTIONS, before its
	// operation. When it is nil, the requests are not authenticated.
	Authenticate func(r *http.Request) (any, error)
	// RateLimiter limits the requests of each client, after they are authenticated, e.g. NewRateLimiter(10, 20). A
	// request over the limit is the problem TOO_MANY_REQUESTS. It is nil for no limit.
	RateLimiter RateLimiter
	// RateLimitKey returns the key of the limit of a request, e.g. the user of its Principal, or ClientIPKey of its
	// ClientIP, which is the key when it is nil.
	RateLimitKey func(r *http.Request) string
	// TrustedProxies are the networks of the proxies in front of the API, e.g. of its load balancer, whose
	// X-Forwarded-For header has the IP of the client, see ClientIP.
	TrustedProxies []netip.Prefix
	// DisableSecurityHeaders leaves out the headers of security of the responses: X-Content-Type-Options: nosniff,
	// X-Frame-Options: DENY, Content-Security-Policy: default-src 'none'; frame-ancestors 'none', Referrer-Policy:
	// no-referrer and Cache-Control, e.g. when a proxy in front of the API sets them.
	DisableSecurityHeaders bool
	// CacheControl is the Cache-Control header of the responses, no-store when it is empty, so that the records of a
	// user are not kept by browsers and proxies. Set it e.g. to "public, max-age=60" for an API of public data.
	CacheControl string
	// HSTSMaxAge sends the Strict-Transport-Security header, with includeSubDomains, so that browsers use only HTTPS
	// for the domain for this long, e.g. a year. It is not sent when it is 0, e.g. when the proxy that ends TLS sends
	// it, as it should be sent only over HTTPS.
	HSTSMaxAge time.Duration
	// MaxBodyBytes is the largest body of a create or an update, jsonreq.DefaultMaxBytes (1 MiB) when it is 0, and no
	// limit when it is negative. A larger body is the problem REQUEST_TOO_LARGE.
	MaxBodyBytes int64
	// AllowUnknownFields ignores the fields of the body of a create or an update that the struct does not have,
	// instead of the problem VALIDATION_FAILED, e.g. for clients that send the fields of a newer version.
	AllowUnknownFields bool
	// ProblemTypeBaseURL is the beginning of the type URIs of the problems of the error responses, e.g.
	// https://api.example.com/problems/, followed by the code of the problem, e.g. unique-violation. When it is empty,
	// the type of all the problems is about:blank, and their code tells them apart.
	ProblemTypeBaseURL string
	// Logger logs the errors of the hooks of the routes, and the problems of the routes, or slog.Default() when it is
	// nil. The errors of the queries are logged by the registry, see registry.CRUD.WithLogger.
	Logger *slog.Logger
	// TracerProvider and MeterProvider are the providers of OpenTelemetry of the spans and the metrics of the requests,
	// or the global ones, of otel.SetTracerProvider and otel.SetMeterProvider, when they are nil.
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
	// AccessLog logs every request with Logger, at the info level: its method, route, path, the status and the size of
	// its response, and its duration.
	AccessLog bool
	Routes    map[string]Route
	// UserIDFunc is called on every create and update request to obtain the
	// current user's ID, which is passed to the registry as ModifiedBy: a
	// uint64, or a string for CreatedBy and ModifiedBy fields of type string.
	// When nil, or when it returns nil, ModifiedBy is the zero value.
	UserIDFunc func(r *http.Request) any

	// MaxFilters is the largest number of the filters of a list request: DefaultMaxFilters when it is 0, and no
	// limit when it is negative.
	MaxFilters int
	// MaxInValues is the largest number of the values of an in filter: DefaultMaxInValues when it is 0, and no limit
	// when it is negative.
	MaxInValues int
	// Timeout is the longest time of the queries of a request, which are cancelled after it: DefaultTimeout when it
	// is 0, and no timeout when it is negative.
	Timeout time.Duration
}

// orDefault returns value, or def when value is zero. A negative value is no limit.
func orDefault[T int | time.Duration](value T, def T) T {
	if value == 0 {
		return def
	}
	return value
}

type Handler struct {
	options   Options
	reg       Registry
	telemetry *telemetry

	// responseFieldsCache caches the fields of the responses of List, see responseFields.
	responseFieldsCache sync.Map
	// refusedPaths holds the paths of AllowedFilters that cannot be used and were logged, see pathAllowed.
	refusedPaths sync.Map
}

// Registry is what the handler needs of a registry of structs. *registry.CRUD of gocrud implements it, and so can a
// layer of the application in front of it, e.g. with the rules of its domain, or a fake in tests. The handler maps the
// errors of the registry to its responses, so an implementation returns the same errors, e.g. a
// *registry.ModelValidationError for an invalid record, or an error that matches gocrud.ErrNotFound.
type Registry interface {
	New(key string) any
	ID(obj any) any
	Read(ctx context.Context, key string, id any, constructor func() any) (any, error)
	List(ctx context.Context, key string, options registry.ListOptions) ([]any, error)
	NextCursor(obj any, order string, orderDirection string) (string, error)
	Save(ctx context.Context, obj any, userID any) error
	Delete(ctx context.Context, key string, id any) error
	PasswordFieldNames(key string) []string
	RelatedType(obj any, field string) (reflect.Type, bool)
}

var _ Registry = (*registry.CRUD)(nil)

func New(reg Registry, options Options) *Handler {
	return &Handler{
		options:   options,
		reg:       reg,
		telemetry: newTelemetry(options.TracerProvider, options.MeterProvider),
	}
}

// handlerAttr is the hand attribute of the logs, with the type of the handler.
var handlerAttr = slog.String("hand", reflect.TypeFor[Handler]().PkgPath()+".Handler")

func (h *Handler) log() *slog.Logger {
	if h.options.Logger != nil {
		return h.options.Logger
	}
	return slog.Default()
}

// writeServiceError writes the problem of an error of the registry, which logs its errors itself.
func (h *Handler) writeServiceError(w http.ResponseWriter, r *http.Request) {
	h.writeProblem(w, r, CodeServiceError, "", nil)
}

// logServiceError logs err, e.g. of a hook of a route, and writes the problem of an error of the server.
func (h *Handler) logServiceError(w http.ResponseWriter, r *http.Request, msg string, err error) {
	h.log().LogAttrs(r.Context(), slog.LevelError, msg, handlerAttr, slog.String("err", err.Error()))
	h.writeServiceError(w, r)
}
