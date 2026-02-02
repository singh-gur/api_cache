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

	result := cfg.GetEndpointCacheConfig("/api/v1/users", "GET", nil)
	if result == nil {
		t.Error("Expected to find exact match, got nil")
	}

	result = cfg.GetEndpointCacheConfig("/api/v1/users", "POST", nil)
	if result != nil {
		t.Error("Expected nil for wrong method, got result")
	}

	result = cfg.GetEndpointCacheConfig("/api/v1/products", "GET", nil)
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
			result := cfg.GetEndpointCacheConfig(tt.path, tt.method, nil)
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
	result := cfg.GetEndpointCacheConfig("/api/v1/users", "GET", nil)
	if result == nil {
		t.Fatal("Expected to find match")
	}
	if result.TTL != 600*time.Second {
		t.Errorf("Expected exact match TTL (600s), got %v", result.TTL)
	}

	// Regex should match other paths
	result = cfg.GetEndpointCacheConfig("/api/v1/products", "GET", nil)
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

func TestGetEndpointCacheConfig_QueryParamExactMatch(t *testing.T) {
	cfg := &Config{
		Cache: CacheConfig{
			Endpoints: []EndpointCacheConfig{
				{
					Path:             "/query",
					Methods:          []string{"GET"},
					TTL:              24 * time.Hour,
					MatchQueryParams: map[string][]string{"function": {"EOD", "WEEKLY", "MONTHLY"}},
				},
				{
					Path:             "/query",
					Methods:          []string{"GET"},
					TTL:              5 * time.Minute,
					MatchQueryParams: map[string][]string{"function": {"INTRADAY"}},
				},
				{
					Path:    "/query",
					Methods: []string{"GET"},
					TTL:     1 * time.Hour,
				},
			},
		},
	}

	tests := []struct {
		name        string
		queryParams map[string][]string
		expectedTTL time.Duration
		description string
	}{
		{
			name:        "EOD function",
			queryParams: map[string][]string{"function": {"EOD"}},
			expectedTTL: 24 * time.Hour,
			description: "should match EOD endpoint with 24h TTL",
		},
		{
			name:        "WEEKLY function",
			queryParams: map[string][]string{"function": {"WEEKLY"}},
			expectedTTL: 24 * time.Hour,
			description: "should match WEEKLY in same list as EOD",
		},
		{
			name:        "INTRADAY function",
			queryParams: map[string][]string{"function": {"INTRADAY"}},
			expectedTTL: 5 * time.Minute,
			description: "should match INTRADAY endpoint with 5m TTL",
		},
		{
			name:        "unknown function",
			queryParams: map[string][]string{"function": {"UNKNOWN"}},
			expectedTTL: 1 * time.Hour,
			description: "should fall back to default endpoint with 1h TTL",
		},
		{
			name:        "no function param",
			queryParams: map[string][]string{"other": {"value"}},
			expectedTTL: 1 * time.Hour,
			description: "should fall back to default endpoint when param missing",
		},
		{
			name:        "nil query params",
			queryParams: nil,
			expectedTTL: 1 * time.Hour,
			description: "should fall back to default endpoint with nil params",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cfg.GetEndpointCacheConfig("/query", "GET", tt.queryParams)
			if result == nil {
				t.Fatalf("Expected to find match for %s", tt.description)
			}
			if result.TTL != tt.expectedTTL {
				t.Errorf("%s: expected TTL %v, got %v", tt.description, tt.expectedTTL, result.TTL)
			}
		})
	}
}

func TestGetEndpointCacheConfig_QueryParamRegexMatch(t *testing.T) {
	cfg := &Config{
		Cache: CacheConfig{
			Endpoints: []EndpointCacheConfig{
				{
					Path:                  "/api/data",
					Methods:               []string{"GET"},
					TTL:                   10 * time.Minute,
					MatchQueryParamsRegex: map[string][]string{"type": {"^daily$", "^weekly$"}},
					compiledQueryParamRegex: map[string][]*regexp.Regexp{
						"type": {regexp.MustCompile("^daily$"), regexp.MustCompile("^weekly$")},
					},
				},
			},
		},
	}

	tests := []struct {
		name        string
		queryParams map[string][]string
		shouldMatch bool
	}{
		{"daily type", map[string][]string{"type": {"daily"}}, true},
		{"weekly type", map[string][]string{"type": {"weekly"}}, true},
		{"monthly type", map[string][]string{"type": {"monthly"}}, false},
		{"partial match", map[string][]string{"type": {"daily-report"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cfg.GetEndpointCacheConfig("/api/data", "GET", tt.queryParams)
			if tt.shouldMatch && result == nil {
				t.Errorf("Expected match for %s", tt.name)
			}
			if !tt.shouldMatch && result != nil {
				t.Errorf("Expected no match for %s", tt.name)
			}
		})
	}
}

func TestGetEndpointCacheConfig_MixedExactAndRegex(t *testing.T) {
	cfg := &Config{
		Cache: CacheConfig{
			Endpoints: []EndpointCacheConfig{
				{
					Path:                  "/api/report",
					Methods:               []string{"GET"},
					TTL:                   30 * time.Minute,
					MatchQueryParams:      map[string][]string{"format": {"json", "xml"}},
					MatchQueryParamsRegex: map[string][]string{"type": {"^summary.*$"}},
					compiledQueryParamRegex: map[string][]*regexp.Regexp{
						"type": {regexp.MustCompile("^summary.*$")},
					},
				},
			},
		},
	}

	tests := []struct {
		name        string
		queryParams map[string][]string
		shouldMatch bool
	}{
		{
			name:        "both match",
			queryParams: map[string][]string{"format": {"json"}, "type": {"summary-v2"}},
			shouldMatch: true,
		},
		{
			name:        "exact matches but regex fails",
			queryParams: map[string][]string{"format": {"json"}, "type": {"detailed"}},
			shouldMatch: false,
		},
		{
			name:        "regex matches but exact fails",
			queryParams: map[string][]string{"format": {"csv"}, "type": {"summary"}},
			shouldMatch: false,
		},
		{
			name:        "missing regex param",
			queryParams: map[string][]string{"format": {"json"}},
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cfg.GetEndpointCacheConfig("/api/report", "GET", tt.queryParams)
			if tt.shouldMatch && result == nil {
				t.Errorf("Expected match for %s", tt.name)
			}
			if !tt.shouldMatch && result != nil {
				t.Errorf("Expected no match for %s, got TTL %v", tt.name, result.TTL)
			}
		})
	}
}

