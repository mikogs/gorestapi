package jwt

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"

	"github.com/go-jose/go-jose/v3"
)

func JWKS(publicKey *rsa.PublicKey) ([]byte, error) {
	jsonWebKey := jose.JSONWebKey{
		Key:       publicKey,
		Algorithm: string(jose.RS256),
		Use:       "sig",
		KeyID:     "",
	}

	container := struct {
		Keys []jose.JSONWebKey `json:"keys"`
	}{Keys: []jose.JSONWebKey{jsonWebKey}}

	pretty, err := json.MarshalIndent(container, "", "  ")
	if err != nil {
		return []byte{}, fmt.Errorf("marshal JWKS: %w", err)
	}

	return pretty, nil
}
