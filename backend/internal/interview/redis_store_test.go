package interview

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
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
	_ adk.CheckPointDeleter   = (*RedisStore)(nil)
)

// 测试专用的 key 与地址常量。
//
// 关键约束（AGENTS.md「本地开发和自动化测试不得依赖外部 Redis 服务」）：
// 本文件不连接任何真实 Redis。所有用例都跑在进程内的 miniredis 测试替身上，
// 每个用例一个独立实例，实例销毁即清空，无需额外 cleanup，也不会污染任何共享实例。
const (
	testCheckPointID   = "t-cp-1"
	testLockKey        = "t-lock-1"
	testProbeCheckID   = "t-probe-cp"
	testProbeLockKey   = "t-probe-lock"
	testRaceLockKey    = "t-lock-race"
	testExpireLockKey  = "t-lock-expire"
	testUnavailableAID = "127.0.0.1:1" // 测试专属：保留端口无人监听，用于验证「Redis 不可用」信号
)

// redisStore 启动一个进程内 miniredis 作为 Redis 测试替身，并返回连上去的
// RedisStore 与该替身实例。
//
// 测试替身总是可用（进程内 TCP 监听），所以这里不允许 skip：一旦连不上，
// 说明被测代码或测试替身本身有问题，必须直接 FAIL。
func redisStore(t *testing.T) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	s, err := NewRedisStore(mr.Addr(), "", 0)
	if err != nil {
		t.Fatalf("NewRedisStore(miniredis %s): %v", mr.Addr(), err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, mr
}

// dialTestDouble 直接用 redigo 连到测试替身，用于校验协议层行为（EXISTS/TTL）。
func dialTestDouble(t *testing.T, mr *miniredis.Miniredis) redis.Conn {
	t.Helper()
	conn, err := redis.Dial("tcp", mr.Addr())
	if err != nil {
		t.Fatalf("dial test double: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// TestRedisStoreConnectsToTestDouble 校验 RedisStore 能连上测试替身并正常读写。
//
// 这是原「Redis 必须可达」守卫测试的替代版本：原测试直连本机真实 Redis 端口，
// 既让 CI（无 Redis service）必然失败，又违反「测试不得连接共享 Redis」。
// 现在改为连进程内 miniredis：断言依然严格（连不上直接 FAIL，不 skip），
// 但不再依赖任何外部服务。
func TestRedisStoreConnectsToTestDouble(t *testing.T) {
	s, mr := redisStore(t)
	ctx := context.Background()

	if err := s.Set(ctx, testCheckPointID, []byte("payload")); err != nil {
		t.Fatalf("set: %v", err)
	}
	v, ok, err := s.Get(ctx, testCheckPointID)
	if err != nil || !ok || string(v) != "payload" {
		t.Fatalf("get = %q ok=%v err=%v", v, ok, err)
	}
	if !mr.Exists(checkpointKeyPrefix + testCheckPointID) {
		// miniredis 替身里 key 不存在，说明写入没有落到替身上。
		t.Fatalf("expected key %q in test double", checkpointKeyPrefix+testCheckPointID)
	}
}

// TestRedisStoreRoundTrip 校验 checkpoint 的写入-读取-删除闭环。
func TestRedisStoreRoundTrip(t *testing.T) {
	s, _ := redisStore(t)
	ctx := context.Background()
	if _, ok, _ := s.Get(ctx, testCheckPointID); ok {
		t.Fatal("fresh key should miss")
	}
	if err := s.Set(ctx, testCheckPointID, []byte("payload")); err != nil {
		t.Fatalf("set: %v", err)
	}
	v, ok, err := s.Get(ctx, testCheckPointID)
	if err != nil || !ok || string(v) != "payload" {
		t.Fatalf("get = %q ok=%v err=%v", v, ok, err)
	}
	if err := s.Delete(ctx, testCheckPointID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, _ := s.Get(ctx, testCheckPointID); ok {
		t.Fatal("key should be gone after delete")
	}
}

// TestRedisStoreTryLock 校验 SET NX 语义：首次加锁成功、重复加锁失败、解锁后可再获取。
func TestRedisStoreTryLock(t *testing.T) {
	s, _ := redisStore(t)
	ctx := context.Background()
	ok1, err := s.TryLock(ctx, testLockKey, 5)
	if err != nil || !ok1 {
		t.Fatalf("first lock: ok=%v err=%v", ok1, err)
	}
	ok2, err := s.TryLock(ctx, testLockKey, 5)
	if err != nil {
		t.Fatalf("second lock: unexpected err=%v", err)
	}
	if ok2 {
		t.Fatal("second lock should fail")
	}
	if err := s.Unlock(ctx, testLockKey); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	ok3, err := s.TryLock(ctx, testLockKey, 5)
	if err != nil || !ok3 {
		t.Fatalf("lock after unlock: ok=%v err=%v", ok3, err)
	}
}

// TestRedisStoreGetMissing 命中缺失 key 时返回 (nil, false, nil)，不得报错。
func TestRedisStoreGetMissing(t *testing.T) {
	s, _ := redisStore(t)
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
	s, _ := redisStore(t)
	if err := s.Delete(context.Background(), "t-missing-cp"); err != nil {
		t.Fatalf("delete missing should be idempotent: %v", err)
	}
}

// TestRedisStoreKeyNamespaces 校验 key 构造：checkpoint 走 interview:cp:，
// 并发锁走 interview:lock:，两个命名空间互不干扰。
func TestRedisStoreKeyNamespaces(t *testing.T) {
	s, mr := redisStore(t)
	ctx := context.Background()

	if err := s.Set(ctx, testProbeCheckID, []byte("cp")); err != nil {
		t.Fatalf("set: %v", err)
	}
	locked, err := s.TryLock(ctx, testProbeLockKey, 30)
	if err != nil || !locked {
		t.Fatalf("try lock: locked=%v err=%v", locked, err)
	}

	conn := dialTestDouble(t, mr)

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
	//
	// 注意：这里必须用运行时构造的字符串比较。若直接比较两个常量表达式
	// （checkpointKeyPrefix+x 与 lockKeyPrefix+x），Go 编译器会在编译期把
	// 条件折叠成恒假，t.Fatal 永远不会触发——那是一条没有任何保护力的断言。
	cpKey := fmt.Sprintf("%s%s", checkpointKeyPrefix, testProbeCheckID)
	lockKey := fmt.Sprintf("%s%s", lockKeyPrefix, testProbeCheckID)
	if cpKey == lockKey {
		t.Fatalf("checkpoint 与 lock 的 key 前缀冲突: %q", cpKey)
	}
	if _, ok, _ := s.Get(ctx, testProbeLockKey); ok {
		t.Fatal("lock key 不应被当作 checkpoint 读出")
	}
}

// TestRedisStoreTTL 校验 checkpoint 的 TTL 约为 7 天、lock 的 TTL 等于传入值。
func TestRedisStoreTTL(t *testing.T) {
	s, mr := redisStore(t)
	ctx := context.Background()

	if err := s.Set(ctx, testProbeCheckID, []byte("cp")); err != nil {
		t.Fatalf("set: %v", err)
	}
	locked, err := s.TryLock(ctx, testProbeLockKey, 42)
	if err != nil || !locked {
		t.Fatalf("try lock: locked=%v err=%v", locked, err)
	}

	conn := dialTestDouble(t, mr)

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

// TestRedisStoreLockExpires 校验并发锁在 TTL 到期后自动释放。
//
// 用 miniredis 的 FastForward 把替身时钟快进，等价于真实 Redis 里锁自然过期，
// 不需要真的 sleep 30 秒。
func TestRedisStoreLockExpires(t *testing.T) {
	s, mr := redisStore(t)
	ctx := context.Background()

	locked, err := s.TryLock(ctx, testExpireLockKey, 30)
	if err != nil || !locked {
		t.Fatalf("try lock: locked=%v err=%v", locked, err)
	}
	again, err := s.TryLock(ctx, testExpireLockKey, 30)
	if err != nil {
		t.Fatalf("try lock again: %v", err)
	}
	if again {
		t.Fatal("lock should still be held before TTL")
	}

	mr.FastForward(31 * time.Second)

	after, err := s.TryLock(ctx, testExpireLockKey, 30)
	if err != nil {
		t.Fatalf("try lock after expiry: %v", err)
	}
	if !after {
		t.Fatal("lock should be acquirable after TTL expiry")
	}
}

// TestRedisStoreLockConcurrency 并发抢锁时只有一个成功。
func TestRedisStoreLockConcurrency(t *testing.T) {
	s, _ := redisStore(t)
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
	s, _ := redisStore(t)

	var opt compose.GraphCompileOption = compose.WithCheckPointStore(s)
	if opt == nil {
		t.Fatal("compose.WithCheckPointStore must accept *RedisStore")
	}
	if _, ok := interface{}(s).(adk.CheckPointDeleter); !ok {
		t.Fatal("*RedisStore must implement CheckPointDeleter")
	}
}

// TestRedisStoreUnavailable Redis 不可达时 NewRedisStore 必须返回错误（T8/T9 依赖该信号）。
//
// 这里用的是测试专属的保留端口 127.0.0.1:1（确定无人监听），不是任何可能被
// 占用的真实 Redis 端口。
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
