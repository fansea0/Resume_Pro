import { useCallback, useEffect, useRef, useState, type CSSProperties, type KeyboardEvent } from 'react';

import {
  answerInterview,
  getInterviewQuestions,
  type Interview,
  type InterviewGrade,
  type InterviewLabel,
  type InterviewQuestion,
} from '../api/interviews';
import { fetchQuestionAudio } from '../api/voice';
import type { AudioState } from '../api/voice';
import { useVoiceRecorder } from '../hooks/useVoiceRecorder';

const LABEL_NAMES: Record<InterviewLabel, string> = { '0': '八股', '1': '算法', '2': '项目' };
// 与 .interview-dock textarea 的 max-height 保持一致（12rem @16px）。
const DOCK_MAX_HEIGHT_PX = 192;
const GRADE_CLASS: Record<InterviewGrade, string> = {
  C: 'c', 'B-': 'bm', B: 'b', 'B+': 'bp', A: 'a', S: 's',
};

/** 当前题的语音自动播放：就绪即播，未就绪则轮询等待。 */
const AUDIO_POLL_INTERVAL_MS = 1500;
const AUDIO_POLL_MAX_ATTEMPTS = 40;
/** 题目没带 audioState（后端没返回状态）时的轮询上限：最多 3 次，避免无谓请求。 */
const AUDIO_POLL_MAX_ATTEMPTS_UNKNOWN = 3;

/*
 * 语音控件的样式：styles/global.css 属于其他会话的改动范围，这里用行内样式保证观感一致。
 * TODO(样式迁移)：等 styles/global.css 的归属清晰后，删掉下面三个行内样式对象，
 * 只保留 JSX 里已占位的 class（interview-msg__speak / interview-msg__audiopending /
 * interview-chat__voiceerror），样式改回 CSS 文件维护。
 */
const VOICE_SPEAK_STYLE: CSSProperties = {
  background: 'transparent',
  border: '1px solid #cbd5e1',
  borderRadius: '999px',
  cursor: 'pointer',
  fontSize: '.8rem',
  lineHeight: 1,
  marginLeft: '.4rem',
  padding: '.2rem .45rem',
  verticalAlign: 'middle',
};
const VOICE_PENDING_STYLE: CSSProperties = {
  color: '#536176',
  fontSize: '.72rem',
  fontWeight: 400,
  marginLeft: '.4rem',
};
const VOICE_ERROR_STYLE: CSSProperties = {
  background: '#fff8e6',
  border: '1px solid #f2d391',
  borderRadius: '9px',
  color: '#8a5a00',
  fontSize: '.82rem',
  margin: 0,
  padding: '.6rem .8rem',
};

/** 播放一段音频。自动播放被浏览器拦截时交给调用方决定提示，静默兜住。 */
function playAudioBlob(blob: Blob, onBlocked: () => void) {
  const url = URL.createObjectURL(blob);
  const audio = new Audio(url);
  // 播完再释放：过早 revoke 会把正在播放的音频掐断。
  const release = () => {
    try {
      URL.revokeObjectURL(url);
    } catch {
      // 环境不支持 revoke 时不影响播放。
    }
  };
  audio.addEventListener('ended', release);
  audio.addEventListener('error', release);
  void audio.play().catch(() => {
    release();
    onBlocked();
  });
};

/** 当前题的语音自动播放：就绪即播，未就绪则轮询等待。 */
function useQuestionAudio(
  interviewId: number,
  questionId: number | null,
  enabled: boolean,
  audioState: AudioState | null | undefined,
  onBlocked: () => void,
) {
  const [status, setStatus] = useState<'idle' | 'pending' | 'failed'>('idle');

  useEffect(() => {
    if (!enabled || questionId === null) return;
    // 明确生成失败就不再请求：语音是附加层，不做无谓轮询。
    if (audioState === 'failed') return;
    // 题目没带 audioState 时不知道语音是否在生成，只做少量轮询。
    const maxAttempts = audioState == null
      ? AUDIO_POLL_MAX_ATTEMPTS_UNKNOWN
      : AUDIO_POLL_MAX_ATTEMPTS;
    let cancelled = false;
    let timer: number | undefined;
    let attempts = 0;

    const play = async () => {
      try {
        const blob = await fetchQuestionAudio(interviewId, questionId);
        if (cancelled) return;
        // 浏览器拦截自动播放：提示用户点喇叭重听（追问同样有喇叭按钮）。
        playAudioBlob(blob, onBlocked);
        setStatus('idle');
      } catch (error) {
        const code = (error as { status?: number }).status;
        if (code === 409 && attempts < maxAttempts) {
          attempts += 1;
          setStatus('pending');
          timer = window.setTimeout(() => void play(), AUDIO_POLL_INTERVAL_MS);
          return;
        }
        setStatus(code === 503 ? 'failed' : 'idle');
      }
    };

    void play();
    return () => {
      cancelled = true;
      if (timer) window.clearTimeout(timer);
    };
  }, [interviewId, questionId, enabled, audioState]);

  return { status };
}

