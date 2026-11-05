package speech

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"resumepro/backend/internal/interview"
)

const (
	// generatorQueueSize 是待生成任务队列的容量。
	// 队列满时入队方直接放弃并归还生成权，不阻塞调用方。
	generatorQueueSize = 256
	// generatorWorkerExitTimeout 是 Close 等待 worker 退出的上限：
	// 上游 TTS 不响应 ctx 取消时，不能把进程退出卡死。
	generatorWorkerExitTimeout = 5 * time.Second
)

// AudioStateStore 是生成器需要的最小持久化接口，由 interview.Repository 实现。
//
// 依赖方向是 speech → interview：speech 依赖 interview，interview 不依赖 speech，
// 因此 interview.AudioHooks 由 speech.Generator 实现（见文件末尾的编译期断言）。
type AudioStateStore interface {
	// UpdateAudioState 回写题目的语音状态（pending / ready / failed）。
	UpdateAudioState(ctx context.Context, questionID int64, state string) error
	// ClaimAudioState 原子抢占生成权：抢到返回 true。
	ClaimAudioState(ctx context.Context, questionID int64) (bool, error)
	// ReleaseAudioState 归还生成权（pending → 未生成）。
	ReleaseAudioState(ctx context.Context, questionID int64) error
	// ResetStalePendingAudio 把进行中面试里卡在 pending 的题目归零（启动时调用）。
	ResetStalePendingAudio(ctx context.Context) (int64, error)
	// ResetAudioStateByInterview 清空一场面试全部题目的语音状态。
	ResetAudioStateByInterview(ctx context.Context, interviewID int64) error
	// ActiveQuestionsNeedingAudio 返回进行中面试里语音未就绪的题目。
	ActiveQuestionsNeedingAudio(ctx context.Context) ([]interview.PendingAudio, error)
}

var (
	// errInterviewClosed 表示目标面试已终止，任务应直接丢弃。
	errInterviewClosed = errors.New("speech: interview closed")
	// errInvalidAudio 表示合成产物不是合法 WAV：按「合成失败」处理，可以重试。
	errInvalidAudio = errors.New("speech: invalid synthesized audio")
	// errPersistFailed 表示落盘或状态回写失败：属于本地问题，重试合成是浪费上游算力。
	errPersistFailed = errors.New("speech: persist question audio failed")

	// ErrReloadAfterStart 表示 Reload 在 Start 之后被调用。
	//
	// Reload 会把「进行中面试」里所有 pending 归零，而 pending 表示「已抢占生成权」。
	// worker 已经在跑时调用 Reload，正在合成的题会被归零 → 被重复合成、状态错乱。
	// 因此 Reload 只允许在启动阶段（Start 之前）调用，误用时直接返回本错误且不做任何状态改动。
	ErrReloadAfterStart = errors.New("speech: reload must be called before start")
)

// Job 是一条待生成的题目语音。
type Job struct {
	TenantID    int64
	InterviewID int64
	QuestionID  int64
	Text        string
}

// Generator 是后台串行生成器：单 worker 消费队列，逐题合成、落盘并回写 audio_state。
//
// 播放接口只读文件，绝不在请求内合成 —— TTS 单题数秒到数十秒，
// 放进请求会让用户干等，还会与后台队列抢 GPU。
//
// 单 worker 串行与 mlx-audio 的单 GPU worker 特性对齐：并发合成只会互相拖慢。
type Generator struct {
	synth Synthesizer
	store *AudioStore
	repo  AudioStateStore

	maxAttempts int
	backoff     []time.Duration
	voice       string

	jobs chan Job
	// enqueue 是进程内的快速去重标记：questionID → struct{}。
	//
	// 键用 questionID 即可，前提是 questionID 是数据库全局自增主键、跨租户唯一
	// （interview_questions.id），因此不会发生不同租户撞键。
	// 真正的去重真源仍是数据库抢占（ClaimAudioState），这里只是省掉一次无谓的 UPDATE。
	enqueue  sync.Map
	closed   chan struct{}
	done     chan struct{}
	doneOnce sync.Once

	lifeMu  sync.Mutex
	started bool
	stopped bool
	cancel  context.CancelFunc

	startOnce sync.Once
	stopOnce  sync.Once

	// mu 保护 closedInterviews。写锁用于「面试终止」，读锁用于「落盘 / 回写状态」，
	// 二者互斥，保证清理后不会再产生孤儿音频文件或残留状态。
	//
	// 键用 interviewID 即可，前提是 interviewID 是数据库全局自增主键、跨租户唯一
	// （interviews.id）；不需要 tenantID + interviewID 复合键，那是过度设计。
	mu               sync.RWMutex
	closedInterviews map[int64]struct{}
}

