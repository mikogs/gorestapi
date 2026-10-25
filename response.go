package gorestapi

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/mikogs/gocrud/pkg/registry"
)

// isNotFound checks if err is returned for a record that does not exist, or for an ID that is not valid for the
// records of the route, e.g. text for a numeric ID. Both are answered with 404, so that the response does not tell
// which IDs could exist.
func isNotFound(err error) bool {
	return errors.Is(err, registry.ErrNotFound) || errors.Is(err, registry.ErrInvalidID)
}

// responseData returns a JSON-safe representation of obj with password fields
// removed. If passwordFields is empty obj is returned as-is. Otherwise the
// object is marshalled to a map and the password keys are deleted before the
// map is returned, ensuring password fields are absent from the response body
// rather than present as empty strings.
func responseData(obj any, passwordFields []string) any {
	if len(passwordFields) == 0 {
		return obj
	}

	// Marshal to JSON then back to a generic map so that json struct tags
	// (e.g. json:"email") are honoured as map keys.
	data, err := json.Marshal(obj)
	if err != nil {
		return obj
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return obj
	}

	// Resolve each password field's JSON key name via reflection.
	t := reflect.Indirect(reflect.ValueOf(obj)).Type()
	for _, fieldName := range passwordFields {
		jsonKey := fieldName // default: use the struct field name
		for i := 0; i < t.NumField(); i++ {
			if t.Field(i).Name != fieldName {
				continue
			}
			if tag := t.Field(i).Tag.Get("json"); tag != "" {
				parts := strings.SplitN(tag, ",", 2)
				if parts[0] != "" && parts[0] != "-" {
					jsonKey = parts[0]
				}
			}
			break
		}
		delete(m, jsonKey)
	}

	return m
}
