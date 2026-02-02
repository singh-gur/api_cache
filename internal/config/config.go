package config

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Valkey    ValkeyConfig    `yaml:"valkey"`
	Cache     CacheConfig     `yaml:"cache"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Retry     RetryConfig     `yaml:"retry"`
	Upstream  UpstreamConfig  `yaml:"upstream"`
	Logging   LoggingConfig   `yaml:"logging"`
}

type ServerConfig struct {
	Host         string        `yaml:"host"`
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

type ValkeyConfig struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	Password     string `yaml:"password"`
	DB           int    `yaml:"db"`
	MaxRetries   int    `yaml:"max_retries"`
	PoolSize     int    `yaml:"pool_size"`
	MinIdleConns int    `yaml:"min_idle_conns"`
}

type CacheConfig struct {
	DefaultTTL time.Duration         `yaml:"default_ttl"`
	MaxTTL     time.Duration         `yaml:"max_ttl"`
	Endpoints  []EndpointCacheConfig `yaml:"endpoints"`
}

type EndpointCacheConfig struct {
	Path                  string              `yaml:"path"`
	PathRegex             string              `yaml:"path_regex"`
	Methods               []string            `yaml:"methods"`
	TTL                   time.Duration       `yaml:"ttl"`
	CacheKeyHeaders       []string            `yaml:"cache_key_headers"`
	CacheKeyQueryParams   []string            `yaml:"cache_key_query_params"`
	MatchQueryParams      map[string][]string `yaml:"match_query_params"`
	MatchQueryParamsRegex map[string][]string `yaml:"match_query_params_regex"`

	// Compiled regex pattern (not serialized)
	compiledRegex           *regexp.Regexp              `yaml:"-"`
	compiledQueryParamRegex map[string][]*regexp.Regexp `yaml:"-"`
}

type RateLimitConfig struct {
	Enabled           bool                      `yaml:"enabled"`
	RequestsPerSecond float64                   `yaml:"requests_per_second"`
	Burst             int                       `yaml:"burst"`
	Endpoints         []EndpointRateLimitConfig `yaml:"endpoints"`
}

type EndpointRateLimitConfig struct {
	Path              string  `yaml:"path"`
	PathRegex         string  `yaml:"path_regex"`
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	Burst             int     `yaml:"burst"`

	// Compiled regex pattern (not serialized)
	compiledRegex *regexp.Regexp `yaml:"-"`
}

type RetryConfig struct {
	Enabled              bool          `yaml:"enabled"`
	MaxAttempts          int           `yaml:"max_attempts"`
	InitialBackoff       time.Duration `yaml:"initial_backoff"`
	MaxBackoff           time.Duration `yaml:"max_backoff"`
	BackoffMultiplier    float64       `yaml:"backoff_multiplier"`
	RetryableStatusCodes []int         `yaml:"retryable_status_codes"`
}

type UpstreamConfig struct {
	BaseURL         string        `yaml:"base_url"`
	Timeout         time.Duration `yaml:"timeout"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	MaxConnsPerHost int           `yaml:"max_conns_per_host"`
}

type LoggingConfig struct {
	Level             string   `yaml:"level"`
	Format            string   `yaml:"format"`
	Output            string   `yaml:"output"`
	FilePath          string   `yaml:"file_path"`
	RedactQueryParams []string `yaml:"redact_query_params"`
}

const redactedValue = "[REDACTED]"