// NewGenerator 创建生成器（不自动启动，需显式 Start）。
func NewGenerator(synth Synthesizer, store *AudioStore, repo AudioStateStore, cfg Config) *Generator {
	backoff := cfg.Backoff
	if len(backoff) == 0 {
		backoff = []time.Duration{time.Second}
	}
	maxAttempts := cfg.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	return &Generator{
		synth:            synth,
		store:            store,
		repo:             repo,
		maxAttempts:      maxAttempts,
		backoff:          backoff,
		voice:            cfg.TTSVoice,
		jobs:             make(chan Job, generatorQueueSize),
		closed:           make(chan struct{}),
		done:             make(chan struct{}),
		closedInterviews: map[int64]struct{}{},
	}
}

// Start 启动后台 worker。
//
// 可以重复调用，只有第一次生效。worker 使用 ctx 的派生上下文：
// Close 会取消它，从而中断正在进行的合成请求。
func (g *Generator) Start(ctx context.Context) {
	g.startOnce.Do(func() {
		g.lifeMu.Lock()
		defer g.lifeMu.Unlock()
		if g.started {
			return
		}
		workerCtx, cancel := context.WithCancel(ctx)
		g.cancel = cancel
		g.started = true
		if g.stopped {
			// Close 先于 Start：worker 起来后立即退出。
			cancel()
		}
		go g.run(workerCtx)
	})
}

// Close 停止 worker 并等待它退出。
//
// 语义：
//   - 取消 worker 的上下文，正在进行的合成请求会被中断（上游遵守 ctx 时）；
//   - 等待 worker 退出，最长 generatorWorkerExitTimeout；
//     上游不响应取消时只记日志，不把进程退出卡死。
//
// Close 可以重复调用。未 Start 就 Close 时直接返回（没有 worker 需要退出）。
func (g *Generator) Close() {
	g.stopOnce.Do(func() {
		close(g.closed)
		g.lifeMu.Lock()
		cancel := g.cancel
		started := g.started
		g.stopped = true
		g.lifeMu.Unlock()
		if cancel != nil {
			cancel()
		}
		if !started {
			g.signalDone()
		}
	})

	select {
	case <-g.done:
	case <-time.After(generatorWorkerExitTimeout):
		logrus.Warn("voice: audio generator worker did not exit in time")
	}
}

// Enqueue 把题目加入生成队列。
//
// 去重靠数据库抢占（ClaimAudioState）而不是内存标记：
// 「重启重载」与「创建面试」可能同时给同一题入队，
// 只有抢到生成权（audio_state 从 NULL/failed 变成 pending）的一方才会真正合成。
//
// 绝不阻塞调用方：创建面试与答题链路不能被语音拖住。
// 队列满时直接放弃并归还生成权，等下次 Reload 补齐。
func (g *Generator) Enqueue(ctx context.Context, tenantID, interviewID, questionID int64, text string) {
	if text == "" {
		return
	}
	if g.isClosed(interviewID) {
		return
	}
	if _, loaded := g.enqueue.LoadOrStore(questionID, struct{}{}); loaded {
		return
	}
	claimed, err := g.repo.ClaimAudioState(ctx, questionID)
	if err != nil {
		g.enqueue.Delete(questionID)
		logrus.WithError(err).WithField("question_id", questionID).Warn("voice: claim audio state failed")
		return
	}
	if !claimed {
		// 别的路径已经抢到（或已就绪）：不重复合成。
		g.enqueue.Delete(questionID)
		return
	}
	job := Job{TenantID: tenantID, InterviewID: interviewID, QuestionID: questionID, Text: text}
	select {
	case g.jobs <- job:
	default:
		// 队列满：归还生成权，让下次 Reload 能补上。
		g.enqueue.Delete(questionID)
		if err := g.repo.ReleaseAudioState(ctx, questionID); err != nil {
			logrus.WithError(err).WithField("question_id", questionID).Warn("voice: release audio state failed")
		}
		logrus.WithField("question_id", questionID).Warn("voice: generation queue full, skip")
	}
}

