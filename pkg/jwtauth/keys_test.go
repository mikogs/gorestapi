package jwtauth

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/golang-jwt/jwt/v5"
)

func pemBlock(blockType string, data []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: data})
}

func TestParseKeyPEM(t *testing.T) {
	key := mustRSAKey(t)
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(key)
	pkix, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)

	for name, data := range map[string][]byte{
		"PKCS #1": pemBlock("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key)),
		"PKCS #8": pemBlock("PRIVATE KEY", pkcs8),
	} {
		parsed, err := ParsePrivateKeyPEM(data)
		if err != nil || !parsed.Equal(key) {
			t.Fatalf("ParsePrivateKeyPEM of %s returned %v, want the key", name, err)
		}
	}

	for name, data := range map[string][]byte{
		"PKIX":    pemBlock("PUBLIC KEY", pkix),
		"PKCS #1": pemBlock("RSA PUBLIC KEY", x509.MarshalPKCS1PublicKey(&key.PublicKey)),
	} {
		parsed, err := ParsePublicKeyPEM(data)
		if err != nil || !parsed.Equal(&key.PublicKey) {
			t.Fatalf("ParsePublicKeyPEM of %s returned %v, want the key", name, err)
		}
	}

	small, _ := rsa.GenerateKey(rand.Reader, 1024) // #nosec G403 -- a key that is too small, which has to be rejected
	ecKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ecPKCS8, _ := x509.MarshalPKCS8PrivateKey(ecKey)
	ecPKIX, _ := x509.MarshalPKIXPublicKey(&ecKey.PublicKey)
	smallPKIX, _ := x509.MarshalPKIXPublicKey(&small.PublicKey)

	invalidPrivate := map[string]struct {
		data []byte
		err  string
	}{
		"not PEM":            {[]byte("secret"), "no PEM data found"},
		"a public key":       {pemBlock("PUBLIC KEY", pkix), `PEM block "PUBLIC KEY" is not a private key`},
		"an ECDSA key":       {pemBlock("PRIVATE KEY", ecPKCS8), "not an RSA key"},
		"a key of 1024 bits": {pemBlock("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(small)), "RSA key has 1024 bits, at least 2048 are needed"},
		"broken data":        {pemBlock("RSA PRIVATE KEY", []byte("broken")), "parse PKCS #1 private key"},
	}
	for name, test := range invalidPrivate {
		if _, err := ParsePrivateKeyPEM(test.data); err == nil || !strings.Contains(err.Error(), test.err) {
			t.Fatalf("ParsePrivateKeyPEM of %s returned %v, want an error with %q", name, err, test.err)
		}
	}

	invalidPublic := map[string]struct {
		data []byte
		err  string
	}{
		"a private key":      {pemBlock("PRIVATE KEY", pkcs8), `PEM block "PRIVATE KEY" is not a public key`},
		"an ECDSA key":       {pemBlock("PUBLIC KEY", ecPKIX), "not an RSA key"},
		"a key of 1024 bits": {pemBlock("PUBLIC KEY", smallPKIX), "RSA key has 1024 bits"},
	}
	for name, test := range invalidPublic {
		if _, err := ParsePublicKeyPEM(test.data); err == nil || !strings.Contains(err.Error(), test.err) {
			t.Fatalf("ParsePublicKeyPEM of %s returned %v, want an error with %q", name, err, test.err)
		}
	}
}

func TestNewProvider_InvalidOptions(t *testing.T) {
	small, _ := rsa.GenerateKey(rand.Reader, 1024) // #nosec G403 -- a key that is too small, which has to be rejected
	key := mustRSAKey(t)

	tests := map[string]struct {
		key     *rsa.PrivateKey
		options []Option
		err     string
	}{
		"no key":                     {nil, nil, "signing key is nil"},
		"a small key":                {small, nil, "RSA key has 1024 bits"},
		"a small verification key":   {key, []Option{WithVerificationKeys(&small.PublicKey)}, "RSA key has 1024 bits"},
		"a TTL that is not positive": {key, []Option{WithTokenTTL(0)}, "token TTL must be positive"},
	}
	for name, test := range tests {
		if _, err := NewProvider(testIssuer, test.key, test.options...); err == nil || !strings.Contains(err.Error(), test.err) {
			t.Fatalf("NewProvider with %s returned %v, want an error with %q", name, err, test.err)
		}
	}
}

func TestNewProvider_SameKey(t *testing.T) {
	data := pemBlock("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(mustRSAKey(t)))

	newProvider := func() *Provider {
		key, err := ParsePrivateKeyPEM(data)
		if err != nil {
			t.Fatalf("ParsePrivateKeyPEM failed: %s", err.Error())
		}
		p, err := NewProvider(testIssuer, key)
		if err != nil {
			t.Fatalf("NewProvider failed: %s", err.Error())
		}
		return p
	}

	// two instances of an application, or an application before and after a restart
	first, second := newProvider(), newProvider()

	signed, _, _, _, err := first.Issue(testAudience, "42", "admin")
	if err != nil {
		t.Fatalf("Issue failed: %s", err.Error())
	}
	if claims, ok := second.Parse(signed, testAudience); !ok || claims.Subject != "42" {
		t.Fatal("Parse rejected a token issued by a provider with the same key")
	}
	if !bytes.Equal(first.JWKS(), second.JWKS()) || first.KeyID() != second.KeyID() {
		t.Fatal("providers with the same key returned different JWKS documents or key IDs")
	}
}

func TestNewProvider_KeyRotation(t *testing.T) {
	ctx := context.Background()
	oldKey, newKey := mustRSAKey(t), mustRSAKey(t)

	before, _ := NewProvider(testIssuer, oldKey)
	oldToken, _, _, _, _ := before.Issue(testAudience, "1", "member")

	after, err := NewProvider(testIssuer, newKey, WithVerificationKeys(&oldKey.PublicKey))
	if err != nil {
		t.Fatalf("NewProvider failed: %s", err.Error())
	}
	newToken, _, _, _, _ := after.Issue(testAudience, "2", "member")

	for name, token := range map[string]string{"old": oldToken, "new": newToken} {
		if _, ok := after.Parse(token, testAudience); !ok {
			t.Fatalf("Parse after the rotation rejected the %s token", name)
		}
	}

	parsed, _, _ := jwt.NewParser().ParseUnverified(newToken, &CustomClaims{})
	if parsed.Header["kid"] != after.KeyID() || after.KeyID() == before.KeyID() {
		t.Fatalf("the new token has kid %v, want the kid of the new key %s", parsed.Header["kid"], after.KeyID())
	}

	var keySet jose.JSONWebKeySet
	if err := json.Unmarshal(after.JWKS(), &keySet); err != nil {
		t.Fatalf("Unmarshal failed: %s", err.Error())
	}
	if len(keySet.Keys) != 2 || keySet.Keys[0].KeyID != after.KeyID() || keySet.Keys[1].KeyID != before.KeyID() {
		t.Fatalf("JWKS returned %+v, want the new key, and then the old one", keySet.Keys)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(after.JWKS())
	}))
	defer server.Close()
	for name, token := range map[string]string{"old": oldToken, "new": newToken} {
		if _, err := ValidateWithJWKS(ctx, server.URL, testIssuer, testAudience, token); err != nil {
			t.Fatalf("ValidateWithJWKS of the %s token returned %v, want the claims", name, err)
		}
	}

	retired, _ := NewProvider(testIssuer, newKey)
	if _, ok := retired.Parse(oldToken, testAudience); ok {
		t.Fatal("Parse accepted a token of a key that is no longer a verification key")
	}
}

func TestProvider_TokenTTL(t *testing.T) {
	p, err := NewProvider(testIssuer, mustRSAKey(t), WithTokenTTL(15*time.Minute))
	if err != nil {
		t.Fatalf("NewProvider failed: %s", err.Error())
	}

	_, _, issuedAt, expiresAt, err := p.Issue(testAudience, "42", "admin")
	if err != nil || expiresAt.Sub(*issuedAt) != 15*time.Minute || p.TokenTTL() != 15*time.Minute {
		t.Fatalf("Issue returned a token issued at %v and expiring at %v, %v, want it valid for 15 minutes", issuedAt, expiresAt, err)
	}
}

func TestProvider_ParseByKeyID(t *testing.T) {
	p := mustNewProvider(t)

	tests := map[string]string{
		"without kid":  "",
		"unknown kid":  "unknown",
		"kid of a key": p.KeyID(),
	}
	for name, keyID := range tests {
		token := signToken(t, p.PrivateKey(), keyID, testIssuer)
		if _, ok := p.Parse(token, testAudience); ok != (name == "kid of a key") {
			t.Fatalf("Parse of a token %s returned %v", name, ok)
		}
	}
}