// SanitizeQuery returns a query string with sensitive parameter values replaced
// by [REDACTED]. The list of parameters to redact is configured via
// logging.redact_query_params. If the list is empty, the raw query is returned
// unchanged.
func (c *Config) SanitizeQuery(rawQuery string) string {
	if len(c.Logging.RedactQueryParams) == 0 || rawQuery == "" {
		return rawQuery
	}

	parsed, err := url.ParseQuery(rawQuery)
	if err != nil {
		return rawQuery
	}

	for _, param := range c.Logging.RedactQueryParams {
		if _, exists := parsed[param]; exists {
			parsed.Set(param, redactedValue)
		}
	}

	// Rebuild query string preserving original parameter order.
	// url.Values.Encode() sorts keys alphabetically which makes diffing
	// against the original harder, so we rebuild manually.
	var parts []string
	seen := make(map[string]bool)

	// Walk the original query to preserve key order
	for rawQuery != "" {
		var key string
		key, rawQuery, _ = strings.Cut(rawQuery, "&")
		paramName, _, _ := strings.Cut(key, "=")
		paramName, _ = url.QueryUnescape(paramName)
		if seen[paramName] {
			continue
		}
		seen[paramName] = true
		for _, v := range parsed[paramName] {
			parts = append(parts, url.QueryEscape(paramName)+"="+url.QueryEscape(v))
		}
	}

	return strings.Join(parts, "&")
}

// Load reads and parses the configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Compile regex patterns for endpoints
	if err := cfg.compileRegexPatterns(); err != nil {
		return nil, fmt.Errorf("failed to compile regex patterns: %w", err)
	}

	return &cfg, nil
}

// validate checks if the configuration is valid
func (c *Config) validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	if c.Valkey.Port <= 0 || c.Valkey.Port > 65535 {
		return fmt.Errorf("invalid valkey port: %d", c.Valkey.Port)
	}

	if c.Upstream.BaseURL == "" {
		return fmt.Errorf("upstream base_url is required")
	}

	return nil
}

// compileRegexPatterns compiles regex patterns for all endpoints
func (c *Config) compileRegexPatterns() error {
	// Compile cache endpoint patterns
	for i := range c.Cache.Endpoints {
		ep := &c.Cache.Endpoints[i]
		if ep.PathRegex != "" {
			regex, err := regexp.Compile(ep.PathRegex)
			if err != nil {
				return fmt.Errorf("invalid cache regex pattern for endpoint %q: %w", ep.PathRegex, err)
			}
			ep.compiledRegex = regex
		}

		// Compile query param regex patterns
		if len(ep.MatchQueryParamsRegex) > 0 {
			ep.compiledQueryParamRegex = make(map[string][]*regexp.Regexp)
			for param, patterns := range ep.MatchQueryParamsRegex {
				for _, pattern := range patterns {
					regex, err := regexp.Compile(pattern)
					if err != nil {
						return fmt.Errorf("invalid query param regex pattern for param %q: %w", param, err)
					}
					ep.compiledQueryParamRegex[param] = append(ep.compiledQueryParamRegex[param], regex)
				}
			}
		}
	}

	// Compile rate limit endpoint patterns
	for i := range c.RateLimit.Endpoints {
		ep := &c.RateLimit.Endpoints[i]
		if ep.PathRegex != "" {
			regex, err := regexp.Compile(ep.PathRegex)
			if err != nil {
				return fmt.Errorf("invalid rate limit regex pattern for endpoint %q: %w", ep.PathRegex, err)
			}
			ep.compiledRegex = regex
		}
	}

	return nil
}

// EndpointIdentifier returns a human-readable string identifying the endpoint config.
// Useful for logging which config entry matched a request.
func (ep *EndpointCacheConfig) EndpointIdentifier() string {
	if ep.Path != "" {
		return ep.Path
	}
	if ep.PathRegex != "" {
		return "regex:" + ep.PathRegex
	}
	return "<unknown>"
}

// MatchType describes how a request path matched this endpoint config.
type MatchType string

const (
	MatchTypeExact         MatchType = "exact"
	MatchTypeRegex         MatchType = "regex"
	MatchTypeQueryParam    MatchType = "query_param"
	MatchTypeFallbackExact MatchType = "fallback_exact"
	MatchTypeFallbackRegex MatchType = "fallback_regex"
	MatchTypeDefault       MatchType = "default"
)

