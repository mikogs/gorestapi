# JWT

The `pkg/jwt` package provides RS256-based JSON Web Token issuance and verification, plus a JWKS endpoint helper. It wraps [`github.com/golang-jwt/jwt/v5`](https://github.com/golang-jwt/jwt) for token operations and [`github.com/go-jose/go-jose/v3`](https://github.com/go-jose/go-jose) for JWKS serialisation.

## Initialization

`NewProvider` generates a fresh 2048-bit RSA key pair and pre-builds the JWKS document:

```go
import pkgjwt "github.com/mikogs/gorestapi/pkg/jwt"

p, err := pkgjwt.NewProvider("https://auth.example.com")
if err != nil {
    log.Fatal(err)
}
```

`NewProvider` takes one argument:

| Argument | Type | Description |
|---|---|---|
| `issuer` | `string` | The `iss` claim written into every issued token and validated on parse |

### Provider

```go
type Provider struct { /* unexported */ }

func (p *Provider) Issuer()     string
func (p *Provider) PrivateKey() *rsa.PrivateKey
func (p *Provider) PublicKey()  *rsa.PublicKey
func (p *Provider) JWKS()       []byte
func (p *Provider) KeyID()      string
func (p *Provider) Issue(audience, subject, role string) (string, string, *time.Time, *time.Time, error)
func (p *Provider) Parse(rawToken, audience string) (*CustomClaims, bool)
```

| Method | Description |
|---|---|
| `Issuer()` | Returns the issuer string passed to `NewProvider` |
| `PrivateKey()` | Returns the RSA private key (used for signing) |
| `PublicKey()` | Returns the RSA public key (used for verification) |
| `JWKS()` | Returns the pre-built JWKS document as JSON bytes |
| `KeyID()` | Returns the ID of the key, written as `kid` into the header of every token and into the JWKS |
| `Issue(...)` | Signs and returns a new token (see below) |
| `Parse(...)` | Validates and parses a token (see below) |

## Issuing tokens

```go
signed, jti, issuedAt, expiresAt, err := p.Issue("myapp", "user:42", "admin")
```

| Parameter | Type | Description |
|---|---|---|
| `audience` | `string` | Written as the `aud` claim; must match what `Parse` expects |
| `subject` | `string` | Written as the `sub` claim — typically a user ID or unique identifier |
| `role` | `string` | Written as the `role` claim in `CustomClaims` |

**Returns:**

| Value | Type | Description |
|---|---|---|
| `signed` | `string` | The compact-serialised JWT string, with the `kid` of the provider's key in its header |
| `jti` | `string` | Unique token ID (UUID v4) — store this to support revocation |
| `issuedAt` | `*time.Time` | Timestamp the token was created |
| `expiresAt` | `*time.Time` | Timestamp the token expires (1 hour after `issuedAt`) |

## Parsing and verifying tokens

```go
claims, ok := p.Parse(rawToken, "myapp")
if !ok {
    // token is invalid, expired, or has wrong issuer/audience
}

subject := claims.Subject
role    := claims.Role
```

`Parse` validates the signature, expiry, issuer, and audience in one call. It returns `(nil, false)` on any failure so callers never need to inspect a partial `claims` struct.

| Parameter | Type | Description |
|---|---|---|
| `rawToken` | `string` | Compact-serialised JWT from the request |
| `audience` | `string` | Expected `aud` value |

A clock skew tolerance of 30 seconds is applied automatically.

## Claims

```go
type CustomClaims struct {
    Role string `json:"role"`
    jwt.RegisteredClaims
}
```

`CustomClaims` embeds the standard `jwt.RegisteredClaims` (which carries `Subject`, `Issuer`, `Audience`, `ExpiresAt`, `IssuedAt`, `NotBefore`, `ID`) and adds a `Role` field. Access standard fields directly via the embedded struct:

```go
subject := claims.Subject   // whatever was passed to Issue
expiry  := claims.ExpiresAt // *jwt.NumericDate
role    := claims.Role      // "admin"
```

## Validating tokens from a remote issuer

When your service consumes tokens issued by a separate auth server, use `ValidateWithJWKS`. It fetches the JWKS document at the given URI, chooses the key that signed the token, and verifies the token — no local `Provider` needed.

The key is the one whose `kid` is the `kid` in the header of the token. This way, a JWKS can have several keys, e.g. while the issuer rotates them: tokens signed with the old key and tokens signed with the new key are both valid until the old key is removed from the JWKS. A token without a `kid` is accepted only when the JWKS has exactly one key, as there is no way to tell which key to use otherwise.

```go
claims, err := pkgjwt.ValidateWithJWKS(
    ctx,
    "https://auth.example.com/.well-known/jwks.json",
    "https://auth.example.com", // expected issuer
    "myapp",                    // expected audience
    bearerToken,
)
if err != nil {
    // fetch failed, key missing, or token invalid
}
```

| Parameter | Type | Description |
|---|---|---|
| `ctx` | `context.Context` | Controls the HTTP request to the JWKS endpoint |
| `jwksURI` | `string` | Full URL of the JWKS document |
| `issuer` | `string` | Expected `iss` claim value |
| `audience` | `string` | Expected `aud` claim value |
| `bearerToken` | `string` | The raw JWT string from the request |

Returns a descriptive error if the JWKS cannot be fetched or decoded, if it has no keys, if it has no key with the `kid` of the token, if that key is not an RSA key, or if the token has no `kid` and the JWKS has more than one key. A token that fails signature, expiry, issuer, or audience validation returns the error `invalid token`.

## JWKS endpoint

Expose the public key to external verifiers (e.g. an API gateway or a frontend using a JWKS library):

```go
http.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.Write(p.JWKS())
})
```

The document is generated once at startup by `NewProvider` and cached on the `Provider`. To generate a JWKS from an existing public key directly:

```go
jwksBytes, err := pkgjwt.JWKS(publicKey)
```

The `kid` of the key is its RFC 7638 thumbprint, a base64url-encoded SHA-256 hash of the public key, so the same key always gets the same `kid`. `pkgjwt.KeyID(publicKey)` returns it, e.g. to set the `kid` of tokens signed with the key by other code.

`NewProvider` generates a new key every time it is called, so tokens issued before a restart cannot be verified after it.

## Complete example

```go
package main

import (
    "fmt"
    "log"
    "net/http"
    "strings"

    pkgjwt "github.com/mikogs/gorestapi/pkg/jwt"
)

const audience = "myapp"

func main() {
    p, err := pkgjwt.NewProvider("https://auth.example.com")
    if err != nil {
        log.Fatal(err)
    }

    // Issue a token for user 42.
    token, _, _, _, err := p.Issue(audience, "42", "member")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("token:", token)

    mux := http.NewServeMux()

    mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Write(p.JWKS())
    })

    mux.HandleFunc("/protected", func(w http.ResponseWriter, r *http.Request) {
        raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
        claims, ok := p.Parse(raw, audience)
        if !ok {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        fmt.Fprintf(w, "hello %s (role: %s)", claims.Subject, claims.Role)
    })

    http.ListenAndServe(":8080", mux)
}
```
