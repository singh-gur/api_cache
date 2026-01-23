package cache

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/singh-gur/api_cache/internal/config"
)

func TestGenerateCacheKey(t *testing.T) {
	cfg := &config.Config{}
	client := &Client{config: cfg}

	endpointConfig := &config.EndpointCacheConfig{
		Path:                "/api/users",
		Methods:             []string{"GET"},
		CacheKeyQueryParams: []string{"page"},
	}

	tests := []struct {
		name           string
		method         string
		path           string
		query          map[string]string
		headers        map[string]string
		endpointConfig *config.EndpointCacheConfig
		expectUnique   bool
	}{
		{
			name:           "same request should generate same key",
			method:         "GET",
			path:           "/api/users",
			query:          map[string]string{"page": "1"},
			endpointConfig: endpointConfig,
			expectUnique:   false,
		},
		{
			name:           "different query params should generate different key",
			method:         "GET",
			path:           "/api/users",
			query:          map[string]string{"page": "2"},
			endpointConfig: endpointConfig,
			expectUnique:   true,
		},
		{
			name:           "different methods should generate different key",
			method:         "POST",
			path:           "/api/users",
			query:          map[string]string{"page": "1"},
			endpointConfig: endpointConfig,
			expectUnique:   true,
		},
	}

	var previousKey string
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			req := &http.Request{
				Method: tt.method,
				URL: &url.URL{
					Path: tt.path,
				},
				Header: make(http.Header),
			}

			// Add query parameters
			q := req.URL.Query()
			for k, v := range tt.query {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()

			// Add headers
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			// Generate cache key
			key := client.GenerateCacheKey(req, tt.endpointConfig)

			if key == "" {
				t.Error("cache key should not be empty")
			}

			// Check uniqueness
			if previousKey != "" {
				if tt.expectUnique && key == previousKey {
					t.Errorf("expected unique key, got same key: %s", key)
				}
				if !tt.expectUnique && key != previousKey {
					t.Errorf("expected same key, got different key: %s vs %s", key, previousKey)
				}
			}

			previousKey = key
		})
	}
}

func TestGenerateCacheKeyWithEndpointConfig(t *testing.T) {
	cfg := &config.Config{}
	client := &Client{config: cfg}

	endpointConfig := &config.EndpointCacheConfig{
		Path:                "/api/users",
		Methods:             []string{"GET"},
		CacheKeyHeaders:     []string{"Authorization"},
		CacheKeyQueryParams: []string{"page"},
	}

	// Create two requests with same path but different Authorization headers
	req1 := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Path:     "/api/users",
			RawQuery: "page=1&limit=10",
		},
		Header: http.Header{
			"Authorization": []string{"Bearer token1"},
		},
	}

	req2 := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Path:     "/api/users",
			RawQuery: "page=1&limit=10",
		},
		Header: http.Header{
			"Authorization": []string{"Bearer token2"},
		},
	}

	key1 := client.GenerateCacheKey(req1, endpointConfig)
	key2 := client.GenerateCacheKey(req2, endpointConfig)

	if key1 == key2 {
		t.Error("different Authorization headers should generate different cache keys")
	}

	// Test that unconfigured query params don't affect cache key
	req3 := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Path:     "/api/users",
			RawQuery: "page=1&limit=20", // different limit
		},
		Header: http.Header{
			"Authorization": []string{"Bearer token1"},
		},
	}

	key3 := client.GenerateCacheKey(req3, endpointConfig)

	if key1 != key3 {
		t.Error("unconfigured query params should not affect cache key")
	}
}
