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
	jwks       []byte
}

func NewProvider(issuer string) (*Provider, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	publicKey := &privateKey.PublicKey

	jwks, err := JWKS(publicKey)
	if err != nil {
		return nil, errors.New("failed to generate JWKS")
	}

	jwtProvider := &Provider{
		issuer:     issuer,
		privateKey: privateKey,
		publicKey:  publicKey,
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
