package gorestapi

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

// problemType is a type of the problems of the responses, by its code.
type problemType struct {
	status int
	title  string
	detail string
}

var problemTypes = map[string]problemType{
	jsonresp.CodeURLPathID:           {http.StatusBadRequest, "Invalid ID", "The part of the URL after the route is not an ID."},
	jsonresp.CodeUnmarshalRequest:    {http.StatusBadRequest, "Invalid request body", "The request body is not a JSON object of the record."},
	jsonresp.CodeValidationFailed:    {http.StatusBadRequest, "Validation failed", "The request has invalid values, see errors."},
	CodeForbidden:                    {http.StatusForbidden, "Forbidden", "The operation is not allowed for the client."},
	jsonresp.CodeNotFound:            {http.StatusNotFound, "Not found", "There is no record with the ID."},
	CodeNotAllowed:                   {http.StatusMethodNotAllowed, "Method not allowed", "The method is not allowed for the URL, see the Allow header."},
	jsonresp.CodeUniqueViolation:     {http.StatusConflict, "Unique violation", "A unique field already has the value in another record."},
	jsonresp.CodeForeignKeyViolation: {http.StatusConflict, "Foreign key violation", "The record references a record that does not exist."},
	jsonresp.CodeVersionConflict:     {http.StatusConflict, "Version conflict", "The record was changed since the client read it."},
	CodeServiceError:                 {http.StatusInternalServerError, "Service error", "The request failed on the server."},
	CodeServiceUnavailable:           {http.StatusServiceUnavailable, "Service unavailable", "The service cannot serve requests yet, e.g. without its database."},
}

// writeProblem writes the problem of code, with the detail of its type when detail is empty, and the violations of the
// fields of the request in errors.
func (h *Handler) writeProblem(w http.ResponseWriter, r *http.Request, code string, detail string, errors map[string]string) {
	problemType := problemTypes[code]
	problem := &jsonresp.Problem{
		Type:     "about:blank",
		Title:    http.StatusText(problemType.status),
		Status:   problemType.status,
		Detail:   problemType.detail,
		Instance: requestPath(r),
		Code:     code,
		Errors:   errors,
	}
	// without a base URL, the problems are only as specific as their statuses, and the code tells them apart
	if h.options.ProblemTypeBaseURL != "" {
		problem.Type = h.options.ProblemTypeBaseURL + problemSlug(code)
		problem.Title = problemType.title
	}
	if detail != "" {
		problem.Detail = detail
	}
	jsonresp.WriteProblem(w, problem)
}

// problemSlug returns the last part of the type URI of the problem of code, e.g. unique-violation for UNIQUE_VIOLATION.
func problemSlug(code string) string {
	return strings.ReplaceAll(strings.ToLower(code), "_", "-")
}

// requestPath returns the path of the request as the client sent it, also when the handler is mounted with
// http.StripPrefix, which changes r.URL but not r.RequestURI.
func requestPath(r *http.Request) string {
	if requestURL, err := url.ParseRequestURI(r.RequestURI); err == nil && requestURL.Path != "" {
		return requestURL.Path
	}
	return r.URL.Path
}
