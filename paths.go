package gorestapi

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"sort"
	"strings"
)

// routeKey returns the registry key of a route.
func routeKey(urlPath string, route Route) string {
	if route.RegistryKey != "" {
		return route.RegistryKey
	}
	return urlPath
}

// pathAllowed checks if clients of the route of key can use a path through relation fields that is listed in its
// AllowedFilters, e.g. Customer.Name. A path queries the records of other structs, so it can be used only when the
// clients can read its value through a route of the related records, see pathRefusal. A path that cannot be used is
// logged once.
func (h *Handler) pathAllowed(ctx context.Context, key string, path string) bool {
	obj := h.reg.New(key)
	if obj == nil {
		return false
	}

	reason := h.pathRefusal(obj, path)
	if reason == "" {
		return true
	}

	if _, logged := h.refusedPaths.LoadOrStore(key+"\x00"+path, true); !logged {
		h.log().LogAttrs(ctx, slog.LevelWarn, "relation path in AllowedFilters cannot be used", handlerAttr,
			slog.String("key", key), slog.String("path", path), slog.String("reason", reason))
	}
	return false
}

// pathRefusal returns why a path from the struct of obj cannot be used, or an empty string when it can. For each
// relation field of the path, there has to be a route of the related struct that:
//   - returns the next field of the path, or lists it in AllowedFilters, and
//   - shows the same records to every client: it has no FilterList, FilterRead and AllowRead, which a path cannot
//     apply to the related records, and it does not disable List or filters.
func (h *Handler) pathRefusal(obj any, path string) string {
	field, rest, _ := strings.Cut(path, ".")
	relatedType, ok := h.reg.RelatedType(obj, field)
	if !ok {
		return field + " is not a relation field"
	}

	var reasons []string
	for urlPath, route := range h.options.Routes {
		relatedKey := routeKey(urlPath, route)
		related := h.reg.New(relatedKey)
		if related == nil || reflect.Indirect(reflect.ValueOf(related)).Type() != relatedType {
			continue
		}

		if rules := recordRules(route); rules != "" {
			reasons = append(reasons, fmt.Sprintf("route %s %s", urlPath, rules))
			continue
		}

		if strings.Contains(rest, ".") {
			reason := h.pathRefusal(related, rest)
			if reason == "" {
				return ""
			}
			reasons = append(reasons, fmt.Sprintf("route %s: %s", urlPath, reason))
			continue
		}

		if h.responseFields(relatedKey, route)[rest] || slices.Contains(route.AllowedFilters, rest) {
			return ""
		}
		reasons = append(reasons, fmt.Sprintf("route %s does not return %s or list it in AllowedFilters", urlPath, rest))
	}

	if len(reasons) == 0 {
		return "no route serves " + relatedType.Name()
	}
	sort.Strings(reasons)
	return strings.Join(reasons, "; ")
}

// recordRules returns what makes a route show different records to different clients, or limit how they can be
// found, or an empty string when it does not.
func recordRules(route Route) string {
	switch {
	case route.FilterList != nil:
		return "has FilterList"
	case route.FilterRead != nil:
		return "has FilterRead"
	case route.AllowRead != nil:
		return "has AllowRead"
	case route.Flags&DisableList != 0:
		return "disables List"
	case route.Flags&DisableFilters != 0:
		return "disables filters"
	}
	return ""
}
