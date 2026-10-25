package jsonreq

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type note struct {
	Title string `json:"title"`
}

func TestDecode(t *testing.T) {
	for name, test := range map[string]struct {
		contentType string
		body        string
		options     Options
		check       func(err error) bool
	}{
		"valid":                  {"application/json", `{"title":"a"}`, Options{}, func(err error) bool { return err == nil }},
		"with a charset":         {"application/json; charset=utf-8", `{"title":"a"}`, Options{}, func(err error) bool { return err == nil }},
		"a JSON type":            {"application/merge-patch+json", `{"title":"a"}`, Options{}, func(err error) bool { return err == nil }},
		"no content type":        {"", `{"title":"a"}`, Options{}, func(err error) bool { return errors.Is(err, ErrContentType) }},
		"a form":                 {"application/x-www-form-urlencoded", `title=a`, Options{}, func(err error) bool { return errors.Is(err, ErrContentType) }},
		"too large":              {"application/json", `{"title":"` + strings.Repeat("a", 100) + `"}`, Options{MaxBytes: 50}, func(err error) bool { return errors.Is(err, ErrTooLarge) }},
		"the default limit":      {"application/json", `{"title":"` + strings.Repeat("a", DefaultMaxBytes) + `"}`, Options{}, func(err error) bool { return errors.Is(err, ErrTooLarge) }},
		"no limit":               {"application/json", `{"title":"` + strings.Repeat("a", DefaultMaxBytes) + `"}`, Options{MaxBytes: -1}, func(err error) bool { return err == nil }},
		"an unknown field":       {"application/json", `{"title":"a","tittle":"b"}`, Options{}, func(err error) bool { var e *UnknownFieldError; return errors.As(err, &e) && e.Field == "tittle" }},
		"unknown fields allowed": {"application/json", `{"title":"a","tittle":"b"}`, Options{AllowUnknownFields: true}, func(err error) bool { return err == nil }},
		"data after the value":   {"application/json", `{"title":"a"} {"title":"b"}`, Options{}, func(err error) bool { return err != nil && !errors.Is(err, ErrContentType) }},
		"an empty object after":  {"application/json", `{"title":"a"} {}`, Options{}, func(err error) bool { return err != nil }},
		"not JSON":               {"application/json", `{"title":`, Options{}, func(err error) bool { return err != nil && !errors.Is(err, ErrTooLarge) }},
	} {
		req := httptest.NewRequest(http.MethodPut, "/notes/", strings.NewReader(test.body))
		if test.contentType != "" {
			req.Header.Set("Content-Type", test.contentType)
		}
		var v note
		if err := Decode(httptest.NewRecorder(), req, &v, test.options); !test.check(err) {
			t.Fatalf("Decode of %s returned %v", name, err)
		}
	}
}