// Reload 在服务重启后补齐「进行中且未就绪」的题目。
//
// 前置条件：必须在 Start 之前调用（启动阶段）。
// ResetStalePendingAudio 会把「进行中面试」里所有 pending 归零，
// 而 pending 表示「已抢占生成权」；worker 已经在跑时归零，会让正在合成的题被重复合成。
// 因此本方法在 Start 之后被调用时直接返回 ErrReloadAfterStart，且不做任何状态改动。
//
// 顺序：先把上次崩溃残留的 pending 归零（启动时还没有 worker 在跑，pending 一定是残留），
// 再重新扫描入队。
func (g *Generator) Reload(ctx context.Context) error {
	g.lifeMu.Lock()
	started := g.started
	g.lifeMu.Unlock()
	if started {
		logrus.Warn("voice: reload called after start, skipped (pending reset would break in-flight generation)")
		return ErrReloadAfterStart
	}

	if _, err := g.repo.ResetStalePendingAudio(ctx); err != nil {
		return fmt.Errorf("speech: reset stale pending audio: %w", err)
	}
	items, err := g.repo.ActiveQuestionsNeedingAudio(ctx)
	if err != nil {
		return fmt.Errorf("speech: reload pending audio: %w", err)
	}
	for _, item := range items {
		g.Enqueue(ctx, item.TenantID, item.InterviewID, item.QuestionID, item.Text)
	}
	return nil
}

// CleanupInterview 清理一场面试的音频目录与语音状态（面试终态时由 interview 层调用）。
//
// 先在写锁内置「已终止」标记并把该面试的语音状态归零，再删目录：
//   - 置标记与状态归零必须在同一个临界区，否则 worker 可能在状态归零之后又写回 failed；
//   - 正在合成中的任务在读锁内看到标记后不会落盘，
//     不会出现「清理完之后又冒出音频文件」的孤儿。
func (g *Generator) CleanupInterview(ctx context.Context, tenantID, interviewID int64) {
	g.mu.Lock()
	g.closedInterviews[interviewID] = struct{}{}
	if err := g.repo.ResetAudioStateByInterview(ctx, interviewID); err != nil {
		logrus.WithError(err).WithField("interview_id", interviewID).Warn("voice: reset interview audio state failed")
	}
	g.mu.Unlock()

	if err := g.store.CleanupInterview(tenantID, interviewID); err != nil {
		logrus.WithError(err).WithField("interview_id", interviewID).Warn("voice: cleanup interview audio failed")
	}
}

// isClosed 判断面试是否已终止。
func (g *Generator) isClosed(interviewID int64) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, closed := g.closedInterviews[interviewID]
	return closed
}

// signalDone 标记 worker 已退出（可重复调用）。
func (g *Generator) signalDone() {
	g.doneOnce.Do(func() { close(g.done) })
}

// run 是 worker 主循环：串行消费队列。
func (g *Generator) run(ctx context.Context) {
	defer g.signalDone()
	for {
		select {
		case <-ctx.Done():
			return
		case <-g.closed:
			return
		case job := <-g.jobs:
			g.process(ctx, job)
		}
	}
}

