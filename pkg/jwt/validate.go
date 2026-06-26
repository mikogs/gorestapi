package jwt

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-jose/go-jose/v3"
)

const jwksMaxBytes = 1 << 17 // 128 KiB — ample for any real JWKS document

// ValidateWithJWKS fetches the JWKS document at jwksURI, extracts the first
// RSA public key, and verifies bearerToken against it. Use this on the
// consumer side when you do not hold a local Provider — only the remote
// issuer's JWKS URL is known.
func ValidateWithJWKS(ctx context.Context, jwksURI, issuer, audience, bearerToken string) (*CustomClaims, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURI, nil)
	if err != nil {
		return nil, fmt.Errorf("build JWKS request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch JWKS from %s: %w", jwksURI, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode)
	}

	var keySet jose.JSONWebKeySet
	if err := json.NewDecoder(io.LimitReader(resp.Body, jwksMaxBytes)).Decode(&keySet); err != nil {
		return nil, fmt.Errorf("parse JWKS: %w", err)
	}

	if len(keySet.Keys) == 0 {
		return nil, errors.New("JWKS contains no keys")
	}

	publicKey, ok := keySet.Keys[0].Public().Key.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("JWKS key is not an RSA public key")
	}

	claims, valid := parseToken(bearerToken, issuer, audience, publicKey)
	if !valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}
