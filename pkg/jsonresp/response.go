package jsonresp

import (
	"encoding/json"
	"net/http"
)

const (
	CodeSuccess          = "SUCCESS"
	CodeCreated          = "CREATED"
	CodeURLPathID        = "URL_PATH_ID"
	CodeUnmarshalRequest = "UNMARSHAL_REQUEST"
	CodeValidationFailed = "VALIDATION_FAILED"
	CodeNotFound         = "NOT_FOUND"

	// CodeUniqueViolation is returned when a unique field already has the value.
	CodeUniqueViolation = "UNIQUE_VIOLATION"
	// CodeForeignKeyViolation is returned when an object references an object that does not exist,
	// or is deleted while other objects reference it.
	CodeForeignKeyViolation = "FOREIGN_KEY_VIOLATION"
	// CodeVersionConflict is returned when an update has a version of the object other than the stored one, as the
	// object was changed since the client read it.
	CodeVersionConflict = "VERSION_CONFLICT"
	// CodeRequestTooLarge is returned for a request body larger than the limit of the handler.
	CodeRequestTooLarge = "REQUEST_TOO_LARGE"
	// CodeUnsupportedMediaType is returned for a request body that is not JSON, by its Content-Type.
	CodeUnsupportedMediaType = "UNSUPPORTED_MEDIA_TYPE"
)

type Response struct {
	Ok      bool    `json:"ok"`
	Code    string  `json:"code"`
	Message *string `json:"message,omitempty"` // nil -> omitted
	Data    any     `json:"data,omitempty"`    // any payload
	Meta    any     `json:"meta,omitempty"`    // optional extra info
}

func Write(w http.ResponseWriter, httpCode int, response *Response) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(httpCode)
	jsonBytes, _ := json.Marshal(response)
	_, _ = w.Write(jsonBytes)
}

// Problem is an error response of RFC 9457, Problem Details for HTTP APIs, written as application/problem+json. Code
// and Errors are its extension members: the code of the problem, which is also in Type when the API has a base URL of
// its problem types, and the failed validation rules of the fields of the request.
type Problem struct {
	Type     string            `json:"type"`
	Title    string            `json:"title"`
	Status   int               `json:"status"`
	Detail   string            `json:"detail,omitempty"`
	Instance string            `json:"instance,omitempty"`
	Code     string            `json:"code"`
	Errors   map[string]string `json:"errors,omitempty"`
}

func WriteProblem(w http.ResponseWriter, problem *Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(problem.Status)
	jsonBytes, _ := json.Marshal(problem)
	_, _ = w.Write(jsonBytes)
}
