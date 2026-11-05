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

func (f *fakeStateStore) updateLog() []stateUpdate {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]stateUpdate(nil), f.updates...)
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
func (f *fakeStateStore) ClaimAudioState(_ context.Context, questionID int64) (bool, error) {
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

func (f *fakeStateStore) ReleaseAudioState(_ context.Context, questionID int64) error {
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

func (f *fakeStateStore) ResetAudioStateByInterview(_ context.Context, interviewID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensure()
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

// countingSynth 是可控的假合成器：可指定前 N 次失败、可阻塞、可返回指定字节。
type countingSynth struct {
	mu       sync.Mutex
	calls    int
	failures int
	payload  []byte

	entered     chan struct{}
	enteredOnce sync.Once
	release     chan struct{}
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
	store := NewAudioStore(t.TempDir())
	cfg := Config{MaxAttempts: maxAttempts, Backoff: []time.Duration{time.Millisecond}}
	return NewGenerator(synth, store, states, cfg), store
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

// 队列满时必须归还生成权：否则这题永远停在 pending，只能等下次重启。
func TestGeneratorReleasesClaimWhenQueueFull(t *testing.T) {
	synth := &countingSynth{}
	states := &fakeStateStore{}
	generator, _ := newTestGenerator(t, synth, states, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 不 Start：worker 不消费，队列会被填满。
	for i := 0; i < generatorQueueSize; i++ {
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
