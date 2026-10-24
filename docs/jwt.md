# JWT

The `pkg/jwtauth` package provides RS256-based JSON Web Token issuance and verification, plus a JWKS endpoint helper. It wraps [`github.com/golang-jwt/jwt/v5`](https://github.com/golang-jwt/jwt) for token operations and [`github.com/go-jose/go-jose/v3`](https://github.com/go-jose/go-jose) for JWKS serialisation.

## Initialization

`NewProvider` signs tokens with an RSA key, which you load from a secret. All the instances of an application, and the application after a restart, must use the same key: tokens signed with one key are rejected by a provider with another key.

Create the key once, e.g. with OpenSSL, and store it as a secret, e.g. in a secret manager, or in an environment variable of the deployment:

```bash
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out jwt-signing-key.pem
```

```go
import "github.com/mikogs/gorestapi/pkg/jwtauth"

signingKey, err := jwtauth.ParsePrivateKeyPEM([]byte(os.Getenv("JWT_SIGNING_KEY")))
if err != nil {
    log.Fatalf("JWT_SIGNING_KEY: %v", err)
}

p, err := jwtauth.NewProvider("https://auth.example.com", signingKey)
if err != nil {
    log.Fatal(err)
}
```

```go
func NewProvider(issuer string, signingKey *rsa.PrivateKey, options ...Option) (*Provider, error)
```

| Argument | Type | Description |
|---|---|---|
| `issuer` | `string` | The `iss` claim written into every issued token and validated on parse |
| `signingKey` | `*rsa.PrivateKey` | The key that signs the tokens, of at least 2048 bits (`jwtauth.MinKeyBits`) |
| `options` | `...Option` | `WithTokenTTL` and `WithVerificationKeys`, see below |

| Option | Description |
|---|---|
| `WithTokenTTL(ttl time.Duration)` | The time for which issued tokens are valid. Default: `jwtauth.DefaultTokenTTL`, 1 hour |
| `WithVerificationKeys(keys ...*rsa.PublicKey)` | Keys that verify tokens, but do not sign them, e.g. the previous key after a [rotation](#key-rotation) |

The keys are loaded with:

| Function | Formats |
|---|---|
| `ParsePrivateKeyPEM(data []byte) (*rsa.PrivateKey, error)` | PKCS #8 (`BEGIN PRIVATE KEY`, from `openssl genpkey`) and PKCS #1 (`BEGIN RSA PRIVATE KEY`) |
| `ParsePublicKeyPEM(data []byte) (*rsa.PublicKey, error)` | PKIX (`BEGIN PUBLIC KEY`, from `openssl pkey -pubout`) and PKCS #1 (`BEGIN RSA PUBLIC KEY`) |

Keys that are not RSA keys, or have fewer than 2048 bits, are rejected. In tests, `jwtauth.GenerateKey()` returns a new key. Do not use it in an application: with a new key on every start, users are logged out by every restart, and the instances of the application reject the tokens of each other.

### Provider

```go
type Provider struct { /* unexported */ }

func (p *Provider) Issuer()     string
func (p *Provider) PrivateKey() *rsa.PrivateKey
func (p *Provider) PublicKey()  *rsa.PublicKey
func (p *Provider) JWKS()       []byte
func (p *Provider) KeyID()      string
func (p *Provider) TokenTTL()   time.Duration
func (p *Provider) Issue(audience, subject, role string) (string, string, *time.Time, *time.Time, error)
func (p *Provider) Parse(rawToken, audience string) (*CustomClaims, bool)
```

| Method | Description |
|---|---|
| `Issuer()` | Returns the issuer string passed to `NewProvider` |
| `PrivateKey()` | Returns the signing key |
| `PublicKey()` | Returns the public key of the signing key |
| `JWKS()` | Returns the pre-built JWKS document, with the signing key and the verification keys, as JSON bytes |
| `KeyID()` | Returns the ID of the signing key, written as `kid` into the header of every token |
| `TokenTTL()` | Returns the time for which issued tokens are valid |
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
| `expiresAt` | `*time.Time` | Timestamp the token expires, `TokenTTL()` after `issuedAt` |

## Parsing and verifying tokens

```go
claims, ok := p.Parse(rawToken, "myapp")
if !ok {
    // token is invalid, expired, or has wrong issuer/audience
}

subject := claims.Subject
role    := claims.Role
```

`Parse` validates the signature, expiry, issuer, and audience in one call. A token has to have an expiry (`exp`), otherwise it would be valid forever. The signature is verified with the key of the `kid` in the header of the token: the signing key, or one of the verification keys. A token without a `kid`, or with the `kid` of another key, is rejected. It returns `(nil, false)` on any failure so callers never need to inspect a partial `claims` struct.

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

When your service consumes tokens issued by a separate auth server, use a `JWKSValidator`. It fetches the JWKS document at the given URI, keeps its keys, chooses the key that signed the token, and verifies the token — no local `Provider` needed:

```go
validator, err := jwtauth.NewJWKSValidator(
    "https://auth.example.com/.well-known/jwks.json",
    "https://auth.example.com", // expected issuer
    "myapp",                    // expected audience
)
if err != nil {
    log.Fatal(err) // the URI does not use https
}

// for each request
claims, err := validator.Validate(r.Context(), bearerToken)
if err != nil {
    // fetch failed, key missing, or token invalid
}
```

* **The keys are kept** for `DefaultJWKSCacheTTL` (5 minutes), so a request does not fetch them. A token with a `kid` that is not in them, e.g. of a key that the issuer has just added, fetches them again, at most once in `DefaultJWKSMinRefresh` (1 minute), so that tokens with made-up `kid`s cannot make the service fetch the JWKS on every request. When the JWKS cannot be fetched, the validation fails.
* **A validator is safe for concurrent use.** Create one when the service starts, and use it in all the requests.

`ValidateWithJWKS` does the same once, fetching the JWKS document on every call, e.g. in a script or a test:

The key is the one whose `kid` is the `kid` in the header of the token. This way, a JWKS can have several keys, e.g. while the issuer rotates them: tokens signed with the old key and tokens signed with the new key are both valid until the old key is removed from the JWKS. A token without a `kid` is accepted only when the JWKS has exactly one key, as there is no way to tell which key to use otherwise.

```go
claims, err := jwtauth.ValidateWithJWKS(
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

Returns a descriptive error if the JWKS cannot be fetched or decoded, if it has no keys, if it has no key with the `kid` of the token, if that key is not an RSA key, or if the token has no `kid` and the JWKS has more than one key. A token that fails signature, expiry, issuer, or audience validation, or that has no expiry, returns the error `invalid token`.

**Fetching the keys safely.** Whoever can change the keys on the way could sign any token, so:

* **The URI has to use `https`**, or `http` for `localhost`, `127.0.0.1` and `::1`, e.g. in tests. Another URI is an error of `NewJWKSValidator` and `ValidateWithJWKS`.
* **Redirects** are followed only to `https` URLs, at most 10.
* **A fetch takes at most 10 seconds**, also when `ctx` has no deadline, and the document at most 128 KiB.
* **The URI is always the one that you pass**: the `jku` and `x5u` headers of a token, which name other URIs of keys, are never used.

## JWKS endpoint

Expose the public key to external verifiers (e.g. an API gateway or a frontend using a JWKS library):

```go
http.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.Write(p.JWKS())
})
```

The document is generated once by `NewProvider` and cached on the `Provider`. It has the signing key first, followed by the verification keys. To generate a JWKS from existing public keys directly:

```go
jwksBytes, err := jwtauth.JWKS(publicKey, previousPublicKey)
```

The `kid` of the key is its RFC 7638 thumbprint, a base64url-encoded SHA-256 hash of the public key, so the same key always gets the same `kid`. `jwtauth.KeyID(publicKey)` returns it, e.g. to set the `kid` of tokens signed with the key by other code.

## Key rotation

Replace the signing key from time to time, or at once when it may have leaked. Tokens signed with the old key stay valid until they expire, when the old key is a verification key of the new provider:

1. Create a new key, and keep the public key of the old one:
   ```bash
   openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out jwt-signing-key-2.pem
   openssl pkey -in jwt-signing-key.pem -pubout -out jwt-previous-public-key.pem
   ```
2. Deploy the application with the new key as `JWT_SIGNING_KEY`, and the old public key as `JWT_PREVIOUS_PUBLIC_KEY`:
   ```go
   var options []jwtauth.Option
   if previous := os.Getenv("JWT_PREVIOUS_PUBLIC_KEY"); previous != "" {
       previousKey, err := jwtauth.ParsePublicKeyPEM([]byte(previous))
       if err != nil {
           log.Fatalf("JWT_PREVIOUS_PUBLIC_KEY: %v", err)
       }
       options = append(options, jwtauth.WithVerificationKeys(previousKey))
   }

   p, err := jwtauth.NewProvider("https://auth.example.com", signingKey, options...)
   ```
   New tokens are signed with the new key. Tokens of both keys are accepted by `Parse`, and by `ValidateWithJWKS` of other services, as the JWKS has both keys.
3. After the longest lifetime of a token, `TokenTTL()`, remove `JWT_PREVIOUS_PUBLIC_KEY`. The tokens of the old key have expired by then.

When the old key leaked, skip the waiting: deploy the new key without `JWT_PREVIOUS_PUBLIC_KEY`, which logs out all the users, but also rejects the tokens that someone else signed with the old key.

## Complete example

```go
package main

import (
    "fmt"
    "log"
    "net/http"
    "os"
    "strings"
    "time"

    "github.com/mikogs/gorestapi/pkg/jwtauth"
)

const audience = "myapp"

func main() {
    signingKey, err := jwtauth.ParsePrivateKeyPEM([]byte(os.Getenv("JWT_SIGNING_KEY")))
    if err != nil {
        log.Fatalf("JWT_SIGNING_KEY: %v", err)
    }

    p, err := jwtauth.NewProvider("https://auth.example.com", signingKey, jwtauth.WithTokenTTL(15*time.Minute))
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

    srv := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 10 * time.Second}
    log.Fatal(srv.ListenAndServe())
}
```
