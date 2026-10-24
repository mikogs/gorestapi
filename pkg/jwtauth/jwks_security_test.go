package jwtauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/golang-jwt/jwt/v5"
)

func TestJWKS_RequiresHTTPS(t *testing.T) {
	for _, uri := range []string{"http://auth.example.com/jwks.json", "ftp://auth.example.com/jwks.json", "auth.example.com/jwks.json"} {
		if _, err := ValidateWithJWKS(context.Background(), uri, testIssuer, testAudience, "token"); err == nil || !strings.Contains(err.Error(), "must use https") {
			t.Fatalf("ValidateWithJWKS with %s returned %v, want an error", uri, err)
		}
		if _, err := NewJWKSValidator(uri, testIssuer, testAudience); err == nil || !strings.Contains(err.Error(), "must use https") {
			t.Fatalf("NewJWKSValidator with %s returned %v, want an error", uri, err)
		}
	}

	for _, uri := range []string{"https://auth.example.com/jwks.json", "http://localhost:8080/jwks.json", "http://127.0.0.1/jwks.json", "http://[::1]/jwks.json"} {
		if _, err := NewJWKSValidator(uri, testIssuer, testAudience); err != nil {
			t.Fatalf("NewJWKSValidator with %s returned %v", uri, err)
		}
	}
}

func TestJWKS_RedirectToHTTP(t *testing.T) {
	key := mustRSAKey(t)
	target := serveJWKS(t, rsaJWK(key, "key"))
	redirect := httptest.NewServer(http.RedirectHandler(target, http.StatusFound))
	defer redirect.Close()

	_, err := ValidateWithJWKS(context.Background(), redirect.URL, testIssuer, testAudience, signToken(t, key, "key", testIssuer))
	if err == nil || !strings.Contains(err.Error(), "only https redirects are followed") {
		t.Fatalf("ValidateWithJWKS with a redirect to http returned %v, want an error", err)
	}
}

func TestJWKS_Timeout(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer server.Close()
	defer close(release)

	timeout := jwksClient.Timeout
	if timeout <= 0 {
		t.Fatalf("the timeout of fetching a JWKS is %v, want one", timeout)
	}
	jwksClient.Timeout = 100 * time.Millisecond
	defer func() { jwksClient.Timeout = timeout }()

	start := time.Now()
	_, err := ValidateWithJWKS(context.Background(), server.URL, testIssuer, testAudience, "token")
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("ValidateWithJWKS of a JWKS that does not answer returned %v after %v, want an error after the timeout", err, time.Since(start))
	}
}

func TestJWKS_ExpirationRequired(t *testing.T) {
	p := mustNewProvider(t)
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, CustomClaims{"member", jwt.RegisteredClaims{
		Issuer: testIssuer, Subject: "42", Audience: []string{testAudience}, IssuedAt: jwt.NewNumericDate(time.Now()),
	}})
	token.Header["kid"] = p.KeyID()
	withoutExpiration, err := token.SignedString(p.PrivateKey())
	if err != nil {
		t.Fatal(err)
	}

	if claims, ok := p.Parse(withoutExpiration, testAudience); ok {
		t.Fatalf("Parse of a token without an expiration time returned %+v, want it invalid", claims)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(p.JWKS()) }))
	defer server.Close()
	if claims, err := ValidateWithJWKS(context.Background(), server.URL, testIssuer, testAudience, withoutExpiration); err == nil {
		t.Fatalf("ValidateWithJWKS of a token without an expiration time returned %+v, want an error", claims)
	}
}

// keyServer serves a JWKS with keys that can be changed, and counts the requests.
type keyServer struct {
	mu      sync.Mutex
	keys    []jose.JSONWebKey
	status  int
	fetches atomic.Int32
}

func newKeyServer(t *testing.T, keys ...jose.JSONWebKey) (*keyServer, string) {
	t.Helper()

	s := &keyServer{keys: keys, status: http.StatusOK}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.fetches.Add(1)
		s.mu.Lock()
		defer s.mu.Unlock()
		w.WriteHeader(s.status)
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: s.keys})
	}))
	t.Cleanup(server.Close)
	return s, server.URL
}

