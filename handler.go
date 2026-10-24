package gorestapi

import (
	"context"
	"log/slog"
	"net/http"
	"reflect"
	"regexp"
	"sync"
	"time"

	"github.com/mikogs/gocrud/pkg/registry"
	"github.com/mikogs/gorestapi/pkg/cors"
	"github.com/mikogs/gorestapi/pkg/jsonresp"
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
	CORS cors.CORS
	// Logger logs the errors of the hooks of the routes, and the problems of the routes, or slog.Default() when it is
	// nil. The errors of the queries are logged by the registry, see registry.CRUD.WithLogger.
	Logger *slog.Logger
	Routes map[string]Route
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
	options Options
	reg     *registry.CRUD

	// responseFieldsCache caches the fields of the responses of List, see responseFields.
	responseFieldsCache sync.Map
	// refusedPaths holds the paths of AllowedFilters that cannot be used and were logged, see pathAllowed.
	refusedPaths sync.Map
}

func New(reg *registry.CRUD, options Options) *Handler {
	return &Handler{
		options: options,
		reg:     reg,
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

// writeServiceError writes the response of an error of the registry, which logs its errors itself.
func writeServiceError(w http.ResponseWriter) {
	jsonresp.Write(w, http.StatusInternalServerError, &jsonresp.Response{
		Ok:   true,
		Code: CodeServiceError,
	})
}

// logServiceError logs err, e.g. of a hook of a route, and writes the response of an error of the server.
func (h *Handler) logServiceError(ctx context.Context, w http.ResponseWriter, msg string, err error) {
	h.log().LogAttrs(ctx, slog.LevelError, msg, handlerAttr, slog.String("err", err.Error()))
	writeServiceError(w)
}
