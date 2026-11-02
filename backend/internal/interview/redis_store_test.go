package interview

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/gomodule/redigo/redis"
)

// RedisStore 必须天然满足 eino 的 checkpoint 存取契约：
// compose.CheckPointStore（Task 7 可直接传给 compose.WithCheckPointStore）
// 以及可选的删除接口 adk.CheckPointDeleter（eino compose 包未导出该别名，
// adk.CheckPointDeleter 与 core.CheckPointDeleter 是同一个类型别名，方法集一致）。
var (
	_ compose.CheckPointStore = (*RedisStore)(nil)
	_ adk.CheckPointDeleter    = (*RedisStore)(nil)
)

// 测试专用 key：与真实面试数据隔离，测试结束由 t.Cleanup 清理。
const (
	testRedisAddr      = "127.0.0.1:6379"
	testCheckPointID   = "t-cp-1"
	testLockKey        = "t-lock-1"
	testProbeCheckID   = "t-probe-cp"
	testProbeLockKey   = "t-probe-lock"
	testRaceLockKey    = "t-lock-race"
	testUnavailableAID = "127.0.0.1:1" // 该端口无人监听，用于验证「Redis 不可用」信号
)

func redisStore(t *testing.T) *RedisStore {
	t.Helper()
	s, err := NewRedisStore(testRedisAddr, "", 0)
	if err != nil {
		t.Skipf("redis unavailable: %v", err)
	}
	t.Cleanup(func() {
		// 清理本测试写入的 key，避免污染真实数据
		ctx := context.Background()
		_ = s.Delete(ctx, testCheckPointID)
		_ = s.Unlock(ctx, testLockKey)
		_ = s.Delete(ctx, testProbeCheckID)
		_ = s.Unlock(ctx, testProbeLockKey)
		_ = s.Unlock(ctx, testRaceLockKey)
		_ = s.Close()
	})
	return s
}

