package speech

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"resumepro/backend/internal/interview"
)

// stateUpdate 记录一次 UpdateAudioState 调用，用于断言「没有写回状态」。
type stateUpdate struct {
	questionID int64
	state      string
}

// fakeStateStore 模拟 interview_questions.audio_state 的状态机，
// 包括 ClaimAudioState 的原子抢占语义。
type fakeStateStore struct {
	mu       sync.Mutex
	states   map[int64]string
	owners   map[int64]int64 // questionID -> interviewID
	pending  []interview.PendingAudio
	claims   int
	releases int
	updates  []stateUpdate

	// resetInterviewErr 让 ResetAudioStateByInterview 返回错误（模拟 DB 写失败）。
	resetInterviewErr error

	// claimEntered / claimRelease 让 ClaimAudioState 阻塞，
	// 用于制造「抢占已发出但尚未返回」的窗口。
	claimEntered     chan struct{}
	claimRelease     chan struct{}
	claimEnteredOnce sync.Once

	// claimHasDeadline 记录最近一次抢占收到的 ctx 是否带 deadline。
	claimHasDeadline bool
}

func (f *fakeStateStore) seed(questionID, interviewID int64, state string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensure()
	f.states[questionID] = state
	f.owners[questionID] = interviewID
}

func (f *fakeStateStore) ensure() {
	if f.states == nil {
		f.states = map[int64]string{}
	}
	if f.owners == nil {
		f.owners = map[int64]int64{}
	}
}

func (f *fakeStateStore) state(questionID int64) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.states[questionID]
}

func (f *fakeStateStore) claimCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.claims
}

// claimDeadlineRecorded 报告最近一次抢占收到的 ctx 是否带 deadline。
func (f *fakeStateStore) claimDeadlineRecorded() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.claimHasDeadline
}

func (f *fakeStateStore) updateLog() []stateUpdate {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]stateUpdate(nil), f.updates...)
}

// setResetInterviewError 注入 ResetAudioStateByInterview 的失败。
func (f *fakeStateStore) setResetInterviewError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resetInterviewErr = err
}

func (f *fakeStateStore) UpdateAudioState(_ context.Context, questionID int64, state string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensure()
	f.states[questionID] = state
	f.updates = append(f.updates, stateUpdate{questionID: questionID, state: state})
	return nil
}

// ClaimAudioState 模拟条件 UPDATE：只有 NULL / failed 能被抢到。
//
// 真实仓储用的是带 ctx 的 SQL：ctx 取消时抢占会失败。
// 这里同样尊重 ctx，否则「请求 ctx 被取消」的用例测不出问题。
func (f *fakeStateStore) ClaimAudioState(ctx context.Context, questionID int64) (bool, error) {
	f.mu.Lock()
	if _, ok := ctx.Deadline(); ok {
		f.claimHasDeadline = true
	}
	entered, release := f.claimEntered, f.claimRelease
	f.mu.Unlock()

	if entered != nil {
		// 模拟「抢占请求已发出、尚未返回」的窗口。
		f.claimEnteredOnce.Do(func() { close(entered) })
		select {
		case <-release:
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}

	if err := ctx.Err(); err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensure()
	f.claims++
	current := f.states[questionID]
	if current == "pending" || current == "ready" {
		return false, nil
	}
	f.states[questionID] = "pending"
	return true, nil
}

func (f *fakeStateStore) ReleaseAudioState(ctx context.Context, questionID int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensure()
	f.releases++
	if f.states[questionID] == "pending" {
		f.states[questionID] = ""
	}
	return nil
}

func (f *fakeStateStore) ResetStalePendingAudio(_ context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensure()
	var n int64
	for id, s := range f.states {
		if s == "pending" {
			f.states[id] = ""
			n++
		}
	}
	return n, nil
}

func (f *fakeStateStore) ResetAudioStateByInterview(ctx context.Context, interviewID int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensure()
	if f.resetInterviewErr != nil {
		return f.resetInterviewErr
	}
	for id, owner := range f.owners {
		if owner == interviewID {
			f.states[id] = ""
		}
	}
	return nil
}

func (f *fakeStateStore) ActiveQuestionsNeedingAudio(_ context.Context) ([]interview.PendingAudio, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]interview.PendingAudio(nil), f.pending...), nil
}

