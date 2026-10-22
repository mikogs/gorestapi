package jwt

import (
	"crypto"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/go-jose/go-jose/v3"
)

// KeyID returns the ID of publicKey: its RFC 7638 thumbprint, a base64url-encoded SHA-256 hash of the key.
// The same key always has the same ID.
func KeyID(publicKey *rsa.PublicKey) (string, error) {
	jsonWebKey := jose.JSONWebKey{Key: publicKey}
	thumbprint, err := jsonWebKey.Thumbprint(crypto.SHA256)
	if err != nil {
		return "", fmt.Errorf("key thumbprint: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(thumbprint), nil
}

// JWKS returns a JWKS document with the public keys, in their order, each with KeyID(key) as its "kid".
func JWKS(publicKeys ...*rsa.PublicKey) ([]byte, error) {
	jsonWebKeys := make([]jose.JSONWebKey, 0, len(publicKeys))
	for _, publicKey := range publicKeys {
		keyID, err := KeyID(publicKey)
		if err != nil {
			return []byte{}, err
		}

		jsonWebKeys = append(jsonWebKeys, jose.JSONWebKey{
			Key:       publicKey,
			Algorithm: string(jose.RS256),
			Use:       "sig",
			KeyID:     keyID,
		})
	}

	container := struct {
		Keys []jose.JSONWebKey `json:"keys"`
	}{Keys: jsonWebKeys}

	pretty, err := json.MarshalIndent(container, "", "  ")
	if err != nil {
		return []byte{}, fmt.Errorf("marshal JWKS: %w", err)
	}

	return pretty, nil
}
