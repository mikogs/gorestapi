package gorestapi

import (
	"context"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// CodeTooManyRequests is the code of the problem of a request over the limit of Options.RateLimiter.
const CodeTooManyRequests = "TOO_MANY_REQUESTS"

// RateLimiter limits the requests of each key, e.g. of an IP of a client, see Options.RateLimiter. NewRateLimiter keeps
// the limits in the memory of the process, and an implementation with e.g. Redis shares them between the instances of
// the API.
type RateLimiter interface {
	// Allow returns whether a request of key can be served now, or how long until the next one can be.
	Allow(ctx context.Context, key string) (bool, time.Duration, error)
}

// NewRateLimiter returns a RateLimiter of the memory of the process, which serves perSecond requests of each key, and up
// to burst requests at once, e.g. 10 a second, with bursts of 20.
func NewRateLimiter(perSecond float64, burst int) RateLimiter {
	// a bucket that has not been used for this long is full again, so it is the same as a new one, and is dropped
	idle := time.Duration(float64(burst) / perSecond * float64(time.Second))
	return &memoryRateLimiter{limit: rate.Limit(perSecond), burst: burst, idle: max(idle, time.Minute), buckets: map[string]*bucket{}}
}

type memoryRateLimiter struct {
	mu        sync.Mutex
	limit     rate.Limit
	burst     int
	idle      time.Duration
	buckets   map[string]*bucket
	lastSweep time.Time
}

type bucket struct {
	limiter *rate.Limiter
	used    time.Time
}

func (m *memoryRateLimiter) Allow(_ context.Context, key string) (bool, time.Duration, error) {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	// the buckets of the keys that are no longer used are dropped while serving others, without a goroutine
	if now.Sub(m.lastSweep) > m.idle {
		for k, b := range m.buckets {
			if now.Sub(b.used) > m.idle {
				delete(m.buckets, k)
			}
		}
		m.lastSweep = now
	}
	b, ok := m.buckets[key]
	if !ok {
		b = &bucket{limiter: rate.NewLimiter(m.limit, m.burst)}
		m.buckets[key] = b
	}
	b.used = now

	reservation := b.limiter.ReserveN(now, 1)
	if !reservation.OK() {
		return false, m.idle, nil
	}
	if delay := reservation.DelayFrom(now); delay > 0 {
		reservation.CancelAt(now)
		return false, delay, nil
	}
	return true, 0, nil
}

// rateLimit returns whether the request can be served, after it writes the problem TOO_MANY_REQUESTS of a request over
// the limit, with the seconds until the next one in Retry-After. A request whose limiter fails is served, and its error
// is logged, so that the API works while e.g. its Redis does not.
func (h *Handler) rateLimit(ctx context.Context, w http.ResponseWriter, r *http.Request) bool {
	if h.options.RateLimiter == nil {
		return true
	}
	key := h.rateLimitKey(r)
	allowed, retryAfter, err := h.options.RateLimiter.Allow(ctx, key)
	if err != nil {
		h.log().LogAttrs(ctx, slog.LevelError, "rate limiter failed", handlerAttr, slog.String("err", err.Error()))
		return true
	}
	if allowed {
		return true
	}
	h.log().LogAttrs(ctx, slog.LevelDebug, "request over the rate limit", handlerAttr, slog.String("key", key))
	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(max(retryAfter, time.Second).Seconds()))))
	h.writeProblem(w, r, CodeTooManyRequests, "", nil)
	return false
}

func (h *Handler) rateLimitKey(r *http.Request) string {
	if h.options.RateLimitKey != nil {
		return h.options.RateLimitKey(r)
	}
	return ClientIPKey(ClientIP(r, h.options.TrustedProxies))
}

// ClientIP returns the IP of the client of r: the IP of its connection, or, when it is one of trustedProxies, e.g. of
// a load balancer, the last IP of its X-Forwarded-For header that is not of a trusted proxy. The header of other
// clients is not read, as they can send any IP in it.
func ClientIP(r *http.Request, trustedProxies []netip.Prefix) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	addr = addr.Unmap()
	if !trusted(addr, trustedProxies) {
		return addr
	}
	forwarded := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(forwarded) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(forwarded[i]))
		if err != nil {
			return addr
		}
		addr = hop.Unmap()
		if !trusted(addr, trustedProxies) {
			return addr
		}
	}
	return addr
}

func trusted(addr netip.Addr, trustedProxies []netip.Prefix) bool {
	for _, prefix := range trustedProxies {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// ClientIPKey returns the key of the rate limit of an IP: the IP, or its network /64 for IPv6, as a client usually has
// a whole /64, and could change its IP in it for every request.
func ClientIPKey(addr netip.Addr) string {
	if addr.Is6() {
		prefix, _ := addr.Prefix(64)
		return prefix.String()
	}
	return addr.String()
}