// TestRedisStoreRedisReachable 是「不得用 skip 掩盖失败」的守卫测试：
// 本机 Redis 必须真实可达，不可用时直接 FAIL（而不是跳过）。
func TestRedisStoreRedisReachable(t *testing.T) {
	s, err := NewRedisStore(testRedisAddr, "", 0)
	if err != nil {
		t.Fatalf("redis 必须可达（不得 skip 掩盖连接失败）: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
}

func TestRedisStoreRoundTrip(t *testing.T) {
	s := redisStore(t)
	ctx := context.Background()
	if _, ok, _ := s.Get(ctx, "t-cp-1"); ok {
		t.Fatal("fresh key should miss")
	}
	if err := s.Set(ctx, "t-cp-1", []byte("payload")); err != nil {
		t.Fatalf("set: %v", err)
	}
	v, ok, err := s.Get(ctx, "t-cp-1")
	if err != nil || !ok || string(v) != "payload" {
		t.Fatalf("get = %q ok=%v err=%v", v, ok, err)
	}
	if err := s.Delete(ctx, "t-cp-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, _ := s.Get(ctx, "t-cp-1"); ok {
		t.Fatal("key should be gone after delete")
	}
}

func TestRedisStoreTryLock(t *testing.T) {
	s := redisStore(t)
	ctx := context.Background()
	ok1, err := s.TryLock(ctx, "t-lock-1", 5)
	if err != nil || !ok1 {
		t.Fatalf("first lock: ok=%v err=%v", ok1, err)
	}
	ok2, _ := s.TryLock(ctx, "t-lock-1", 5)
	if ok2 {
		t.Fatal("second lock should fail")
	}
	_ = s.Unlock(ctx, "t-lock-1")
	ok3, _ := s.TryLock(ctx, "t-lock-1", 5)
	if !ok3 {
		t.Fatal("lock should succeed after unlock")
	}
}

// TestRedisStoreGetMissing 命中缺失 key 时返回 (nil, false, nil)，不得报错。
func TestRedisStoreGetMissing(t *testing.T) {
	s := redisStore(t)
	ctx := context.Background()
	v, ok, err := s.Get(ctx, "t-missing-cp")
	if err != nil {
		t.Fatalf("get missing: %v", err)
	}
	if ok {
		t.Fatalf("missing key should not be found, got %q", v)
	}
	if v != nil {
		t.Fatalf("missing key should return nil bytes, got %q", v)
	}
}

// TestRedisStoreDeleteMissing 删除不存在的 key 不报错（幂等）。
func TestRedisStoreDeleteMissing(t *testing.T) {
	s := redisStore(t)
	if err := s.Delete(context.Background(), "t-missing-cp"); err != nil {
		t.Fatalf("delete missing should be idempotent: %v", err)
	}
}

// TestRedisStoreKeyNamespaces 校验 key 构造：checkpoint 走 interview:cp:，
// 并发锁走 interview:lock:，两个命名空间互不干扰。
func TestRedisStoreKeyNamespaces(t *testing.T) {
	s := redisStore(t)
	ctx := context.Background()

	if err := s.Set(ctx, testProbeCheckID, []byte("cp")); err != nil {
		t.Fatalf("set: %v", err)
	}
	locked, err := s.TryLock(ctx, testProbeLockKey, 30)
	if err != nil || !locked {
		t.Fatalf("try lock: locked=%v err=%v", locked, err)
	}

	conn, err := redis.Dial("tcp", testRedisAddr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	for _, key := range []string{
		checkpointKeyPrefix + testProbeCheckID,
		lockKeyPrefix + testProbeLockKey,
	} {
		n, err := redis.Int(conn.Do("EXISTS", key))
		if err != nil {
			t.Fatalf("exists %s: %v", key, err)
		}
		if n != 1 {
			t.Fatalf("expected key %s to exist", key)
		}
	}

	// 同名后缀下，checkpoint 与 lock 必须是两个不同的 key。
	if checkpointKeyPrefix+testProbeCheckID == lockKeyPrefix+testProbeCheckID {
		t.Fatal("checkpoint 与 lock 的 key 前缀冲突")
	}
	if _, ok, _ := s.Get(ctx, testProbeLockKey); ok {
		t.Fatal("lock key 不应被当作 checkpoint 读出")
	}
}

// TestRedisStoreTTL 校验 checkpoint 的 TTL 约为 7 天、lock 的 TTL 等于传入值。
func TestRedisStoreTTL(t *testing.T) {
	s := redisStore(t)
	ctx := context.Background()

	if err := s.Set(ctx, testProbeCheckID, []byte("cp")); err != nil {
		t.Fatalf("set: %v", err)
	}
	locked, err := s.TryLock(ctx, testProbeLockKey, 42)
	if err != nil || !locked {
		t.Fatalf("try lock: locked=%v err=%v", locked, err)
	}

	conn, err := redis.Dial("tcp", testRedisAddr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	cpTTL, err := redis.Int(conn.Do("TTL", checkpointKeyPrefix+testProbeCheckID))
	if err != nil {
		t.Fatalf("ttl cp: %v", err)
	}
	if cpTTL < int(checkpointTTL.Seconds())-60 || cpTTL > int(checkpointTTL.Seconds()) {
		t.Fatalf("checkpoint ttl = %d, want ~%d", cpTTL, int(checkpointTTL.Seconds()))
	}

	lockTTL, err := redis.Int(conn.Do("TTL", lockKeyPrefix+testProbeLockKey))
	if err != nil {
		t.Fatalf("ttl lock: %v", err)
	}
	if lockTTL < 40 || lockTTL > 42 {
		t.Fatalf("lock ttl = %d, want ~42", lockTTL)
	}
}

// TestRedisStoreLockConcurrency 并发抢锁时只有一个成功。
func TestRedisStoreLockConcurrency(t *testing.T) {
	s := redisStore(t)
	ctx := context.Background()

	const workers = 8
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		winners int
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := s.TryLock(ctx, testRaceLockKey, 10)
			if err != nil {
				t.Errorf("try lock: %v", err)
				return
			}
			if ok {
				mu.Lock()
				winners++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if winners != 1 {
		t.Fatalf("winners = %d, want exactly 1", winners)
	}
}

// TestRedisStoreUsableAsCheckPointStore 验证 Task 7 的接入方式：
// RedisStore 可以直接传给 compose.WithCheckPointStore，不需要任何适配层；
// 同时它实现了 eino 可选的 CheckPointDeleter（删除接口）。
func TestRedisStoreUsableAsCheckPointStore(t *testing.T) {
	s := redisStore(t)

	var opt compose.GraphCompileOption = compose.WithCheckPointStore(s)
	if opt == nil {
		t.Fatal("compose.WithCheckPointStore must accept *RedisStore")
	}
	if _, ok := interface{}(s).(adk.CheckPointDeleter); !ok {
		t.Fatal("*RedisStore must implement CheckPointDeleter")
	}
}

// TestRedisStoreUnavailable Redis 不可达时 NewRedisStore 必须返回错误（T8/T9 依赖该信号）。
func TestRedisStoreUnavailable(t *testing.T) {
	start := time.Now()
	s, err := NewRedisStore(testUnavailableAID, "", 0)
	if err == nil {
		_ = s.Close()
		t.Fatal("expected error when redis is unreachable")
	}
	if s != nil {
		t.Fatal("expected nil store on error")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("dial took too long: %v", elapsed)
	}
}
