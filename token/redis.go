package token

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisManager implements Manager using Redis.
// Each entry is stored as JSON with its appropriate TTL.
//
// Key layout (configurable via keyPrefix):
//
//	{prefix}access:{hash}    → JSON(Entry)  TTL = access token TTL
//	{prefix}refresh:{hash}  → JSON(Entry)  TTL = refresh token TTL
//	{prefix}account:{UIN}   → JSON(Entry)  TTL = refresh token TTL
//	{prefix}blacklist:{hash} → "1"         TTL = remaining TTL
//	{prefix}jti:{jti}        → "1"         TTL = configured
type RedisManager struct {
	client    *redis.Client
	keyPrefix string
}

// NewRedisManager creates a Redis-backed token manager.
//
//	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	mgr := token.NewRedisManager(rdb, "myapp:token:")
func NewRedisManager(client *redis.Client, keyPrefix string) *RedisManager {
	return &RedisManager{
		client:    client,
		keyPrefix: keyPrefix,
	}
}

// ─── Manager interface ──────────────────────────────────────────────────────

func (r *RedisManager) Issue(ctx context.Context, uin int64, accessTTL, refreshTTL time.Duration) (string, string, *Entry, error) {
	// SSO: evict previous pair for this user.
	if err := r.DeleteByAccount(ctx, uin); err != nil {
		return "", "", nil, fmt.Errorf("token: sso evict: %w", err)
	}

	accessToken := generateToken()
	refreshToken := generateToken()
	now := time.Now()

	entry := &Entry{
		UIN:           uin,
		TokenHash:     hashToken(accessToken),
		RefreshHash:   hashToken(refreshToken),
		ExpiresAt:     now.Add(accessTTL).Unix(),
		RefreshExpiry: now.Add(refreshTTL).Unix(),
		CreatedAt:     now.Unix(),
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		return "", "", nil, fmt.Errorf("token: marshal: %w", err)
	}

	pipe := r.client.TxPipeline()
	pipe.Set(ctx, r.accessKey(entry.TokenHash), payload, accessTTL)
	pipe.Set(ctx, r.refreshKey(entry.RefreshHash), payload, refreshTTL)
	pipe.Set(ctx, r.accountKey(uin), payload, refreshTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", "", nil, fmt.Errorf("token: redis pipeline: %w", err)
	}

	return accessToken, refreshToken, entry, nil
}

func (r *RedisManager) FindByAccess(ctx context.Context, token string) (*Entry, error) {
	// Check blacklist first.
	blacklisted, err := r.IsBlacklisted(ctx, token)
	if err != nil {
		return nil, err
	}
	if blacklisted {
		return nil, nil
	}

	payload, err := r.client.Get(ctx, r.accessKey(hashToken(token))).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("token: redis get: %w", err)
	}
	return r.decodeEntry(payload)
}

func (r *RedisManager) FindByRefresh(ctx context.Context, refreshToken string) (*Entry, error) {
	payload, err := r.client.Get(ctx, r.refreshKey(hashToken(refreshToken))).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("token: redis get: %w", err)
	}
	return r.decodeEntry(payload)
}

func (r *RedisManager) DeleteByAccess(ctx context.Context, token string) error {
	entry, err := r.FindByAccess(ctx, token)
	if err != nil {
		return err
	}
	if entry == nil {
		return nil
	}
	return r.client.Del(ctx,
		r.accessKey(entry.TokenHash),
		r.refreshKey(entry.RefreshHash),
		r.accountKey(entry.UIN),
	).Err()
}

func (r *RedisManager) DeleteByRefresh(ctx context.Context, refreshToken string) error {
	entry, err := r.FindByRefresh(ctx, refreshToken)
	if err != nil {
		return err
	}
	if entry == nil {
		return nil
	}
	return r.client.Del(ctx,
		r.accessKey(entry.TokenHash),
		r.refreshKey(entry.RefreshHash),
		r.accountKey(entry.UIN),
	).Err()
}

func (r *RedisManager) DeleteByAccount(ctx context.Context, uin int64) error {
	payload, err := r.client.Get(ctx, r.accountKey(uin)).Bytes()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("token: redis get account: %w", err)
	}
	entry, err := r.decodeEntry(payload)
	if err != nil {
		return err
	}
	return r.client.Del(ctx,
		r.accessKey(entry.TokenHash),
		r.refreshKey(entry.RefreshHash),
		r.accountKey(uin),
	).Err()
}

func (r *RedisManager) Blacklist(ctx context.Context, token string, ttl time.Duration) error {
	key := r.blacklistKey(hashToken(token))
	return r.client.Set(ctx, key, "1", ttl).Err()
}

func (r *RedisManager) IsBlacklisted(ctx context.Context, token string) (bool, error) {
	key := r.blacklistKey(hashToken(token))
	result, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("token: redis exists: %w", err)
	}
	return result > 0, nil
}

func (r *RedisManager) SetJTI(ctx context.Context, jti string, ttl time.Duration) error {
	key := r.jtiKey(jti)
	return r.client.Set(ctx, key, "1", ttl).Err()
}

func (r *RedisManager) IsJTIUsed(ctx context.Context, jti string) (bool, error) {
	key := r.jtiKey(jti)
	result, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("token: redis exists jti: %w", err)
	}
	return result > 0, nil
}

// ─── key helpers ─────────────────────────────────────────────────────────────

func (r *RedisManager) accessKey(hash string) string  { return r.keyPrefix + "access:" + hash }
func (r *RedisManager) refreshKey(hash string) string { return r.keyPrefix + "refresh:" + hash }
func (r *RedisManager) accountKey(uin int64) string   { return fmt.Sprintf("%saccount:%d", r.keyPrefix, uin) }
func (r *RedisManager) blacklistKey(hash string) string { return r.keyPrefix + "blacklist:" + hash }
func (r *RedisManager) jtiKey(jti string) string      { return r.keyPrefix + "jti:" + jti }

// ─── helpers ─────────────────────────────────────────────────────────────────

func (r *RedisManager) decodeEntry(payload []byte) (*Entry, error) {
	var entry Entry
	if err := json.Unmarshal(payload, &entry); err != nil {
		return nil, fmt.Errorf("token: unmarshal entry: %w", err)
	}
	return &entry, nil
}
