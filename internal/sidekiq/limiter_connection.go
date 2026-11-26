package sidekiq

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// LimiterConnection optionally separates limiter inspection from the job pool.
// Cluster URLs accept additional seeds as repeated addr query parameters.
type LimiterConnection struct {
	URL      string
	Cluster  bool
	Timezone string
}

// WithLimiterConnection configures the limiter pool and bucket calendar zone.
func WithLimiterConnection(config LimiterConnection) ClientOption {
	return func(c *Client) { c.limiterConfig = config }
}

func (c *Client) openLimiterConnection() error {
	config := c.limiterConfig
	if config.Timezone == "" {
		config.Timezone = "UTC"
	}
	location, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return errors.New("invalid limiter timezone (use an IANA timezone name)")
	}
	c.limiterLocation = location
	if config.URL == "" {
		if config.Cluster {
			return errors.New("--limiter-cluster requires --limiter-redis")
		}
		c.limiterRedis, c.limiterDisplay = c.redis, "Job Redis"
		return nil
	}
	if !c.capabilities.SupportsRateLimits() {
		return errors.New("--limiter-redis requires --enable=ent")
	}
	if config.Cluster {
		parsed, parseErr := url.Parse(config.URL)
		if parseErr != nil {
			return fmt.Errorf("parse limiter cluster URL: %w", redisURLParseError(parseErr))
		}
		if parsed.Path != "" && parsed.Path != "/" && parsed.Path != "/0" {
			return errors.New("limiter Redis Cluster supports database 0 only")
		}
		opts, parseErr := redis.ParseClusterURL(config.URL)
		if parseErr != nil {
			return fmt.Errorf("parse limiter cluster URL: %w", redisURLParseError(parseErr))
		}
		for _, address := range opts.Addrs {
			if strings.ContainsAny(address, "@/\\?#\r\n\t") {
				return errors.New("invalid limiter cluster address")
			}
		}
		opts.MaxRetries, opts.MaxRedirects = -1, 3
		opts.DialTimeout, opts.ReadTimeout, opts.WriteTimeout = 2*time.Second, 2*time.Second, 2*time.Second
		opts.ContextTimeoutEnabled = true
		opts.PoolSize, opts.MaxActiveConns, opts.PipelinePoolSize = uiRedisPoolSize, uiRedisPoolSize, -1
		// Discovery and record reads must agree on primary-owned state.
		opts.ReadOnly, opts.RouteByLatency, opts.RouteRandomly = false, false, false
		c.limiterRedis = redis.NewClusterClient(opts)
		c.limiterDisplay = "Cluster: " + strings.Join(opts.Addrs, ", ")
		return nil
	}
	opts, err := redis.ParseURL(config.URL)
	if err != nil {
		return fmt.Errorf("parse limiter Redis URL: %w", redisURLParseError(err))
	}
	opts.MaxRetries = -1
	opts.DialTimeout, opts.ReadTimeout, opts.WriteTimeout = 2*time.Second, 2*time.Second, 2*time.Second
	opts.ContextTimeoutEnabled = true
	opts.PoolSize, opts.MaxActiveConns, opts.PipelinePoolSize = uiRedisPoolSize, uiRedisPoolSize, -1
	c.limiterRedis = redis.NewClient(opts)
	// Construct from parsed options; never display userinfo or query options.
	c.limiterDisplay = fmt.Sprintf("%s %s / DB %d", opts.Network, opts.Addr, opts.DB)
	return nil
}

// DisplayLimiterRedis returns a credential-free pool description.
func (c *Client) DisplayLimiterRedis() string { return c.limiterDisplay }
