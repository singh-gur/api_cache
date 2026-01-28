package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/singh-gur/api_cache/internal/cache"
	"github.com/singh-gur/api_cache/internal/config"
	"github.com/singh-gur/api_cache/internal/logger"
)

type Handler struct {
	cache      *cache.Client
	config     *config.Config
	httpClient *http.Client
}

// NewHandler creates a new proxy handler
func NewHandler(cacheClient *cache.Client, cfg *config.Config) *Handler {
	return &Handler{
		cache:  cacheClient,
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Upstream.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        cfg.Upstream.MaxIdleConns,
				MaxConnsPerHost:     cfg.Upstream.MaxConnsPerHost,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
	}
}

// ServeHTTP handles incoming requests with caching
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	ctx := r.Context()

	// Only cache GET requests
	if r.Method != http.MethodGet {
		h.forwardRequest(w, r, ctx, startTime)
		return
	}

	// Get endpoint-specific cache config
	endpointConfig := h.config.GetEndpointCacheConfig(r.URL.Path, r.Method, r.URL.Query())

	// Generate cache key
	cacheKey := h.cache.GenerateCacheKey(r, endpointConfig)

	// Try to get from cache
	cached, err := h.cache.Get(ctx, cacheKey)
	if err != nil {
		logger.WithField("error", err).Error("Failed to get from cache")
	}

	if cached != nil {
		// Serve from cache
		h.serveCachedResponse(w, cached, startTime)
		return
	}

	// Cache miss - forward request to upstream
	h.forwardAndCache(w, r, ctx, cacheKey, endpointConfig, startTime)
}

// serveCachedResponse writes a cached response to the client
func (h *Handler) serveCachedResponse(w http.ResponseWriter, cached *cache.CachedResponse, startTime time.Time) {
	// Copy headers
	for key, values := range cached.Headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Add cache headers
	w.Header().Set("X-Cache", "HIT")
	w.Header().Set("X-Cache-Time", cached.CachedAt.Format(time.RFC3339))

	// Write status and body
	w.WriteHeader(cached.StatusCode)
	w.Write(cached.Body)

	duration := time.Since(startTime)
	logger.WithFields(map[string]interface{}{
		"cache":    "hit",
		"status":   cached.StatusCode,
		"duration": duration.Milliseconds(),
	}).Info("Request served from cache")
}

// forwardAndCache forwards the request to upstream and caches the response
func (h *Handler) forwardAndCache(w http.ResponseWriter, r *http.Request, ctx context.Context, cacheKey string, endpointConfig *config.EndpointCacheConfig, startTime time.Time) {
	// Forward request with retry logic
	resp, err := h.forwardWithRetry(r, ctx)
	if err != nil {
		logger.WithField("error", err).Error("Failed to forward request")
		http.Error(w, "upstream service unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.WithField("error", err).Error("Failed to read response body")
		http.Error(w, "failed to read upstream response", http.StatusInternalServerError)
		return
	}

	// Cache successful responses (2xx status codes)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		cachedResp := &cache.CachedResponse{
			StatusCode: resp.StatusCode,
			Headers:    resp.Header,
			Body:       body,
			CachedAt:   time.Now(),
		}

		ttl := h.getTTL(endpointConfig)
		if err := h.cache.Set(ctx, cacheKey, cachedResp, ttl); err != nil {
			logger.WithField("error", err).Error("Failed to cache response")
		}
	}

	// Copy headers to response
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Add cache headers
	w.Header().Set("X-Cache", "MISS")

	// Write response
	w.WriteHeader(resp.StatusCode)
	w.Write(body)

	duration := time.Since(startTime)
	logger.WithFields(map[string]interface{}{
		"cache":    "miss",
		"status":   resp.StatusCode,
		"duration": duration.Milliseconds(),
	}).Info("Request forwarded to upstream")
}

// forwardRequest forwards a non-cacheable request to upstream
func (h *Handler) forwardRequest(w http.ResponseWriter, r *http.Request, ctx context.Context, startTime time.Time) {
	resp, err := h.forwardWithRetry(r, ctx)
	if err != nil {
		logger.WithField("error", err).Error("Failed to forward request")
		http.Error(w, "upstream service unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Write status
	w.WriteHeader(resp.StatusCode)

	// Copy body
	io.Copy(w, resp.Body)

	duration := time.Since(startTime)
	logger.WithFields(map[string]interface{}{
		"method":   r.Method,
		"status":   resp.StatusCode,
		"duration": duration.Milliseconds(),
	}).Info("Request forwarded (non-cacheable)")
}

// forwardWithRetry forwards a request with retry logic
func (h *Handler) forwardWithRetry(r *http.Request, ctx context.Context) (*http.Response, error) {
	var lastErr error
	backoff := h.config.Retry.InitialBackoff

	maxAttempts := 1
	if h.config.Retry.Enabled {
		maxAttempts = h.config.Retry.MaxAttempts
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Create upstream request
		upstreamURL := h.config.Upstream.BaseURL + r.URL.Path
		if r.URL.RawQuery != "" {
			upstreamURL += "?" + r.URL.RawQuery
		}

		req, err := http.NewRequestWithContext(ctx, r.Method, upstreamURL, r.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create upstream request: %w", err)
		}

		// Copy headers
		req.Header = r.Header.Clone()

		// Execute request
		resp, err := h.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxAttempts {
				logger.WithFields(map[string]interface{}{
					"attempt": attempt,
					"error":   err,
					"backoff": backoff,
				}).Warn("Request failed, retrying")
				time.Sleep(backoff)
				backoff = time.Duration(float64(backoff) * h.config.Retry.BackoffMultiplier)
				if backoff > h.config.Retry.MaxBackoff {
					backoff = h.config.Retry.MaxBackoff
				}
				continue
			}
			return nil, fmt.Errorf("all retry attempts failed: %w", lastErr)
		}

		// Check if status code is retryable
		if h.isRetryableStatus(resp.StatusCode) && attempt < maxAttempts {
			resp.Body.Close()
			logger.WithFields(map[string]interface{}{
				"attempt": attempt,
				"status":  resp.StatusCode,
				"backoff": backoff,
			}).Warn("Retryable status code, retrying")
			time.Sleep(backoff)
			backoff = time.Duration(float64(backoff) * h.config.Retry.BackoffMultiplier)
			if backoff > h.config.Retry.MaxBackoff {
				backoff = h.config.Retry.MaxBackoff
			}
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("all retry attempts failed: %w", lastErr)
}

// isRetryableStatus checks if a status code is retryable
func (h *Handler) isRetryableStatus(statusCode int) bool {
	if !h.config.Retry.Enabled {
		return false
	}
	for _, code := range h.config.Retry.RetryableStatusCodes {
		if code == statusCode {
			return true
		}
	}
	return false
}

// getTTL returns the TTL from endpoint config or falls back to default
func (h *Handler) getTTL(endpointConfig *config.EndpointCacheConfig) time.Duration {
	if endpointConfig != nil && endpointConfig.TTL > 0 {
		return endpointConfig.TTL
	}
	return h.config.Cache.DefaultTTL
}

// Health returns a health check handler
func (h *Handler) Health() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy","service":"api-cache"}`))
	}
}