func TestCompileRegexPatterns_QueryParamRegex(t *testing.T) {
	cfg := &Config{
		Cache: CacheConfig{
			Endpoints: []EndpointCacheConfig{
				{
					Path:                  "/query",
					Methods:               []string{"GET"},
					MatchQueryParamsRegex: map[string][]string{"function": {"^EOD$", "^INTRADAY$"}},
				},
			},
		},
	}

	err := cfg.compileRegexPatterns()
	if err != nil {
		t.Fatalf("Failed to compile regex patterns: %v", err)
	}

	if cfg.Cache.Endpoints[0].compiledQueryParamRegex == nil {
		t.Error("Query param regex map not initialized")
	}

	regexList := cfg.Cache.Endpoints[0].compiledQueryParamRegex["function"]
	if len(regexList) != 2 {
		t.Errorf("Expected 2 compiled regexes for 'function', got %d", len(regexList))
	}
}

func TestCompileRegexPatterns_InvalidQueryParamRegex(t *testing.T) {
	cfg := &Config{
		Cache: CacheConfig{
			Endpoints: []EndpointCacheConfig{
				{
					Path:                  "/query",
					Methods:               []string{"GET"},
					MatchQueryParamsRegex: map[string][]string{"function": {"[invalid(regex"}},
				},
			},
		},
	}

	err := cfg.compileRegexPatterns()
	if err == nil {
		t.Error("Expected error for invalid query param regex, got nil")
	}
}

func TestSanitizeQuery(t *testing.T) {
	tests := []struct {
		name              string
		redactQueryParams []string
		rawQuery          string
		expected          string
	}{
		{
			name:              "redacts single param",
			redactQueryParams: []string{"apikey"},
			rawQuery:          "function=TIME_SERIES_DAILY&symbol=IBM&apikey=secret123",
			expected:          "function=TIME_SERIES_DAILY&symbol=IBM&apikey=%5BREDACTED%5D",
		},
		{
			name:              "redacts multiple params",
			redactQueryParams: []string{"apikey", "token"},
			rawQuery:          "apikey=secret&function=QUOTE&token=abc123",
			expected:          "apikey=%5BREDACTED%5D&function=QUOTE&token=%5BREDACTED%5D",
		},
		{
			name:              "no redaction when param not present",
			redactQueryParams: []string{"apikey"},
			rawQuery:          "function=TIME_SERIES_DAILY&symbol=IBM",
			expected:          "function=TIME_SERIES_DAILY&symbol=IBM",
		},
		{
			name:              "empty redact list returns raw query unchanged",
			redactQueryParams: []string{},
			rawQuery:          "function=QUOTE&apikey=secret",
			expected:          "function=QUOTE&apikey=secret",
		},
		{
			name:              "nil redact list returns raw query unchanged",
			redactQueryParams: nil,
			rawQuery:          "function=QUOTE&apikey=secret",
			expected:          "function=QUOTE&apikey=secret",
		},
		{
			name:              "empty query string",
			redactQueryParams: []string{"apikey"},
			rawQuery:          "",
			expected:          "",
		},
		{
			name:              "preserves parameter order",
			redactQueryParams: []string{"apikey"},
			rawQuery:          "symbol=IBM&apikey=secret&function=QUOTE",
			expected:          "symbol=IBM&apikey=%5BREDACTED%5D&function=QUOTE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Logging: LoggingConfig{
					RedactQueryParams: tt.redactQueryParams,
				},
			}

			result := cfg.SanitizeQuery(tt.rawQuery)
			if result != tt.expected {
				t.Errorf("SanitizeQuery(%q) = %q, want %q", tt.rawQuery, result, tt.expected)
			}
		})
	}
}
