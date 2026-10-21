package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
)

type Provider struct {
	issuer     string
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	keyID      string
	jwks       []byte
}

func NewProvider(issuer string) (*Provider, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	publicKey := &privateKey.PublicKey

	keyID, err := KeyID(publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key ID: %w", err)
	}

	jwks, err := JWKS(publicKey)
	if err != nil {
		return nil, errors.New("failed to generate JWKS")
	}

	jwtProvider := &Provider{
		issuer:     issuer,
		privateKey: privateKey,
		publicKey:  publicKey,
		keyID:      keyID,
		jwks:       jwks,
	}
	return jwtProvider, nil
}

func (j *Provider) Issuer() string {
	return j.issuer
}

func (j *Provider) PrivateKey() *rsa.PrivateKey {
	return j.privateKey
}

func (j *Provider) PublicKey() *rsa.PublicKey {
	return j.publicKey
}

func (j *Provider) JWKS() []byte {
	return j.jwks
}

// KeyID returns the ID of the key of the provider, which is the "kid" of its tokens and of its JWKS key.
func (j *Provider) KeyID() string {
	return j.keyID
}
