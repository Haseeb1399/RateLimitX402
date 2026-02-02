package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the server.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	RateLimit RateLimitConfig `yaml:"ratelimit"`
	Payment   PaymentConfig   `yaml:"payment"`
	Redis     RedisConfig     `yaml:"redis"`
}

// ServerConfig holds server-related configuration.
type ServerConfig struct {
	Port string `yaml:"port"`
}

// RateLimitConfig holds rate limiter configuration.
type RateLimitConfig struct {
	Capacity   float64 `yaml:"capacity"`
	RefillRate float64 `yaml:"refill_rate"`
	Strategy   string  `yaml:"strategy"` // "memory" or "redis"
}

// RedisConfig holds Redis connection configuration.
type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// PaymentConfig holds payment configuration for 402 responses.
type PaymentConfig struct {
	Enabled          bool   `yaml:"enabled"`
	FacilitatorURL   string `yaml:"facilitator_url"`
	WalletAddress    string `yaml:"wallet_address"`
	PricePerCapacity string `yaml:"price_per_capacity"`
	Network          string `yaml:"network"`
	Currency         string `yaml:"currency"`
}

// Load reads a YAML config file and returns a Config struct.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
