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
)

type Response struct {
	Ok      bool        `json:"ok"`
	Code    string      `json:"code"`
	Message *string     `json:"message,omitempty"` // nil -> omitted
	Data    interface{} `json:"data,omitempty"`    // any payload
	Meta    interface{} `json:"meta,omitempty"`    // optional extra info
}

func Write(w http.ResponseWriter, httpCode int, response *Response) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(httpCode)
	jsonBytes, _ := json.Marshal(response)
	_, _ = w.Write(jsonBytes)
}
