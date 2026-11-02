package interview

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/gomodule/redigo/redis"
)

const (
	// checkpointKeyPrefix 是面试 checkpoint 的 Redis key 前缀：
	// 完整 key 为 interview:cp:{checkPointID}。
	checkpointKeyPrefix = "interview:cp:"
	// lockKeyPrefix 是并发锁的 Redis key 前缀：完整 key 为 interview:lock:{key}。
	//
	// 它与 checkpointKeyPrefix 是两个互不相交的命名空间：即便 checkPointID 与
	// lock key 同名，interview:cp:x 与 interview:lock:x 也是两个不同的 key，
	// 因此「按面试 id 加锁」不会覆盖任何 checkpoint。
	lockKeyPrefix = "interview:lock:"
	// checkpointTTL 是 checkpoint 的存活时长：7 天。
	checkpointTTL = 7 * 24 * time.Hour
)

// RedisStore 是面试 checkpoint 的持久化层，同时提供基于 SET NX 的并发锁。
//
// 它天然满足 eino 的 compose.CheckPointStore（Get/Set）与可选的
// CheckPointDeleter（Delete）接口，因此可以直接
// compose.WithCheckPointStore(store) 交给图编译，不需要额外适配层。
type RedisStore struct {
	pool *redis.Pool
}

// RedisStore 必须满足 eino 的 checkpoint 存取契约（编译期断言）。
var _ compose.CheckPointStore = (*RedisStore)(nil)

// NewRedisStore 连接 Redis 并做一次 PING 验证。
//
// 返回非 nil 的 error 表示 Redis 不可用（连不上、认证失败、PING 无响应），
// 调用方据此决定降级：例如接口层返回 503，或干脆不注册面试相关的 service。
// 失败时不会留下半初始化的连接池。
func NewRedisStore(addr, password string, db int) (*RedisStore, error) {
	pool := &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 60 * time.Second,
		// MaxActive 限制单进程最多占用的连接数；Wait 让请求排队而不是直接失败。
		MaxActive: 16,
		Wait:      true,
		DialContext: func(ctx context.Context) (redis.Conn, error) {
			return redis.DialContext(ctx, "tcp", addr,
				redis.DialPassword(password),
				redis.DialDatabase(db),
				redis.DialConnectTimeout(3*time.Second),
				redis.DialReadTimeout(5*time.Second),
				redis.DialWriteTimeout(5*time.Second),
			)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := pool.GetContext(ctx)
	if err != nil {
		_ = pool.Close()
		return nil, fmt.Errorf("redis dial %s: %w", addr, err)
	}
	defer conn.Close()

	if _, err := conn.Do("PING"); err != nil {
		_ = pool.Close()
		return nil, fmt.Errorf("redis ping %s: %w", addr, err)
	}
	return &RedisStore{pool: pool}, nil
}

// Close 关闭连接池。可重复调用。
func (s *RedisStore) Close() error {
	return s.pool.Close()
}

// Get 读取 checkpoint；key 不存在时返回 (nil, false, nil)，不算错误。
func (s *RedisStore) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	conn, err := s.pool.GetContext(ctx)
	if err != nil {
		return nil, false, err
	}
	defer conn.Close()

	value, err := redis.Bytes(conn.Do("GET", checkpointKeyPrefix+checkPointID))
	if errors.Is(err, redis.ErrNil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

// Set 写入 checkpoint 并设置 7 天过期时间（SET ... EX，一次命令完成，不会出现漏设 TTL）。
func (s *RedisStore) Set(ctx context.Context, checkPointID string, cp []byte) error {
	conn, err := s.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Do("SET", checkpointKeyPrefix+checkPointID, cp, "EX", int(checkpointTTL.Seconds()))
	return err
}

// Delete 删除 checkpoint；key 不存在时也不报错（幂等）。
func (s *RedisStore) Delete(ctx context.Context, checkPointID string) error {
	conn, err := s.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Do("DEL", checkpointKeyPrefix+checkPointID)
	return err
}

// TryLock 尝试对 key 加锁（SET key 1 NX EX ttl）。
//
// 返回 true 表示本次调用抢到了锁；返回 (false, nil) 表示锁已被别人持有——
// 这不是错误，调用方据此跳过或稍后重试。只有 Redis 本身出错时才返回 error。
// ttlSeconds 必须为正数：非正数直接报错，避免产生永不过期的死锁锁。
func (s *RedisStore) TryLock(ctx context.Context, key string, ttlSeconds int) (bool, error) {
	if ttlSeconds <= 0 {
		return false, fmt.Errorf("interview: lock ttl must be positive, got %d", ttlSeconds)
	}
	conn, err := s.pool.GetContext(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	reply, err := conn.Do("SET", lockKeyPrefix+key, "1", "NX", "EX", ttlSeconds)
	if err != nil {
		return false, err
	}
	// SET NX 在 key 已存在时不写入，回复是 nil bulk reply：这是「没抢到」，不是错误。
	if reply == nil {
		return false, nil
	}
	status, err := redis.String(reply, nil)
	if err != nil {
		return false, err
	}
	return status == "OK", nil
}

// Unlock 释放锁（DEL）。
//
// 注意：这是不带持有者校验的删除。调用方必须保证「谁加锁谁解锁」，且 TTL 要大于
// 业务处理耗时；否则锁过期后被他人重新获取时，本次 Unlock 可能误删他人的锁。
func (s *RedisStore) Unlock(ctx context.Context, key string) error {
	conn, err := s.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Do("DEL", lockKeyPrefix+key)
	return err
}
