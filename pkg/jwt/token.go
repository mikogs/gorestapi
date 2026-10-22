package jwt

import (
	"crypto/rsa"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Issue returns a token signed with the signing key, with its ID and the times when it is issued and expires.
func (p *Provider) Issue(audience, subject, role string) (string, string, *time.Time, *time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(p.tokenTTL)
	jti := uuid.New().String()
	claims := CustomClaims{
		role,
		jwt.RegisteredClaims{
			Issuer:    p.issuer,
			Subject:   subject,
			Audience:  []string{audience},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        jti,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = p.keyID

	signed, err := token.SignedString(p.privateKey)
	if err != nil {
		return "", "", nil, nil, fmt.Errorf("failed to sign token: %w", err)
	}

	return signed, jti, &now, &expiresAt, nil
}

// Parse returns the claims of rawToken when it is valid: signed with the key of its "kid", which is the signing key or
// one of the verification keys, issued by the provider for audience, and not expired.
func (p *Provider) Parse(rawToken string, audience string) (*CustomClaims, bool) {
	token, _, err := jwt.NewParser().ParseUnverified(rawToken, &CustomClaims{})
	if err != nil {
		return nil, false
	}

	keyID, _ := token.Header["kid"].(string)
	publicKey, ok := p.verificationKeys[keyID]
	if !ok {
		return nil, false
	}

	return parseToken(rawToken, p.issuer, audience, publicKey)
}

func parseToken(rawToken, issuer, audience string, publicKey *rsa.PublicKey) (*CustomClaims, bool) {
	claims := &CustomClaims{}

	token, err := jwt.ParseWithClaims(
		rawToken,
		claims,
		func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, jwt.ErrTokenSignatureInvalid
			}
			return publicKey, nil
		},
		jwt.WithAudience(audience),
		jwt.WithIssuer(issuer),
		jwt.WithLeeway(30*time.Second),
	)

	if err != nil || !token.Valid {
		return nil, false
	}

	return claims, true
}
