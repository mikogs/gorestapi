package jwtauth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// TestProvider_Concurrent issues and validates tokens with one provider on goroutines at the same time, so that the
// race detector can find the data races of the code that they share.
func TestProvider_Concurrent(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewProvider("https://auth.example.com", key)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(p.JWKS())
	}))
	defer server.Close()

	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Go(func() {
			for i := range 10 {
				if err := issueAndValidate(p, server.URL, fmt.Sprint(worker)); err != nil {
					t.Errorf("worker %d, iteration %d: %s", worker, i, err.Error())
					return
				}
			}
		})
	}
	wg.Wait()
}

func issueAndValidate(p *Provider, jwksURI string, subject string) error {
	token, _, _, _, err := p.Issue("api", subject, "user")
	if err != nil {
		return fmt.Errorf("Issue: %w", err)
	}
	if claims, ok := p.Parse(token, "api"); !ok || claims.Subject != subject {
		return fmt.Errorf("Parse returned %+v, %v, want the subject %s", claims, ok, subject)
	}
	claims, err := ValidateWithJWKS(context.Background(), jwksURI, p.Issuer(), "api", token)
	if err != nil || claims.Subject != subject {
		return fmt.Errorf("ValidateWithJWKS returned %+v, %w, want the subject %s", claims, err, subject)
	}
	return nil
}
