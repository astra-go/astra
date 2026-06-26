package security

import (
	"context"
	"fmt"
	"time"

	"github.com/astra-go/astra"
	"github.com/redis/go-redis/v9"
)

// DefaultBlacklistKeyPrefix is the default Redis key prefix for the blacklist set.
const DefaultBlacklistKeyPrefix = "astra:ip_blacklist"

// IPBlacklistOption configures the IP blacklist middleware.
type IPBlacklistOption func(*ipBlacklistConfig)

type ipBlacklistConfig struct {
	keyPrefix    string
	skipper      Skipper
	errorHandler ErrorHandler
}

func newIPBlacklistConfig(keyPrefix string, opts []IPBlacklistOption) *ipBlacklistConfig {
	cfg := &ipBlacklistConfig{
		keyPrefix: keyPrefix,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// WithBlacklistSkipper sets a skipper for the IP blacklist middleware.
// When the skipper returns true the middleware passes the request through
// without checking the blacklist.
func WithBlacklistSkipper(skipper Skipper) IPBlacklistOption {
	return func(cfg *ipBlacklistConfig) {
		cfg.skipper = skipper
	}
}

// WithBlacklistErrorHandler sets a custom error handler for blocked requests.
// The default handler returns a 403 JSON response.
func WithBlacklistErrorHandler(handler ErrorHandler) IPBlacklistOption {
	return func(cfg *ipBlacklistConfig) {
		cfg.errorHandler = handler
	}
}

// IPBlacklist returns a middleware that blocks requests from blacklisted IPs.
//
// The middleware reads the client IP from the request and checks it against a
// Redis SET (SISMEMBER).  When the IP is in the set the request is rejected
// with HTTP 403.
//
// Pass nil for redisClient to make the middleware a no‑op — useful when the
// blacklist feature is optional or disabled in dev/test.
//
//	app.Use(security.IPBlacklist(redisClient, security.DefaultBlacklistKeyPrefix))
//
// Options allow a custom Skipper and ErrorHandler.
//	app.Use(security.IPBlacklist(redisClient, "my:prefix",
//	    security.WithBlacklistSkipper(skipper),
//	    security.WithBlacklistErrorHandler(myHandler),
//	))
func IPBlacklist(redisClient *redis.Client, keyPrefix string, opts ...IPBlacklistOption) astra.HandlerFunc {
	cfg := newIPBlacklistConfig(keyPrefix, opts)

	return func(c *astra.Ctx) error {
		if shouldSkip(cfg.skipper, c) {
			return c.Next()
		}
		if redisClient == nil {
			return c.Next()
		}

		ip := c.ClientIP()
		if ip == "" {
			return c.Next()
		}

		member, err := redisClient.SIsMember(context.Background(), cfg.keyPrefix, ip).Result()
		if err != nil {
			// On Redis errors, fail open (allow the request).
			return c.Next()
		}
		if member {
			if cfg.errorHandler != nil {
				return cfg.errorHandler(c)
			}
			return c.JSON(403, map[string]string{
				"code":    "SEC-IP-4031",
				"message": "access denied",
			})
		}
		return c.Next()
	}
}

// BlockIP adds an IP to the blacklist set in Redis with an optional TTL.
// When redisClient is nil the call is a no‑op.
func BlockIP(ctx context.Context, redisClient *redis.Client, keyPrefix, ip string, ttl time.Duration) error {
	if redisClient == nil {
		return nil
	}
	if err := redisClient.SAdd(ctx, keyPrefix, ip).Err(); err != nil {
		return fmt.Errorf("block ip: %w", err)
	}
	if ttl > 0 {
		return redisClient.Expire(ctx, keyPrefix, ttl).Err()
	}
	return nil
}

// UnblockIP removes an IP from the blacklist set.
// When redisClient is nil the call is a no‑op.
func UnblockIP(ctx context.Context, redisClient *redis.Client, keyPrefix, ip string) error {
	if redisClient == nil {
		return nil
	}
	return redisClient.SRem(ctx, keyPrefix, ip).Err()
}

// ListBlockedIPs returns all currently blacklisted IPs from the set.
// When redisClient is nil it returns an empty slice.
func ListBlockedIPs(ctx context.Context, redisClient *redis.Client, keyPrefix string) ([]string, error) {
	if redisClient == nil {
		return nil, nil
	}
	return redisClient.SMembers(ctx, keyPrefix).Result()
}
