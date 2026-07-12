package gorestapi

import (
	"net/http"
	"regexp"

	svccrud "github.com/mikolajgasior/gocrud/pkg/service"
	"github.com/mikolajgasior/gorestapi/pkg/cors"
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
)

var (
	filterValRegexp = regexp.MustCompile("^filter_val_[a-zA-Z0-9_]+$")
	filterOpRegexp  = regexp.MustCompile("^filter_op_[a-zA-Z0-9_]+$")
)

// FilterSet holds server-side filters injected independently of client input.
// Ops maps field names to op strings ("eq", "ne", "lt", …); an absent entry
// defaults to "eq".
type FilterSet struct {
	Vals map[string]string
	Ops  map[string]string
}

// Route maps a URL path segment to a service registry key and configures
// per-operation behavior. When RegistryKey is empty the URL path segment is
// used as the registry key.
type Route struct {
	RegistryKey    string
	Flags          int64
	AllowedFilters []string // when non-empty, only the listed fields may be used as filters

	// Allow hooks — called with the object and request; return non-nil to reject
	// with 403. AllowUpdate receives the loaded record before the request body is
	// applied, so it reflects the stored state (e.g. the original owner).
	AllowCreate func(obj interface{}, r *http.Request) error
	AllowUpdate func(obj interface{}, r *http.Request) error
	AllowRead   func(obj interface{}, r *http.Request) error
	AllowDelete func(obj interface{}, r *http.Request) error

	// Pre hooks — called with the fully prepared object just before it is saved;
	// the request body has already been applied and the ID is set. Return non-nil
	// to abort the operation with 500.
	PreCreate func(obj interface{}, r *http.Request) error
	PreUpdate func(obj interface{}, r *http.Request) error

	// PostRead is called on a single loaded record before it is serialised to
	// JSON. Return non-nil to abort with 500.
	PostRead func(obj interface{}, r *http.Request) error

	// PostListItem is called on each item in a list response before it is
	// serialised to JSON. Return non-nil to abort the entire list with 500.
	PostListItem func(obj interface{}, r *http.Request) error

	// FilterList returns server-side filters merged into every list request.
	// Injected filters take precedence over client-supplied ones, so clients
	// cannot override them.
	FilterList func(r *http.Request) FilterSet

	// FilterRead returns server-side filters applied when reading a single
	// record by ID. The handler calls List(limit=1) with the injected filters
	// plus the ID constraint; a mismatch returns 404 so record existence is
	// not revealed to unauthorised callers.
	FilterRead func(r *http.Request) FilterSet

	// Per-operation constructor overrides. When nil the service's registered
	// constructor for the key is used. Set to use a different struct type for
	// a specific operation — for example a create-only struct with fewer fields
	// that implements a custom insertQueryBuilder.
	CreateConstructor func() interface{}
	UpdateConstructor func() interface{}
	ReadConstructor   func() interface{}
	ListConstructor   func() interface{}
}

type Options struct {
	CORS   cors.CORS
	Routes map[string]Route
	// UserIDFunc is called on every create and update request to obtain the
	// current user's ID, which is passed to the service as ModifiedBy.
	// When nil, ModifiedBy is always 0.
	UserIDFunc func(r *http.Request) uint64
}

type Handler struct {
	options Options
	svc     *svccrud.CRUD
}

func New(svc *svccrud.CRUD, options Options) *Handler {
	return &Handler{
		options: options,
		svc:     svc,
	}
}