// countingSynth 是可控的假合成器：可指定前 N 次失败、可阻塞、可返回指定字节、可 panic。
type countingSynth struct {
	mu          sync.Mutex
	calls       int
	failures    int
	payload     []byte
	panicOnCall int // 第 N 次调用直接 panic（0 表示不 panic）

	entered     chan struct{}
	enteredOnce sync.Once
	release     chan struct{}

	// onCall 在每次合成时被调用（用于制造持续入队压力等场景）。
	onCall func(call int)
}

func (s *countingSynth) Synthesize(ctx context.Context, _ SynthesisRequest) (SynthesisResult, error) {
	s.mu.Lock()
	s.calls++
	current := s.calls
	s.mu.Unlock()

	if s.entered != nil {
		s.enteredOnce.Do(func() { close(s.entered) })
	}
	if s.release != nil {
		select {
		case <-s.release:
		case <-ctx.Done():
			return SynthesisResult{}, ctx.Err()
		}
	}
	if s.onCall != nil {
		s.onCall(current)
	}
	if s.panicOnCall > 0 && current == s.panicOnCall {
		panic("provider exploded")
	}
	if current <= s.failures {
		return SynthesisResult{}, errors.New("boom")
	}
	payload := s.payload
	if payload == nil {
		payload = testWAV()
	}
	return SynthesisResult{Audio: payload, ContentType: "audio/wav"}, nil
}

func (s *countingSynth) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// testWAV 构造一个最小的合法 PCM WAV：单声道 8000Hz 16bit，data 块 16 字节。
//
// 生成器落盘前会跑 ValidateWAV，所以假合成器必须返回真 WAV，不能用 "RIFFok" 糊弄。
func testWAV() []byte {
	const sampleRate = 8000
	const channels = 1
	const bitsPerSample = 16
	const dataSize = 16

	out := make([]byte, 0, 44+dataSize)
	out = append(out, 'R', 'I', 'F', 'F')
	out = binary.LittleEndian.AppendUint32(out, uint32(36+dataSize))
	out = append(out, 'W', 'A', 'V', 'E')
	out = append(out, 'f', 'm', 't', ' ')
	out = binary.LittleEndian.AppendUint32(out, 16)
	out = binary.LittleEndian.AppendUint16(out, 1) // PCM
	out = binary.LittleEndian.AppendUint16(out, channels)
	out = binary.LittleEndian.AppendUint32(out, sampleRate)
	out = binary.LittleEndian.AppendUint32(out, sampleRate*channels*bitsPerSample/8)
	out = binary.LittleEndian.AppendUint16(out, channels*bitsPerSample/8)
	out = binary.LittleEndian.AppendUint16(out, bitsPerSample)
	out = append(out, 'd', 'a', 't', 'a')
	out = binary.LittleEndian.AppendUint32(out, dataSize)
	return append(out, make([]byte, dataSize)...)
}

// waitFor 轮询到条件成立为止；超时即失败。
//
// 全部并发用例都用它做同步，不用固定 sleep 判定「某事有没有发生」：
// 固定 sleep 只能证明「一段时间内没发生」，且容易 flaky。
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func waitForState(t *testing.T, store *fakeStateStore, questionID int64, want string) {
	t.Helper()
	waitFor(t, "question "+strconv.FormatInt(questionID, 10)+" state = "+want, func() bool {
		return store.state(questionID) == want
	})
}

func newTestGenerator(t *testing.T, synth Synthesizer, states *fakeStateStore, maxAttempts int) (*Generator, *AudioStore) {
	t.Helper()
	return newTestGeneratorWithSizes(t, synth, states, maxAttempts, generatorQueueSize, generatorBacklogSize)
}

// newTestGeneratorWithSizes 用小容量队列，避免为了测 backlog 跑几千个任务。
func newTestGeneratorWithSizes(t *testing.T, synth Synthesizer, states *fakeStateStore, maxAttempts, queueSize, backlogLimit int) (*Generator, *AudioStore) {
	t.Helper()
	store := NewAudioStore(t.TempDir())
	cfg := Config{MaxAttempts: maxAttempts, Backoff: []time.Duration{time.Millisecond}}
	return newGenerator(synth, store, states, cfg, queueSize, backlogLimit), store
}

