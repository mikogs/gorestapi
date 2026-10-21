package jwt

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/golang-jwt/jwt/v5"
)

const (
	testIssuer   = "https://auth.example.com"
	testAudience = "myapp"
)

func mustNewProvider(t *testing.T) *Provider {
	t.Helper()

	p, err := NewProvider(testIssuer)
	if err != nil {
		t.Fatalf("NewProvider failed: %s", err.Error())
	}
	return p
}

func mustRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey failed: %s", err.Error())
	}
	return key
}

// signToken signs a token for testIssuer and testAudience with key, with the kid header when it is not empty.
func signToken(t *testing.T, key *rsa.PrivateKey, keyID string, issuer string) string {
	t.Helper()

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, CustomClaims{"member", jwt.RegisteredClaims{
		Issuer:    issuer,
		Subject:   "42",
		Audience:  []string{testAudience},
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(now),
	}})
	if keyID != "" {
		token.Header["kid"] = keyID
	}

	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("SignedString failed: %s", err.Error())
	}
	return signed
}

func serveJWKS(t *testing.T, keys ...jose.JSONWebKey) string {
	t.Helper()

	data, err := json.Marshal(jose.JSONWebKeySet{Keys: keys})
	if err != nil {
		t.Fatalf("marshal JWKS failed: %s", err.Error())
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(data)
	}))
	t.Cleanup(server.Close)

	return server.URL
}

func rsaJWK(key *rsa.PrivateKey, keyID string) jose.JSONWebKey {
	return jose.JSONWebKey{Key: &key.PublicKey, KeyID: keyID, Algorithm: string(jose.RS256), Use: "sig"}
}

func TestProvider_IssueAndParse(t *testing.T) {
	p := mustNewProvider(t)

	signed, jti, issuedAt, expiresAt, err := p.Issue(testAudience, "42", "admin")
	if err != nil {
		t.Fatalf("Issue failed: %s", err.Error())
	}
	if jti == "" || expiresAt.Sub(*issuedAt) != tokenTTL {
		t.Fatalf("Issue returned jti %q, issued at %v and expiring at %v", jti, issuedAt, expiresAt)
	}

	token, _, err := jwt.NewParser().ParseUnverified(signed, &CustomClaims{})
	if err != nil || token.Header["kid"] != p.KeyID() || p.KeyID() == "" {
		t.Fatalf("Issue returned a token with kid %v, want %q", token.Header["kid"], p.KeyID())
	}

	claims, ok := p.Parse(signed, testAudience)
	if !ok || claims.Subject != "42" || claims.Role != "admin" || claims.ID != jti {
		t.Fatalf("Parse returned %+v, %v", claims, ok)
	}

	if _, ok := p.Parse(signed, "otherapp"); ok {
		t.Fatal("Parse accepted a token for another audience")
	}
	if _, ok := mustNewProvider(t).Parse(signed, testAudience); ok {
		t.Fatal("Parse accepted a token signed with another key")
	}
}

func TestJWKS_KeyID(t *testing.T) {
	p := mustNewProvider(t)

	var keySet jose.JSONWebKeySet
	if err := json.Unmarshal(p.JWKS(), &keySet); err != nil {
		t.Fatalf("Unmarshal failed: %s", err.Error())
	}
	if len(keySet.Keys) != 1 || keySet.Keys[0].KeyID != p.KeyID() || keySet.Keys[0].Use != "sig" {
		t.Fatalf("JWKS returned keys %+v, want one key with kid %q", keySet.Keys, p.KeyID())
	}

	again, _ := KeyID(p.PublicKey())
	if again != p.KeyID() {
		t.Fatalf("KeyID returned %q for the same key, want %q", again, p.KeyID())
	}
	other, _ := KeyID(&mustRSAKey(t).PublicKey)
	if other == p.KeyID() {
		t.Fatal("KeyID returned the same ID for another key")
	}
}

func TestValidateWithJWKS(t *testing.T) {
	ctx := context.Background()
	oldKey, newKey := mustRSAKey(t), mustRSAKey(t)
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %s", err.Error())
	}

	p := mustNewProvider(t)
	providerToken, _, _, _, _ := p.Issue(testAudience, "42", "admin")
	providerJWKS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(p.JWKS())
	}))
	defer providerJWKS.Close()

	rotation := serveJWKS(t, rsaJWK(oldKey, "old"), rsaJWK(newKey, "new"), jose.JSONWebKey{Key: &ecKey.PublicKey, KeyID: "ec"})
	single := serveJWKS(t, rsaJWK(newKey, "new"))

	valid := map[string][2]string{
		"token of a provider":                {providerJWKS.URL, providerToken},
		"token signed with the first key":    {rotation, signToken(t, oldKey, "old", testIssuer)},
		"token signed with the second key":   {rotation, signToken(t, newKey, "new", testIssuer)},
		"token without kid and a single key": {single, signToken(t, newKey, "", testIssuer)},
		"token with kid and a single key":    {single, signToken(t, newKey, "new", testIssuer)},
	}
	for name, test := range valid {
		t.Run(name, func(t *testing.T) {
			claims, err := ValidateWithJWKS(ctx, test[0], testIssuer, testAudience, test[1])
			if err != nil || claims.Subject != "42" {
				t.Fatalf("ValidateWithJWKS returned %+v, %v, want the claims", claims, err)
			}
		})
	}

	invalid := map[string]struct {
		uri, token, err string
	}{
		"kid that is not in the JWKS":           {rotation, signToken(t, newKey, "unknown", testIssuer), `JWKS has no key with kid "unknown"`},
		"kid of a key that is not RSA":          {rotation, signToken(t, newKey, "ec", testIssuer), `JWKS key "ec" is not an RSA public key`},
		"token without kid and several keys":    {rotation, signToken(t, newKey, "", testIssuer), "token has no kid, and the JWKS has more than one key"},
		"kid of another key than the signature": {rotation, signToken(t, oldKey, "new", testIssuer), "invalid token"},
		"wrong issuer":                          {rotation, signToken(t, newKey, "new", "https://evil.example.com"), "invalid token"},
		"not a token":                           {rotation, "not-a-token", "invalid token"},
	}
	for name, test := range invalid {
		t.Run(name, func(t *testing.T) {
			claims, err := ValidateWithJWKS(ctx, test.uri, testIssuer, testAudience, test.token)
			if err == nil || !strings.Contains(err.Error(), test.err) || claims != nil {
				t.Fatalf("ValidateWithJWKS returned %+v, %v, want err %q", claims, err, test.err)
			}
		})
	}
}
