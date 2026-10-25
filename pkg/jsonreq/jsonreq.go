// Package jsonreq decodes the JSON bodies of requests strictly: of a JSON content type, of a limited size, with no
// fields that the value does not have, and with nothing after the value.
package jsonreq

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

// DefaultMaxBytes is the largest body of Decode when Options.MaxBytes is 0.
const DefaultMaxBytes = 1 << 20

var (
	// ErrContentType is returned for a body whose Content-Type is not application/json, or another JSON type, e.g.
	// application/merge-patch+json. It also keeps browsers from sending the body of a form of another site.
	ErrContentType = errors.New("content type is not JSON")
	// ErrTooLarge is returned for a body larger than Options.MaxBytes.
	ErrTooLarge = errors.New("request body is too large")
)

// UnknownFieldError is returned for a body with a field that the value does not have, e.g. of a typo of a client.
type UnknownFieldError struct {
	// Field is the name of the field in the JSON.
	Field string
}

func (e *UnknownFieldError) Error() string {
	return fmt.Sprintf("unknown field %q", e.Field)
}

type Options struct {
	// MaxBytes is the largest body, DefaultMaxBytes when it is 0, and no limit when it is negative.
	MaxBytes int64
	// AllowUnknownFields ignores the fields of the body that the value does not have, instead of an error.
	AllowUnknownFields bool
}

// Decode decodes the JSON body of r into v. w closes the connection of a body that is too large, as http.MaxBytesReader
// does.
func Decode(w http.ResponseWriter, r *http.Request, v any, options Options) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json") {
		return ErrContentType
	}

	body := r.Body
	switch {
	case options.MaxBytes == 0:
		body = http.MaxBytesReader(w, r.Body, DefaultMaxBytes)
	case options.MaxBytes > 0:
		body = http.MaxBytesReader(w, r.Body, options.MaxBytes)
	}

	decoder := json.NewDecoder(body)
	if !options.AllowUnknownFields {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(v); err != nil {
		return decodeError(err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err != nil {
			return decodeError(err)
		}
		return errors.New("request body has data after the JSON value")
	}
	return nil
}

// decodeError returns the error of a failed decoding of a body: of its size, of a field that the value does not have,
// or of its JSON.
func decodeError(err error) error {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return ErrTooLarge
	}
	// encoding/json has no type of the error of an unknown field, only its message
	if field, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return &UnknownFieldError{Field: strings.Trim(field, `"`)}
	}
	return fmt.Errorf("decode request body: %w", err)
}