export interface InterviewChatFinishedResult {
  overallGrade: InterviewGrade;
  overallReview?: string | null;
}

interface InterviewChatProps {
  interview: Interview;
  onExit: () => void;
  onFinished: (result: InterviewChatFinishedResult) => void;
}

function isAwaiting(question: InterviewQuestion) {
  return question.grade == null && question.userAnswer == null;
}

export function InterviewChat({ interview, onExit, onFinished }: InterviewChatProps) {
  const [questions, setQuestions] = useState<InterviewQuestion[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [answer, setAnswer] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState('');
  const mounted = useRef(false);
  const requestVersion = useRef(0);
  const activeRef = useRef<HTMLElement | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);

  const load = useCallback(async () => {
    const version = ++requestVersion.current;
    try {
      const rows = await getInterviewQuestions(interview.id);
      if (mounted.current && version === requestVersion.current) setQuestions(rows);
    } catch {
      if (mounted.current && version === requestVersion.current) setLoadError(true);
    }
  }, [interview.id]);

  useEffect(() => {
    mounted.current = true;
    setLoading(true);
    void load().finally(() => {
      if (mounted.current) setLoading(false);
    });
    return () => {
      mounted.current = false;
      requestVersion.current += 1;
    };
  }, [load]);

  const mains = questions.filter((question) => !question.parentId);
  const followUpsByParent = new Map<number, InterviewQuestion[]>();
  for (const question of questions) {
    if (!question.parentId) continue;
    const siblings = followUpsByParent.get(question.parentId) ?? [];
    siblings.push(question);
    followUpsByParent.set(question.parentId, siblings);
  }
  // 面试图按顺序推进，因此「当前题」= 未回答的追问（刚被抛出），否则 = 最早一道未回答的主问题。
  // 全部主问题在建面试时就一次性落库，取「最后一个未回答」会永远停在末题。
  const awaiting = questions.filter(isAwaiting);
  const current = awaiting.find((question) => question.parentId) ?? awaiting[0];
  const currentMainId = current ? (current.parentId ?? current.id) : null;
  // 只渲染到当前题为止：后续题面提前摊开等于给候选人透题。
  const visibleThrough = currentMainId == null
    ? mains.length - 1
    : Math.max(0, mains.findIndex((main) => main.id === currentMainId));
  const answeredMains = mains.filter((question) => question.grade != null).length;
  const followUpUsed = questions.filter((question) => question.parentId).length;
  const budgetLeft = Math.max(0, interview.configSnapshot.followUpBudget - followUpUsed);
  const totalMains = Math.max(interview.configSnapshot.questionCount, mains.length);
  const currentNumber = currentMainId == null
    ? Math.min(answeredMains + 1, totalMains)
    : visibleThrough + 1;
  const voiceEnabled = interview.voice?.enabled ?? false;
  const currentQuestionId = current?.id ?? null;
  const currentAudioState = current?.audioState ?? null;
  const [voiceError, setVoiceError] = useState('');
  // 自动播放被浏览器拦截时的提示：用稳定的回调，避免每次渲染都重置自动播放 effect。
  const onAutoPlayBlocked = useCallback(() => setVoiceError('点击喇叭播放本题'), []);
  const { status: audioStatus } = useQuestionAudio(
    interview.id,
    currentQuestionId,
    voiceEnabled,
    currentAudioState,
    onAutoPlayBlocked,
  );

  // 识别文本插入到当前光标位置（不是追加到末尾）。
  const insertAtCursor = useCallback((text: string) => {
    const area = textareaRef.current;
    if (!area) {
      setAnswer((previous) => previous + text);
      return;
    }
    const start = area.selectionStart ?? area.value.length;
    const end = area.selectionEnd ?? start;
    const next = area.value.slice(0, start) + text + area.value.slice(end);
    setAnswer(next);
    window.requestAnimationFrame(() => {
      area.focus();
      const caret = start + text.length;
      area.setSelectionRange(caret, caret);
    });
  }, []);

  const recorder = useVoiceRecorder({
    interviewId: interview.id,
    questionId: currentQuestionId ?? 0,
    onText: insertAtCursor,
    onError: setVoiceError,
  });

  const speakQuestion = useCallback(async (questionId: number) => {
    setVoiceError('');
    try {
      const blob = await fetchQuestionAudio(interview.id, questionId);
      playAudioBlob(blob, () => setVoiceError('浏览器拦截了播放，请再点一次喇叭。'));
    } catch {
      setVoiceError('语音暂不可用，可继续打字');
    }
  }, [interview.id]);

  // 喇叭按钮：主问题与追问共用，保证追问在自动播放被拦时也有补救入口。
  const renderSpeakButton = (question: InterviewQuestion) => (
    voiceEnabled && question.audioState === 'ready' ? (
      <button
        aria-label={`朗读题目 ${question.id}`}
        className="interview-msg__speak"
        onClick={() => void speakQuestion(question.id)}
        style={VOICE_SPEAK_STYLE}
        title="朗读题目"
        type="button"
      >
        🔊
      </button>
    ) : null
  );

  // 「语音生成中…」只挂在当前题上（当前题可能是追问）。
  const renderAudioHint = (question: InterviewQuestion) => (
    voiceEnabled && current?.id === question.id
      && (question.audioState === 'pending' || audioStatus === 'pending') ? (
        <span className="interview-msg__audiopending" style={VOICE_PENDING_STYLE}>语音生成中…</span>
      ) : null
  );

  useEffect(() => {
    if (!current) return;
    activeRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    textareaRef.current?.focus({ preventScroll: true });
  }, [current?.id]);

  // 输入框随内容长高，到 DOCK_MAX_HEIGHT 后内部滚动：长答案不会把题面顶出屏幕。
  useEffect(() => {
    const area = textareaRef.current;
    if (!area) return;
    area.style.height = 'auto';
    area.style.height = `${Math.min(area.scrollHeight, DOCK_MAX_HEIGHT_PX)}px`;
  }, [answer]);

  const submit = async (skip = false) => {
    if (submitting) return;
    if (!skip && !answer.trim()) return;
    if (skip && !window.confirm('跳过当前题？将按最低档 C 结算本题，不消耗追问预算，直接进入下一题。')) return;
    setSubmitting(true);
    setSubmitError('');
    try {
      const result = skip
        ? await answerInterview(interview.id, undefined, true)
        : await answerInterview(interview.id, answer.trim());
      if (!mounted.current) return;
      if (result.finished && result.overallGrade) {
        onFinished({ overallGrade: result.overallGrade, overallReview: result.overallReview });
        return;
      }
      setAnswer('');
      await load();
    } catch {
      if (mounted.current) setSubmitError('评分失败，请重试。');
    } finally {
      if (mounted.current) setSubmitting(false);
    }
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
      event.preventDefault();
      void submit();
    }
  };

  const renderQuestionBody = (question: InterviewQuestion) => (
    question.userAnswer ? (
      <div className="interview-bubble">
        <p className="interview-bubble__label">我的回答</p>
        <p className="interview-bubble__text">{question.userAnswer}</p>
      </div>
    ) : null
  );

  const renderTurn = (question: InterviewQuestion, isCurrent: boolean) => {
    const index = mains.findIndex((main) => main.id === question.id);
    const followUps = followUpsByParent.get(question.id) ?? [];
    return (
      <article
        className={`interview-msg interview-msg--${isCurrent ? 'current' : 'past'}`}
        key={question.id}
        ref={isCurrent ? activeRef : undefined}
      >
        <header className="interview-msg__head">
          <span aria-hidden="true" className="interview-msg__avatar">面</span>
          <strong>面试官</strong>
          <span className={`interview-chip interview-chip--${question.label === '0' ? 'octopus' : question.label === '1' ? 'algorithm' : 'project'}`}>{LABEL_NAMES[question.label]}</span>
          {index >= 0 ? <span className="interview-msg__no">第 {index + 1} / {totalMains} 题</span> : null}
          {question.id === current?.id ? (
            <span className="interview-msg__wait"><span aria-hidden="true" />{submitting ? '评分中…' : '等待回答'}</span>
          ) : question.grade ? (
            <span className={`interview-grade interview-grade--${GRADE_CLASS[question.grade]}`}>{question.grade}</span>
          ) : null}
        </header>
        <p className="interview-msg__question">
          {question.question}
          {renderSpeakButton(question)}
          {renderAudioHint(question)}
        </p>
        {renderQuestionBody(question)}
        {followUps.map((followUp) => {
          const isCurrentFollowUp = current?.id === followUp.id;
          return (
            <div className={`interview-follow${isCurrentFollowUp ? ' interview-follow--current' : ''}`} key={followUp.id}>
              <header className="interview-msg__head">
                <span className="interview-follow__chip">追问 {followUp.followUpRound}</span>
                {isCurrentFollowUp ? (
                  <span className="interview-msg__wait"><span aria-hidden="true" />{submitting ? '评分中…' : '等待回答'}</span>
                ) : followUp.grade ? (
                  <span className={`interview-grade interview-grade--${GRADE_CLASS[followUp.grade]}`}>{followUp.grade}</span>
                ) : null}
              </header>
              <p className="interview-msg__question">
                {followUp.question}
                {renderSpeakButton(followUp)}
                {renderAudioHint(followUp)}
              </p>
              {renderQuestionBody(followUp)}
            </div>
          );
        })}
      </article>
    );
  };

  return (
    <section aria-label="AI 模拟面试进行中" className="interview-chat">
      <header className="interview-progress">
        <div className="interview-live-bar">
          <div className="interview-progress__heading">
            <p className="interview-progress__eyebrow">当前题目</p>
            <p className="interview-progress__count">第 <b>{currentNumber}</b> / {totalMains} 题</p>
          </div>
          <span className="interview-stat">进度 <b>{answeredMains}/{interview.configSnapshot.questionCount}</b></span>
          <span className="interview-stat">追问预算 <b>{budgetLeft}/{interview.configSnapshot.followUpBudget}</b></span>
          <span className="interview-stat interview-stat--live"><span aria-hidden="true" />进行中</span>
          <button onClick={onExit} type="button">返回列表</button>
        </div>
        <ol aria-hidden="true" className="interview-progress__track">
          {Array.from({ length: totalMains }, (_, position) => {
            const main = mains[position];
            const grade = main?.grade;
            const state = grade ? GRADE_CLASS[grade] : main?.id === currentMainId ? 'current' : 'todo';
            return (
              <li className={`interview-progress__seg interview-progress__seg--${state}`} key={position}>
                {grade ?? position + 1}
              </li>
            );
          })}
        </ol>
      </header>

      {loading ? (
        <p aria-live="polite" className="interview-chat__status" role="status">正在载入面试进度…</p>
      ) : loadError ? (
        <p className="interview-chat__error" role="alert">无法载入面试进度，请稍后重试。</p>
      ) : (
        <div className="interview-chat__stream">
          {mains.slice(0, visibleThrough + 1).map((main) => renderTurn(main, main.id === currentMainId))}
        </div>
      )}

      {submitError && <p className="interview-chat__error" role="alert">{submitError}</p>}
      {voiceError && <p className="interview-chat__voiceerror" role="status" style={VOICE_ERROR_STYLE}>{voiceError}</p>}

      {current ? (
        <div aria-busy={submitting} className="interview-dock">
          <textarea
            aria-label="回答输入"
            onChange={(event) => setAnswer(event.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="输入你的回答…"
            readOnly={submitting}
            ref={textareaRef}
            value={answer}
          />
          <div className="interview-dock__foot">
            <span aria-live="polite" className="interview-dock__hint">
              {submitting ? (
                <>
                  <span aria-hidden="true" className="interview-typing__dots"><i /><i /><i /></span>
                  面试官正在评分，约需 3~8 秒…
                </>
              ) : '⌘ / Ctrl + Enter 提交；不会答可跳过'}
            </span>
            {answer.length ? <span className="interview-dock__count">{answer.length} 字</span> : null}
            <div className="interview-dock__actions">
              {voiceEnabled ? (
                // TODO(样式迁移)：录音态的红色高亮等样式待 styles/global.css 归属清晰后补。
                <button
                  aria-label="录音回答"
                  className={`interview-dock__mic interview-dock__mic--${recorder.state}`}
                  disabled={recorder.state === 'starting' || recorder.state === 'transcribing' || !current}
                  onClick={() => {
                    if (recorder.state === 'recording') recorder.stop();
                    else void recorder.start();
                  }}
                  type="button"
                >
                  {recorder.state === 'starting'
                    ? '准备中…'
                    : recorder.state === 'recording'
                      ? `停止 ${recorder.seconds}s`
                      : recorder.state === 'transcribing'
                        ? '识别中…'
                        : '🎤'}
                </button>
              ) : null}
              <button className="interview-dock__skip" disabled={submitting} onClick={() => void submit(true)} type="button">跳过 · 不会</button>
              <button disabled={submitting || !answer.trim()} onClick={() => void submit()} type="button">
                {submitting ? '评分中…' : '提交回答'}
              </button>
            </div>
          </div>
        </div>
      ) : (
        !loading && !loadError && (
          <p aria-live="polite" className="interview-chat__status" role="status">本轮已答完，正在生成总评…</p>
        )
      )}
    </section>
  );
}
