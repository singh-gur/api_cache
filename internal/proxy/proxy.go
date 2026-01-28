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
	"github.com/singh-gur/api_cache/internal/middleware"
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
	requestID := middleware.GetRequestID(ctx)

	// Log incoming request
	logger.WithFields(map[string]interface{}{
		"request_id":  requestID,
		"method":      r.Method,
		"path":        r.URL.Path,
		"query":       r.URL.RawQuery,
		"remote_addr": r.RemoteAddr,
		"user_agent":  r.Header.Get("User-Agent"),
	}).Info("Incoming request")

	// Only cache GET requests
	if r.Method != http.MethodGet {
		logger.WithFields(map[string]interface{}{
			"request_id": requestID,
			"method":     r.Method,
			"path":       r.URL.Path,
		}).Debug("Non-GET request, bypassing cache")
		h.forwardRequest(w, r, ctx, requestID, startTime)
		return
	}

	// Get endpoint-specific cache config
	endpointConfig := h.config.GetEndpointCacheConfig(r.URL.Path, r.Method, r.URL.Query())

	// Generate cache key
	cacheKey := h.cache.GenerateCacheKey(r, endpointConfig)

	// Log cache key and endpoint config details
	logFields := map[string]interface{}{
		"request_id": requestID,
		"cache_key":  cacheKey,
		"path":       r.URL.Path,
		"method":     r.Method,
	}
	if endpointConfig != nil {
		logFields["ttl"] = endpointConfig.TTL.Seconds()
		if len(endpointConfig.CacheKeyQueryParams) > 0 {
			logFields["cache_key_query_params"] = endpointConfig.CacheKeyQueryParams
		}
		if len(endpointConfig.CacheKeyHeaders) > 0 {
			logFields["cache_key_headers"] = endpointConfig.CacheKeyHeaders
		}
	} else {
		logFields["ttl"] = h.config.Cache.DefaultTTL.Seconds()
	}
	logger.WithFields(logFields).Debug("Cache key generated")

	// Try to get from cache
	cached, err := h.cache.Get(ctx, cacheKey)
	if err != nil {
		logger.WithFields(map[string]interface{}{
			"request_id": requestID,
			"error":      err,
			"cache_key":  cacheKey,
		}).Error("Failed to get from cache")
	}

	if cached != nil {
		// Serve from cache
		h.serveCachedResponse(w, cached, cacheKey, requestID, startTime)
		return
	}

	logger.WithFields(map[string]interface{}{
		"request_id": requestID,
		"cache_key":  cacheKey,
	}).Debug("Cache miss")

	// Cache miss - forward request to upstream
	h.forwardAndCache(w, r, ctx, cacheKey, endpointConfig, requestID, startTime)
}

// serveCachedResponse writes a cached response to the client
func (h *Handler) serveCachedResponse(w http.ResponseWriter, cached *cache.CachedResponse, cacheKey string, requestID string, startTime time.Time) {
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
	cacheAge := time.Since(cached.CachedAt)
	logger.WithFields(map[string]interface{}{
		"request_id": requestID,
		"cache":      "hit",
		"cache_key":  cacheKey,
		"status":     cached.StatusCode,
		"duration":   duration.Milliseconds(),
		"cache_age":  cacheAge.Seconds(),
		"body_size":  len(cached.Body),
		"cached_at":  cached.CachedAt.Format(time.RFC3339),
	}).Info("Request served from cache")
}

