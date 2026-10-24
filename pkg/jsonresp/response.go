package jsonresp

import (
	"encoding/json"
	"net/http"
)

const (
	CodeSuccess          = "SUCCESS"
	CodeCreated          = "CREATED"
	CodeURLPathID        = "URL_PATH_ID"
	CodeBadRequest       = "BAD_REQUEST"
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