func TestGeneratorRetriesThenMarksReady(t *testing.T) {
	synth := &countingSynth{failures: 1}
	states := &fakeStateStore{}
	states.seed(100, 10, "")
	generator, store := newTestGenerator(t, synth, states, 3)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	generator.Enqueue(ctx, 1, 10, 100, "题面")
	waitForState(t, states, 100, "ready")

	data, err := store.ReadQuestionAudio(1, 10, 100)
	if err != nil {
		t.Fatalf("read audio: %v", err)
	}
	if !bytes.Equal(data, testWAV()) {
		t.Fatalf("audio = %d bytes, want %d bytes", len(data), len(testWAV()))
	}
	if got := synth.callCount(); got != 2 {
		t.Fatalf("synthesized %d times, want 2 (one failure then one success)", got)
	}
}

func TestGeneratorMarksFailedAfterMaxAttempts(t *testing.T) {
	synth := &countingSynth{failures: 99}
	states := &fakeStateStore{}
	states.seed(100, 10, "")
	generator, _ := newTestGenerator(t, synth, states, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	generator.Enqueue(ctx, 1, 10, 100, "题面")
	waitForState(t, states, 100, "failed")

	if got := synth.callCount(); got != 2 {
		t.Fatalf("synthesized %d times, want 2 (max attempts)", got)
	}
}

// 竞争 1：同一题被并发入队（重启重载 + 创建面试）时只合成一次。
//
// 断言不用 sleep：借一题「屏障题」确定性地证明 worker 已经把队列排空 ——
// worker 串行消费，屏障题变 ready 说明前面的题都处理完了。
func TestGeneratorEnqueueSkipsWhenAlreadyClaimed(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{}
	states.seed(100, 10, "pending") // 已被别的路径抢占
	states.seed(200, 20, "")        // 屏障题
	generator, _ := newTestGenerator(t, synth, states, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	generator.Enqueue(ctx, 1, 10, 100, "题面")
	generator.Enqueue(ctx, 1, 20, 200, "题面")
	waitForState(t, states, 200, "ready")

	if got := synth.callCount(); got != 1 {
		t.Fatalf("synthesized %d times, want 1 (the claimed question must be skipped)", got)
	}
	if got := states.state(100); got != "pending" {
		t.Fatalf("question 100 state = %q, want pending (untouched)", got)
	}
}

// 竞争 3：同一题重复入队只合成一次。
func TestGeneratorEnqueueDeduplicatesSameQuestion(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{}
	states.seed(100, 10, "")
	states.seed(300, 30, "") // 屏障题
	generator, _ := newTestGenerator(t, synth, states, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	generator.Enqueue(ctx, 1, 10, 100, "题面")
	generator.Enqueue(ctx, 1, 10, 100, "题面")
	waitForState(t, states, 100, "ready")

	// 屏障题：它变 ready 说明 worker 已经把队列排空，此后再来重复任务也能被观察到。
	generator.Enqueue(ctx, 1, 30, 300, "题面")
	waitForState(t, states, 300, "ready")

	if got := synth.callCount(); got != 2 {
		t.Fatalf("synthesized %d times, want 2 (one per question)", got)
	}
}

// 竞争 2：合成进行中面试结束 —— 不得落盘、不得把状态写成 ready 或 failed。
func TestGeneratorDoesNotStoreAfterInterviewClosed(t *testing.T) {
	synth := &countingSynth{entered: make(chan struct{}), release: make(chan struct{})}
	states := &fakeStateStore{}
	states.seed(100, 10, "")
	states.seed(300, 30, "") // 屏障题
	generator, store := newTestGenerator(t, synth, states, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	generator.Enqueue(ctx, 1, 10, 100, "题面")
	<-synth.entered // 合成已在进行中

	// 面试在此刻结束：置终止标记、状态归零、删目录。
	generator.CleanupInterview(ctx, 1, 10)
	close(synth.release)

	// 屏障题：变 ready 说明上一题已经被 worker 处理完。
	generator.Enqueue(ctx, 1, 30, 300, "题面")
	waitForState(t, states, 300, "ready")

	if got := states.state(100); got != "" {
		t.Fatalf("question 100 state = %q, want empty after the interview closed", got)
	}
	for _, update := range states.updateLog() {
		if update.questionID == 100 && (update.state == "ready" || update.state == "failed") {
			t.Fatalf("question 100 audio state written %q after the interview closed", update.state)
		}
	}
	if _, err := store.ReadQuestionAudio(1, 10, 100); err == nil {
		t.Fatal("audio file must not exist after the interview closed")
	}
	if got := synth.callCount(); got != 2 {
		t.Fatalf("synthesized %d times, want 2 (closed question + barrier question)", got)
	}
}

// markFailed 的「已终止」守卫：合成失败发生在面试结束之后时，不得写回 failed。
func TestGeneratorDoesNotMarkFailedAfterInterviewClosed(t *testing.T) {
	synth := &countingSynth{failures: 1, entered: make(chan struct{}), release: make(chan struct{})}
	states := &fakeStateStore{}
	states.seed(100, 10, "")
	states.seed(300, 30, "") // 屏障题
	generator, _ := newTestGenerator(t, synth, states, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	generator.Enqueue(ctx, 1, 10, 100, "题面")
	<-synth.entered // 合成已在进行中

	generator.CleanupInterview(ctx, 1, 10)
	close(synth.release) // 放行合成 → 合成失败 → 走 markFailed

	// 屏障题：变 ready 说明被清理的那题已经走完 process。
	generator.Enqueue(ctx, 1, 30, 300, "题面")
	waitForState(t, states, 300, "ready")

	if got := states.state(100); got != "" {
		t.Fatalf("question 100 state = %q, want empty (must not be marked failed after close)", got)
	}
	for _, update := range states.updateLog() {
		if update.questionID == 100 && update.state == "failed" {
			t.Fatalf("question 100 marked failed after the interview closed")
		}
	}
}

// M3：已结束面试的排队题不该再白跑一次上游合成。
func TestGeneratorSkipsSynthesisForClosedInterview(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{}
	states.seed(100, 10, "")
	states.seed(200, 20, "") // 屏障题
	generator, _ := newTestGenerator(t, synth, states, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 先入队，再结束面试，最后才启动 worker：队列里残留的题不该再触发合成。
	generator.Enqueue(ctx, 1, 10, 100, "题面")
	generator.CleanupInterview(ctx, 1, 10)
	generator.Start(ctx)
	defer generator.Close()

	generator.Enqueue(ctx, 1, 20, 200, "题面")
	waitForState(t, states, 200, "ready")

	if got := synth.callCount(); got != 1 {
		t.Fatalf("synthesized %d times, want 1 (closed question must be skipped)", got)
	}
	if got := states.state(100); got != "" {
		t.Fatalf("question 100 state = %q, want empty", got)
	}
}

// C1：provider panic 不能带崩进程，且 worker 必须继续工作。
//
// 没有 recover 时这个用例会直接把测试二进制打崩（不是断言失败）。
func TestGeneratorRecoversFromProviderPanic(t *testing.T) {
	synth := &countingSynth{panicOnCall: 1}
	states := &fakeStateStore{}
	states.seed(100, 10, "")
	states.seed(200, 20, "") // 屏障题：验证 worker 还活着
	generator, _ := newTestGenerator(t, synth, states, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	generator.Enqueue(ctx, 1, 10, 100, "题面")
	waitForState(t, states, 100, "failed")

	// worker 必须还活着：再入队一题，断言它能被正常处理。
	generator.Enqueue(ctx, 1, 20, 200, "题面")
	waitForState(t, states, 200, "ready")
}

// 重启重载：先把上次崩溃残留的 pending 归零，再重新入队补齐。
//
// Reload 必须在 Start 之前调用（见 TestGeneratorReloadAfterStartIsRejected）。
func TestGeneratorReloadEnqueuesPendingAudio(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{pending: []interview.PendingAudio{
		{TenantID: 1, InterviewID: 10, QuestionID: 100, Text: "补齐"},
	}}
	states.seed(100, 10, "pending") // 崩溃残留
	generator, _ := newTestGenerator(t, synth, states, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := generator.Reload(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	generator.Start(ctx)
	defer generator.Close()

	waitForState(t, states, 100, "ready")
	if got := synth.callCount(); got != 1 {
		t.Fatalf("synthesized %d times, want 1", got)
	}
}

// Reload 在 Start 之后调用是接线错误：必须直接拒绝，不能真的归零 pending，
// 否则正在合成的题会被重复合成。
func TestGeneratorReloadAfterStartIsRejected(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{}
	states.seed(100, 10, "pending")
	generator, _ := newTestGenerator(t, synth, states, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	err := generator.Reload(ctx)
	if !errors.Is(err, ErrReloadAfterStart) {
		t.Fatalf("reload error = %v, want ErrReloadAfterStart", err)
	}
	if got := states.state(100); got != "pending" {
		t.Fatalf("question 100 state = %q, want pending (reload must not reset after start)", got)
	}
}

// I3：队列满时任务进 backlog，不当场丢弃——backlog 也满才归还生成权。
func TestGeneratorBacklogOverflowReleasesClaim(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{}
	generator, _ := newTestGeneratorWithSizes(t, synth, states, 1, 2, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 不 Start：worker 不消费，队列(2) + backlog(4) 会被填满。
	for i := 0; i < 6; i++ {
		questionID := int64(1000 + i)
		states.seed(questionID, 10, "")
		generator.Enqueue(ctx, 1, 10, questionID, "题面")
	}
	overflow := int64(9999)
	states.seed(overflow, 10, "")
	generator.Enqueue(ctx, 1, 10, overflow, "题面")

	if got := states.state(overflow); got != "" {
		t.Fatalf("overflow question state = %q, want empty (claim released)", got)
	}
}

// I3：进 backlog 的题最终会被处理成 ready，不会在本进程内落空。
func TestGeneratorBacklogJobsAreProcessed(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{}
	generator, _ := newTestGeneratorWithSizes(t, synth, states, 1, 2, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < 5; i++ {
		questionID := int64(1000 + i)
		states.seed(questionID, 10, "")
		generator.Enqueue(ctx, 1, 10, questionID, "题面")
	}
	generator.Start(ctx)
	defer generator.Close()

	for i := 0; i < 5; i++ {
		waitForState(t, states, int64(1000+i), "ready")
	}
	if got := synth.callCount(); got != 5 {
		t.Fatalf("synthesized %d times, want 5", got)
	}
}

// Important-1：持续入队压力下 backlog 里的题不能永久饥饿。
//
// 形状与评审复现一致：channel 容量 1，每次合成时再入队一个新题 → channel 恒满。
// 旧实现（处理完一题再把 backlog 灌回 channel）下，合成期间新任务会补满刚腾出的槽位，
// pump 永远失败，backlog 里的题一直停在 pending。
func TestGeneratorBacklogDoesNotStarveUnderConstantEnqueue(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{}
	generator, _ := newTestGeneratorWithSizes(t, synth, states, 1, 1, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var counter int64
	synth.onCall = func(call int) {
		// 每次合成都再入队一个新题：worker 取走 channel 任务后槽位立刻被补上，
		// channel 恒满 → backlog 持续有积压。
		id := atomic.AddInt64(&counter, 1) + 1000
		states.seed(id, 10, "")
		generator.Enqueue(ctx, 1, 10, id, "题面")
	}

	states.seed(100, 10, "")
	generator.Enqueue(ctx, 1, 10, 100, "题面") // 进 channel
	states.seed(101, 10, "")
	generator.Enqueue(ctx, 1, 10, 101, "题面") // channel 满 → backlog
	states.seed(102, 10, "")
	generator.Enqueue(ctx, 1, 10, 102, "题面") // backlog

	generator.Start(ctx)
	defer generator.Close()

	waitForState(t, states, 101, "ready")
	waitForState(t, states, 102, "ready")
}

// 公平性：backlog 优先消费时，channel 里的老任务也不能被永久推迟。
func TestGeneratorChannelJobNotStarvedByBacklog(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{}
	generator, _ := newTestGeneratorWithSizes(t, synth, states, 1, 1, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var counter int64
	synth.onCall = func(call int) {
		id := atomic.AddInt64(&counter, 1) + 1000
		states.seed(id, 10, "")
		generator.Enqueue(ctx, 1, 10, id, "题面")
	}

	states.seed(100, 10, "")
	generator.Enqueue(ctx, 1, 10, 100, "题面") // 进 channel
	states.seed(101, 10, "")
	generator.Enqueue(ctx, 1, 10, 101, "题面") // channel 满 → backlog

	generator.Start(ctx)
	defer generator.Close()

	// channel 里的老任务最终会被处理（连续消费 backlog 达上限后强制让位）。
	waitForState(t, states, 100, "ready")
}

// Minor-2：抢占进行中发生 Close 时，抢占成功后必须再次确认并归还生成权。
func TestGeneratorEnqueueReleasesClaimWhenClosedDuringClaim(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{}
	states.seed(100, 10, "")
	states.claimEntered = make(chan struct{})
	states.claimRelease = make(chan struct{})
	generator, _ := newTestGenerator(t, synth, states, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)

	done := make(chan struct{})
	go func() {
		generator.Enqueue(ctx, 1, 10, 100, "题面")
		close(done)
	}()

	<-states.claimEntered // 抢占已发出、尚未返回
	generator.Close()     // 此刻关闭生成器：worker 退出
	close(states.claimRelease)
	<-done

	// 抢占成功后才发现已关闭 → 必须归还生成权，否则这题永远卡在 pending。
	if got := states.state(100); got != "" {
		t.Fatalf("question 100 state = %q, want empty (claim must be released)", got)
	}
}

// Minor-3：抢占必须带超时，不能无限期占住调用方 goroutine。
func TestGeneratorClaimUsesBoundedContext(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{}
	states.seed(100, 10, "")
	generator, _ := newTestGenerator(t, synth, states, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	generator.Enqueue(ctx, 1, 10, 100, "题面")
	waitForState(t, states, 100, "ready")

	if !states.claimDeadlineRecorded() {
		t.Fatal("claim context must carry a deadline")
	}
}

// Minor-5：清理路径不能受调用方 ctx 取消影响（否则状态归零失败、目录残留）。
func TestGeneratorCleanupSurvivesCanceledContext(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{}
	states.seed(100, 10, "")
	generator, store := newTestGenerator(t, synth, states, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	if err := store.SaveQuestionAudio(ctx, 1, 10, 100, testWAV()); err != nil {
		t.Fatalf("seed audio: %v", err)
	}

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()
	generator.CleanupInterview(requestCtx, 1, 10)

	if got := states.state(100); got != "" {
		t.Fatalf("question 100 state = %q, want empty (state reset must survive a canceled ctx)", got)
	}
	if _, err := store.ReadQuestionAudio(1, 10, 100); err == nil {
		t.Fatal("audio file must be cleaned up even when the caller ctx is canceled")
	}
}

// I2：生成器关闭后入队不得再抢占生成权（否则这题永远卡在 pending）。
func TestGeneratorEnqueueAfterCloseDoesNotClaim(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{}
	states.seed(100, 10, "")
	generator, _ := newTestGenerator(t, synth, states, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	generator.Close()

	generator.Enqueue(ctx, 1, 10, 100, "题面")

	if got := states.claimCount(); got != 0 {
		t.Fatalf("claim count = %d, want 0 (enqueue after close must not claim)", got)
	}
	if got := states.state(100); got != "" {
		t.Fatalf("question 100 state = %q, want empty", got)
	}
}

// I4：抢占属于后台任务语义，不能因为请求 ctx 被取消就放弃这题。
func TestGeneratorEnqueueSurvivesCanceledContext(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{}
	states.seed(100, 10, "")
	generator, _ := newTestGenerator(t, synth, states, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	// 模拟客户端中断请求：请求 ctx 已被取消。
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()

	generator.Enqueue(requestCtx, 1, 10, 100, "题面")

	if got := states.claimCount(); got != 1 {
		t.Fatalf("claim count = %d, want 1 (claim must survive a canceled request ctx)", got)
	}
	waitForState(t, states, 100, "ready")
}

// I1：状态归零失败时不能删目录 —— 否则会出现「状态 ready 但文件不存在」。
func TestGeneratorKeepsFilesWhenStateResetFails(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{}
	states.seed(100, 10, "")
	generator, store := newTestGenerator(t, synth, states, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	if err := store.SaveQuestionAudio(ctx, 1, 10, 100, testWAV()); err != nil {
		t.Fatalf("seed audio: %v", err)
	}
	states.setResetInterviewError(errors.New("database is locked"))

	generator.CleanupInterview(ctx, 1, 10)

	if _, err := store.ReadQuestionAudio(1, 10, 100); err != nil {
		t.Fatalf("audio file must survive when the state reset failed: %v", err)
	}
}

// 落盘失败不重试合成：音频字节已经拿到了，重跑 TTS 是白烧上游算力。
func TestGeneratorDoesNotRetrySynthesisWhenPersistFails(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{}
	states.seed(100, 10, "")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "1"), 0o755); err != nil {
		t.Fatalf("prepare blocked dir: %v", err)
	}
	// 用一个普通文件占住面试目录：MkdirAll 会失败，落盘必然报错。
	if err := os.WriteFile(filepath.Join(root, "1", "10"), []byte("blocked"), 0o600); err != nil {
		t.Fatalf("prepare blocked dir: %v", err)
	}

	cfg := Config{MaxAttempts: 3, Backoff: []time.Duration{time.Millisecond}}
	generator := NewGenerator(synth, NewAudioStore(root), states, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	generator.Enqueue(ctx, 1, 10, 100, "题面")
	waitForState(t, states, 100, "failed")

	if got := synth.callCount(); got != 1 {
		t.Fatalf("synthesized %d times, want 1 (persist failure must not trigger re-synthesis)", got)
	}
}

// 合成产物不是合法 WAV：当作合成失败处理（重试），且绝不落盘。
func TestGeneratorTreatsInvalidAudioAsSynthesisFailure(t *testing.T) {
	synth := &countingSynth{payload: []byte("RIFF-not-a-real-wav")}
	states := &fakeStateStore{}
	states.seed(100, 10, "")
	generator, store := newTestGenerator(t, synth, states, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	generator.Enqueue(ctx, 1, 10, 100, "题面")
	waitForState(t, states, 100, "failed")

	if got := synth.callCount(); got != 2 {
		t.Fatalf("synthesized %d times, want 2 (invalid audio is retried)", got)
	}
	if _, err := store.ReadQuestionAudio(1, 10, 100); err == nil {
		t.Fatal("invalid audio must not be stored")
	}
}

// 空题面直接忽略：既不抢占生成权，也不入队。
func TestGeneratorEnqueueIgnoresEmptyText(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{}
	states.seed(100, 10, "")
	generator, _ := newTestGenerator(t, synth, states, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)
	defer generator.Close()

	generator.Enqueue(ctx, 1, 10, 100, "")
	generator.Enqueue(ctx, 1, 20, 200, "题面")
	waitForState(t, states, 200, "ready")

	if got := states.claimCount(); got != 1 {
		t.Fatalf("claim count = %d, want 1 (empty text must not claim)", got)
	}
	if got := synth.callCount(); got != 1 {
		t.Fatalf("synthesized %d times, want 1", got)
	}
}

// Close 必须能中断正在进行的合成：否则进程退出会被上游 TTS 请求拖住。
func TestGeneratorCloseCancelsInFlightSynthesis(t *testing.T) {
	synth := &countingSynth{entered: make(chan struct{}), release: make(chan struct{})}
	states := &fakeStateStore{}
	states.seed(100, 10, "")
	generator, _ := newTestGenerator(t, synth, states, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator.Start(ctx)

	generator.Enqueue(ctx, 1, 10, 100, "题面")
	<-synth.entered // 合成卡在网络请求里

	started := time.Now()
	generator.Close()
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("close took %s, want the in-flight synthesis to be cancelled", elapsed)
	}
	if got := states.state(100); got != "pending" {
		t.Fatalf("question 100 state = %q, want pending (shutdown must not mark failed)", got)
	}
}