// forwardAndCache forwards the request to upstream and caches the response
func (h *Handler) forwardAndCache(w http.ResponseWriter, r *http.Request, ctx context.Context, cacheKey string, endpointConfig *config.EndpointCacheConfig, requestID string, startTime time.Time) {
	logger.WithFields(map[string]interface{}{
		"request_id": requestID,
		"cache_key":  cacheKey,
		"path":       r.URL.Path,
		"method":     r.Method,
	}).Debug("Forwarding request to upstream")

	// Forward request with retry logic
	resp, err := h.forwardWithRetry(r, ctx, requestID)
	if err != nil {
		logger.WithFields(map[string]interface{}{
			"request_id": requestID,
			"error":      err,
			"cache_key":  cacheKey,
			"path":       r.URL.Path,
		}).Error("Failed to forward request")
		http.Error(w, "upstream service unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.WithFields(map[string]interface{}{
			"request_id": requestID,
			"error":      err,
			"cache_key":  cacheKey,
		}).Error("Failed to read response body")
		http.Error(w, "failed to read upstream response", http.StatusInternalServerError)
		return
	}

	// Cache successful responses (2xx status codes)
	cached := false
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		cachedResp := &cache.CachedResponse{
			StatusCode: resp.StatusCode,
			Headers:    resp.Header,
			Body:       body,
			CachedAt:   time.Now(),
		}

		ttl := h.getTTL(endpointConfig)
		if err := h.cache.Set(ctx, cacheKey, cachedResp, ttl); err != nil {
			logger.WithFields(map[string]interface{}{
				"request_id": requestID,
				"error":      err,
				"cache_key":  cacheKey,
			}).Error("Failed to cache response")
		} else {
			cached = true
			logger.WithFields(map[string]interface{}{
				"request_id": requestID,
				"cache_key":  cacheKey,
				"ttl":        ttl.Seconds(),
				"body_size":  len(body),
			}).Debug("Response cached successfully")
		}
	} else {
		logger.WithFields(map[string]interface{}{
			"request_id": requestID,
			"cache_key":  cacheKey,
			"status":     resp.StatusCode,
		}).Debug("Response not cached (non-2xx status)")
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
		"request_id": requestID,
		"cache":      "miss",
		"cache_key":  cacheKey,
		"status":     resp.StatusCode,
		"duration":   duration.Milliseconds(),
		"body_size":  len(body),
		"cached":     cached,
	}).Info("Request forwarded to upstream")
}

// forwardRequest forwards a non-cacheable request to upstream
func (h *Handler) forwardRequest(w http.ResponseWriter, r *http.Request, ctx context.Context, requestID string, startTime time.Time) {
	logger.WithFields(map[string]interface{}{
		"request_id": requestID,
		"method":     r.Method,
		"path":       r.URL.Path,
	}).Debug("Forwarding non-cacheable request to upstream")

	resp, err := h.forwardWithRetry(r, ctx, requestID)
	if err != nil {
		logger.WithFields(map[string]interface{}{
			"request_id": requestID,
			"error":      err,
			"method":     r.Method,
			"path":       r.URL.Path,
		}).Error("Failed to forward request")
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
	bytesWritten, _ := io.Copy(w, resp.Body)

	duration := time.Since(startTime)
	logger.WithFields(map[string]interface{}{
		"request_id": requestID,
		"method":     r.Method,
		"path":       r.URL.Path,
		"status":     resp.StatusCode,
		"duration":   duration.Milliseconds(),
		"body_size":  bytesWritten,
	}).Info("Request forwarded (non-cacheable)")
}

// forwardWithRetry forwards a request with retry logic
func (h *Handler) forwardWithRetry(r *http.Request, ctx context.Context, requestID string) (*http.Response, error) {
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

		logger.WithFields(map[string]interface{}{
			"request_id":   requestID,
			"attempt":      attempt,
			"max_attempts": maxAttempts,
			"upstream_url": upstreamURL,
			"method":       r.Method,
		}).Debug("Attempting upstream request")

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
					"request_id":   requestID,
					"attempt":      attempt,
					"max_attempts": maxAttempts,
					"error":        err,
					"backoff_ms":   backoff.Milliseconds(),
					"upstream_url": upstreamURL,
				}).Warn("Request failed, retrying")
				time.Sleep(backoff)
				backoff = time.Duration(float64(backoff) * h.config.Retry.BackoffMultiplier)
				if backoff > h.config.Retry.MaxBackoff {
					backoff = h.config.Retry.MaxBackoff
				}
				continue
			}
			logger.WithFields(map[string]interface{}{
				"request_id":   requestID,
				"attempts":     maxAttempts,
				"error":        lastErr,
				"upstream_url": upstreamURL,
			}).Error("All retry attempts exhausted")
			return nil, fmt.Errorf("all retry attempts failed: %w", lastErr)
		}

		// Check if status code is retryable
		if h.isRetryableStatus(resp.StatusCode) && attempt < maxAttempts {
			resp.Body.Close()
			logger.WithFields(map[string]interface{}{
				"request_id":   requestID,
				"attempt":      attempt,
				"max_attempts": maxAttempts,
				"status":       resp.StatusCode,
				"backoff_ms":   backoff.Milliseconds(),
				"upstream_url": upstreamURL,
			}).Warn("Retryable status code, retrying")
			time.Sleep(backoff)
			backoff = time.Duration(float64(backoff) * h.config.Retry.BackoffMultiplier)
			if backoff > h.config.Retry.MaxBackoff {
				backoff = h.config.Retry.MaxBackoff
			}
			continue
		}

		if attempt > 1 {
			logger.WithFields(map[string]interface{}{
				"request_id":   requestID,
				"attempt":      attempt,
				"status":       resp.StatusCode,
				"upstream_url": upstreamURL,
			}).Info("Request succeeded after retry")
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
