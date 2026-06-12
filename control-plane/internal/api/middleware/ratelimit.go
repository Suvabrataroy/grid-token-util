package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

// RateLimiter is the Redis interface required by the rate-limit middleware.
type RateLimiter interface {
	// Increment increments a counter key with an expiry and returns the new value.
	RateLimitIncr(ctx context.Context, key string, window time.Duration) (int64, error)
}

// RateLimit returns a per-org-unit token-bucket rate-limiting middleware.
// It uses a Redis counter keyed by org unit ID with a 60-second sliding window.
// When the counter exceeds requestsPerMin the request is rejected with 429.
func RateLimit(limiter RateLimiter, requestsPerMin int) func(http.Handler) http.Handler {
	window := time.Minute

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			orgID, ok := OrgUnitIDFromContext(r.Context())
			if !ok {
				// No org context — skip rate limiting (e.g. health endpoints).
				next.ServeHTTP(w, r)
				return
			}

			key := fmt.Sprintf("ratelimit:org:%s", orgID.String())

			count, err := limiter.RateLimitIncr(r.Context(), key, window)
			if err != nil {
				// On Redis failure, fail open to avoid total outage.
				log.Warn().Err(err).Msg("ratelimit: redis error, allowing request")
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(requestsPerMin))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(int64(requestsPerMin)-count, 10))

			if count > int64(requestsPerMin) {
				retryAfter := int(window.Seconds())
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"rate_limit_exceeded","retry_after":` + strconv.Itoa(retryAfter) + `}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
