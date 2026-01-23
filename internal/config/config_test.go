package config

import (
	"regexp"
	"testing"
	"time"
)

func TestGetEndpointCacheConfig_ExactMatch(t *testing.T) {
	cfg := &Config{
		Cache: CacheConfig{
			Endpoints: []EndpointCacheConfig{
				{
					Path:    "/api/v1/users",
					Methods: []string{"GET"},
					TTL:     600 * time.Second,
				},
			},
		},
	}

	result := cfg.GetEndpointCacheConfig("/api/v1/users", "GET")
	if result == nil {
		t.Error("Expected to find exact match, got nil")
	}

	result = cfg.GetEndpointCacheConfig("/api/v1/users", "POST")
	if result != nil {
		t.Error("Expected nil for wrong method, got result")
	}

	result = cfg.GetEndpointCacheConfig("/api/v1/products", "GET")
	if result != nil {
		t.Error("Expected nil for different path, got result")
	}
}

func TestGetEndpointCacheConfig_RegexMatch(t *testing.T) {
	cfg := &Config{
		Cache: CacheConfig{
			Endpoints: []EndpointCacheConfig{
				{
					PathRegex:     "^/api/v1/users/[0-9]+$",
					Methods:       []string{"GET"},
					TTL:           900 * time.Second,
					compiledRegex: regexp.MustCompile("^/api/v1/users/[0-9]+$"),
				},
			},
		},
	}

	tests := []struct {
		path        string
		method      string
		shouldMatch bool
		description string
	}{
		{"/api/v1/users/123", "GET", true, "numeric user ID"},
		{"/api/v1/users/456", "GET", true, "different numeric user ID"},
		{"/api/v1/users/abc", "GET", false, "non-numeric user ID"},
		{"/api/v1/users/123/posts", "GET", false, "nested path"},
		{"/api/v1/users/123", "POST", false, "wrong method"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := cfg.GetEndpointCacheConfig(tt.path, tt.method)
			if tt.shouldMatch && result == nil {
				t.Errorf("Expected match for %s %s, got nil", tt.method, tt.path)
			}
			if !tt.shouldMatch && result != nil {
				t.Errorf("Expected no match for %s %s, got result", tt.method, tt.path)
			}
		})
	}
}

func TestGetEndpointCacheConfig_PriorityExactOverRegex(t *testing.T) {
	cfg := &Config{
		Cache: CacheConfig{
			Endpoints: []EndpointCacheConfig{
				{
					Path:    "/api/v1/users",
					Methods: []string{"GET"},
					TTL:     600 * time.Second,
				},
				{
					PathRegex:     "^/api/v1/.*",
					Methods:       []string{"GET"},
					TTL:           300 * time.Second,
					compiledRegex: regexp.MustCompile("^/api/v1/.*"),
				},
			},
		},
	}

	// Exact match should be found first
	result := cfg.GetEndpointCacheConfig("/api/v1/users", "GET")
	if result == nil {
		t.Fatal("Expected to find match")
	}
	if result.TTL != 600*time.Second {
		t.Errorf("Expected exact match TTL (600s), got %v", result.TTL)
	}

	// Regex should match other paths
	result = cfg.GetEndpointCacheConfig("/api/v1/products", "GET")
	if result == nil {
		t.Fatal("Expected regex to match")
	}
	if result.TTL != 300*time.Second {
		t.Errorf("Expected regex match TTL (300s), got %v", result.TTL)
	}
}

func TestGetEndpointRateLimitConfig_RegexMatch(t *testing.T) {
	cfg := &Config{
		RateLimit: RateLimitConfig{
			Endpoints: []EndpointRateLimitConfig{
				{
					Path:              "/api/v1/search",
					RequestsPerSecond: 10,
					Burst:             20,
				},
				{
					PathRegex:         "^/api/v1/admin/.*",
					RequestsPerSecond: 5,
					Burst:             10,
					compiledRegex:     regexp.MustCompile("^/api/v1/admin/.*"),
				},
			},
		},
	}

	// Exact match
	result := cfg.GetEndpointRateLimitConfig("/api/v1/search")
	if result == nil || result.RequestsPerSecond != 10 {
		t.Error("Expected exact match for search endpoint")
	}

	// Regex match
	result = cfg.GetEndpointRateLimitConfig("/api/v1/admin/users")
	if result == nil || result.RequestsPerSecond != 5 {
		t.Error("Expected regex match for admin endpoint")
	}

	result = cfg.GetEndpointRateLimitConfig("/api/v1/admin/settings")
	if result == nil || result.RequestsPerSecond != 5 {
		t.Error("Expected regex match for admin settings endpoint")
	}

	// No match
	result = cfg.GetEndpointRateLimitConfig("/api/v1/products")
	if result != nil {
		t.Error("Expected no match for products endpoint")
	}
}

func TestCompileRegexPatterns(t *testing.T) {
	cfg := &Config{
		Cache: CacheConfig{
			Endpoints: []EndpointCacheConfig{
				{
					PathRegex: "^/api/v1/users/[0-9]+$",
					Methods:   []string{"GET"},
				},
			},
		},
		RateLimit: RateLimitConfig{
			Endpoints: []EndpointRateLimitConfig{
				{
					PathRegex: "^/api/v1/admin/.*",
				},
			},
		},
	}

	err := cfg.compileRegexPatterns()
	if err != nil {
		t.Fatalf("Failed to compile regex patterns: %v", err)
	}

	if cfg.Cache.Endpoints[0].compiledRegex == nil {
		t.Error("Cache endpoint regex not compiled")
	}

	if cfg.RateLimit.Endpoints[0].compiledRegex == nil {
		t.Error("Rate limit endpoint regex not compiled")
	}
}

func TestCompileRegexPatterns_InvalidRegex(t *testing.T) {
	cfg := &Config{
		Cache: CacheConfig{
			Endpoints: []EndpointCacheConfig{
				{
					PathRegex: "[invalid(regex",
					Methods:   []string{"GET"},
				},
			},
		},
	}

	err := cfg.compileRegexPatterns()
	if err == nil {
		t.Error("Expected error for invalid regex, got nil")
	}
}