// EndpointMatch holds the result of endpoint config resolution, including
// how the match was made for debugging purposes.
type EndpointMatch struct {
	Config    *EndpointCacheConfig
	MatchType MatchType
}

// GetEndpointCacheConfig returns the cache configuration for a specific endpoint
// Supports both exact path matching and regex patterns
// Query params with match_query_params take precedence over endpoints without
func (c *Config) GetEndpointCacheConfig(path, method string, queryParams map[string][]string) *EndpointCacheConfig {
	match := c.GetEndpointCacheConfigMatch(path, method, queryParams)
	return match.Config
}

// GetEndpointCacheConfigMatch returns the cache configuration and match metadata
// for a specific endpoint. The MatchType field describes how the endpoint was resolved.
func (c *Config) GetEndpointCacheConfigMatch(path, method string, queryParams map[string][]string) EndpointMatch {
	var fallbackMatch *EndpointCacheConfig
	var fallbackMatchType MatchType

	for i := range c.Cache.Endpoints {
		ep := &c.Cache.Endpoints[i]

		// Check if method matches
		if !slices.Contains(ep.Methods, method) {
			continue
		}

		// Check path match (exact or regex)
		pathMatches := false
		pathMatchType := MatchTypeExact
		if ep.Path != "" && ep.Path == path {
			pathMatches = true
			pathMatchType = MatchTypeExact
		} else if ep.compiledRegex != nil && ep.compiledRegex.MatchString(path) {
			pathMatches = true
			pathMatchType = MatchTypeRegex
		}

		if !pathMatches {
			continue
		}

		// If no query param matching configured, this is a potential fallback
		hasQueryParamMatching := len(ep.MatchQueryParams) > 0 || len(ep.compiledQueryParamRegex) > 0
		if !hasQueryParamMatching {
			if fallbackMatch == nil {
				fallbackMatch = ep
				fallbackMatchType = pathMatchType
			}
			continue
		}

		// Check query param matching - all configured params must match
		allParamsMatch := true

		// Check exact match params
		for param, allowedValues := range ep.MatchQueryParams {
			values, exists := queryParams[param]
			if !exists || len(values) == 0 {
				allParamsMatch = false
				break
			}
			matched := false
			for _, v := range allowedValues {
				if v == values[0] {
					matched = true
					break
				}
			}
			if !matched {
				allParamsMatch = false
				break
			}
		}

		// Check regex match params
		if allParamsMatch {
			for param, regexList := range ep.compiledQueryParamRegex {
				values, exists := queryParams[param]
				if !exists || len(values) == 0 {
					allParamsMatch = false
					break
				}
				matched := false
				for _, regex := range regexList {
					if regex.MatchString(values[0]) {
						matched = true
						break
					}
				}
				if !matched {
					allParamsMatch = false
					break
				}
			}
		}

		if allParamsMatch {
			return EndpointMatch{Config: ep, MatchType: MatchTypeQueryParam}
		}
	}

	if fallbackMatch != nil {
		matchType := MatchTypeFallbackExact
		if fallbackMatchType == MatchTypeRegex {
			matchType = MatchTypeFallbackRegex
		}
		return EndpointMatch{Config: fallbackMatch, MatchType: matchType}
	}

	return EndpointMatch{Config: nil, MatchType: MatchTypeDefault}
}

// GetEndpointRateLimitConfig returns the rate limit configuration for a specific endpoint
// Supports both exact path matching and regex patterns
func (c *Config) GetEndpointRateLimitConfig(path string) *EndpointRateLimitConfig {
	for i := range c.RateLimit.Endpoints {
		ep := &c.RateLimit.Endpoints[i]

		// Check exact path match first
		if ep.Path != "" && ep.Path == path {
			return ep
		}

		// Check regex match
		if ep.compiledRegex != nil && ep.compiledRegex.MatchString(path) {
			return ep
		}
	}
	return nil
}
