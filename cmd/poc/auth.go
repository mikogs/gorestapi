package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/mikogs/gocrud/pkg/registry"
	"github.com/mikogs/gorestapi"
	"github.com/mikogs/gorestapi/pkg/jsonreq"
	"github.com/mikogs/gorestapi/pkg/jsonresp"
	"github.com/mikogs/gorestapi/pkg/jwtauth"
)

// audience is the audience of the tokens of the demo, which the API checks, so that tokens of other APIs of the same
// issuer are not accepted.
const audience = "gorestapi-poc"

func authenticate(provider *jwtauth.Provider) func(r *http.Request) (any, error) {
	return gorestapi.Bearer(func(_ context.Context, token string) (any, error) {
		claims, ok := provider.Parse(token, audience)
		if !ok {
			return nil, errors.New("invalid token")
		}
		return claims, nil
	})
}

// userID returns the ID of the user of the token of r, or 0 for a request without a token.
func userID(r *http.Request) uint64 {
	claims, ok := gorestapi.Principal(r.Context()).(*jwtauth.CustomClaims)
	if !ok {
		return 0
	}
	id, _ := strconv.ParseUint(claims.Subject, 10, 64)
	return id
}

type login struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type token struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// tokenHandler issues a token for the e-mail and the password of a user, POST /token, in the shape of a token of
// OAuth 2. The logins of each client are limited by limiter, against guessing the passwords. A real application can
// use an identity provider instead, e.g. Keycloak, whose tokens jwtauth.JWKSValidator validates.
func tokenHandler(reg *registry.CRUD, provider *jwtauth.Provider, limiter gorestapi.RateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		problem := func(status int, code string, detail string) {
			jsonresp.WriteProblem(w, &jsonresp.Problem{
				Type: "about:blank", Title: http.StatusText(status), Status: status, Detail: detail, Instance: r.URL.Path, Code: code,
			})
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			problem(http.StatusMethodNotAllowed, gorestapi.CodeNotAllowed, "")
			return
		}
		allowed, retryAfter, err := limiter.Allow(r.Context(), gorestapi.ClientIPKey(gorestapi.ClientIP(r, nil)))
		if err == nil && !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(max(retryAfter, time.Second).Seconds()))))
			problem(http.StatusTooManyRequests, gorestapi.CodeTooManyRequests, "Too many logins, see the Retry-After header.")
			return
		}

		var credentials login
		if err := jsonreq.Decode(w, r, &credentials, jsonreq.Options{MaxBytes: 4096}); err != nil {
			problem(http.StatusBadRequest, jsonresp.CodeUnmarshalRequest, "The body is not a JSON object of an e-mail and a password.")
			return
		}
		obj, ok, err := reg.VerifyPasswords(r.Context(), "users", registry.Filters{Vals: map[string]string{"Email": credentials.Email}},
			map[string]string{"Password": credentials.Password})
		var filterErr *registry.FilterValidationError
		if err != nil && !errors.As(err, &filterErr) {
			problem(http.StatusInternalServerError, "SERVICE_ERROR", "")
			return
		}
		// the same problem for an unknown e-mail and a wrong password, so that it does not tell which users exist
		if !ok {
			problem(http.StatusUnauthorized, "INVALID_CREDENTIALS", "The e-mail or the password is not valid.")
			return
		}

		signed, _, _, expiresAt, err := provider.Issue(audience, strconv.FormatUint(obj.(*User).ID, 10), "member")
		if err != nil {
			slog.ErrorContext(r.Context(), "failed to issue a token", "error", err)
			problem(http.StatusInternalServerError, "SERVICE_ERROR", "")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		response := token{AccessToken: signed, TokenType: "Bearer", ExpiresIn: int(time.Until(*expiresAt).Seconds())}
		_ = json.NewEncoder(w).Encode(response) // #nosec G117 -- the token is the response of a login
	}
}
