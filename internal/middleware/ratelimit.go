package middleware

import (
	"net/http"
	"sync"

	"github.com/singh-gur/api_cache/internal/config"
	"github.com/singh-gur/api_cache/internal/logger"
	"golang.org/x/time/rate"
)

type RateLimiter struct {
	config   *config.RateLimitConfig
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(cfg *config.RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		config:   cfg,
		limiters: make(map[string]*rate.Limiter),
	}
}

// getLimiter returns the rate limiter for a specific path
func (rl *RateLimiter) getLimiter(path string, endpointConfig *config.EndpointRateLimitConfig) *rate.Limiter {
	rl.mu.RLock()
	limiter, exists := rl.limiters[path]
	rl.mu.RUnlock()

	if exists {
		return limiter
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists := rl.limiters[path]; exists {
		return limiter
	}

	// Use endpoint-specific config or default
	rps := rl.config.RequestsPerSecond
	burst := rl.config.Burst

	if endpointConfig != nil {
		rps = endpointConfig.RequestsPerSecond
		burst = endpointConfig.Burst
	}

	limiter = rate.NewLimiter(rate.Limit(rps), burst)
	rl.limiters[path] = limiter

	return limiter
}

// Middleware returns a rate limiting middleware
func (rl *RateLimiter) Middleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.config.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			endpointConfig := cfg.GetEndpointRateLimitConfig(r.URL.Path)
			limiter := rl.getLimiter(r.URL.Path, endpointConfig)

			if !limiter.Allow() {
				logger.WithFields(map[string]interface{}{
					"path":   r.URL.Path,
					"method": r.Method,
					"remote": r.RemoteAddr,
				}).Warn("Rate limit exceeded")

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"rate limit exceeded","message":"too many requests"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
