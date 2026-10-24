package jwtauth

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/golang-jwt/jwt/v5"
)

const jwksMaxBytes = 1 << 17 // 128 KiB — ample for any real JWKS document

const (
	// DefaultJWKSCacheTTL is how long a JWKSValidator keeps the keys of the JWKS document before it fetches them again.
	DefaultJWKSCacheTTL = 5 * time.Minute
	// DefaultJWKSMinRefresh is how long a JWKSValidator waits after a fetch before it fetches the keys again for a token with
	// a kid that is not in them, e.g. after the issuer added a new key.
	DefaultJWKSMinRefresh = time.Minute
)

// jwksClient fetches JWKS documents, with a timeout, and only follows redirects to https.
var jwksClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("JWKS redirect to %s: only https redirects are followed", req.URL.Redacted())
		}
		if len(via) >= 10 {
			return errors.New("JWKS: stopped after 10 redirects")
		}
		return nil
	},
}

// ValidateWithJWKS fetches the JWKS document at jwksURI, and verifies bearerToken against the RSA key whose "kid"
// is the "kid" in the header of the token. A token without a "kid" is verified against the only key of a JWKS that has
// one key. Use this on the consumer side when you do not hold a local Provider — only the remote issuer's JWKS URL is known.
// The JWKS document is fetched on every call, so use NewJWKSValidator to validate the tokens of requests.
func ValidateWithJWKS(ctx context.Context, jwksURI, issuer, audience, bearerToken string) (*CustomClaims, error) {
	if err := checkJWKSURI(jwksURI); err != nil {
		return nil, err
	}

	keySet, err := fetchJWKS(ctx, jwksURI)
	if err != nil {
		return nil, err
	}

	publicKey, err := keyForToken(keySet, bearerToken)
	if err != nil {
		return nil, err
	}

	return validateToken(bearerToken, issuer, audience, publicKey)
}

// JWKSValidator validates the tokens of a remote issuer with the keys of its JWKS document, which it keeps for
// DefaultJWKSCacheTTL, so that a request does not fetch them. It fetches them again earlier for a token with a kid
// that is not in them, at most once in DefaultJWKSMinRefresh. It is safe for concurrent use.
type JWKSValidator struct {
	uri      string
	issuer   string
	audience string

	cacheTTL   time.Duration
	minRefresh time.Duration

	mu        sync.Mutex
	keySet    jose.JSONWebKeySet
	fetchedAt time.Time
}

// NewJWKSValidator returns a JWKSValidator for the tokens of issuer for audience, whose keys are at jwksURI. The URI has to use https,
// or http for localhost, e.g. in tests.
func NewJWKSValidator(jwksURI, issuer, audience string) (*JWKSValidator, error) {
	if err := checkJWKSURI(jwksURI); err != nil {
		return nil, err
	}

	return &JWKSValidator{
		uri:        jwksURI,
		issuer:     issuer,
		audience:   audience,
		cacheTTL:   DefaultJWKSCacheTTL,
		minRefresh: DefaultJWKSMinRefresh,
	}, nil
}

// Validate returns the claims of bearerToken when it is valid, as ValidateWithJWKS, with the kept keys.
func (j *JWKSValidator) Validate(ctx context.Context, bearerToken string) (*CustomClaims, error) {
	publicKey, err := j.key(ctx, bearerToken)
	if err != nil {
		return nil, err
	}

	return validateToken(bearerToken, j.issuer, j.audience, publicKey)
}

// key returns the key of the token, from the kept keys, which it fetches when they are older than the cache TTL, or
// when they do not have the key of the token and are older than the minimum time between refreshes.
func (j *JWKSValidator) key(ctx context.Context, bearerToken string) (*rsa.PublicKey, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.fetchedAt.IsZero() || time.Since(j.fetchedAt) >= j.cacheTTL {
		if err := j.fetch(ctx); err != nil {
			return nil, err
		}
	}

	publicKey, err := keyForToken(j.keySet, bearerToken)
	if errors.Is(err, errUnknownKeyID) && time.Since(j.fetchedAt) >= j.minRefresh {
		if err := j.fetch(ctx); err != nil {
			return nil, err
		}
		publicKey, err = keyForToken(j.keySet, bearerToken)
	}
	return publicKey, err
}

