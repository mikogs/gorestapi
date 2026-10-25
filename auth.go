package gorestapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
)

// CodeUnauthorized is the code of the problem of a request without valid credentials, see Options.Authenticate.
const CodeUnauthorized = "UNAUTHORIZED"

// ErrNoCredentials is returned by an Authenticate for a request without credentials, e.g. without an Authorization
// header. It lets the requests of the routes with Anonymous through, and its problem UNAUTHORIZED has the header
// WWW-Authenticate: Bearer, without the error="invalid_token" of other errors, as RFC 6750 says.
var ErrNoCredentials = errors.New("no credentials")

type principalKey struct{}

// Principal returns the principal of the request of ctx, from Options.Authenticate, e.g. the claims of its token, or nil
// for a request without one, e.g. of a route with Anonymous.
func Principal(ctx context.Context) any {
	return ctx.Value(principalKey{})
}

// BearerToken returns the token of the Authorization: Bearer header of r, or false when it has none. Tokens are read
// only from the header, not from the URL, which is kept in logs and the history of browsers.
func BearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return "", false
	}
	return strings.TrimSpace(token), true
}

// Bearer returns an Authenticate of the tokens of the Authorization: Bearer header, e.g. JWTs or PASETOs: verify returns
// the principal of a token, e.g. its claims, or an error for a token that is not valid. A request without a token is
// ErrNoCredentials.
func Bearer(verify func(ctx context.Context, token string) (any, error)) func(r *http.Request) (any, error) {
	return func(r *http.Request) (any, error) {
		token, ok := BearerToken(r)
		if !ok {
			return nil, ErrNoCredentials
		}
		return verify(r.Context(), token)
	}
}

// authenticate returns the context of r with its principal, or false when the request cannot be served without valid
// credentials, after it writes its problem UNAUTHORIZED. The reason is logged, and not sent, so that clients cannot
// learn which part of a token is wrong.
func (h *Handler) authenticate(ctx context.Context, w http.ResponseWriter, r *http.Request, route Route) (context.Context, bool) {
	if h.options.Authenticate == nil {
		return ctx, true
	}
	principal, err := h.options.Authenticate(r)
	switch {
	case err == nil:
		return context.WithValue(ctx, principalKey{}, principal), true
	case errors.Is(err, ErrNoCredentials) && route.Anonymous:
		return ctx, true
	}
	h.log().LogAttrs(ctx, slog.LevelDebug, "request not authenticated", handlerAttr, slog.String("err", err.Error()))
	challenge := "Bearer"
	if !errors.Is(err, ErrNoCredentials) {
		challenge += ` error="invalid_token"`
	}
	w.Header().Set("WWW-Authenticate", challenge)
	h.writeProblem(w, r, CodeUnauthorized, "", nil)
	return ctx, false
}