// process 合成一题：合成失败按退避重试，耗尽后标记 failed。
//
// 错误分类决定重试语义：
//   - 合成失败（上游报错）与音频非法：重试合成（同一次失败很可能是抖动）；
//   - 落盘失败（磁盘、体积、路径、状态回写）：不重试合成，
//     音频字节已经拿到了，重跑一次 TTS 是白烧上游算力，直接判失败。
func (g *Generator) process(ctx context.Context, job Job) {
	defer g.enqueue.Delete(job.QuestionID)

	var lastErr error
	for attempt := 0; attempt < g.maxAttempts; attempt++ {
		res, err := g.synth.Synthesize(ctx, SynthesisRequest{Text: job.Text, Voice: g.voice})
		if err == nil {
			err = g.saveAudio(ctx, job, res.Audio)
			switch {
			case err == nil:
				return
			case errors.Is(err, errInterviewClosed):
				// 面试已终止：丢弃结果，不落盘也不写状态。
				return
			case errors.Is(err, errPersistFailed):
				// 落盘失败：不重试合成，直接判失败。
				g.markFailed(ctx, job, err)
				return
			}
			// errInvalidAudio：按合成失败继续走重试。
		}
		lastErr = err
		if attempt == g.maxAttempts-1 {
			break
		}
		if !g.waitBackoff(ctx, g.backoff[min(attempt, len(g.backoff)-1)]) {
			// 生成器正在关闭：不再重试，状态留在 pending，由下次启动的 Reload 归零补齐。
			return
		}
	}

	if ctx.Err() != nil {
		// 进程正在退出：不写 failed，状态留在 pending 等待下次启动重载。
		return
	}
	g.markFailed(ctx, job, lastErr)
}

// waitBackoff 等待退避间隔；返回 false 表示生成器已关闭，应当放弃。
func (g *Generator) waitBackoff(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	case <-g.closed:
		return false
	}
}

// saveAudio 校验并落盘题目语音，然后回写 ready。
//
// 校验（ValidateWAV）必须在落盘之前：存储层刻意不校验格式（那是它的契约），
// 而脏音频一旦落盘，播放接口会按 audio/wav 播给用户。
//
// 整个落盘 + 回写过程在读锁内，与 CleanupInterview 的写锁互斥；
// 面试已被终止时返回 errInterviewClosed，调用方直接丢弃结果。
func (g *Generator) saveAudio(ctx context.Context, job Job, audio []byte) error {
	if _, err := ValidateWAV(audio); err != nil {
		return fmt.Errorf("%w: %w", errInvalidAudio, err)
	}

	g.mu.RLock()
	defer g.mu.RUnlock()
	if _, closed := g.closedInterviews[job.InterviewID]; closed {
		return errInterviewClosed
	}
	if err := g.store.SaveQuestionAudio(ctx, job.TenantID, job.InterviewID, job.QuestionID, audio); err != nil {
		return fmt.Errorf("%w: %w", errPersistFailed, err)
	}
	if err := g.repo.UpdateAudioState(ctx, job.QuestionID, "ready"); err != nil {
		return fmt.Errorf("%w: %w", errPersistFailed, err)
	}
	return nil
}

// markFailed 把题目标记为 failed。
//
// 「检查已终止 + 写状态」在同一个读锁临界区里完成，
// 与 CleanupInterview 的写锁互斥：不会在面试结束后又把状态写回 failed。
func (g *Generator) markFailed(ctx context.Context, job Job, cause error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if _, closed := g.closedInterviews[job.InterviewID]; closed {
		// 面试已终止：状态已被清理归零，不再写回，避免留下孤儿 failed。
		logrus.WithFields(logrus.Fields{
			"interview_id": job.InterviewID,
			"question_id":  job.QuestionID,
		}).Warn("voice: interview closed before marking audio failed, skipped")
		return
	}
	if err := g.repo.UpdateAudioState(ctx, job.QuestionID, "failed"); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"interview_id": job.InterviewID,
			"question_id":  job.QuestionID,
		}).Warn("voice: mark question audio failed failed")
	}
	logrus.WithError(cause).WithFields(logrus.Fields{
		"interview_id": job.InterviewID,
		"question_id":  job.QuestionID,
	}).Warn("voice: question audio generation failed")
}

var (
	// Generator 实现 interview.AudioHooks：面试层只依赖这两个窄方法。
	_ interview.AudioHooks = (*Generator)(nil)
	// interview.Repository 满足 AudioStateStore：接口与仓储方法签名一一对应。
	_ AudioStateStore = (*interview.Repository)(nil)
)
