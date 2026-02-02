package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"

	"github.com/singh-gur/api_cache/internal/config"
	"github.com/singh-gur/api_cache/internal/logger"
)

type Client struct {
	redis  *redis.Client
	config *config.Config
}

type CachedResponse struct {
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers"`
	Body       []byte              `json:"body"`
	CachedAt   time.Time           `json:"cached_at"`
}

// NewClient creates a new cache client
func NewClient(cfg *config.Config) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Valkey.Host, cfg.Valkey.Port),
		Password:     cfg.Valkey.Password,
		DB:           cfg.Valkey.DB,
		MaxRetries:   cfg.Valkey.MaxRetries,
		PoolSize:     cfg.Valkey.PoolSize,
		MinIdleConns: cfg.Valkey.MinIdleConns,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to valkey: %w", err)
	}

	logger.Log.Info("Successfully connected to Valkey")

	return &Client{
		redis:  rdb,
		config: cfg,
	}, nil
}

// GenerateCacheKey creates a unique cache key based on request properties
func (c *Client) GenerateCacheKey(r *http.Request, endpointConfig *config.EndpointCacheConfig) string {
	var keyParts []string

	// Add method and path
	keyParts = append(keyParts, r.Method, r.URL.Path)

	// Add configured query parameters
	if endpointConfig != nil && len(endpointConfig.CacheKeyQueryParams) > 0 {
		query := r.URL.Query()
		var queryParts []string
		for _, param := range endpointConfig.CacheKeyQueryParams {
			if val := query.Get(param); val != "" {
				queryParts = append(queryParts, fmt.Sprintf("%s=%s", param, val))
			}
		}
		sort.Strings(queryParts)
		if len(queryParts) > 0 {
			keyParts = append(keyParts, strings.Join(queryParts, "&"))
		}
	}

	// Add configured headers
	if endpointConfig != nil && len(endpointConfig.CacheKeyHeaders) > 0 {
		var headerParts []string
		for _, header := range endpointConfig.CacheKeyHeaders {
			if val := r.Header.Get(header); val != "" {
				headerParts = append(headerParts, fmt.Sprintf("%s=%s", header, val))
			}
		}
		sort.Strings(headerParts)
		if len(headerParts) > 0 {
			keyParts = append(keyParts, strings.Join(headerParts, "|"))
		}
	}

	// Create hash of the key parts
	keyString := strings.Join(keyParts, ":")
	hash := sha256.Sum256([]byte(keyString))
	return "cache:" + hex.EncodeToString(hash[:])
}

// Get retrieves a cached response
func (c *Client) Get(ctx context.Context, key string) (*CachedResponse, error) {
	data, err := c.redis.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			// Check remaining TTL for debugging (key doesn't exist)
			logger.WithFields(map[string]interface{}{
				"cache_key": key,
			}).Debug("Cache miss (key not found in store)")
			return nil, nil // Cache miss
		}
		return nil, fmt.Errorf("failed to get cache: %w", err)
	}

	var cached CachedResponse
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached response: %w", err)
	}

	logFields := map[string]interface{}{
		"cache_key": key,
	}
	// Only pay the extra Valkey RTT for remaining TTL when debug logging is active
	if logger.Log.IsLevelEnabled(logrus.DebugLevel) {
		logFields["cache_age"] = time.Since(cached.CachedAt).Seconds()
		logFields["cached_at"] = cached.CachedAt.Format(time.RFC3339)
		if remainingTTL, err := c.redis.TTL(ctx, key).Result(); err == nil {
			logFields["remaining_ttl"] = remainingTTL.Seconds()
		}
	}
	logger.WithFields(logFields).Debug("Cache hit")
	return &cached, nil
}

// Set stores a response in cache
func (c *Client) Set(ctx context.Context, key string, response *CachedResponse, ttl time.Duration) error {
	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	if err := c.redis.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("failed to set cache: %w", err)
	}

	logger.WithFields(map[string]interface{}{
		"cache_key":  key,
		"ttl":        ttl.Seconds(),
		"expires_at": time.Now().Add(ttl).Format(time.RFC3339),
		"body_size":  len(response.Body),
	}).Debug("Cached response")

	return nil
}

// Delete removes a cached response
func (c *Client) Delete(ctx context.Context, key string) error {
	if err := c.redis.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete cache: %w", err)
	}
	return nil
}

// DeletePattern removes all cached responses matching a pattern
func (c *Client) DeletePattern(ctx context.Context, pattern string) error {
	iter := c.redis.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		if err := c.redis.Del(ctx, iter.Val()).Err(); err != nil {
			logger.WithField("key", iter.Val()).Error("Failed to delete cache key")
		}
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("failed to scan cache: %w", err)
	}
	return nil
}

// Close closes the cache client connection
func (c *Client) Close() error {
	return c.redis.Close()
}

// GetTTL returns the TTL for a specific endpoint or the default TTL
func (c *Client) GetTTL(path, method string, queryParams map[string][]string) time.Duration {
	if endpointConfig := c.config.GetEndpointCacheConfig(path, method, queryParams); endpointConfig != nil {
		return endpointConfig.TTL
	}
	return c.config.Cache.DefaultTTL
}
