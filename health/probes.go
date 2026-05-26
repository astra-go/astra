package health

import (
	"context"
	"fmt"
	"net/http"
)

// RedisProbe returns a ProbeFunc that pings a redis.UniversalClient.
// Accepts any value with a Ping method matching the redis client interface,
// so the health package does not import a specific Redis driver.
func RedisProbe(client interface {
	Ping(ctx context.Context) interface{ Err() error }
}) ProbeFunc {
	return func(ctx context.Context) error {
		return client.Ping(ctx).Err()
	}
}

// HTTPProbe returns a ProbeFunc that performs an HTTP GET to url.
// Succeeds when the response status code is < 500.
func HTTPProbe(url string) ProbeFunc {
	return func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= http.StatusInternalServerError {
			return fmt.Errorf("health: upstream %s returned %d", url, resp.StatusCode)
		}
		return nil
	}
}