func (j *JWKSValidator) fetch(ctx context.Context) error {
	keySet, err := fetchJWKS(ctx, j.uri)
	if err != nil {
		return err
	}

	j.keySet, j.fetchedAt = keySet, time.Now()
	return nil
}

// checkJWKSURI checks that the keys are fetched with https, so that they cannot be replaced on the way, or with http
// from the same machine, e.g. in tests.
func checkJWKSURI(jwksURI string) error {
	// the error of url.Parse has the URI, which can have a password
	u, err := url.Parse(jwksURI)
	if err != nil {
		return errors.New("JWKS URI is not a valid URL")
	}

	host := u.Hostname()
	if u.Scheme == "https" || (u.Scheme == "http" && (host == "localhost" || net.ParseIP(host).IsLoopback())) {
		return nil
	}
	return fmt.Errorf("JWKS URI %s: must use https, or http for localhost", u.Redacted())
}

func fetchJWKS(ctx context.Context, jwksURI string) (jose.JSONWebKeySet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURI, nil)
	if err != nil {
		return jose.JSONWebKeySet{}, fmt.Errorf("build JWKS request: %w", err)
	}

	// the errors have the URI without its password, as the errors of http.Client, so that it is not logged
	resp, err := jwksClient.Do(req)
	if err != nil {
		return jose.JSONWebKeySet{}, fmt.Errorf("fetch JWKS from %s: %w", req.URL.Redacted(), err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return jose.JSONWebKeySet{}, fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode)
	}

	var keySet jose.JSONWebKeySet
	if err := json.NewDecoder(io.LimitReader(resp.Body, jwksMaxBytes)).Decode(&keySet); err != nil {
		return jose.JSONWebKeySet{}, fmt.Errorf("parse JWKS: %w", err)
	}

	if len(keySet.Keys) == 0 {
		return jose.JSONWebKeySet{}, errors.New("JWKS contains no keys")
	}
	return keySet, nil
}

// validateToken returns the claims of bearerToken when it is valid for publicKey.
func validateToken(bearerToken, issuer, audience string, publicKey *rsa.PublicKey) (*CustomClaims, error) {
	claims, valid := parseToken(bearerToken, issuer, audience, publicKey)
	if !valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

var errUnknownKeyID = errors.New("JWKS has no key with kid")

// keyForToken returns the RSA key of keySet that verifies bearerToken: the key with the "kid" of the token, or the
// only key when the token does not have a "kid". The signature is not verified here, only the header is read.
func keyForToken(keySet jose.JSONWebKeySet, bearerToken string) (*rsa.PublicKey, error) {
	token, _, err := jwt.NewParser().ParseUnverified(bearerToken, &CustomClaims{})
	if err != nil {
		return nil, errors.New("invalid token")
	}

	keyID, _ := token.Header["kid"].(string)
	if keyID == "" {
		if len(keySet.Keys) != 1 {
			return nil, errors.New("token has no kid, and the JWKS has more than one key")
		}
		return rsaPublicKey(keySet.Keys[0])
	}

	keys := keySet.Key(keyID)
	if len(keys) == 0 {
		return nil, fmt.Errorf("%w %q", errUnknownKeyID, keyID)
	}

	return rsaPublicKey(keys[0])
}

func rsaPublicKey(key jose.JSONWebKey) (*rsa.PublicKey, error) {
	publicKey, ok := key.Public().Key.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("JWKS key %q is not an RSA public key", key.KeyID)
	}

	return publicKey, nil
}
