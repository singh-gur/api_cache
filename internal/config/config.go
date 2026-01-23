package config

import (
	"fmt"
	"os"
	"regexp"
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
	Path                string        `yaml:"path"`
	PathRegex           string        `yaml:"path_regex"`
	Methods             []string      `yaml:"methods"`
	TTL                 time.Duration `yaml:"ttl"`
	CacheKeyHeaders     []string      `yaml:"cache_key_headers"`
	CacheKeyQueryParams []string      `yaml:"cache_key_query_params"`

	// Compiled regex pattern (not serialized)
	compiledRegex *regexp.Regexp `yaml:"-"`
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
	Level    string `yaml:"level"`
	Format   string `yaml:"format"`
	Output   string `yaml:"output"`
	FilePath string `yaml:"file_path"`
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

// GetEndpointCacheConfig returns the cache configuration for a specific endpoint
// Supports both exact path matching and regex patterns
func (c *Config) GetEndpointCacheConfig(path, method string) *EndpointCacheConfig {
	for i := range c.Cache.Endpoints {
		ep := &c.Cache.Endpoints[i]

		// Check if method matches
		methodMatches := false
		for _, m := range ep.Methods {
			if m == method {
				methodMatches = true
				break
			}
		}
		if !methodMatches {
			continue
		}

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
