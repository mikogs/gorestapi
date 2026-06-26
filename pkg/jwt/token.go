package jwt

import (
	"crypto/rsa"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const tokenTTL = time.Hour

func (p *Provider) Issue(audience, subject, role string) (string, string, *time.Time, *time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(tokenTTL)
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

	signed, err := token.SignedString(p.privateKey)
	if err != nil {
		return "", "", nil, nil, fmt.Errorf("failed to sign token: %w", err)
	}

	return signed, jti, &now, &expiresAt, nil
}

func (p *Provider) Parse(rawToken string, audience string) (*CustomClaims, bool) {
	return parseToken(rawToken, p.issuer, audience, p.publicKey)
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
