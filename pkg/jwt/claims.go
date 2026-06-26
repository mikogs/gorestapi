package jwt

import "github.com/golang-jwt/jwt/v5"

type CustomClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

func (cc CustomClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	return cc.RegisteredClaims.GetExpirationTime()
}
func (cc CustomClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	return cc.RegisteredClaims.GetIssuedAt()
}
func (cc CustomClaims) GetNotBefore() (*jwt.NumericDate, error) {
	return cc.RegisteredClaims.GetNotBefore()
}
func (cc CustomClaims) GetIssuer() (string, error) {
	return cc.RegisteredClaims.GetIssuer()
}
func (cc CustomClaims) GetSubject() (string, error) {
	return cc.RegisteredClaims.GetSubject()
}
func (cc CustomClaims) GetAudience() (jwt.ClaimStrings, error) {
	return cc.RegisteredClaims.GetAudience()
}
