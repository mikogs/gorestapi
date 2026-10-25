package gorestapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mikogs/gocrud"

	"github.com/mikogs/gorestapi/internal/fixtures"
	"github.com/mikogs/gorestapi/pkg/jwtauth"
)

func TestAuthenticate(t *testing.T) {
	recreateTestStructTable()
	key, err := jwtauth.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %s", err.Error())
	}
	provider, err := jwtauth.NewProvider("https://auth.example.com", key)
	if err != nil {
		t.Fatalf("NewProvider failed: %s", err.Error())
	}
	token, _, _, _, err := provider.Issue("api", "user-1", "member")
	if err != nil {
		t.Fatalf("Issue failed: %s", err.Error())
	}

	var principals []any
	recordPrincipal := func(obj any, r *http.Request) error {
		principals = append(principals, Principal(r.Context()))
		return nil
	}
	h := New(testService, Options{
		Authenticate: Bearer(func(_ context.Context, token string) (any, error) {
			claims, ok := provider.Parse(token, "api")
			if !ok {
				return nil, errors.New("invalid token")
			}
			return claims, nil
		}),
		Routes: map[string]Route{
			"teststruct": {PostListItem: recordPrincipal},
			"public":     {RegistryKey: "teststruct", Anonymous: true, PostListItem: recordPrincipal},
		},
	})
	if err := testCRUD.Save(context.Background(), fixtures.TestStructWithData(), gocrud.SaveOptions{}); err != nil {
		t.Fatalf("Save failed: %s", err.Error())
	}

	for name, test := range map[string]struct {
		method, path, authorization string
		status                      int
		challenge                   string
	}{
		"no token":                 {http.MethodGet, "/teststruct/", "", http.StatusUnauthorized, "Bearer"},
		"a token in the query":     {http.MethodGet, "/teststruct/?access_token=" + token, "", http.StatusUnauthorized, "Bearer"},
		"another scheme":           {http.MethodGet, "/teststruct/", "Basic dXNlcjpwYXNz", http.StatusUnauthorized, "Bearer"},
		"an invalid token":         {http.MethodGet, "/teststruct/", "Bearer " + token + "x", http.StatusUnauthorized, `Bearer error="invalid_token"`},
		"a valid token":            {http.MethodGet, "/teststruct/", "Bearer " + token, http.StatusOK, ""},
		"the scheme in lower case": {http.MethodGet, "/teststruct/", "bearer " + token, http.StatusOK, ""},
		"anonymous":                {http.MethodGet, "/public/", "", http.StatusOK, ""},
		"anonymous, invalid":       {http.MethodGet, "/public/", "Bearer x", http.StatusUnauthorized, `Bearer error="invalid_token"`},
		"OPTIONS":                  {http.MethodOptions, "/teststruct/", "", http.StatusNoContent, ""},
	} {
		req := httptest.NewRequest(test.method, test.path, nil)
		if test.authorization != "" {
			req.Header.Set("Authorization", test.authorization)
		}
		w := httptest.NewRecorder()
		h.Serve(w, req)
		if w.Code != test.status || w.Header().Get("WWW-Authenticate") != test.challenge {
			t.Fatalf("%s returned %d with WWW-Authenticate %q, want %d with %q", name, w.Code, w.Header().Get("WWW-Authenticate"), test.status, test.challenge)
		}
		if w.Code == http.StatusUnauthorized && !strings.Contains(w.Body.String(), `"code":"`+CodeUnauthorized+`"`) {
			t.Fatalf("%s returned %s, want the problem UNAUTHORIZED", name, w.Body.String())
		}
	}

	// the hooks get the claims of the token, and nil for an anonymous request
	principals = nil
	for _, path := range []string{"/teststruct/", "/public/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if path == "/teststruct/" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		h.Serve(httptest.NewRecorder(), req)
	}
	if len(principals) != 2 || principals[0].(*jwtauth.CustomClaims).Subject != "user-1" || principals[1] != nil {
		t.Fatalf("the hooks got the principals %v, want the claims of user-1, and nil", principals)
	}
}

func TestBearerToken(t *testing.T) {
	for header, want := range map[string]string{"Bearer abc": "abc", "bearer  abc ": "abc", "Bearer": "", "Bearer ": "", "Basic abc": "", "": ""} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", header)
		if got, ok := BearerToken(req); got != want || ok != (want != "") {
			t.Fatalf("BearerToken of %q returned %q, %t, want %q", header, got, ok, want)
		}
	}
}