func (s *keyServer) set(status int, keys ...jose.JSONWebKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status, s.keys = status, keys
}

func mustValidate(t *testing.T, v *JWKSValidator, token string) {
	t.Helper()
	if claims, err := v.Validate(context.Background(), token); err != nil || claims.Subject != "42" {
		t.Fatalf("Validate returned %+v, %v, want the claims", claims, err)
	}
}

func TestJWKSValidator_KeepsKeys(t *testing.T) {
	key := mustRSAKey(t)
	server, uri := newKeyServer(t, rsaJWK(key, "key"))
	v, err := NewJWKSValidator(uri, testIssuer, testAudience)
	if err != nil {
		t.Fatal(err)
	}

	for range 3 {
		mustValidate(t, v, signToken(t, key, "key", testIssuer))
	}
	if n := server.fetches.Load(); n != 1 {
		t.Fatalf("3 validations fetched the JWKS %d times, want once", n)
	}

	v.cacheTTL = 50 * time.Millisecond
	time.Sleep(60 * time.Millisecond)
	mustValidate(t, v, signToken(t, key, "key", testIssuer))
	if n := server.fetches.Load(); n != 2 {
		t.Fatalf("a validation after the cache TTL fetched the JWKS %d times in all, want twice", n)
	}

	server.set(http.StatusInternalServerError)
	time.Sleep(60 * time.Millisecond)
	if _, err := v.Validate(context.Background(), signToken(t, key, "key", testIssuer)); err == nil || !strings.Contains(err.Error(), "returned 500") {
		t.Fatalf("Validate with a JWKS that cannot be fetched returned %v, want an error", err)
	}
}

func TestJWKSValidator_NewKey(t *testing.T) {
	oldKey, newKey := mustRSAKey(t), mustRSAKey(t)
	server, uri := newKeyServer(t, rsaJWK(oldKey, "old"))
	v, err := NewJWKSValidator(uri, testIssuer, testAudience)
	if err != nil {
		t.Fatal(err)
	}
	mustValidate(t, v, signToken(t, oldKey, "old", testIssuer))
	server.set(http.StatusOK, rsaJWK(oldKey, "old"), rsaJWK(newKey, "new"))

	if _, err := v.Validate(context.Background(), signToken(t, newKey, "new", testIssuer)); err == nil || !strings.Contains(err.Error(), `JWKS has no key with kid "new"`) {
		t.Fatalf("Validate of a token with a new kid soon after a fetch returned %v, want an error", err)
	}
	if n := server.fetches.Load(); n != 1 {
		t.Fatalf("the JWKS was fetched %d times, want once, as the last fetch is too recent", n)
	}

	v.minRefresh = 0
	mustValidate(t, v, signToken(t, newKey, "new", testIssuer))
	mustValidate(t, v, signToken(t, oldKey, "old", testIssuer))
	if n := server.fetches.Load(); n != 2 {
		t.Fatalf("the JWKS was fetched %d times, want twice, once more for the new kid", n)
	}
}

func TestJWKSValidator_Concurrent(t *testing.T) {
	key := mustRSAKey(t)
	server, uri := newKeyServer(t, rsaJWK(key, "key"))
	v, err := NewJWKSValidator(uri, testIssuer, testAudience)
	if err != nil {
		t.Fatal(err)
	}
	token := signToken(t, key, "key", testIssuer)

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 10 {
				if _, err := v.Validate(context.Background(), token); err != nil {
					t.Errorf("Validate failed: %s", err.Error())
					return
				}
			}
		})
	}
	wg.Wait()

	if n := server.fetches.Load(); n != 1 {
		t.Fatalf("80 validations on goroutines fetched the JWKS %d times, want once", n)
	}
}

func TestJWKS_ErrorsWithoutPassword(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()

	for _, uri := range []string{
		strings.Replace(closed.URL, "http://", "http://user:secret-password@", 1),
		"https://user:secret-password@auth.example.com/%zz",
	} {
		_, err := ValidateWithJWKS(context.Background(), uri, testIssuer, testAudience, "token")
		if err == nil || strings.Contains(err.Error(), "secret-password") {
			t.Fatalf("ValidateWithJWKS with %s returned %v, want an error without the password", uri, err)
		}
	}
}
