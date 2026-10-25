package gorestapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestMemoryRateLimiter(t *testing.T) {
	ctx := context.Background()
	limiter := NewRateLimiter(1, 2).(*memoryRateLimiter)
	// a request over the limit takes no token, so the next one waits no longer
	for i, want := range []bool{true, true, false, false} {
		allowed, retryAfter, err := limiter.Allow(ctx, "a")
		if err != nil || allowed != want || !allowed && (retryAfter <= 0 || retryAfter > time.Second) {
			t.Fatalf("the request %d returned %t, %v, %v, want %t", i, allowed, retryAfter, err, want)
		}
	}
	if allowed, _, _ := limiter.Allow(ctx, "b"); !allowed {
		t.Fatal("the first request of another key is over the limit")
	}

	// the buckets that are full again are dropped, and the ones in use are kept
	limiter.mu.Lock()
	for _, b := range limiter.buckets {
		b.used = b.used.Add(-2 * limiter.idle)
	}
	limiter.mu.Unlock()
	_, _, _ = limiter.Allow(ctx, "a")
	limiter.mu.Lock()
	limiter.lastSweep = limiter.lastSweep.Add(-2 * limiter.idle)
	limiter.mu.Unlock()
	if allowed, _, _ := limiter.Allow(ctx, "c"); !allowed || len(limiter.buckets) != 2 || limiter.buckets["b"] != nil {
		t.Fatalf("the limiter has the buckets %v after b is idle, want a and c", limiter.buckets)
	}
}

type rateLimiterFunc func(key string) (bool, time.Duration, error)

func (f rateLimiterFunc) Allow(_ context.Context, key string) (bool, time.Duration, error) {
	return f(key)
}

func TestRateLimit(t *testing.T) {
	recreateTestStructTable()
	h := New(testService, Options{Routes: map[string]Route{"teststruct": {}}, RateLimiter: NewRateLimiter(0.001, 2)})
	serve := func(remoteAddr string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/teststruct/", nil)
		req.RemoteAddr = remoteAddr
		w := httptest.NewRecorder()
		h.Serve(w, req)
		return w
	}
	for i, want := range []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests} {
		w := serve("192.0.2.1:1234")
		if w.Code != want {
			t.Fatalf("the request %d returned %d, want %d", i, w.Code, want)
		}
		if w.Code == http.StatusTooManyRequests && (w.Header().Get("Retry-After") == "" || w.Header().Get("Content-Type") != "application/problem+json" ||
			strings.Contains(w.Body.String(), `"data"`)) {
			t.Fatalf("the request over the limit returned %v %s, want Retry-After and only a problem", w.Header(), w.Body.String())
		}
	}
	if w := serve("192.0.2.2:1234"); w.Code != http.StatusOK {
		t.Fatalf("the request of another client returned %d, want 200", w.Code)
	}
	// the addresses of an IPv6 network /64 share the limit
	for i, addr := range []string{"[2001:db8::1]:1", "[2001:db8::2]:1", "[2001:db8::3]:1"} {
		if w := serve(addr); (w.Code == http.StatusOK) != (i < 2) {
			t.Fatalf("the request %d of the network 2001:db8::/64 returned %d", i, w.Code)
		}
	}

	h = New(testService, Options{Routes: map[string]Route{"teststruct": {}}, RateLimiter: rateLimiterFunc(func(string) (bool, time.Duration, error) {
		return false, 0, errors.New("redis is down")
	})})
	if w := serve("192.0.2.1:1234"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"data"`) {
		t.Fatalf("a request with a failing limiter returned %d %s, want the list", w.Code, w.Body.String())
	}

	// a limiter without a wait still has the client wait a second
	h = New(testService, Options{Routes: map[string]Route{"teststruct": {}}, RateLimiter: rateLimiterFunc(func(string) (bool, time.Duration, error) {
		return false, 0, nil
	})})
	if w := serve("192.0.2.1:1234"); w.Header().Get("Retry-After") != "1" {
		t.Fatalf("a request over the limit without a wait has Retry-After %q, want 1", w.Header().Get("Retry-After"))
	}

	var keys []string
	h = New(testService, Options{
		Routes:       map[string]Route{"teststruct": {}},
		RateLimitKey: func(r *http.Request) string { return r.Header.Get("X-Test-User") },
		RateLimiter: rateLimiterFunc(func(key string) (bool, time.Duration, error) {
			keys = append(keys, key)
			return true, 0, nil
		}),
	})
	req := httptest.NewRequest(http.MethodGet, "/teststruct/", nil)
	req.Header.Set("X-Test-User", "u1")
	h.Serve(httptest.NewRecorder(), req)
	if len(keys) != 1 || keys[0] != "u1" {
		t.Fatalf("the limiter got the keys %v, want the one of RateLimitKey", keys)
	}
}

func TestClientIP(t *testing.T) {
	proxies := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	for name, test := range map[string]struct {
		remoteAddr, forwarded string
		want                  string
	}{
		"a client":                   {"192.0.2.1:1234", "", "192.0.2.1"},
		"a client with a header":     {"192.0.2.1:1234", "203.0.113.9", "192.0.2.1"},
		"behind a proxy":             {"10.0.0.2:80", "203.0.113.9", "203.0.113.9"},
		"behind two proxies":         {"10.0.0.2:80", "203.0.113.9, 10.0.0.3", "203.0.113.9"},
		"a header set by the client": {"10.0.0.2:80", "198.51.100.1, 203.0.113.9", "203.0.113.9"},
		"a proxy without a header":   {"10.0.0.2:80", "", "10.0.0.2"},
		"IPv4 in IPv6":               {"[::ffff:192.0.2.1]:1234", "", "192.0.2.1"},
		"a header that is not an IP": {"10.0.0.2:80", "unknown", "10.0.0.2"},
		"a hop that is not an IP":    {"10.0.0.2:80", "203.0.113.9, unknown", "10.0.0.2"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = test.remoteAddr
		if test.forwarded != "" {
			req.Header.Set("X-Forwarded-For", test.forwarded)
		}
		if got := ClientIP(req, proxies).String(); got != test.want {
			t.Fatalf("ClientIP of %s returned %s, want %s", name, got, test.want)
		}
	}
	if key := ClientIPKey(netip.MustParseAddr("2001:db8::1")); key != "2001:db8::/64" {
		t.Fatalf("ClientIPKey of an IPv6 returned %s, want its /64", key)
	}
}
