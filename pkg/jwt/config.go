package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"
)

// MinKeyBits is the minimum size of an RSA key.
const MinKeyBits = 2048

// DefaultTokenTTL is the time for which the tokens of a Provider are valid, unless WithTokenTTL sets another one.
const DefaultTokenTTL = time.Hour

type Provider struct {
	issuer           string
	privateKey       *rsa.PrivateKey
	keyID            string
	verificationKeys map[string]*rsa.PublicKey
	// publicKeys are the keys of the JWKS: the signing key, followed by the verification keys in their order.
	publicKeys []*rsa.PublicKey
	tokenTTL   time.Duration
	jwks       []byte
}

// Option configures a Provider.
type Option func(*Provider) error

// WithVerificationKeys adds keys that verify tokens, but do not sign them, e.g. the previous key after a key
// rotation, so that the tokens that it signed are valid until they expire. They are published in the JWKS too.
func WithVerificationKeys(keys ...*rsa.PublicKey) Option {
	return func(p *Provider) error {
		for _, key := range keys {
			if err := checkKeySize(key); err != nil {
				return err
			}

			keyID, err := KeyID(key)
			if err != nil {
				return err
			}
			if _, ok := p.verificationKeys[keyID]; !ok {
				p.verificationKeys[keyID] = key
				p.publicKeys = append(p.publicKeys, key)
			}
		}
		return nil
	}
}

// WithTokenTTL sets the time for which the issued tokens are valid.
func WithTokenTTL(ttl time.Duration) Option {
	return func(p *Provider) error {
		if ttl <= 0 {
			return fmt.Errorf("token TTL must be positive, got %s", ttl)
		}
		p.tokenTTL = ttl
		return nil
	}
}

// NewProvider returns a Provider that signs tokens with signingKey, and verifies them with it and with the keys of
// WithVerificationKeys. Load the key from a secret, e.g. with ParsePrivateKeyPEM, so that all the instances of the
// application, and the application after a restart, use the same key.
func NewProvider(issuer string, signingKey *rsa.PrivateKey, options ...Option) (*Provider, error) {
	if signingKey == nil {
		return nil, errors.New("signing key is nil")
	}
	if err := checkKeySize(&signingKey.PublicKey); err != nil {
		return nil, err
	}

	keyID, err := KeyID(&signingKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key ID: %w", err)
	}

	p := &Provider{
		issuer:           issuer,
		privateKey:       signingKey,
		keyID:            keyID,
		verificationKeys: map[string]*rsa.PublicKey{keyID: &signingKey.PublicKey},
		publicKeys:       []*rsa.PublicKey{&signingKey.PublicKey},
		tokenTTL:         DefaultTokenTTL,
	}
	for _, option := range options {
		if err := option(p); err != nil {
			return nil, err
		}
	}

	p.jwks, err = JWKS(p.publicKeys...)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWKS: %w", err)
	}

	return p, nil
}

// GenerateKey returns a new RSA key of MinKeyBits bits, e.g. for tests. An application that uses a new key on every
// start rejects the tokens issued before a restart, and its instances reject the tokens of each other, so use a key
// loaded from a secret instead.
func GenerateKey() (*rsa.PrivateKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, MinKeyBits)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}
	return key, nil
}

// ParsePrivateKeyPEM returns the RSA private key in PEM data, in the PKCS #1 ("RSA PRIVATE KEY") or the PKCS #8
// ("PRIVATE KEY") format, e.g. created with `openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048`.
func ParsePrivateKeyPEM(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM data found")
	}

	var key *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		var err error
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS #1 private key: %w", err)
		}
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS #8 private key: %w", err)
		}
		var ok bool
		if key, ok = parsed.(*rsa.PrivateKey); !ok {
			return nil, fmt.Errorf("private key is %T, not an RSA key", parsed)
		}
	default:
		return nil, fmt.Errorf("PEM block %q is not a private key", block.Type)
	}

	if err := checkKeySize(&key.PublicKey); err != nil {
		return nil, err
	}
	return key, nil
}

// ParsePublicKeyPEM returns the RSA public key in PEM data, in the PKIX ("PUBLIC KEY") or the PKCS #1
// ("RSA PUBLIC KEY") format, e.g. the previous key for WithVerificationKeys.
func ParsePublicKeyPEM(data []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM data found")
	}

	var key *rsa.PublicKey
	switch block.Type {
	case "RSA PUBLIC KEY":
		var err error
		key, err = x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS #1 public key: %w", err)
		}
	case "PUBLIC KEY":
		parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKIX public key: %w", err)
		}
		var ok bool
		if key, ok = parsed.(*rsa.PublicKey); !ok {
			return nil, fmt.Errorf("public key is %T, not an RSA key", parsed)
		}
	default:
		return nil, fmt.Errorf("PEM block %q is not a public key", block.Type)
	}

	if err := checkKeySize(key); err != nil {
		return nil, err
	}
	return key, nil
}

func checkKeySize(key *rsa.PublicKey) error {
	if key == nil {
		return errors.New("key is nil")
	}
	if bits := key.N.BitLen(); bits < MinKeyBits {
		return fmt.Errorf("RSA key has %d bits, at least %d are needed", bits, MinKeyBits)
	}
	return nil
}

func (p *Provider) Issuer() string {
	return p.issuer
}

// PrivateKey returns the key that signs the tokens.
func (p *Provider) PrivateKey() *rsa.PrivateKey {
	return p.privateKey
}

// PublicKey returns the public key of the key that signs the tokens.
func (p *Provider) PublicKey() *rsa.PublicKey {
	return &p.privateKey.PublicKey
}

// JWKS returns the JWKS document with the signing key and the verification keys.
func (p *Provider) JWKS() []byte {
	return p.jwks
}

// KeyID returns the ID of the signing key, which is the "kid" of the issued tokens.
func (p *Provider) KeyID() string {
	return p.keyID
}

// TokenTTL returns the time for which the issued tokens are valid.
func (p *Provider) TokenTTL() time.Duration {
	return p.tokenTTL
}
