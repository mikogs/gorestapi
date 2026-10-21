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
	"github.com/golang-jwt/jwt/v5"
)

const jwksMaxBytes = 1 << 17 // 128 KiB — ample for any real JWKS document

// ValidateWithJWKS fetches the JWKS document at jwksURI, and verifies bearerToken against the RSA key whose "kid"
// is the "kid" in the header of the token. A token without a "kid" is verified against the only key of a JWKS that has
// one key. Use this on the consumer side when you do not hold a local Provider — only the remote issuer's JWKS URL is known.
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

	publicKey, err := keyForToken(keySet, bearerToken)
	if err != nil {
		return nil, err
	}

	claims, valid := parseToken(bearerToken, issuer, audience, publicKey)
	if !valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

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
		return nil, fmt.Errorf("JWKS has no key with kid %q", keyID)
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
